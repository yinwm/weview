package articles

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"wxview/internal/messages"
	"wxview/internal/sqlitedb"
	"wxview/internal/sqlitedb/sqlitetest"
)

func TestAccountsFiltersOfficialAccounts(t *testing.T) {
	dir := t.TempDir()
	contactDB := filepath.Join(dir, "contact.db")
	createArticleContactDB(t, contactDB, `
INSERT INTO contact (username, alias, remark, nick_name, description, big_head_url, verify_flag)
VALUES ('gh_news', 'news_alias', '', 'News Account', 'AI news', 'https://example.com/head.jpg', 0);
INSERT INTO contact (username, alias, remark, nick_name, description, big_head_url, verify_flag)
VALUES ('wxid_verified', '', 'Verified Friend', '', 'verified contact', '', 8);
INSERT INTO contact (username, alias, remark, nick_name, description, big_head_url, verify_flag)
VALUES ('wxid_friend', '', 'Friend', '', '', '', 0);
INSERT INTO contact (username, alias, remark, nick_name, description, big_head_url, verify_flag)
VALUES ('123@chatroom', '', 'Room', '', '', '', 8);
`)

	got, err := NewService(contactDB, nil).Accounts(context.Background(), "news")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got accounts %+v, want only gh_news", got)
	}
	if got[0].Username != "gh_news" || got[0].DisplayName() != "News Account" {
		t.Fatalf("unexpected account: %+v", got[0])
	}
}

func TestListExtractsOfficialAccountArticle(t *testing.T) {
	dir := t.TempDir()
	contactDB := filepath.Join(dir, "contact.db")
	messageDB := filepath.Join(dir, "message_0.db")
	createArticleContactDB(t, contactDB, `
INSERT INTO contact (username, alias, remark, nick_name, description, big_head_url, verify_flag)
VALUES ('gh_news', 'news_alias', 'Daily News', 'News Account', 'AI news', '', 0);
`)
	content := `<?xml version="1.0"?>
<msg>
  <appmsg>
    <title>Readable title</title>
    <des>Readable desc</des>
    <type>5</type>
    <url>https://example.com/a?x=1&amp;y=2</url>
    <sourceusername>gh_source</sourceusername>
    <sourcedisplayname>Source Name</sourcedisplayname>
  </appmsg>
</msg>`
	createArticleMessageDB(t, messageDB, messages.TableName("gh_news"), []articleMessageRow{{
		LocalID:    3,
		SortSeq:    3001,
		CreateTime: 1710000000,
		LocalType:  21474836529,
		Content:    content,
	}})

	got, err := NewService(contactDB, []string{messageDB}).List(context.Background(), QueryOptions{Username: "gh_news", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d articles, want 1", len(got))
	}
	article := got[0]
	if !strings.HasPrefix(article.ID, "wechat:article:wechat:msg:") {
		t.Fatalf("unexpected article id: %q", article.ID)
	}
	if article.AccountUsername != "gh_news" || article.AccountDisplayName != "Daily News" {
		t.Fatalf("unexpected account fields: %+v", article)
	}
	if article.Type != "link" || article.Title != "Readable title" || article.Desc != "Readable desc" {
		t.Fatalf("unexpected article detail: %+v", article)
	}
	if article.URL != "https://example.com/a?x=1&y=2" {
		t.Fatalf("url = %q", article.URL)
	}
	if article.SourceUsername != "gh_source" || article.SourceDisplayName != "Source Name" {
		t.Fatalf("source fields = %+v", article)
	}
}

type articleMessageRow struct {
	LocalID    int64
	SortSeq    int64
	CreateTime int64
	LocalType  int64
	Content    string
}

func createArticleContactDB(t *testing.T, path string, inserts string) {
	t.Helper()
	sql := `
CREATE TABLE contact (
  username TEXT,
  alias TEXT,
  remark TEXT,
  nick_name TEXT,
  description TEXT,
  big_head_url TEXT,
  verify_flag INTEGER
);
` + inserts
	db := sqlitetest.CreateDB(t, path, sql)
	db.Close()
}

func createArticleMessageDB(t *testing.T, path string, table string, rows []articleMessageRow) {
	t.Helper()
	db := sqlitetest.CreateDB(t, path, fmt.Sprintf(`
CREATE TABLE %s (
  local_id INTEGER PRIMARY KEY,
  server_id INTEGER,
  local_type INTEGER,
  sort_seq INTEGER,
  real_sender_id INTEGER,
  create_time INTEGER,
  status INTEGER,
  message_content BLOB,
  WCDB_CT_message_content INTEGER
);
CREATE TABLE Name2Id(user_name TEXT PRIMARY KEY, is_session INTEGER);
INSERT INTO Name2Id(rowid, user_name, is_session) VALUES (2, 'self_user', 0);
`, sqlitedb.QuoteIdent(table)))
	defer db.Close()
	for _, row := range rows {
		sqlitetest.Exec(t, db,
			fmt.Sprintf("INSERT INTO %s (local_id, server_id, local_type, sort_seq, real_sender_id, create_time, status, message_content, WCDB_CT_message_content) VALUES (?, 0, ?, ?, 0, ?, 0, ?, 0);", sqlitedb.QuoteIdent(table)),
			row.LocalID,
			row.LocalType,
			row.SortSeq,
			row.CreateTime,
			[]byte(row.Content),
		)
	}
}
