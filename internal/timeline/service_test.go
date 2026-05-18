package timeline

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

func TestListMergesChatsAndPagesWithCursor(t *testing.T) {
	dir := t.TempDir()
	db1 := filepath.Join(dir, "message_0.db")
	db2 := filepath.Join(dir, "message_1.db")
	createTimelineMessageDB(t, db1, messages.TableName("ai-a@chatroom"), []timelineMessageRow{
		{LocalID: 1, SortSeq: 1001, CreateTime: 100, Content: "a first"},
		{LocalID: 2, SortSeq: 3001, CreateTime: 300, Content: "a third"},
	})
	createTimelineMessageDB(t, db2, messages.TableName("ai-b@chatroom"), []timelineMessageRow{
		{LocalID: 1, SortSeq: 2001, CreateTime: 200, Content: "b second"},
	})
	hash, err := QueryHash(QueryIdentity{Kind: "chatroom", Query: "AI", Start: 100, End: 300})
	if err != nil {
		t.Fatal(err)
	}
	chats := []messages.ChatInfo{
		{Username: "ai-a@chatroom", Kind: "chatroom", DisplayName: "AI A"},
		{Username: "ai-b@chatroom", Kind: "chatroom", DisplayName: "AI B"},
	}

	first, err := List(context.Background(), messages.NewService([]string{db1, db2}), QueryOptions{
		Chats:     chats,
		Start:     100,
		End:       300,
		Limit:     2,
		QueryHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page cursor missing: %+v", first)
	}
	if len(first.Items) != 2 || first.Items[0].Content != "a first" || first.Items[1].Content != "b second" {
		t.Fatalf("first page order = %+v", first.Items)
	}
	if first.Items[0].ChatDisplayName != "AI A" || first.Items[1].ChatDisplayName != "AI B" {
		t.Fatalf("chat metadata missing: %+v", first.Items)
	}

	second, err := List(context.Background(), messages.NewService([]string{db1, db2}), QueryOptions{
		Chats:     chats,
		Start:     100,
		End:       300,
		Limit:     2,
		Cursor:    first.NextCursor,
		QueryHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.HasMore || len(second.Items) != 1 || second.Items[0].Content != "a third" {
		t.Fatalf("second page = %+v", second)
	}
}

func TestListRejectsCursorForDifferentQuery(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "message_0.db")
	createTimelineMessageDB(t, db, messages.TableName("ai-a@chatroom"), []timelineMessageRow{
		{LocalID: 1, SortSeq: 1001, CreateTime: 100, Content: "a first"},
		{LocalID: 2, SortSeq: 2001, CreateTime: 200, Content: "a second"},
	})
	hash, err := QueryHash(QueryIdentity{Kind: "chatroom", Query: "AI", Start: 100, End: 200})
	if err != nil {
		t.Fatal(err)
	}
	first, err := List(context.Background(), messages.NewService([]string{db}), QueryOptions{
		Chats:     []messages.ChatInfo{{Username: "ai-a@chatroom", Kind: "chatroom", DisplayName: "AI A"}},
		Start:     100,
		End:       200,
		Limit:     1,
		QueryHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = List(context.Background(), messages.NewService([]string{db}), QueryOptions{
		Chats:     []messages.ChatInfo{{Username: "ai-a@chatroom", Kind: "chatroom", DisplayName: "AI A"}},
		Start:     100,
		End:       200,
		Limit:     1,
		Cursor:    first.NextCursor,
		QueryHash: "different",
	})
	if err == nil || !strings.Contains(err.Error(), "cursor does not match query") {
		t.Fatalf("expected query mismatch error, got %v", err)
	}
}

type timelineMessageRow struct {
	LocalID    int64
	SortSeq    int64
	CreateTime int64
	Content    string
}

func createTimelineMessageDB(t *testing.T, path string, table string, rows []timelineMessageRow) {
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
			fmt.Sprintf("INSERT INTO %s (local_id, server_id, local_type, sort_seq, real_sender_id, create_time, status, message_content, WCDB_CT_message_content) VALUES (?, 0, 1, ?, 0, ?, 4, ?, 0);", sqlitedb.QuoteIdent(table)),
			row.LocalID,
			row.SortSeq,
			row.CreateTime,
			[]byte(row.Content),
		)
	}
}
