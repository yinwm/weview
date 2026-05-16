//go:build windows

package key

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverWindowsContactDBFromExplicitEnv(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "xwechat_files", "wxid_a", "db_storage")
	contactDB := filepath.Join(dataDir, "contact", "contact.db")
	writeWindowsTestFile(t, contactDB)
	t.Setenv(envWindowsDBStorage, dataDir)
	t.Setenv(envWindowsDataRoot, "")
	t.Setenv("APPDATA", filepath.Join(dir, "empty-appdata"))
	t.Setenv("USERPROFILE", filepath.Join(dir, "empty-userprofile"))

	got, err := DiscoverContactDB()
	if err != nil {
		t.Fatal(err)
	}
	if got.Account != "wxid_a" {
		t.Fatalf("account = %q, want wxid_a", got.Account)
	}
	if got.DataDir != dataDir {
		t.Fatalf("data dir = %q, want %q", got.DataDir, dataDir)
	}
	if got.DBPath != contactDB {
		t.Fatalf("db path = %q, want %q", got.DBPath, contactDB)
	}
}

func TestDiscoverWindowsMessageDBsSortsNumericShards(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "xwechat_files", "wxid_a", "db_storage")
	writeWindowsTestFile(t, filepath.Join(dataDir, "contact", "contact.db"))
	writeWindowsTestFile(t, filepath.Join(dataDir, "message", "message_10.db"))
	writeWindowsTestFile(t, filepath.Join(dataDir, "message", "message_2.db"))
	t.Setenv(envWindowsDBStorage, dataDir)
	t.Setenv(envWindowsDataRoot, "")
	t.Setenv("APPDATA", filepath.Join(dir, "empty-appdata"))
	t.Setenv("USERPROFILE", filepath.Join(dir, "empty-userprofile"))

	got, err := DiscoverMessageDBs()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("message db count = %d, want 2", len(got))
	}
	if got[0].DBRelPath != "message/message_2.db" || got[1].DBRelPath != "message/message_10.db" {
		t.Fatalf("message db order = %q, %q", got[0].DBRelPath, got[1].DBRelPath)
	}
}

func TestDecodeWindowsConfigPathUTF16(t *testing.T) {
	got := decodeWindowsConfigPath([]byte{0xff, 0xfe, 'D', 0, ':', 0, '\\', 0, 'W', 0, 'e', 0, 'C', 0, 'h', 0, 'a', 0, 't', 0})
	want := `D:\WeChat`
	if got != want {
		t.Fatalf("decoded path = %q, want %q", got, want)
	}
}

func writeWindowsTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
}
