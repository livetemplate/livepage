package source

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newExecTxDB opens a writable SQLiteSource over a fresh database seeded with
// ddl. The source's table is irrelevant to ExecTx (which runs arbitrary SQL),
// so any table name works.
func newExecTxDB(t *testing.T, ddl string) (*SQLiteSource, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	db.Close()

	src, err := NewSQLiteSource("primary", dbPath, "requests", dir, false)
	if err != nil {
		t.Fatalf("NewSQLiteSource: %v", err)
	}
	t.Cleanup(func() { src.Close() })
	return src, dbPath
}

// TestExecTx_CommitsAllStatements exercises the exact three-statement approve
// shape: a bounded export, an audit append, and a status change, all atomic.
// It also empirically verifies that a scalar-subquery LIMIT is honored — the
// mechanism that makes the export server-authoritative (the cap is read from
// the row, not passed by the client), which the PII console relies on.
func TestExecTx_CommitsAllStatements(t *testing.T) {
	src, dbPath := newExecTxDB(t, `
		CREATE TABLE requests (id INTEGER PRIMARY KEY, status TEXT, dataset TEXT, row_cap INTEGER);
		CREATE TABLE audit   (id INTEGER PRIMARY KEY, request_id INTEGER, decision TEXT);
		CREATE TABLE exports (id INTEGER PRIMARY KEY, request_id INTEGER, val TEXT);
		CREATE TABLE pii     (id INTEGER PRIMARY KEY, dataset TEXT, val TEXT);
		INSERT INTO requests (id, status, dataset, row_cap) VALUES (1, 'pending', 'orders', 3);
		INSERT INTO pii (dataset, val) VALUES ('orders','a'),('orders','b'),('orders','c'),('orders','d'),('orders','e');
	`)

	err := src.ExecTx(context.Background(), []SQLStatement{
		// Bounded export: LIMIT reads row_cap from the request row via a scalar
		// subquery — five PII rows exist, the cap is three, so exactly three
		// must be exported. This proves LIMIT (subquery) works with modernc.
		{Query: "INSERT INTO exports (request_id, val) SELECT ?, val FROM pii WHERE dataset = (SELECT dataset FROM requests WHERE id = ?) LIMIT (SELECT row_cap FROM requests WHERE id = ?)", Args: []interface{}{1, 1, 1}},
		{Query: "INSERT INTO audit (request_id, decision) VALUES (?, 'approved')", Args: []interface{}{1}},
		{Query: "UPDATE requests SET status = 'approved' WHERE id = ?", Args: []interface{}{1}},
	})
	if err != nil {
		t.Fatalf("ExecTx (commit path): %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var exportCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM exports WHERE request_id = 1").Scan(&exportCount); err != nil {
		t.Fatal(err)
	}
	if exportCount != 3 {
		t.Errorf("bounded export: want 3 rows (row_cap), got %d — LIMIT (subquery) not honored, or the cap did not bite", exportCount)
	}

	var status string
	if err := db.QueryRow("SELECT status FROM requests WHERE id = 1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "approved" {
		t.Errorf("status: want approved, got %q", status)
	}

	var auditCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit WHERE request_id = 1").Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Errorf("audit rows: want 1, got %d", auditCount)
	}
}

// TestExecTx_RollsBackOnFailure is the durable-audit guarantee: if any
// statement fails, every prior one is undone. A status change that "succeeds"
// while its audit append fails — leaving access granted with no record — is the
// exact outcome the transaction exists to prevent.
func TestExecTx_RollsBackOnFailure(t *testing.T) {
	src, dbPath := newExecTxDB(t, `
		CREATE TABLE requests (id INTEGER PRIMARY KEY, status TEXT);
		INSERT INTO requests (id, status) VALUES (1, 'pending');
	`)

	err := src.ExecTx(context.Background(), []SQLStatement{
		{Query: "UPDATE requests SET status = 'approved' WHERE id = ?", Args: []interface{}{1}},
		{Query: "INSERT INTO nonexistent_table (x) VALUES (1)"}, // fails
	})
	if err == nil {
		t.Fatal("expected ExecTx to fail on the second statement")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("error should report the rollback, got: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var status string
	if err := db.QueryRow("SELECT status FROM requests WHERE id = 1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Errorf("rollback failed: status is %q, want pending — the first statement was not undone", status)
	}
}

// TestExecTx_ReadonlyRejected confirms a read-only source refuses a
// transactional batch, matching Exec's guard.
func TestExecTx_ReadonlyRejected(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	src, err := NewSQLiteSource("ro", dbPath, "t", dir, true) // readonly
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	err = src.ExecTx(context.Background(), []SQLStatement{{Query: "INSERT INTO t (id) VALUES (1)"}})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("read-only ExecTx: want a read-only error, got %v", err)
	}
}
