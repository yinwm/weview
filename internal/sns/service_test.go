package sns

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFeedParsesTimelineMediaWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "sns.db")
	createSNSDB(t, db, `
CREATE TABLE SnsTimeLine (tid INTEGER, user_name TEXT, content TEXT);
CREATE TABLE SnsMessage_tmp3 (
  local_id INTEGER,
  create_time INTEGER,
  feed_id INTEGER,
  from_username TEXT,
  from_nickname TEXT,
  content TEXT,
  is_unread INTEGER
);
INSERT INTO SnsTimeLine (tid, user_name, content) VALUES (
  10,
  'wxid_author',
  '<TimelineObject><username>wxid_author</username><createTime>1710000000</createTime><contentDesc>Hello SNS</contentDesc><location poiName="Cafe"/><ContentObject><mediaList><media><type>2</type><sub_type>0</sub_type><url md5="img-md5" key="secret-key">https://img.example/full.jpg</url><thumb token="secret-token">https://img.example/thumb.jpg</thumb><size width="640" height="480" totalSize="12345"/></media></mediaList></ContentObject></TimelineObject>'
);
`)

	got, err := NewService(db).Feed(context.Background(), QueryOptions{Username: "wxid_author", Query: "hello", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d posts, want 1", len(got))
	}
	post := got[0]
	if post.AuthorUsername != "wxid_author" || post.Content != "Hello SNS" || post.Location != "Cafe" {
		t.Fatalf("unexpected post: %+v", post)
	}
	if post.MediaCount != 1 || len(post.Media) != 1 {
		t.Fatalf("unexpected media count: %+v", post)
	}
	media := post.Media[0]
	if media.URL != "https://img.example/full.jpg" || media.Thumb != "https://img.example/thumb.jpg" || media.MD5 != "img-md5" {
		t.Fatalf("unexpected media: %+v", media)
	}
	if media.Width != "640" || media.Height != "480" || media.TotalSize != "12345" {
		t.Fatalf("unexpected media size: %+v", media)
	}
	blob, err := json.Marshal(post)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "secret-key") || strings.Contains(string(blob), "secret-token") {
		t.Fatalf("secret CDN attributes leaked into JSON: %s", blob)
	}
}

func TestNotificationsReturnUnreadWithFeedPreview(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "sns.db")
	createSNSDB(t, db, `
CREATE TABLE SnsTimeLine (tid INTEGER, user_name TEXT, content TEXT);
CREATE TABLE SnsMessage_tmp3 (
  local_id INTEGER,
  create_time INTEGER,
  feed_id INTEGER,
  from_username TEXT,
  from_nickname TEXT,
  content TEXT,
  is_unread INTEGER
);
INSERT INTO SnsTimeLine (tid, user_name, content) VALUES (
  20,
  'wxid_author',
  '<TimelineObject><username>wxid_author</username><createTime>1710000000</createTime><contentDesc>Feed preview text</contentDesc></TimelineObject>'
);
INSERT INTO SnsMessage_tmp3 (local_id, create_time, feed_id, from_username, from_nickname, content, is_unread)
VALUES (1, 1710000100, 20, 'wxid_friend', 'Friend', 'nice', 1);
INSERT INTO SnsMessage_tmp3 (local_id, create_time, feed_id, from_username, from_nickname, content, is_unread)
VALUES (2, 1710000200, 20, 'wxid_read', 'Read Friend', '', 0);
`)

	got, err := NewService(db).Notifications(context.Background(), QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d notifications, want unread only", len(got))
	}
	item := got[0]
	if item.Type != "comment" || item.Content != "nice" || item.FromUsername != "wxid_friend" {
		t.Fatalf("unexpected notification: %+v", item)
	}
	if item.FeedAuthorUsername != "wxid_author" || item.FeedPreview != "Feed preview text" {
		t.Fatalf("unexpected feed preview: %+v", item)
	}

	all, err := NewService(db).Notifications(context.Background(), QueryOptions{IncludeRead: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Type != "like" {
		t.Fatalf("include-read notifications = %+v", all)
	}
}

func createSNSDB(t *testing.T, path string, sql string) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is required for sns query tests")
	}
	if out, err := exec.Command("sqlite3", path, sql).CombinedOutput(); err != nil {
		t.Fatalf("create sns db: %v: %s", err, out)
	}
}
