package articles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"wxview/internal/messages"
	"wxview/internal/sqlitecli"
)

type Account struct {
	Username    string `json:"username"`
	Alias       string `json:"alias"`
	Remark      string `json:"remark"`
	NickName    string `json:"nick_name"`
	Description string `json:"description"`
	HeadURL     string `json:"head_url"`
}

type Article struct {
	ID                 string `json:"id"`
	AccountUsername    string `json:"account_username"`
	AccountDisplayName string `json:"account_display_name"`
	Time               string `json:"time"`
	Timestamp          int64  `json:"timestamp"`
	Type               string `json:"type"`
	Text               string `json:"text"`
	Title              string `json:"title,omitempty"`
	Desc               string `json:"desc,omitempty"`
	URL                string `json:"url,omitempty"`
	SourceUsername     string `json:"source_username,omitempty"`
	SourceDisplayName  string `json:"source_display_name,omitempty"`
	Nickname           string `json:"nickname,omitempty"`
	MessageID          string `json:"message_id"`
}

type QueryOptions struct {
	Username string
	Query    string
	Start    int64
	End      int64
	HasStart bool
	HasEnd   bool
	Limit    int
	Offset   int
}

type Service struct {
	ContactDB  string
	MessageDBs []string
}

func NewService(contactDB string, messageDBs []string) Service {
	return Service{ContactDB: contactDB, MessageDBs: messageDBs}
}

func (s Service) Accounts(ctx context.Context, query string) ([]Account, error) {
	if s.ContactDB == "" {
		return nil, fmt.Errorf("contact cache path is empty")
	}
	if _, err := os.Stat(s.ContactDB); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("contact cache does not exist: run `wxview contacts --refresh` or `wxview init` first")
		}
		return nil, err
	}
	sql := `
SELECT
  username,
  COALESCE(alias, '') AS alias,
  COALESCE(remark, '') AS remark,
  COALESCE(nick_name, '') AS nick_name,
  COALESCE(description, '') AS description,
  COALESCE(big_head_url, '') AS head_url,
  COALESCE(verify_flag, 0) AS verify_flag
FROM contact
WHERE username NOT LIKE '%@chatroom'
  AND (username LIKE 'gh_%' OR COALESCE(verify_flag, 0) != 0)
ORDER BY COALESCE(NULLIF(remark, ''), NULLIF(nick_name, ''), username) COLLATE NOCASE;
`
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", sqlitecli.ImmutableURI(s.ContactDB), sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query official accounts: %v: %s", err, bytes.TrimSpace(out))
	}
	var rows []Account
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, fmt.Errorf("parse sqlite json: %w", err)
		}
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return rows, nil
	}
	filtered := make([]Account, 0, len(rows))
	for _, account := range rows {
		if account.matches(needle) {
			filtered = append(filtered, account)
		}
	}
	return filtered, nil
}

func (s Service) List(ctx context.Context, opts QueryOptions) ([]Article, error) {
	if opts.Limit < 0 || opts.Offset < 0 {
		return nil, fmt.Errorf("limit and offset must be >= 0")
	}
	if len(s.MessageDBs) == 0 {
		return nil, fmt.Errorf("message cache does not exist: run `wxview init` with process-memory permissions and retry")
	}
	accounts, err := s.selectedAccounts(ctx, opts)
	if err != nil {
		return nil, err
	}
	msgService := messages.NewService(s.MessageDBs)
	items := make([]Article, 0)
	for _, account := range accounts {
		rows, err := msgService.List(ctx, messages.QueryOptions{
			Username: account.Username,
			Start:    opts.Start,
			End:      opts.End,
			HasStart: opts.HasStart,
			HasEnd:   opts.HasEnd,
		})
		if err != nil {
			continue
		}
		for _, msg := range rows {
			article, ok := articleFromMessage(account, msg)
			if ok {
				items = append(items, article)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Timestamp != items[j].Timestamp {
			return items[i].Timestamp > items[j].Timestamp
		}
		return items[i].ID > items[j].ID
	})
	return paginate(items, opts.Limit, opts.Offset), nil
}

func (s Service) selectedAccounts(ctx context.Context, opts QueryOptions) ([]Account, error) {
	if strings.TrimSpace(opts.Username) != "" {
		accounts, err := s.Accounts(ctx, "")
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if account.Username == strings.TrimSpace(opts.Username) {
				return []Account{account}, nil
			}
		}
		return nil, fmt.Errorf("official account not found: %s", opts.Username)
	}
	return s.Accounts(ctx, opts.Query)
}

func articleFromMessage(account Account, msg messages.Message) (Article, bool) {
	if msg.Type != 49 || msg.ContentDetail == nil {
		return Article{}, false
	}
	kind := msg.ContentDetail["type"]
	switch kind {
	case "link", "channel", "channel_live", "mini_program":
	default:
		return Article{}, false
	}
	return Article{
		ID:                 "wechat:article:" + msg.ID,
		AccountUsername:    account.Username,
		AccountDisplayName: account.DisplayName(),
		Time:               msg.Time,
		Timestamp:          msg.CreateTime,
		Type:               kind,
		Text:               msg.ContentDetail["text"],
		Title:              msg.ContentDetail["title"],
		Desc:               msg.ContentDetail["desc"],
		URL:                msg.ContentDetail["url"],
		SourceUsername:     msg.ContentDetail["source_username"],
		SourceDisplayName:  msg.ContentDetail["source_display_name"],
		Nickname:           msg.ContentDetail["nickname"],
		MessageID:          msg.ID,
	}, true
}

func (a Account) DisplayName() string {
	for _, value := range []string{a.Remark, a.NickName, a.Alias, a.Username} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return a.Username
}

func (a Account) matches(needle string) bool {
	for _, value := range []string{a.Username, a.Alias, a.Remark, a.NickName, a.Description} {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func paginate(items []Article, limit int, offset int) []Article {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []Article{}
	}
	if limit <= 0 {
		return items[offset:]
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
