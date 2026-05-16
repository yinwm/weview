package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSudoOwnerFromEnv(t *testing.T) {
	uid, gid, ok := sudoOwnerFromEnv(0, "501", "20")
	if !ok {
		t.Fatal("expected sudo owner")
	}
	if uid != 501 || gid != 20 {
		t.Fatalf("uid/gid = %d/%d", uid, gid)
	}

	if _, _, ok := sudoOwnerFromEnv(501, "501", "20"); ok {
		t.Fatal("non-root process should not use sudo owner")
	}
	if _, _, ok := sudoOwnerFromEnv(0, "root", "20"); ok {
		t.Fatal("invalid uid should not be accepted")
	}
}

func TestSafeAccountDirKeepsReadableWeChatID(t *testing.T) {
	got := SafeAccountDir("wxid_example_bcc2")
	if got != "wxid_example_bcc2" {
		t.Fatalf("SafeAccountDir = %q", got)
	}
}

func TestSafeAccountDirSanitizesPathSeparators(t *testing.T) {
	got := SafeAccountDir("a/b c")
	if got != "a_b_c" {
		t.Fatalf("SafeAccountDir = %q", got)
	}
}

func TestCacheDBPathCreatesAccountDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := CacheDBPath("a/b c", "keys.json")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ConfigDir, "cache", "a_b_c", "keys.json")
	if got != want {
		t.Fatalf("CacheDBPath = %q, want %q", got, want)
	}
	for _, path := range []string{
		filepath.Join(home, ConfigDir),
		filepath.Join(home, ConfigDir, "cache"),
		filepath.Join(home, ConfigDir, "cache", "a_b_c"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", path)
		}
	}
}
