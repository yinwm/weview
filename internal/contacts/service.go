package contacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wxview/internal/sqlitedb"
)

const (
	KindAll      = "all"
	KindFriend   = "friend"
	KindChatroom = "chatroom"
	KindOther    = "other"
)

type Contact struct {
	Username string `json:"username"`
	Alias    string `json:"alias"`
	Remark   string `json:"remark"`
	NickName string `json:"nick_name"`
	HeadURL  string `json:"head_url"`
	Kind     string `json:"kind"`
}

type Detail struct {
	Username     string `json:"username"`
	Alias        string `json:"alias"`
	Remark       string `json:"remark"`
	NickName     string `json:"nick_name"`
	HeadURL      string `json:"head_url"`
	SmallHeadURL string `json:"small_head_url"`
	BigHeadURL   string `json:"big_head_url"`
	Description  string `json:"description"`
	VerifyFlag   int    `json:"verify_flag"`
	LocalType    int    `json:"local_type"`
	Kind         string `json:"kind"`
	IsChatroom   bool   `json:"is_chatroom"`
	IsOfficial   bool   `json:"is_official"`
	AvatarStatus string `json:"avatar_status,omitempty"`
	AvatarPath   string `json:"avatar_path,omitempty"`
	AvatarReason string `json:"avatar_reason,omitempty"`
}

type Member struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Alias       string `json:"alias"`
	Remark      string `json:"remark"`
	NickName    string `json:"nick_name"`
	Kind        string `json:"kind"`
	IsOwner     bool   `json:"is_owner"`
}

type GroupMembers struct {
	Username         string   `json:"username"`
	DisplayName      string   `json:"display_name"`
	Owner            string   `json:"owner"`
	OwnerDisplayName string   `json:"owner_display_name"`
	Count            int      `json:"count"`
	Members          []Member `json:"members"`
}

type QueryOptions struct {
	Kind     string
	Query    string
	Username string
	Sort     string
	Limit    int
	Offset   int
}

type Service struct {
	CacheDB string
}

func NewService(cacheDB string) Service {
	return Service{CacheDB: cacheDB}
}

func (s Service) List(ctx context.Context) ([]Contact, error) {
	if s.CacheDB == "" {
		return nil, fmt.Errorf("contact cache path is empty")
	}
	if _, err := os.Stat(s.CacheDB); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("contact cache does not exist: run `wxview contacts --refresh` or `wxview init` first")
		}
		return nil, err
	}

	query := `
SELECT
  COALESCE(id, 0) AS id,
  username,
  COALESCE(local_type, 0) AS local_type,
  COALESCE(alias, '') AS alias,
  COALESCE(remark, '') AS remark,
  COALESCE(nick_name, '') AS nick_name,
  COALESCE(big_head_url, '') AS head_url
FROM contact
ORDER BY COALESCE(NULLIF(remark, ''), NULLIF(nick_name, ''), username) COLLATE NOCASE;
`
	db, err := sqlitedb.OpenReadOnly(ctx, s.CacheDB)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query contact cache: %w", err)
	}
	defer rows.Close()
	ownerUsername := ownerUsernameFromCachePath(s.CacheDB)
	contacts := []Contact{}
	for rows.Next() {
		var (
			id        int
			username  string
			localType int
			alias     string
			remark    string
			nickName  string
			headURL   string
		)
		if err := rows.Scan(&id, &username, &localType, &alias, &remark, &nickName, &headURL); err != nil {
			return nil, err
		}
		contact := Contact{
			Username: username,
			Alias:    alias,
			Remark:   remark,
			NickName: nickName,
			HeadURL:  headURL,
			Kind:     ClassifyKindForAccount(username, localType, id, ownerUsername),
		}
		contacts = append(contacts, contact)
	}
	return contacts, rows.Err()
}

func (s Service) Detail(ctx context.Context, username string) (Detail, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return Detail{}, fmt.Errorf("username is required")
	}
	if err := s.ensureCache(); err != nil {
		return Detail{}, err
	}
	query := `
SELECT
  COALESCE(id, 0),
  username,
  COALESCE(local_type, 0),
  COALESCE(alias, ''),
  COALESCE(remark, ''),
  COALESCE(nick_name, ''),
  COALESCE(small_head_url, ''),
  COALESCE(big_head_url, ''),
  COALESCE(description, ''),
  COALESCE(verify_flag, 0)
FROM contact
WHERE username = ?
LIMIT 1;
`
	db, err := sqlitedb.OpenReadOnly(ctx, s.CacheDB)
	if err != nil {
		return Detail{}, err
	}
	defer db.Close()
	var row struct {
		ID          int
		Username    string
		LocalType   int
		Alias       string
		Remark      string
		NickName    string
		SmallHead   string
		BigHead     string
		Description string
		VerifyFlag  int
	}
	err = db.QueryRowContext(ctx, query, username).Scan(
		&row.ID,
		&row.Username,
		&row.LocalType,
		&row.Alias,
		&row.Remark,
		&row.NickName,
		&row.SmallHead,
		&row.BigHead,
		&row.Description,
		&row.VerifyFlag,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, fmt.Errorf("contact not found: %s", username)
	}
	if err != nil {
		return Detail{}, fmt.Errorf("query contact detail: %w", err)
	}
	ownerUsername := ownerUsernameFromCachePath(s.CacheDB)
	kind := ClassifyKindForAccount(row.Username, row.LocalType, row.ID, ownerUsername)
	return Detail{
		Username:     row.Username,
		Alias:        row.Alias,
		Remark:       row.Remark,
		NickName:     row.NickName,
		HeadURL:      firstNonEmpty(row.BigHead, row.SmallHead),
		SmallHeadURL: row.SmallHead,
		BigHeadURL:   row.BigHead,
		Description:  row.Description,
		VerifyFlag:   row.VerifyFlag,
		LocalType:    row.LocalType,
		Kind:         kind,
		IsChatroom:   kind == KindChatroom,
		IsOfficial:   row.VerifyFlag != 0 || strings.HasPrefix(row.Username, "gh_"),
	}, nil
}

func (s Service) Members(ctx context.Context, username string) (GroupMembers, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return GroupMembers{}, fmt.Errorf("username is required")
	}
	if !strings.HasSuffix(username, "@chatroom") {
		return GroupMembers{}, fmt.Errorf("%s is not a chatroom username", username)
	}
	if err := s.ensureCache(); err != nil {
		return GroupMembers{}, err
	}
	if exists, err := s.tableExists(ctx, "chatroom_member"); err != nil {
		return GroupMembers{}, err
	} else if !exists {
		return GroupMembers{
			Username:    username,
			DisplayName: username,
			Members:     []Member{},
		}, nil
	}

	roomID, displayName, err := s.chatroomIdentity(ctx, username)
	if err != nil {
		return GroupMembers{}, err
	}
	if roomID == 0 {
		return GroupMembers{}, fmt.Errorf("chatroom not found: %s", username)
	}
	owner, _ := s.chatroomOwner(ctx, roomID, username)
	members, err := s.chatroomMembers(ctx, roomID, owner)
	if err != nil {
		return GroupMembers{}, err
	}
	ownerDisplay := owner
	for _, member := range members {
		if member.Username == owner {
			ownerDisplay = member.DisplayName
			break
		}
	}
	return GroupMembers{
		Username:         username,
		DisplayName:      displayName,
		Owner:            owner,
		OwnerDisplayName: ownerDisplay,
		Count:            len(members),
		Members:          members,
	}, nil
}

func (s Service) ensureCache() error {
	if s.CacheDB == "" {
		return fmt.Errorf("contact cache path is empty")
	}
	if _, err := os.Stat(s.CacheDB); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("contact cache does not exist: run `wxview contacts --refresh` or `wxview init` first")
		}
		return err
	}
	return nil
}

func (s Service) tableExists(ctx context.Context, table string) (bool, error) {
	db, err := sqlitedb.OpenReadOnly(ctx, s.CacheDB)
	if err != nil {
		return false, err
	}
	defer db.Close()
	return sqlitedb.TableExists(ctx, db, table)
}

func (s Service) chatroomIdentity(ctx context.Context, username string) (int, string, error) {
	query := `
SELECT
  COALESCE(id, 0),
  COALESCE(NULLIF(remark, ''), NULLIF(nick_name, ''), NULLIF(alias, ''), username)
FROM contact
WHERE username = ?
LIMIT 1;
`
	db, err := sqlitedb.OpenReadOnly(ctx, s.CacheDB)
	if err != nil {
		return 0, "", err
	}
	defer db.Close()
	var id int
	var displayName string
	err = db.QueryRowContext(ctx, query, username).Scan(&id, &displayName)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("query chatroom identity: %w", err)
	}
	return id, displayName, nil
}

func (s Service) chatroomOwner(ctx context.Context, roomID int, username string) (string, error) {
	if exists, err := s.tableExists(ctx, "chat_room"); err != nil {
		return "", err
	} else if !exists {
		return "", nil
	}
	queries := []string{
		"SELECT COALESCE(owner, '') FROM chat_room WHERE id = ? LIMIT 1;",
		"SELECT COALESCE(owner, '') FROM chat_room WHERE username = ? LIMIT 1;",
		"SELECT COALESCE(owner, '') FROM chat_room WHERE chat_room_name = ? LIMIT 1;",
		"SELECT COALESCE(owner, '') FROM chat_room WHERE name = ? LIMIT 1;",
	}
	args := []any{roomID, username, username, username}
	db, err := sqlitedb.OpenReadOnly(ctx, s.CacheDB)
	if err != nil {
		return "", err
	}
	defer db.Close()
	for i, query := range queries {
		var owner string
		err := db.QueryRowContext(ctx, query, args[i]).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			continue
		}
		if strings.TrimSpace(owner) != "" {
			return owner, nil
		}
	}
	return "", nil
}

func (s Service) chatroomMembers(ctx context.Context, roomID int, owner string) ([]Member, error) {
	query := `
SELECT
  COALESCE(c.username, ''),
  COALESCE(c.alias, ''),
  COALESCE(c.remark, ''),
  COALESCE(c.nick_name, ''),
  COALESCE(c.local_type, 0),
  COALESCE(c.id, 0)
FROM chatroom_member cm
LEFT JOIN contact c ON c.id = cm.member_id
WHERE cm.room_id = ?
ORDER BY COALESCE(NULLIF(c.remark, ''), NULLIF(c.nick_name, ''), NULLIF(c.alias, ''), c.username) COLLATE NOCASE;
`
	db, err := sqlitedb.OpenReadOnly(ctx, s.CacheDB)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("query chatroom members: %w", err)
	}
	defer rows.Close()
	ownerUsername := ownerUsernameFromCachePath(s.CacheDB)
	members := []Member{}
	for rows.Next() {
		var row struct {
			Username  string
			Alias     string
			Remark    string
			NickName  string
			LocalType int
			ContactID int
		}
		if err := rows.Scan(&row.Username, &row.Alias, &row.Remark, &row.NickName, &row.LocalType, &row.ContactID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(row.Username) == "" {
			continue
		}
		members = append(members, Member{
			Username:    row.Username,
			DisplayName: displayName(row.Alias, row.Remark, row.NickName, row.Username),
			Alias:       row.Alias,
			Remark:      row.Remark,
			NickName:    row.NickName,
			Kind:        ClassifyKindForAccount(row.Username, row.LocalType, row.ContactID, ownerUsername),
			IsOwner:     owner != "" && row.Username == owner,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].IsOwner != members[j].IsOwner {
			return members[i].IsOwner
		}
		left := strings.ToLower(members[i].DisplayName)
		right := strings.ToLower(members[j].DisplayName)
		if left == right {
			return strings.ToLower(members[i].Username) < strings.ToLower(members[j].Username)
		}
		return left < right
	})
	return members, nil
}

func ClassifyKind(username string, localType int) string {
	if strings.HasSuffix(username, "@chatroom") {
		return KindChatroom
	}
	if localType == 1 && !strings.HasPrefix(username, "gh_") {
		return KindFriend
	}
	return KindOther
}

func ClassifyKindForAccount(username string, localType int, contactID int, ownerUsername string) string {
	if isSelfContact(username, contactID, ownerUsername) {
		return KindOther
	}
	return ClassifyKind(username, localType)
}

func isSelfContact(username string, contactID int, ownerUsername string) bool {
	if ownerUsername != "" && username == ownerUsername {
		return true
	}
	return contactID == 2 && strings.HasPrefix(username, "wxid_")
}

func ownerUsernameFromCachePath(cacheDB string) string {
	account := filepath.Base(filepath.Dir(filepath.Dir(cacheDB)))
	if !strings.HasPrefix(account, "wxid_") {
		return account
	}
	idx := strings.LastIndex(account, "_")
	if idx <= len("wxid_") {
		return account
	}
	candidate := account[:idx]
	if strings.HasPrefix(candidate, "wxid_") {
		return candidate
	}
	return account
}

func FilterByKind(list []Contact, kind string) []Contact {
	if kind == "" || kind == KindAll {
		return list
	}
	out := make([]Contact, 0, len(list))
	for _, contact := range list {
		if contact.Kind == kind {
			out = append(out, contact)
		}
	}
	return out
}

func ApplyQueryOptions(list []Contact, opts QueryOptions) []Contact {
	filtered := make([]Contact, 0, len(list))
	query := strings.ToLower(strings.TrimSpace(opts.Query))
	username := strings.TrimSpace(opts.Username)
	for _, contact := range list {
		if opts.Kind != "" && opts.Kind != KindAll && contact.Kind != opts.Kind {
			continue
		}
		if username != "" && contact.Username != username {
			continue
		}
		if query != "" && !matchesQuery(contact, query) {
			continue
		}
		filtered = append(filtered, contact)
	}
	sortContacts(filtered, opts.Sort)
	return paginate(filtered, opts.Limit, opts.Offset)
}

func matchesQuery(contact Contact, query string) bool {
	for _, value := range []string{contact.Username, contact.Alias, contact.Remark, contact.NickName} {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func sortContacts(list []Contact, sortBy string) {
	switch sortBy {
	case "", "username":
		sort.SliceStable(list, func(i, j int) bool {
			return strings.ToLower(list[i].Username) < strings.ToLower(list[j].Username)
		})
	case "name":
		sort.SliceStable(list, func(i, j int) bool {
			left := strings.ToLower(DisplayName(list[i]))
			right := strings.ToLower(DisplayName(list[j]))
			if left == right {
				return strings.ToLower(list[i].Username) < strings.ToLower(list[j].Username)
			}
			return left < right
		})
	}
}

func DisplayName(contact Contact) string {
	if strings.TrimSpace(contact.Remark) != "" {
		return contact.Remark
	}
	if strings.TrimSpace(contact.NickName) != "" {
		return contact.NickName
	}
	if strings.TrimSpace(contact.Alias) != "" {
		return contact.Alias
	}
	return contact.Username
}

func displayName(alias string, remark string, nickName string, username string) string {
	if strings.TrimSpace(remark) != "" {
		return remark
	}
	if strings.TrimSpace(nickName) != "" {
		return nickName
	}
	if strings.TrimSpace(alias) != "" {
		return alias
	}
	return username
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func paginate(list []Contact, limit int, offset int) []Contact {
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset >= len(list) {
		return []Contact{}
	}
	if limit == 0 {
		return list[offset:]
	}
	end := offset + limit
	if end > len(list) {
		end = len(list)
	}
	return list[offset:end]
}
