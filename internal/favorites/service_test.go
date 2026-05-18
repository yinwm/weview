package favorites

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"wxview/internal/sqlitedb/sqlitetest"
)

func TestListExtractsArticleFavorite(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "favorite.db")
	createFavoriteDB(t, db, `
CREATE TABLE fav_db_item (
  local_id INTEGER,
  type INTEGER,
  update_time INTEGER,
  content TEXT,
  fromusr TEXT,
  realchatname TEXT
);
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (
  7,
  5,
  1710000000123,
  '<favitem><weburlitem><pagetitle>Readable Article</pagetitle><pagedesc>Short summary</pagedesc></weburlitem><source><link>https://example.com/article</link></source></favitem>',
  'wxid_from',
  'room@chatroom'
);
`)

	got, err := NewService(db).List(context.Background(), QueryOptions{Type: "article", Query: "Readable", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d favorites, want 1", len(got))
	}
	item := got[0]
	if item.ID != 7 || item.Type != "article" || item.TypeCode != 5 {
		t.Fatalf("unexpected favorite identity: %+v", item)
	}
	if item.Timestamp != 1710000000 {
		t.Fatalf("timestamp = %d, want normalized unix seconds", item.Timestamp)
	}
	if item.Summary != "Readable Article - Short summary" {
		t.Fatalf("summary = %q", item.Summary)
	}
	if item.URL != "https://example.com/article" {
		t.Fatalf("url = %q", item.URL)
	}
	if item.ContentDetail["type"] != "article" || item.ContentDetail["url"] != "https://example.com/article" {
		t.Fatalf("content_detail = %+v", item.ContentDetail)
	}
	if item.From != "wxid_from" || item.SourceChat != "room@chatroom" {
		t.Fatalf("source fields = %+v", item)
	}
}

func TestListLabelsCommonFavoriteTypes(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "favorite.db")
	createFavoriteDB(t, db, `
CREATE TABLE fav_db_item (
  local_id INTEGER,
  type INTEGER,
  update_time INTEGER,
  content TEXT,
  fromusr TEXT,
  realchatname TEXT
);
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (1, 8, 1710000000, '<favitem><datalist><dataitem><datatitle>report.pdf</datatitle><datafmt>pdf</datafmt><fullsize>2048</fullsize></dataitem></datalist></favitem>', '', '');
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (2, 14, 1710000001, '<favitem><title>Project room</title><datalist><dataitem><datadesc>Alice: first line</datadesc></dataitem><dataitem><datadesc>Bob: second line</datadesc></dataitem></datalist></favitem>', '', '');
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (3, 18, 1710000002, '<favitem><datalist><dataitem><datatitle>Note title</datatitle><datadesc>Note body</datadesc></dataitem></datalist></favitem>', '', '');
`)

	got, err := NewService(db).List(context.Background(), QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d favorites, want 3", len(got))
	}
	if got[0].Type != "note" || got[0].Summary != "[笔记] Note title - Note body" {
		t.Fatalf("unexpected note favorite: %+v", got[0])
	}
	if got[1].Type != "chat_history" || got[1].Summary != "[聊天记录] Project room" {
		t.Fatalf("unexpected chat history favorite: %+v", got[1])
	}
	if got[2].Type != "file" || got[2].Summary != "[文件] report.pdf" {
		t.Fatalf("unexpected file favorite: %+v", got[2])
	}
}

func TestListReturnsContentItemsWithoutCDNKeys(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "favorite.db")
	createFavoriteDB(t, db, `
CREATE TABLE fav_db_item (
  local_id INTEGER,
  type INTEGER,
  update_time INTEGER,
  content TEXT,
  fromusr TEXT,
  realchatname TEXT
);
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (
  1,
  2,
  1710000000,
  '<favitem><datalist><dataitem><datafmt>png</datafmt><fullsize>145445</fullsize><cdn_dataurl>https://cdn.example/full.png</cdn_dataurl><cdn_datakey>secret-data-key</cdn_datakey><cdn_thumburl>https://cdn.example/thumb.png</cdn_thumburl><cdn_thumbkey>secret-thumb-key</cdn_thumbkey><fullmd5>image-md5</fullmd5></dataitem></datalist></favitem>',
  '',
  ''
);
`)

	got, err := NewService(db).List(context.Background(), QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d favorites, want 1", len(got))
	}
	item := got[0]
	if item.ContentDetail["media_status"] != "remote_only" {
		t.Fatalf("content_detail = %+v", item.ContentDetail)
	}
	if item.ContentDetail["url"] != "https://cdn.example/full.png" {
		t.Fatalf("url missing: %+v", item.ContentDetail)
	}
	if len(item.ContentItems) != 1 {
		t.Fatalf("content_items = %+v", item.ContentItems)
	}
	contentItem := item.ContentItems[0]
	if contentItem.Format != "png" || contentItem.Size != "145445" || contentItem.URL != "https://cdn.example/full.png" || contentItem.ThumbURL != "https://cdn.example/thumb.png" {
		t.Fatalf("unexpected content item: %+v", contentItem)
	}
	blob := item.ContentDetail["cdn_data_key"] + item.ContentDetail["cdn_thumb_key"] + contentItem.MediaReason
	if strings.Contains(blob, "secret") {
		t.Fatalf("cdn keys leaked: detail=%+v item=%+v", item.ContentDetail, contentItem)
	}
}

func TestListSeparatesWeChatCDNLocatorFromURL(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "favorite.db")
	createFavoriteDB(t, db, `
CREATE TABLE fav_db_item (
  local_id INTEGER,
  type INTEGER,
  update_time INTEGER,
  content TEXT,
  fromusr TEXT,
  realchatname TEXT
);
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (
  1,
  8,
  1710000000,
  '<favitem><datalist><dataitem><datafmt>pdf</datafmt><fullsize>937780</fullsize><datatitle>report.pdf</datatitle><cdn_dataurl>305f02010204abcdef</cdn_dataurl><fullmd5>file-md5</fullmd5></dataitem></datalist></favitem>',
  '',
  ''
);
`)

	got, err := NewService(db).List(context.Background(), QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d favorites, want 1", len(got))
	}
	item := got[0]
	if item.URL != "" {
		t.Fatalf("top-level url should be empty for non-http locator: %+v", item)
	}
	if item.ContentDetail["remote_locator"] != "305f02010204abcdef" || item.ContentDetail["remote_locator_kind"] != "wechat_cdn_locator" {
		t.Fatalf("remote locator missing: %+v", item.ContentDetail)
	}
	if len(item.ContentItems) != 1 || item.ContentItems[0].RemoteLocator != "305f02010204abcdef" {
		t.Fatalf("content item locator missing: %+v", item.ContentItems)
	}
	if item.ContentItems[0].URL != "" {
		t.Fatalf("content item url should be empty for non-http locator: %+v", item.ContentItems[0])
	}
}

func TestListResolvesLocalFavoriteFile(t *testing.T) {
	dir := t.TempDir()
	accountBase := filepath.Join(dir, "wxid_owner_bcc2")
	dataDir := filepath.Join(accountBase, "db_storage")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	createTime := time.Date(2026, time.February, 25, 12, 0, 0, 0, time.Local).Unix()
	filePath := filepath.Join(accountBase, "msg", "file", "2026-02", "report.pdf")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}

	db := filepath.Join(dir, "favorite.db")
	createFavoriteDB(t, db, `
CREATE TABLE fav_db_item (
  local_id INTEGER,
  type INTEGER,
  update_time INTEGER,
  content TEXT,
  fromusr TEXT,
  realchatname TEXT
);
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (
  1,
  8,
  1710000000,
  '<favitem><source><createtime>`+strconv.FormatInt(createTime, 10)+`</createtime></source><datalist><dataitem><datafmt>pdf</datafmt><fullsize>3</fullsize><datatitle>report.pdf</datatitle><cdn_dataurl>305f02010204abcdef</cdn_dataurl><fullmd5>file-md5</fullmd5></dataitem></datalist></favitem>',
  '',
  ''
);
`)

	got, err := NewServiceWithDataDir(db, dataDir).List(context.Background(), QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].ContentItems) != 1 {
		t.Fatalf("got favorites: %+v", got)
	}
	item := got[0]
	if item.ContentDetail["path"] != filePath || item.ContentDetail["media_status"] != "local_path" {
		t.Fatalf("local file was not resolved: %+v", item.ContentDetail)
	}
	if item.ContentItems[0].SourcePath != filePath {
		t.Fatalf("content item source path = %+v", item.ContentItems[0])
	}
}

func TestListParsesCompoundNoteContent(t *testing.T) {
	dir := t.TempDir()
	accountBase := filepath.Join(dir, "wxid_owner_bcc2")
	dataDir := filepath.Join(accountBase, "db_storage")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sourceTime := time.Date(2026, time.January, 16, 8, 48, 0, 0, time.Local).Unix()
	filePath := filepath.Join(accountBase, "msg", "file", "2026-01", "AI 原生组织形态与协作.pdf")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}

	db := filepath.Join(dir, "favorite.db")
	createFavoriteDB(t, db, `
CREATE TABLE fav_db_item (
  local_id INTEGER,
  type INTEGER,
  update_time INTEGER,
  content TEXT,
  fromusr TEXT,
  realchatname TEXT
);
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (
  1,
  18,
  1710000000,
  '<favitem><datalist><dataitem datatype="1"><datadesc>欧盛: 第一段完整文本，不应该被摘要截断。</datadesc></dataitem><dataitem datatype="4" datasourceid="2361525316592321658$3" htmlid="WeNote_7"><datafmt>mp4</datafmt><fullsize>3317276</fullsize><duration>34</duration><datasrcname>胥克谦</datasrcname><srcmsgcreatetime>1768519482</srcmsgcreatetime><datasrctime>2026-1-16 07:24</datasrctime><cdn_dataurl>305f02010204video</cdn_dataurl><messageuuid>uuid-video</messageuuid></dataitem><dataitem datatype="8"><datafmt>pdf</datafmt><datatitle>AI 原生组织形态与协作.pdf</datatitle><fullsize>3</fullsize><srcmsgcreatetime>`+strconv.FormatInt(sourceTime, 10)+`</srcmsgcreatetime><cdn_dataurl>305f02010204pdf</cdn_dataurl></dataitem></datalist></favitem>',
  'wxid_from',
  ''
);
`)

	got, err := NewServiceWithDataDir(db, dataDir).List(context.Background(), QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d favorites, want 1", len(got))
	}
	item := got[0]
	if item.Type != "note" || item.Content != "欧盛: 第一段完整文本，不应该被摘要截断。" {
		t.Fatalf("unexpected note content: %+v", item)
	}
	if item.ContentDetail["text"] != item.Content {
		t.Fatalf("content_detail text missing: %+v", item.ContentDetail)
	}
	if len(item.ContentItems) != 3 {
		t.Fatalf("content_items = %+v", item.ContentItems)
	}
	if item.ContentItems[0].Type != "text" || item.ContentItems[0].Text != item.Content {
		t.Fatalf("text item = %+v", item.ContentItems[0])
	}
	if item.ContentItems[1].Type != "video" || item.ContentItems[1].Duration != "34" || item.ContentItems[1].SourceName != "胥克谦" || item.ContentItems[1].SourceCreateTime != 1768519482 {
		t.Fatalf("video item = %+v", item.ContentItems[1])
	}
	if item.ContentItems[2].Type != "file" || item.ContentItems[2].SourcePath != filePath || item.ContentItems[2].MediaStatus != "local_path" {
		t.Fatalf("file item = %+v", item.ContentItems[2])
	}
}

func TestVideoFilterUsesNormalVideoType(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "favorite.db")
	createFavoriteDB(t, db, `
CREATE TABLE fav_db_item (
  local_id INTEGER,
  type INTEGER,
  update_time INTEGER,
  content TEXT,
  fromusr TEXT,
  realchatname TEXT
);
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (1, 4, 1710000000, '<favitem><datalist><dataitem><datatitle>clip.mp4</datatitle></dataitem></datalist></favitem>', '', '');
INSERT INTO fav_db_item (local_id, type, update_time, content, fromusr, realchatname)
VALUES (2, 20, 1710000001, '<favitem><desc>finder post</desc></favitem>', '', '');
`)

	got, err := NewService(db).List(context.Background(), QueryOptions{Type: "video", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type != "video" || got[0].ID != 1 {
		t.Fatalf("video filter = %+v", got)
	}
}

func TestListRejectsUnknownFavoriteType(t *testing.T) {
	_, err := NewService(filepath.Join(t.TempDir(), "favorite.db")).List(context.Background(), QueryOptions{Type: "bad"})
	if err == nil {
		t.Fatal("expected invalid type error")
	}
}

func createFavoriteDB(t *testing.T, path string, sql string) {
	t.Helper()
	db := sqlitetest.CreateDB(t, path, sql)
	db.Close()
}
