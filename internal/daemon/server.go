package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"wxview/internal/app"
	"wxview/internal/buildinfo"
	"wxview/internal/key"
	"wxview/internal/msgindex"
)

type Server struct {
	SocketPath string
	Shutdown   func()

	mu     sync.Mutex
	target key.TargetDB

	indexMu sync.Mutex

	indexStateMu sync.Mutex
	indexRunning bool
	indexPending bool
}

func (s *Server) Run(ctx context.Context) error {
	if s.SocketPath == "" {
		return fmt.Errorf("socket path is empty")
	}
	if err := s.prepareSocket(ctx); err != nil {
		return err
	}
	listener, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(s.SocketPath)
	_ = os.Chmod(s.SocketPath, 0o600)
	if err := app.ChownForSudo(s.SocketPath); err != nil {
		return err
	}

	go s.watchContacts(ctx)
	go s.watchSessions(ctx)
	go s.watchMessages(ctx)
	go s.watchHeadImage(ctx)
	go s.watchFavorites(ctx)
	go s.watchSNS(ctx)
	if result, err := key.CleanTmp(); err != nil {
		log.Printf("cache tmp cleanup skipped: %v", err)
	} else if result.Removed > 0 {
		log.Printf("cache tmp cleanup removed %d files (%d bytes)", result.Removed, result.RemovedBytes)
	}
	s.refreshBackground(ctx, "startup")

	errCh := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errCh <- err
				return
			}
			go s.handleConn(ctx, conn)
		}
	}()

	select {
	case <-ctx.Done():
		s.markIndexInterrupted()
		return nil
	case err := <-errCh:
		s.markIndexInterrupted()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) refreshBackground(ctx context.Context, reason string) {
	go func() {
		if err := s.refresh(ctx); err != nil {
			log.Printf("%s cache refresh failed: %v", reason, err)
			return
		}
		log.Printf("%s cache refresh completed", reason)
	}()
}

func (s *Server) prepareSocket(ctx context.Context) error {
	client := Client{SocketPath: s.SocketPath, Timeout: 500 * time.Millisecond}
	if client.Healthy(ctx) {
		return fmt.Errorf("daemon already running at %s", s.SocketPath)
	}
	if err := os.Remove(s.SocketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Server) refresh(ctx context.Context) error {
	if err := s.refreshContact(ctx); err != nil {
		return err
	}
	if err := s.refreshMessages(ctx); err != nil {
		return err
	}
	s.refreshIndexBackground(ctx, "startup message cache refresh")
	if err := s.refreshSessions(ctx); err != nil {
		log.Printf("refresh session cache skipped: %v", err)
	}
	if err := s.refreshHeadImage(ctx); err != nil {
		log.Printf("refresh head_image cache skipped: %v", err)
	}
	if err := s.refreshFavorites(ctx); err != nil {
		log.Printf("refresh favorite cache skipped: %v", err)
	}
	if err := s.refreshSNS(ctx); err != nil {
		log.Printf("refresh sns cache skipped: %v", err)
	}
	return nil
}

func (s *Server) refreshContact(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, _, err := key.EnsureContactCache(ctx)
	if err != nil {
		return err
	}
	s.target = res.Target
	return nil
}

func (s *Server) refreshMessages(ctx context.Context) error {
	_, err := key.EnsureMessageRelatedCaches(ctx)
	return err
}

func (s *Server) refreshIndex(ctx context.Context) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	res, contactCache, err := key.EnsureContactCache(ctx)
	if err != nil {
		return err
	}
	messagePaths, err := key.EnsureMessageCaches(ctx)
	if err != nil {
		return err
	}
	_, err = msgindex.Refresh(ctx, msgindex.BuildOptions{
		Account:           res.Target.Account,
		ContactCachePath:  contactCache,
		MessageCachePaths: messagePaths,
	})
	return err
}

func (s *Server) markIndexInterrupted() {
	s.mu.Lock()
	account := s.target.Account
	s.mu.Unlock()
	if account == "" {
		target, err := key.DiscoverContactDB()
		if err != nil {
			return
		}
		account = target.Account
	}
	indexPath, err := msgindex.LocalPath(account)
	if err != nil {
		return
	}
	msgindex.MarkInterrupted(indexPath)
}

func (s *Server) refreshIndexBackground(ctx context.Context, reason string) {
	s.indexStateMu.Lock()
	if s.indexRunning {
		s.indexPending = true
		s.indexStateMu.Unlock()
		log.Printf("message index refresh already running; coalesced request after %s", reason)
		return
	}
	s.indexRunning = true
	s.indexStateMu.Unlock()

	go func() {
		currentReason := reason
		for {
			if err := s.refreshIndex(ctx); err != nil {
				log.Printf("message index refresh skipped after %s: %v", currentReason, err)
			} else {
				log.Printf("message index refreshed after %s", currentReason)
			}

			s.indexStateMu.Lock()
			if !s.indexPending {
				s.indexRunning = false
				s.indexStateMu.Unlock()
				return
			}
			s.indexPending = false
			s.indexStateMu.Unlock()
			currentReason = "coalesced request"
		}
	}()
}

func (s *Server) refreshSessions(ctx context.Context) error {
	_, _, err := key.EnsureSessionCache(ctx)
	return err
}

func (s *Server) refreshHeadImage(ctx context.Context) error {
	_, _, err := key.EnsureHeadImageCache(ctx)
	return err
}

func (s *Server) refreshFavorites(ctx context.Context) error {
	_, _, err := key.EnsureFavoriteCache(ctx)
	return err
}

func (s *Server) refreshSNS(ctx context.Context) error {
	_, _, err := key.EnsureSNSCache(ctx)
	return err
}

func (s *Server) watchContacts(ctx context.Context) {
	paths := func() []string {
		target, err := key.DiscoverContactDB()
		if err != nil {
			log.Printf("discover contact database failed: %v", err)
			return nil
		}
		return []string{target.DBPath}
	}
	WatchFiles(ctx, paths, time.Second, time.Second, func() {
		if err := s.refreshContact(ctx); err != nil {
			log.Printf("refresh contact cache failed: %v", err)
			return
		}
		log.Printf("contact cache refreshed")
	})
}

func (s *Server) watchMessages(ctx context.Context) {
	paths := func() []string {
		targets, err := key.DiscoverMessageRelatedDBs()
		if err != nil {
			log.Printf("discover message databases failed: %v", err)
			return nil
		}
		out := make([]string, 0, len(targets))
		for _, target := range targets {
			out = append(out, target.DBPath)
		}
		return out
	}
	WatchFiles(ctx, paths, time.Second, time.Second, func() {
		if err := s.refreshMessages(ctx); err != nil {
			log.Printf("refresh message caches failed: %v", err)
			return
		}
		log.Printf("message caches refreshed")
		s.refreshIndexBackground(ctx, "message cache refresh")
	})
}

func (s *Server) watchSessions(ctx context.Context) {
	paths := func() []string {
		target, ok := key.DiscoverSessionDB()
		if !ok {
			return nil
		}
		return []string{target.DBPath}
	}
	WatchFiles(ctx, paths, time.Second, time.Second, func() {
		if err := s.refreshSessions(ctx); err != nil {
			log.Printf("refresh session cache failed: %v", err)
			return
		}
		log.Printf("session cache refreshed")
	})
}

func (s *Server) watchHeadImage(ctx context.Context) {
	paths := func() []string {
		target, ok := key.DiscoverHeadImageDB()
		if !ok {
			return nil
		}
		return []string{target.DBPath}
	}
	WatchFiles(ctx, paths, time.Second, time.Second, func() {
		if err := s.refreshHeadImage(ctx); err != nil {
			log.Printf("refresh head_image cache failed: %v", err)
			return
		}
		log.Printf("head_image cache refreshed")
	})
}

func (s *Server) watchFavorites(ctx context.Context) {
	paths := func() []string {
		target, ok := key.DiscoverFavoriteDB()
		if !ok {
			return nil
		}
		return []string{target.DBPath}
	}
	WatchFiles(ctx, paths, time.Second, time.Second, func() {
		if err := s.refreshFavorites(ctx); err != nil {
			log.Printf("refresh favorite cache failed: %v", err)
			return
		}
		log.Printf("favorite cache refreshed")
	})
}

func (s *Server) watchSNS(ctx context.Context) {
	paths := func() []string {
		target, ok := key.DiscoverSNSDB()
		if !ok {
			return nil
		}
		return []string{target.DBPath}
	}
	WatchFiles(ctx, paths, time.Second, time.Second, func() {
		if err := s.refreshSNS(ctx); err != nil {
			log.Printf("refresh sns cache failed: %v", err)
			return
		}
		log.Printf("sns cache refreshed")
	})
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
		return
	}
	switch req.Action {
	case ActionHealth:
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "ok", Version: buildinfo.String()})
	case ActionRefreshContacts:
		if err := s.refreshContact(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
	case ActionRefreshMessages:
		if err := s.refreshMessages(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
		s.refreshIndexBackground(ctx, "refresh_messages action")
	case ActionRefreshIndex:
		if err := s.refreshIndex(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
	case ActionRefreshSessions:
		if err := s.refreshSessions(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
	case ActionRefreshAvatars:
		if err := s.refreshHeadImage(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
	case ActionRefreshFavorites:
		if err := s.refreshFavorites(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
	case ActionRefreshSNS:
		if err := s.refreshSNS(ctx); err != nil {
			_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "refreshed"})
	case ActionStop:
		s.markIndexInterrupted()
		_ = json.NewEncoder(conn).Encode(Response{OK: true, Message: "stopping"})
		if s.Shutdown != nil {
			go s.Shutdown()
		}
	default:
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Message: "unknown daemon action: " + req.Action})
	}
}
