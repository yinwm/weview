package sqlitedb

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenReadWriteAndReadOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenReadWrite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecScript(ctx, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO t(name) VALUES ('a');"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	exists, err := TableExists(ctx, ro, "t")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("table should exist")
	}
}

func TestOpenReadWriteUsesWAL(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wal.db")
	db, err := OpenReadWrite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestOpenReadOnlyLiveReadsWritableDB(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "live.db")
	db, err := OpenReadWrite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecScript(ctx, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO t(name) VALUES ('a');"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := OpenReadOnlyLive(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	var name string
	if err := ro.QueryRowContext(ctx, "SELECT name FROM t WHERE id = 1;").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "a" {
		t.Fatalf("name = %q, want a", name)
	}
}
