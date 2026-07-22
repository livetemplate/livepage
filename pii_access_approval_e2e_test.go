//go:build !ci

package tinkerdown_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/server"
	_ "modernc.org/sqlite"
)

// piiConsoleDir is the committed reference app; the tests copy it into a temp
// dir with a freshly seeded database so the mutating flow stays idempotent.
const piiConsoleDir = "examples/pii-access-approval"

const totalPIIRows = 8 // orders_pii row count in seed.sql — every request cap is below this

// channels is the four-channel capture CLAUDE.md requires of an e2e test:
// browser console, server log, WebSocket frames, and rendered HTML + screenshot.
// It dumps everything on failure so a broken run is diagnosable without a rerun.
type channels struct {
	console   []string
	wsRecv    []string
	wsSent    []string
	serverLog *bytes.Buffer
}

func (c *channels) dump(t *testing.T) {
	t.Helper()
	t.Logf("---- browser console (%d) ----", len(c.console))
	for _, l := range c.console {
		t.Log(l)
	}
	t.Logf("---- websocket frames: %d received, %d sent ----", len(c.wsRecv), len(c.wsSent))
	for _, f := range c.wsRecv {
		t.Logf("  <- %s", truncate(f, 200))
	}
	t.Logf("---- server log ----\n%s", c.serverLog.String())
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// captureServerLog redirects the standard logger (which the in-process server
// writes to) into a buffer, teed to stderr so -v still shows it live.
func captureServerLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(io.MultiWriter(os.Stderr, buf))
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return buf
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// seedAccessDB builds a fresh access.db from the reference app's seed.sql.
func seedAccessDB(t *testing.T, dbPath string) {
	t.Helper()
	seed := mustReadFile(t, filepath.Join(piiConsoleDir, "seed.sql"))
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(seed); err != nil {
		t.Fatalf("seed db: %v", err)
	}
}

// stageConsole copies the manifest and the given page markup into a temp dir with
// a freshly seeded database, and returns the served dir and the db path.
func stageConsole(t *testing.T, pageContent string) (dir, dbPath string) {
	t.Helper()
	dir = t.TempDir()
	manifest := mustReadFile(t, filepath.Join(piiConsoleDir, "tinkerdown.yaml"))
	if err := os.WriteFile(filepath.Join(dir, "tinkerdown.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.md"), []byte(pageContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath = filepath.Join(dir, "data", "access.db")
	seedAccessDB(t, dbPath)
	return dir, dbPath
}

// serveConsole loads the manifest and serves the staged dir exactly as the
// `tinkerdown serve` command does (LoadFromDir + NewWithConfig) — so the
// generation block is present and approved sources are pinned. Bare server.New
// uses DefaultConfig and would silently skip the manifest.
func serveConsole(t *testing.T, dir string) *httptest.Server {
	t.Helper()
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	srv := server.NewWithConfig(dir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("discover: %v", err)
	}
	return httptest.NewServer(server.WithCompression(srv))
}

// listen wires up the console + WebSocket-frame channels on a chrome context.
func listen(ctx context.Context, ch *channels) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			for _, arg := range e.Args {
				ch.console = append(ch.console, fmt.Sprintf("[%s] %s", e.Type, arg.Value))
			}
		case *runtime.EventExceptionThrown:
			ch.console = append(ch.console, "[exception] "+e.ExceptionDetails.Text)
		case *network.EventWebSocketFrameReceived:
			ch.wsRecv = append(ch.wsRecv, e.Response.PayloadData)
		case *network.EventWebSocketFrameSent:
			ch.wsSent = append(ch.wsSent, e.Response.PayloadData)
		}
	})
}

func saveScreenshot(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		t.Logf("screenshot failed: %v", err)
		return
	}
	f, err := os.CreateTemp("", name+"-*.png")
	if err != nil {
		t.Logf("screenshot temp file: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(buf); err != nil {
		t.Logf("screenshot write: %v", err)
		return
	}
	t.Logf("screenshot: %s", f.Name())
}

// TestPIIAccessApproval drives the reference console end to end: the pending
// queue renders, Approve runs a bounded export + a durable audit record + a
// status change atomically (server-authoritative — the button sends only the
// request id), and Deny records a decision without granting access.
func TestPIIAccessApproval(t *testing.T) {
	config.SetOperator("approver@corp.example")
	t.Cleanup(func() { config.SetOperator("") })

	serverLog := captureServerLog(t)
	dir, dbPath := stageConsole(t, mustReadFile(t, filepath.Join(piiConsoleDir, "app.md")))

	ts := serveConsole(t, dir)
	defer ts.Close()

	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	defer cleanup()
	ctx := chromeCtx.Context

	ch := &channels{serverLog: serverLog}
	listen(ctx, ch)

	url := ConvertURLForDockerChrome(ts.URL)

	var approveCount int
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(url),
		chromedp.WaitVisible(`[lvt-source="access_requests"]`, chromedp.ByQuery),
		// data-confirm gates the action via window.confirm; auto-accept it.
		chromedp.Evaluate(`window.confirm = () => true; true`, nil),
		chromedp.Sleep(600*time.Millisecond),
		chromedp.Evaluate(`document.querySelectorAll('button[name="approve-export"]').length`, &approveCount),
	)
	if err != nil {
		ch.dump(t)
		t.Fatalf("load console: %v", err)
	}
	if approveCount != 3 {
		ch.dump(t)
		t.Fatalf("expected 3 pending requests with an Approve button, got %d", approveCount)
	}

	// Approve the first pending request. Capture its id + row_cap first so the
	// bounded-export assertion can compare against the request's own cap.
	var approveID string
	if err := chromedp.Run(ctx,
		chromedp.AttributeValue(`button[name="approve-export"]`, "data-id", &approveID, nil, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("read approve id: %v", err)
	}
	rowCap, requester := requestCapAndRequester(t, dbPath, approveID)
	if rowCap >= totalPIIRows {
		t.Fatalf("test fixture invalid: row_cap %d must be below the %d PII rows so LIMIT bites", rowCap, totalPIIRows)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(fmt.Sprintf(`button[name="approve-export"][data-id="%s"]`, approveID), chromedp.ByQuery),
		chromedp.Sleep(700*time.Millisecond),
	); err != nil {
		ch.dump(t)
		t.Fatalf("click approve: %v", err)
	}

	// UI is server-authoritative: the approved request no longer offers Approve.
	var approveGone bool
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`document.querySelector('button[name="approve-export"][data-id="%s"]') === null`, approveID), &approveGone),
	); err != nil {
		t.Fatalf("check approve gone: %v", err)
	}
	if !approveGone {
		ch.dump(t)
		t.Errorf("after approve, the request still offers an Approve button (state not server-authoritative)")
	}

	// DB: status flipped, audit row appended, export bounded to the request's cap.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var status, approver string
	if err := db.QueryRow(`SELECT status, approver FROM access_requests WHERE id = ?`, approveID).Scan(&status, &approver); err != nil {
		t.Fatal(err)
	}
	if status != "approved" {
		t.Errorf("status: want approved, got %q", status)
	}
	if approver != "approver@corp.example" {
		t.Errorf("approver: want approver@corp.example (from --operator), got %q", approver)
	}

	var auditCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE requester = ? AND decision = 'approved'`, requester).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Errorf("audit rows for approved %s: want 1, got %d — the durable audit record is the point", requester, auditCount)
	}

	var exportCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM exports WHERE request_id = ?`, approveID).Scan(&exportCount); err != nil {
		t.Fatal(err)
	}
	if exportCount != rowCap {
		t.Errorf("bounded export: want exactly %d rows (the request's row cap), got %d — the export must be scoped, not blanket", rowCap, exportCount)
	}
	if exportCount >= totalPIIRows {
		t.Errorf("export returned %d of %d PII rows — the LIMIT did not bind, the export is unbounded", exportCount, totalPIIRows)
	}

	// Deny a different pending request: records a decision, grants nothing.
	var denyID string
	if err := chromedp.Run(ctx,
		chromedp.AttributeValue(`button[name="deny-request"]`, "data-id", &denyID, nil, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("read deny id: %v", err)
	}
	if denyID == approveID {
		t.Fatalf("deny id %s should differ from the already-approved %s", denyID, approveID)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(fmt.Sprintf(`button[name="deny-request"][data-id="%s"]`, denyID), chromedp.ByQuery),
		chromedp.Sleep(700*time.Millisecond),
	); err != nil {
		ch.dump(t)
		t.Fatalf("click deny: %v", err)
	}

	var denyStatus string
	if err := db.QueryRow(`SELECT status FROM access_requests WHERE id = ?`, denyID).Scan(&denyStatus); err != nil {
		t.Fatal(err)
	}
	if denyStatus != "denied" {
		t.Errorf("deny status: want denied, got %q", denyStatus)
	}
	var deniedAudit, deniedExports int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE decision = 'denied'`).Scan(&deniedAudit)
	db.QueryRow(`SELECT COUNT(*) FROM exports WHERE request_id = ?`, denyID).Scan(&deniedExports)
	if deniedAudit != 1 {
		t.Errorf("denied audit rows: want 1, got %d", deniedAudit)
	}
	if deniedExports != 0 {
		t.Errorf("deny must not export: got %d export rows for the denied request", deniedExports)
	}

	// The audit trail is a separate block; Refresh pulls the rows the decisions
	// wrote, so the durable trail is visibly rendered, not just present in the DB.
	var auditText string
	if err := chromedp.Run(ctx,
		chromedp.Click(`button[name="Refresh"]`, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Text(`[lvt-source="audit_log"]`, &auditText, chromedp.ByQuery),
	); err != nil {
		ch.dump(t)
		t.Fatalf("refresh audit trail: %v", err)
	}
	if !strings.Contains(auditText, requester) || !strings.Contains(auditText, "approved") {
		ch.dump(t)
		t.Errorf("audit trail did not render the approved decision for %s after refresh:\n%s", requester, auditText)
	}

	// Four-channel capture: WS frames prove the live diff path ran.
	if len(ch.wsRecv) == 0 {
		t.Errorf("no WebSocket frames received — the live update path did not run")
	}
	var html string
	chromedp.Run(ctx, chromedp.OuterHTML(`html`, &html))
	if !strings.Contains(html, "Recent decisions") {
		t.Errorf("rendered HTML missing the audit section")
	}
	saveScreenshot(t, ctx, "pii-console")
	t.Logf("channels: %d console, %d ws-recv, %d ws-sent", len(ch.console), len(ch.wsRecv), len(ch.wsSent))
}

// TestPIIAccessApprovalShadowing is the runtime demonstration Phase 1 (M1) could
// not build: a served page whose frontmatter tries to redefine the approved
// `access_requests` source to point at a decoy table. Because approval pins the
// source, the page must render the APPROVED queue, not the decoy — and the two
// are deliberately distinguishable so a pass cannot be hollow (an empty render
// would fail whether or not pinning works).
func TestPIIAccessApprovalShadowing(t *testing.T) {
	shadowPage := `---
title: Shadow Attempt
sidebar: false
sources:
  access_requests:
    type: sqlite
    db: ./data/access.db
    table: decoy_requests
    readonly: false
---

# Queue

` + "```lvt" + `
<article lvt-source="access_requests">
  <ul>
    {{range .Data}}<li data-key="{{.Id}}">{{.Requester}} — {{.Dataset}}</li>{{end}}
  </ul>
</article>
` + "```\n"

	serverLog := captureServerLog(t)
	dir, _ := stageConsole(t, shadowPage)

	ts := serveConsole(t, dir)
	defer ts.Close()

	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	defer cleanup()
	ctx := chromeCtx.Context

	ch := &channels{serverLog: serverLog}
	listen(ctx, ch)

	var listText string
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(ConvertURLForDockerChrome(ts.URL)),
		chromedp.WaitVisible(`[lvt-source="access_requests"]`, chromedp.ByQuery),
		chromedp.Sleep(700*time.Millisecond),
		chromedp.Text(`[lvt-source="access_requests"]`, &listText, chromedp.ByQuery),
	)
	if err != nil {
		ch.dump(t)
		t.Fatalf("load shadow page: %v", err)
	}

	// The approved queue's rows render; the decoy's do not.
	if !strings.Contains(listText, "dana@corp.example") {
		ch.dump(t)
		t.Errorf("approved queue did not render: expected an approved request (dana@corp.example), got:\n%s", listText)
	}
	if strings.Contains(listText, "shadow-decoy@evil.example") {
		ch.dump(t)
		t.Errorf("frontmatter shadow took effect: the decoy row rendered, so approval did not pin the source:\n%s", listText)
	}
}

func requestCapAndRequester(t *testing.T, dbPath, id string) (rowCap int, requester string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT row_cap, requester FROM access_requests WHERE id = ?`, id).Scan(&rowCap, &requester); err != nil {
		t.Fatalf("read request %s: %v", id, err)
	}
	return rowCap, requester
}
