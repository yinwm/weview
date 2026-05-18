package sqlitetest

import (
	"context"
	"database/sql"
	"testing"

	"wxview/internal/sqlitedb"
)

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func CreateDB(t *testing.T, path string, script string) *sql.DB {
	t.Helper()
	db, err := sqlitedb.OpenReadWrite(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if script != "" {
		Exec(t, db, script)
	}
	return db
}

func Exec(t *testing.T, db Execer, script string, args ...any) {
	t.Helper()
	if len(args) == 0 {
		if sqlDB, ok := db.(*sql.DB); ok {
			if err := sqlitedb.ExecScript(context.Background(), sqlDB, script); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	if _, err := db.ExecContext(context.Background(), script, args...); err != nil {
		t.Fatal(err)
	}
}
