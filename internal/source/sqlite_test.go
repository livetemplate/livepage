package source

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLiteSource_FetchWithCreatedAt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO items (name, created_at) VALUES ('first', '2024-01-01'), ('second', '2024-01-02')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	src, err := NewSQLiteSource("items", dbPath, "items", dir, true)
	if err != nil {
		t.Fatalf("NewSQLiteSource failed: %v", err)
	}
	defer src.Close()

	data, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(data) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(data))
	}

	// With created_at, rows should be ordered newest-first (DESC)
	if data[0]["name"] != "second" {
		t.Errorf("expected newest row first (created_at DESC), got %v", data[0]["name"])
	}
}

func TestSQLiteSource_FetchWithoutCreatedAt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create table WITHOUT created_at — simulates a user-created database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO items (name) VALUES ('alpha'), ('beta')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	src, err := NewSQLiteSource("items", dbPath, "items", dir, true)
	if err != nil {
		t.Fatalf("NewSQLiteSource failed: %v", err)
	}
	defer src.Close()

	// This was the bug: Fetch would fail with "no such column: created_at"
	data, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed (should work without created_at): %v", err)
	}

	if len(data) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(data))
	}
}

func TestSQLiteSource_AutoCreateHasCreatedAt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	src, err := NewSQLiteSource("items", dbPath, "items", dir, false)
	if err != nil {
		t.Fatalf("NewSQLiteSource failed: %v", err)
	}
	defer src.Close()

	// Write an item — this triggers auto-create with created_at column
	err = src.WriteItem(context.Background(), "add", map[string]interface{}{
		"name": "test item",
	})
	if err != nil {
		t.Fatalf("WriteItem failed: %v", err)
	}

	// Fetch should work and use ORDER BY created_at DESC
	data, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed after auto-create: %v", err)
	}

	if len(data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(data))
	}
}

// TestSQLiteSource_BusyTimeoutApplied verifies that NewSQLiteSource opens its
// connection with a non-zero busy_timeout. A malformed DSN pragma is silently
// ignored by modernc.org/sqlite, so this guards against the lock-contention
// flake (issue #292) silently regressing.
func TestSQLiteSource_BusyTimeoutApplied(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "busy.db")

	src, err := NewSQLiteSource("items", dbPath, "items", dir, false)
	if err != nil {
		t.Fatalf("NewSQLiteSource failed: %v", err)
	}
	defer src.Close()

	var timeout int
	if err := src.db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("failed to read busy_timeout: %v", err)
	}
	if timeout != sqliteBusyTimeoutMs {
		t.Fatalf("expected busy_timeout %d, got %d (DSN pragma not applied?)", sqliteBusyTimeoutMs, timeout)
	}
}
