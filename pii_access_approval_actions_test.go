package tinkerdown_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/source"
	_ "modernc.org/sqlite"
)

// TestPIIActionsAreBoundedAndIdempotent exercises the reference manifest's
// approve-export / deny-request SQL directly through source.RunSQLAction — the
// real execution path — to pin the guarantees a forged or replayed request must
// not break:
//
//   - a non-matching (or already-decided) id exports ZERO rows, not the whole
//     table (the LIMIT must not fall back to unbounded on a NULL row_cap);
//   - a decision writes exactly one audit row and one export set, bounded to the
//     request's cap;
//   - a replay — a double-click that beats the UI refresh, or a direct call with
//     an old id — is a no-op: no second export, no second audit.
//
// It is not a browser test on purpose: these are server-authoritative invariants
// that hold regardless of what the UI renders, so they belong in CI.
func TestPIIActionsAreBoundedAndIdempotent(t *testing.T) {
	config.SetOperator("approver@corp.example")
	t.Cleanup(func() { config.SetOperator("") })

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "access.db")
	seedPIIActionsDB(t, dbPath)

	cfg, err := config.LoadFromDir("examples/pii-access-approval")
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	approve := cfg.Actions["approve-export"]
	deny := cfg.Actions["deny-request"]
	if approve == nil || deny == nil {
		t.Fatal("manifest is missing approve-export/deny-request")
	}

	src, err := source.NewSQLiteSource("access_requests", dbPath, "access_requests", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	ctx := context.Background()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	count := func(q string, args ...interface{}) int {
		var n int
		if err := db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", q, err)
		}
		return n
	}
	status := func(id int) string {
		var s string
		if err := db.QueryRow("SELECT status FROM access_requests WHERE id = ?", id).Scan(&s); err != nil {
			t.Fatalf("status(%d): %v", id, err)
		}
		return s
	}

	totalPII := count("SELECT COUNT(*) FROM orders_pii")

	// A non-matching id: bounded to zero — no unbounded export, no audit.
	if err := source.RunSQLAction(ctx, src, approve, map[string]interface{}{"id": 9999}); err != nil {
		t.Fatalf("approve (non-matching id): %v", err)
	}
	if e := count("SELECT COUNT(*) FROM exports"); e != 0 {
		t.Errorf("non-matching id exported %d rows of %d (want 0) — LIMIT fell back to unbounded on a NULL row_cap", e, totalPII)
	}

	// A real pending request (id 1): capped export + exactly one audit row + approved.
	cap1 := count("SELECT row_cap FROM access_requests WHERE id = 1")
	if cap1 >= totalPII {
		t.Fatalf("fixture invalid: cap %d must be < %d PII rows so the LIMIT bites", cap1, totalPII)
	}
	// The payload smuggles an "operator" key — a client cannot spoof the audit
	// approver, because operator is reserved and server-set (from --operator).
	if err := source.RunSQLAction(ctx, src, approve, map[string]interface{}{"id": 1, "operator": "attacker@evil.example"}); err != nil {
		t.Fatalf("approve (pending): %v", err)
	}
	if e := count("SELECT COUNT(*) FROM exports WHERE request_id = 1"); e != cap1 {
		t.Errorf("bounded export: got %d rows, want the cap %d", e, cap1)
	}
	if a := count("SELECT COUNT(*) FROM audit_log WHERE decision = 'approved'"); a != 1 {
		t.Errorf("approve wrote %d audit rows, want 1", a)
	}
	if status(1) != "approved" {
		t.Errorf("status after approve = %q, want approved", status(1))
	}
	var approver string
	if err := db.QueryRow("SELECT approver FROM audit_log WHERE decision = 'approved'").Scan(&approver); err != nil {
		t.Fatal(err)
	}
	if approver != "approver@corp.example" {
		t.Errorf("audit approver = %q, want the server operator — a client-supplied operator spoofed the audit trail", approver)
	}

	// Replay the same approve: idempotent — no second export, no second audit.
	if err := source.RunSQLAction(ctx, src, approve, map[string]interface{}{"id": 1}); err != nil {
		t.Fatalf("approve (replay): %v", err)
	}
	if e := count("SELECT COUNT(*) FROM exports WHERE request_id = 1"); e != cap1 {
		t.Errorf("replay re-exported: got %d rows, want still %d — idempotency broken", e, cap1)
	}
	if a := count("SELECT COUNT(*) FROM audit_log WHERE decision = 'approved'"); a != 1 {
		t.Errorf("replay appended a second audit row: got %d, want 1", a)
	}

	// Deny a different pending request (id 2), then replay it: same idempotency,
	// and deny must never export.
	if err := source.RunSQLAction(ctx, src, deny, map[string]interface{}{"id": 2}); err != nil {
		t.Fatalf("deny (pending): %v", err)
	}
	if status(2) != "denied" {
		t.Errorf("status after deny = %q, want denied", status(2))
	}
	if err := source.RunSQLAction(ctx, src, deny, map[string]interface{}{"id": 2}); err != nil {
		t.Fatalf("deny (replay): %v", err)
	}
	if a := count("SELECT COUNT(*) FROM audit_log WHERE decision = 'denied'"); a != 1 {
		t.Errorf("deny replay appended a second audit row: got %d, want 1", a)
	}
	if e := count("SELECT COUNT(*) FROM exports WHERE request_id = 2"); e != 0 {
		t.Errorf("deny exported %d rows, want 0 — deny must not grant access", e)
	}

	// Intake is governed: request-access hard-codes status = 'pending' and records
	// no approver, so a requester cannot file a pre-approved row even if the
	// payload smuggles status/approver (they are not statement params, so ignored).
	requestAccess := cfg.Actions["request-access"]
	if requestAccess == nil {
		t.Fatal("manifest is missing request-access")
	}
	before := count("SELECT COUNT(*) FROM access_requests")
	err = source.RunSQLAction(ctx, src, requestAccess, map[string]interface{}{
		"requester": "eve@corp.example", "team": "Growth", "dataset": "orders_pii",
		"row_cap": 2, "scope": "SELECT name,email FROM orders_pii LIMIT 2",
		"reason": "test", "ticket": "GRW-1", "ttl": "24h", "sensitivity": "PII",
		"status": "approved", "approver": "attacker@evil.example", // must be ignored
	})
	if err != nil {
		t.Fatalf("request-access: %v", err)
	}
	if after := count("SELECT COUNT(*) FROM access_requests"); after != before+1 {
		t.Errorf("request-access inserted %d rows, want 1", after-before)
	}
	var newStatus string
	var newApprover sql.NullString
	if err := db.QueryRow("SELECT status, approver FROM access_requests WHERE requester = 'eve@corp.example'").Scan(&newStatus, &newApprover); err != nil {
		t.Fatal(err)
	}
	if newStatus != "pending" {
		t.Errorf("filed request status = %q, want pending — a requester forged a decision via the intake", newStatus)
	}
	if newApprover.Valid {
		t.Errorf("filed request approver = %q, want NULL — the intake must not set an approver", newApprover.String)
	}
}

func seedPIIActionsDB(t *testing.T, dbPath string) {
	t.Helper()
	seed, err := os.ReadFile("examples/pii-access-approval/seed.sql")
	if err != nil {
		t.Fatalf("read seed.sql: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(string(seed)); err != nil {
		t.Fatalf("seed db: %v", err)
	}
}
