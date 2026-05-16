package key

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheMetadataFreshness(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	sourcePath := filepath.Join(home, "source.db")
	cachePath := filepath.Join(home, ".wxview", "cache", "wxid_a", "message", "message_0.db")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Unix(1700000000, 123)
	if err := os.Chtimes(sourcePath, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	target := TargetDB{
		Account:   "wxid_a",
		DataDir:   filepath.Dir(sourcePath),
		DBRelPath: "message/message_0.db",
		DBPath:    sourcePath,
	}
	sig, err := statSourceSignature(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := updateCacheMetadata(target, cachePath, "AABBCC", sig); err != nil {
		t.Fatal(err)
	}

	fresh, err := isCacheFresh(target, cachePath, "aabbcc")
	if err != nil {
		t.Fatal(err)
	}
	if !fresh {
		t.Fatal("expected cache to be fresh")
	}

	changedTime := mtime.Add(2 * time.Second)
	if err := os.Chtimes(sourcePath, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	fresh, err = isCacheFresh(target, cachePath, "aabbcc")
	if err != nil {
		t.Fatal(err)
	}
	if fresh {
		t.Fatal("expected source mtime change to make cache stale")
	}
}

func TestCacheMetadataMissingCacheIsStale(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	sourcePath := filepath.Join(home, "source.db")
	cachePath := filepath.Join(home, ".wxview", "cache", "wxid_a", "message", "message_0.db")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := TargetDB{
		Account:   "wxid_a",
		DataDir:   filepath.Dir(sourcePath),
		DBRelPath: "message/message_0.db",
		DBPath:    sourcePath,
	}

	fresh, err := isCacheFresh(target, cachePath, "aabbcc")
	if err != nil {
		t.Fatal(err)
	}
	if fresh {
		t.Fatal("expected missing cache to be stale")
	}
}
