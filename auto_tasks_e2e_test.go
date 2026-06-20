//go:build !ci

package tinkerdown_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"net/http/httptest"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/livetemplate/tinkerdown/internal/server"
)

// autoTasksConsoleLogs provides thread-safe console log capture for auto-tasks tests
type autoTasksConsoleLogs struct {
	mu   sync.RWMutex
	logs []string
}

func (l *autoTasksConsoleLogs) append(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, msg)
}

func (l *autoTasksConsoleLogs) get() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]string{}, l.logs...)
}

// toggleFirstCheckbox clicks the first checkbox in the given lvt-source section
// and waits until its checked state matches wantChecked.
//
// It uses a JS .click() rather than chromedp.Click: CDP's synthetic mouse click
// does not reliably trigger livetemplate's delegated event handlers in headless
// Docker Chrome (the same reason action_buttons_e2e_test.go uses .click()). It
// then polls for the resulting checked state instead of sleeping a fixed
// duration, so a slow WebSocket round-trip + file write no longer flakes.
func toggleFirstCheckbox(source string, wantChecked bool, timeout time.Duration) chromedp.Action {
	sel := fmt.Sprintf(`document.querySelectorAll('[lvt-source="%s"] input[type="checkbox"]')`, source)
	return chromedp.Tasks{
		chromedp.Evaluate(sel+`[0].click()`, nil),
		waitForDOM(fmt.Sprintf(`%s[0].checked === %t`, sel, wantChecked), timeout),
	}
}

// createAutoTasksExample creates a temp directory with a zero-config markdown file
// containing task list sections (no frontmatter, no separate data file).
func createAutoTasksExample(t *testing.T) (string, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "auto-tasks-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Zero-config markdown: just headings + task lists
	content := `# My Day

## Morning Tasks
- [ ] Make coffee
- [x] Exercise
- [ ] Check email

## Evening Tasks
- [ ] Cook dinner
- [ ] Read book
`

	indexPath := filepath.Join(tempDir, "index.md")
	if err := os.WriteFile(indexPath, []byte(content), 0644); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to write index.md: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

// autoTasksTestContext holds test infrastructure for auto-tasks E2E tests
type autoTasksTestContext struct {
	Server      *httptest.Server
	ChromeCtx   *DockerChromeContext
	URL         string
	ConsoleLogs *autoTasksConsoleLogs
	srv         *server.Server
}

func setupAutoTasksTest(t *testing.T, exampleDir string, enableWatch bool) (*autoTasksTestContext, func()) {
	t.Helper()

	srv := server.New(exampleDir)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Failed to discover pages: %v", err)
	}

	if enableWatch {
		if err := srv.EnableWatch(true); err != nil {
			t.Fatalf("Failed to enable file watching: %v", err)
		}
	}

	handler := server.WithCompression(srv)
	ts := httptest.NewServer(handler)

	chromeCtx, chromeCleanup := SetupDockerChrome(t, 90*time.Second)

	url := ConvertURLForDockerChrome(ts.URL)

	consoleLogs := &autoTasksConsoleLogs{}
	chromedp.ListenTarget(chromeCtx.Context, func(ev any) {
		if ev, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			for _, arg := range ev.Args {
				consoleLogs.append(fmt.Sprintf("[Console] %s", arg.Value))
			}
		}
	})

	testCtx := &autoTasksTestContext{
		Server:      ts,
		ChromeCtx:   chromeCtx,
		URL:         url,
		ConsoleLogs: consoleLogs,
		srv:         srv,
	}

	cleanup := func() {
		chromeCleanup()
		if enableWatch {
			srv.StopWatch()
		}
		ts.Close()
	}

	return testCtx, cleanup
}

// TestAutoTasks_BasicToggle verifies that plain markdown checkboxes render as
// interactive elements and can be toggled, with changes persisted to the file.
func TestAutoTasks_BasicToggle(t *testing.T) {
	tempDir, tempCleanup := createAutoTasksExample(t)
	defer tempCleanup()

	testCtx, cleanup := setupAutoTasksTest(t, tempDir, false)
	defer cleanup()

	ctx := testCtx.ChromeCtx.Context
	t.Logf("Test server URL: %s", testCtx.Server.URL)
	t.Logf("Temp dir: %s", tempDir)

	// Navigate and wait for content to render
	var hasAutoSource bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(testCtx.URL+"/"),
		chromedp.Sleep(15*time.Second), // Wait for plugin compilation
		chromedp.Evaluate(`document.querySelector('[lvt-source="_auto_morning-tasks"]') !== null`, &hasAutoSource),
	)
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	if !hasAutoSource {
		var htmlContent string
		chromedp.Run(ctx, chromedp.OuterHTML("html", &htmlContent))
		t.Logf("HTML (first 3000 chars): %s", htmlContent[:min(3000, len(htmlContent))])
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatal("Auto-generated task list was not rendered")
	}
	t.Log("Auto-generated task list rendered successfully")

	// Verify checkboxes exist
	var checkboxCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelectorAll('[lvt-source="_auto_morning-tasks"] input[type="checkbox"]').length`, &checkboxCount),
	)
	if err != nil {
		t.Fatalf("Failed to count checkboxes: %v", err)
	}

	if checkboxCount != 3 {
		t.Fatalf("Expected 3 checkboxes in morning tasks, got %d", checkboxCount)
	}
	t.Log("Found 3 checkboxes in morning tasks section")

	// Get initial state of "Make coffee" (should be unchecked)
	// Content-based IDs are deterministic via FNV hash
	var makeChecked bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(() => {
				const checkboxes = document.querySelectorAll('[lvt-source="_auto_morning-tasks"] input[type="checkbox"]');
				// First checkbox is "Make coffee" (unchecked)
				return checkboxes.length > 0 ? checkboxes[0].checked : false;
			})()
		`, &makeChecked),
	)
	if err != nil {
		t.Fatalf("Failed to get initial state: %v", err)
	}
	if makeChecked {
		t.Fatal("'Make coffee' should start unchecked")
	}
	t.Log("'Make coffee' starts unchecked")

	// Click the first checkbox to toggle it and wait for the checked state to
	// appear (WebSocket round-trip + file write). A failure here dumps console
	// logs for diagnosis.
	err = chromedp.Run(ctx,
		toggleFirstCheckbox("_auto_morning-tasks", true, 15*time.Second),
	)
	if err != nil {
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatalf("Checkbox should be checked after toggle: %v", err)
	}
	t.Log("Checkbox is now checked in UI")

	// Verify the markdown file was updated on disk
	content, err := os.ReadFile(filepath.Join(tempDir, "index.md"))
	if err != nil {
		t.Fatalf("Failed to read index.md: %v", err)
	}

	fileStr := string(content)
	if !strings.Contains(fileStr, "- [x] Make coffee") {
		t.Logf("File content:\n%s", fileStr)
		t.Fatal("File should contain checked 'Make coffee' task")
	}
	t.Log("File was updated with checked task")

	// Verify the heading is preserved in the file
	if !strings.Contains(fileStr, "## Morning Tasks") {
		t.Fatal("Heading '## Morning Tasks' should still exist in the file")
	}
	t.Log("Headings preserved in file")

	t.Log("BasicToggle test passed!")
}

// TestAutoTasks_AddTask verifies that the auto-generated add form works:
// type a task name, submit, and verify it appears in both UI and file.
func TestAutoTasks_AddTask(t *testing.T) {
	tempDir, tempCleanup := createAutoTasksExample(t)
	defer tempCleanup()

	testCtx, cleanup := setupAutoTasksTest(t, tempDir, false)
	defer cleanup()

	ctx := testCtx.ChromeCtx.Context

	// Navigate and wait
	err := chromedp.Run(ctx,
		chromedp.Navigate(testCtx.URL+"/"),
		chromedp.Sleep(15*time.Second),
		chromedp.WaitVisible(`[lvt-source="_auto_morning-tasks"]`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}
	t.Log("Page loaded with auto-tasks")

	// Type new task text and submit. Use a JS .click() on the submit button
	// (CDP click is unreliable for delegated handlers in headless Docker Chrome)
	// and poll for the new row instead of a fixed sleep.
	err = chromedp.Run(ctx,
		chromedp.SendKeys(`[lvt-source="_auto_morning-tasks"] input[name="text"]`, "Walk the dog", chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('[lvt-source="_auto_morning-tasks"] button[type="submit"]').click()`, nil),
		waitForDOM(`document.querySelectorAll('[lvt-source="_auto_morning-tasks"] input[type="checkbox"]').length === 4`, 15*time.Second),
	)
	if err != nil {
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatalf("Failed to add task: %v", err)
	}
	t.Log("New task appears in UI (4 total)")

	// Verify file contains the new task
	content, err := os.ReadFile(filepath.Join(tempDir, "index.md"))
	if err != nil {
		t.Fatalf("Failed to read index.md: %v", err)
	}

	if !strings.Contains(string(content), "Walk the dog") {
		t.Logf("File content:\n%s", string(content))
		t.Fatal("File should contain 'Walk the dog' task")
	}
	t.Log("New task persisted to file")

	t.Log("AddTask test passed!")
}

// TestAutoTasks_MultipleSections verifies that two independent task sections
// render and operate independently.
func TestAutoTasks_MultipleSections(t *testing.T) {
	tempDir, tempCleanup := createAutoTasksExample(t)
	defer tempCleanup()

	testCtx, cleanup := setupAutoTasksTest(t, tempDir, false)
	defer cleanup()

	ctx := testCtx.ChromeCtx.Context

	// Navigate and wait
	err := chromedp.Run(ctx,
		chromedp.Navigate(testCtx.URL+"/"),
		chromedp.Sleep(15*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	// Verify both sections rendered
	var hasMorning, hasEvening bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector('[lvt-source="_auto_morning-tasks"]') !== null`, &hasMorning),
		chromedp.Evaluate(`document.querySelector('[lvt-source="_auto_evening-tasks"]') !== null`, &hasEvening),
	)
	if err != nil {
		t.Fatalf("Failed to check sections: %v", err)
	}

	if !hasMorning {
		var html string
		chromedp.Run(ctx, chromedp.OuterHTML("html", &html))
		t.Logf("HTML (first 3000 chars): %s", html[:min(3000, len(html))])
		t.Fatal("Morning tasks section not found")
	}
	if !hasEvening {
		t.Fatal("Evening tasks section not found")
	}
	t.Log("Both task sections rendered independently")

	// Count tasks in each section
	var morningCount, eveningCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelectorAll('[lvt-source="_auto_morning-tasks"] input[type="checkbox"]').length`, &morningCount),
		chromedp.Evaluate(`document.querySelectorAll('[lvt-source="_auto_evening-tasks"] input[type="checkbox"]').length`, &eveningCount),
	)
	if err != nil {
		t.Fatalf("Failed to count tasks: %v", err)
	}

	if morningCount != 3 {
		t.Fatalf("Expected 3 morning tasks, got %d", morningCount)
	}
	if eveningCount != 2 {
		t.Fatalf("Expected 2 evening tasks, got %d", eveningCount)
	}
	t.Logf("Morning: %d tasks, Evening: %d tasks", morningCount, eveningCount)

	t.Log("MultipleSections test passed!")
}

// TestAutoTasks_PersistAcrossReload verifies that toggled state survives a page reload.
func TestAutoTasks_PersistAcrossReload(t *testing.T) {
	tempDir, tempCleanup := createAutoTasksExample(t)
	defer tempCleanup()

	testCtx, cleanup := setupAutoTasksTest(t, tempDir, false)
	defer cleanup()

	ctx := testCtx.ChromeCtx.Context

	// Navigate and wait
	err := chromedp.Run(ctx,
		chromedp.Navigate(testCtx.URL+"/"),
		chromedp.Sleep(15*time.Second),
		chromedp.WaitVisible(`[lvt-source="_auto_morning-tasks"]`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	// Toggle first checkbox and wait for the checked state before reloading
	err = chromedp.Run(ctx,
		toggleFirstCheckbox("_auto_morning-tasks", true, 15*time.Second),
	)
	if err != nil {
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatalf("Failed to toggle: %v", err)
	}
	t.Log("Toggled first checkbox")

	// Reload the page
	err = chromedp.Run(ctx,
		chromedp.Navigate(testCtx.URL+"/"),
		chromedp.Sleep(15*time.Second),
		chromedp.WaitVisible(`[lvt-source="_auto_morning-tasks"]`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}
	t.Log("Page reloaded")

	// Verify the toggled state persists
	var isChecked bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(() => {
				const checkboxes = document.querySelectorAll('[lvt-source="_auto_morning-tasks"] input[type="checkbox"]');
				return checkboxes.length > 0 ? checkboxes[0].checked : false;
			})()
		`, &isChecked),
	)
	if err != nil {
		t.Fatalf("Failed to check persisted state: %v", err)
	}

	if !isChecked {
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatal("Checkbox should remain checked after reload")
	}
	t.Log("Checkbox state persisted across reload")

	t.Log("PersistAcrossReload test passed!")
}

// TestAutoTasks_NoFullReload verifies that toggling a checkbox does NOT trigger
// a full browser reload (only a WebSocket data update).
func TestAutoTasks_NoFullReload(t *testing.T) {
	tempDir, tempCleanup := createAutoTasksExample(t)
	defer tempCleanup()

	// Enable file watching — this is where the watcher fix matters
	testCtx, cleanup := setupAutoTasksTest(t, tempDir, true)
	defer cleanup()

	ctx := testCtx.ChromeCtx.Context

	// Navigate and wait
	err := chromedp.Run(ctx,
		chromedp.Navigate(testCtx.URL+"/"),
		chromedp.Sleep(15*time.Second),
		chromedp.WaitVisible(`[lvt-source="_auto_morning-tasks"]`, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	// Inject a marker into the DOM that would be lost on full page reload
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(() => {
				window.__autoTasksReloadMarker = 'not-reloaded';
				return true;
			})()
		`, nil),
	)
	if err != nil {
		t.Fatalf("Failed to set marker: %v", err)
	}
	t.Log("Reload marker set")

	// Toggle checkbox and wait for the checked state (watcher + refresh cycle)
	err = chromedp.Run(ctx,
		toggleFirstCheckbox("_auto_morning-tasks", true, 15*time.Second),
	)
	if err != nil {
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatalf("Failed to toggle: %v", err)
	}
	t.Log("Toggled checkbox with file watcher active")

	// Check that the marker is still present (no full reload happened)
	var marker string
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`window.__autoTasksReloadMarker || 'reloaded'`, &marker),
	)
	if err != nil {
		t.Fatalf("Failed to check marker: %v", err)
	}

	if marker != "not-reloaded" {
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatalf("Expected marker 'not-reloaded', got %q — full page reload occurred!", marker)
	}
	t.Log("No full page reload — watcher fix working correctly")

	// Verify the checkbox was still toggled successfully
	var isChecked bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(() => {
				const checkboxes = document.querySelectorAll('[lvt-source="_auto_morning-tasks"] input[type="checkbox"]');
				return checkboxes.length > 0 ? checkboxes[0].checked : false;
			})()
		`, &isChecked),
	)
	if err != nil {
		t.Fatalf("Failed to verify toggle: %v", err)
	}

	if !isChecked {
		t.Fatal("Checkbox should be checked even without full reload")
	}
	t.Log("Checkbox toggled via WebSocket data refresh (no reload)")

	t.Log("NoFullReload test passed!")
}

// createXSSAutoTasksExample creates a temp directory with a markdown file
// containing task items that use common XSS payloads. Payloads use non-blocking
// sentinels (window flags) instead of alert() so the test fails cleanly rather
// than hanging on a browser dialog if escaping ever regresses.
func createXSSAutoTasksExample(t *testing.T) (string, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "auto-tasks-xss-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	content := `# XSS Test

## Tasks
- [ ] <script>window.__xssExecutedScript = true;</script>
- [ ] <img src=x onerror="window.__xssExecutedImg = true;">
- [ ] Test & <b>bold</b> "quotes"
`

	indexPath := filepath.Join(tempDir, "index.md")
	if err := os.WriteFile(indexPath, []byte(content), 0644); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to write index.md: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

// TestAutoTasks_XSSEscaping verifies that XSS payloads in task text are escaped
// by html/template and rendered as visible text, not as executable HTML elements.
// This guards against regressions like switching to text/template.
func TestAutoTasks_XSSEscaping(t *testing.T) {
	tempDir, tempCleanup := createXSSAutoTasksExample(t)
	defer tempCleanup()

	testCtx, cleanup := setupAutoTasksTest(t, tempDir, false)
	defer cleanup()

	ctx := testCtx.ChromeCtx.Context
	t.Logf("Test server URL: %s", testCtx.Server.URL)

	// Navigate and wait for auto-tasks to render
	var hasAutoSource bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(testCtx.URL+"/"),
		chromedp.Sleep(15*time.Second),
		chromedp.Evaluate(`document.querySelector('[lvt-source="_auto_tasks"]') !== null`, &hasAutoSource),
	)
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	if !hasAutoSource {
		var htmlContent string
		chromedp.Run(ctx, chromedp.OuterHTML("html", &htmlContent))
		t.Logf("HTML (first 3000 chars): %s", htmlContent[:min(3000, len(htmlContent))])
		t.Logf("Console logs: %v", testCtx.ConsoleLogs.get())
		t.Fatal("Auto-generated task list was not rendered")
	}
	t.Log("XSS task list rendered successfully")

	// 1. Verify no <script> tags were injected into the task section
	var scriptCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelectorAll('[lvt-source="_auto_tasks"] script').length`, &scriptCount),
	)
	if err != nil {
		t.Fatalf("Failed to check for script tags: %v", err)
	}
	if scriptCount != 0 {
		t.Fatalf("XSS: Found %d <script> tags in task section — payloads were not escaped!", scriptCount)
	}
	t.Log("No <script> tags injected")

	// 2. Verify no <img> tags were injected (checkboxes are <input>, not <img>)
	var imgCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelectorAll('[lvt-source="_auto_tasks"] img').length`, &imgCount),
	)
	if err != nil {
		t.Fatalf("Failed to check for img tags: %v", err)
	}
	if imgCount != 0 {
		t.Fatalf("XSS: Found %d <img> tags in task section — payloads were not escaped!", imgCount)
	}
	t.Log("No <img> tags injected")

	// 3. Verify no <b> tags from the raw HTML tag payload
	var boldCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelectorAll('[lvt-source="_auto_tasks"] b').length`, &boldCount),
	)
	if err != nil {
		t.Fatalf("Failed to check for bold tags: %v", err)
	}
	if boldCount != 0 {
		t.Fatalf("XSS: Found %d <b> tags in task section — HTML was not escaped!", boldCount)
	}
	t.Log("No <b> tags injected")

	// 4. Verify task text is visible as escaped text content
	var taskTexts string
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(() => {
				const spans = document.querySelectorAll('[lvt-source="_auto_tasks"] label span');
				return Array.from(spans).map(s => s.textContent).join('|||');
			})()
		`, &taskTexts),
	)
	if err != nil {
		t.Fatalf("Failed to get task text content: %v", err)
	}

	// The raw XSS payloads should appear as visible text, not as executed HTML
	if !strings.Contains(taskTexts, "<script>") {
		t.Logf("Task texts: %s", taskTexts)
		t.Fatal("Expected '<script>' to appear as visible text in task span")
	}
	if !strings.Contains(taskTexts, "<img") {
		t.Logf("Task texts: %s", taskTexts)
		t.Fatal("Expected '<img' to appear as visible text in task span")
	}
	if !strings.Contains(taskTexts, "<b>bold</b>") {
		t.Logf("Task texts: %s", taskTexts)
		t.Fatal("Expected '<b>bold</b>' to appear as visible text in task span")
	}
	t.Logf("XSS payloads rendered as escaped text: %s", taskTexts)

	// 5. Check for XSS execution via window sentinel flags.
	// The payloads set window.__xssExecutedScript / window.__xssExecutedImg
	// if they execute. This is deterministic — unlike console log scanning,
	// it directly detects whether the browser ran the injected code.
	var scriptExecuted bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`window.__xssExecutedScript === true`, &scriptExecuted),
	)
	if err != nil {
		t.Fatalf("Failed to check script sentinel flag: %v", err)
	}
	if scriptExecuted {
		t.Fatal("XSS: <script> payload executed — window.__xssExecutedScript was set!")
	}

	var imgExecuted bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`window.__xssExecutedImg === true`, &imgExecuted),
	)
	if err != nil {
		t.Fatalf("Failed to check img sentinel flag: %v", err)
	}
	if imgExecuted {
		t.Fatal("XSS: <img onerror> payload executed — window.__xssExecutedImg was set!")
	}
	t.Log("No XSS execution detected (sentinel flags unset)")

	t.Log("XSSEscaping test passed!")
}
