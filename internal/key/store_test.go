package key

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStoreFindByDataDirRelPathAndSalt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	store := Store{Version: 1}
	entry := Entry{
		Account:   "wxid_a",
		DataDir:   filepath.Join(dir, "db_storage"),
		DBRelPath: "contact/contact.db",
		Key:       "001122",
		Salt:      "aabbcc",
	}
	store.Upsert(entry)
	if err := SaveStore(path, store); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("keys.json mode = %o, want 0600", info.Mode().Perm())
	}
	var file storeFile
	readJSONFile(t, path, &file)
	if file.Version != 1 {
		t.Fatalf("saved version = %d, want 1", file.Version)
	}
	if file.Account != entry.Account {
		t.Fatalf("saved account = %q, want %q", file.Account, entry.Account)
	}
	if file.DataDir != entry.DataDir {
		t.Fatalf("saved data dir = %q, want %q", file.DataDir, entry.DataDir)
	}
	if _, ok := file.Keys[entry.DBRelPath]; !ok {
		t.Fatalf("expected key grouped under rel path %q", entry.DBRelPath)
	}

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Find(entry.DataDir, entry.DBRelPath, "AABBCC")
	if !ok {
		t.Fatal("expected key hit")
	}
	if got.Fingerprint == "" {
		t.Fatal("expected fingerprint")
	}
	if _, ok := loaded.Find(entry.DataDir, entry.DBRelPath, "different"); ok {
		t.Fatal("expected salt mismatch to miss")
	}
}

func TestLoadStoreGroupedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	entry := Entry{
		Account:   "wxid_old",
		DataDir:   filepath.Join(dir, "db_storage"),
		DBRelPath: "message/message_0.db",
		Key:       "aabbcc",
		Salt:      "ddeeff",
	}
	writeJSONFile(t, path, storeFile{
		Version: 1,
		Account: entry.Account,
		DataDir: entry.DataDir,
		Keys: map[string]dbKeyStore{
			entry.DBRelPath: {
				Key:  entry.Key,
				Salt: entry.Salt,
			},
		},
	})

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Find(entry.DataDir, entry.DBRelPath, "DDEEFF")
	if !ok {
		t.Fatal("expected grouped key hit")
	}
	if got.Account != entry.Account {
		t.Fatalf("account = %q, want %q", got.Account, entry.Account)
	}
}

func TestKeyStorePathIsAccountScoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := KeyStorePath("wxid/a b")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".wxview", "cache", "wxid_a_b", "keys.json")
	if path != want {
		t.Fatalf("KeyStorePath = %q, want %q", path, want)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}
