// Package testdb gives integration tests a clean Postgres schema. Set TEST_DATABASE_URL to enable them.
package testdb

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(context.Background(), `DROP TABLE IF EXISTS outbox, payments, events`); err != nil {
		t.Fatal(err)
	}
	return db
}
