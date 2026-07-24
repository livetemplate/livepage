package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/livetemplate/livetemplate"
	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/server"
	"github.com/livetemplate/tinkerdown/internal/source"
	"golang.org/x/net/html"
)

// ValidateCommand implements the validate command.
func ValidateCommand(args []string) error {
	// Parse arguments
	dir := "."
	summaryOnly := false
	for _, arg := range args {
		if arg == "--summary" {
			summaryOnly = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// Fail loudly. A script expecting JSON from a mistyped --sumary would
			// otherwise silently receive human-readable output instead.
			return fmt.Errorf("unknown flag %q (supported: --summary)", arg)
		}
		dir = arg
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", dir)
	}

	// Get absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Nothing but JSON may reach stdout in summary mode: the consumer is a program
	// parsing this output, not a human reading a terminal.
	if !summaryOnly {
		fmt.Printf("🔍 Validating tinkerdown files in: %s\n\n", absDir)
	}

	// Load the project config so policy can be checked. The parse layer is
	// deliberately config-free — ParseFileInSite never reads tinkerdown.yaml — so
	// anything policy-aware has to load it here. A project without one, or with one
	// that declares no generation block, simply has no approved set and lints clean.
	manifest, manifestErr := config.LoadFromDir(configDir(absDir))
	if manifestErr != nil {
		// In summary mode this must be fatal. The consumer is a program deciding
		// whether to interrupt its operator, and a nil manifest summarises to
		// {"privileged": false, "operations": []} — indistinguishable from "this
		// project has no approved surface". That is a policy gate failing open:
		// "couldn't tell" would be reported as "safe", and the warning goes to
		// stderr, which a consumer parsing stdout never sees.
		//
		// It matters more because ValidateGeneration turns an approval typo into
		// exactly this load error. Failing open here would mute the alarm it exists
		// to raise.
		if summaryOnly {
			return fmt.Errorf("cannot summarise operations: project config failed to load: %w", manifestErr)
		}
		// Outside summary mode, keep checking document syntax rather than refusing
		// to run; serve reports config problems separately.
		fmt.Fprintf(os.Stderr, "⚠️  Could not load project config for policy checks: %v\n", manifestErr)
		manifest = nil
	}

	if summaryOnly {
		return printOperationSummary(absDir, manifest)
	}

	// Discover and validate all markdown files
	var totalFiles int
	var validFiles int
	var totalErrors int
	var fileErrors []fileValidationError

	// The component template sets serve renders each block against, built once
	// and reused for every block's template validation (each Validate call
	// re-parses whatever sets it is handed).
	componentSets := server.ComponentTemplates()

	// Per-app config cache for the bound-refs check: a multi-app tree keeps its
	// sources in each app's own tinkerdown.yaml, not the root the walk started at.
	// walkRoot bounds the per-file config walk-up — the directory when the target
	// is a directory, the file's directory when it is a single file (mirroring
	// configDir), so a single-file validate doesn't read configs above the app.
	configCache := map[string]*config.Config{}
	walkRoot := configDir(absDir)

	err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			if skipWalkDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .md files
		if filepath.Ext(path) != ".md" {
			return nil
		}

		// Get relative path for display
		relPath, err := filepath.Rel(absDir, path)
		if err != nil {
			relPath = path
		}

		totalFiles++

		// Site root = absDir so cross-page includes validate as serve sees them.
		page, err := tinkerdown.ParseFileInSite(path, absDir)
		if err != nil {
			// Collect error
			fileErrors = append(fileErrors, fileValidationError{
				file:  relPath,
				error: err.Error(),
			})
			totalErrors++
		} else {
			// Placement: lvt-* markup outside an ```lvt block is inert. It renders,
			// binds to nothing, and reports no error anywhere — the hardest of the
			// three "validates clean but does nothing" cases to notice, because
			// every available signal says success.
			inert := page.InertAttributes()
			unknown := page.UnknownAttributes()
			if len(inert) > 0 {
				fileErrors = append(fileErrors, fileValidationError{
					file: relPath,
					error: fmt.Sprintf("%s used outside an ```lvt block — this markup renders but nothing binds to it; move it inside a ```lvt fence",
						strings.Join(inert, ", ")),
				})
				totalErrors++
			}

			// Vocabulary: does every lvt-* attribute exist? validate otherwise only
			// proves the document parses, and an unknown attribute is emitted as
			// inert HTML — so a generated page could satisfy "validate is clean"
			// while doing nothing.
			for _, u := range unknown {
				msg := fmt.Sprintf("unknown attribute %q", u.Name)
				if u.Hint != "" {
					msg += " (" + u.Hint + ")"
				}
				fileErrors = append(fileErrors, fileValidationError{file: relPath, error: msg})
				totalErrors++
			}

			// Policy: does this document stay inside the project's approved surface?
			violations := manifest.CheckPolicy(pageRefs(page))
			for _, v := range violations {
				fileErrors = append(fileErrors, fileValidationError{
					file:  relPath,
					error: v.Error(),
				})
				totalErrors++
			}

			// The config governing this file's sources — shared by the bound-refs
			// and action-param checks below.
			fileConfig := sourceConfigForFile(path, walkRoot, configCache)

			// Bound refs: does every source the document binds resolve to a declared
			// one? A typo like lvt-source="reqests" passes every check above but errors
			// at serve as "source not found".
			boundRefDiags := unresolvedSourceDiags(page, fileConfig)
			for _, br := range boundRefDiags {
				fileErrors = append(fileErrors, fileValidationError{file: relPath, error: br})
				totalErrors++
			}

			// Action params: does the document supply every :param its SQL actions
			// reference (via a form field or data-* attribute)? A missing one errors
			// at serve on the first :param substitution.
			paramDiags := unsuppliedActionParams(page, fileConfig)
			for _, pd := range paramDiags {
				fileErrors = append(fileErrors, fileValidationError{file: relPath, error: pd})
				totalErrors++
			}

			// Templates: does every lvt block compile as a livetemplate template?
			// The attribute checks above prove the markup is well-formed, not that
			// the {{...}} template parses — an unclosed {{range}} or an unknown
			// function renders nothing at serve with no error reported here.
			// Validate closes that gap, against the same component and function set
			// serve uses.
			templateDiags := validateBlockTemplates(page, componentSets)
			for _, td := range templateDiags {
				fileErrors = append(fileErrors, fileValidationError{file: relPath, error: td})
				totalErrors++
			}

			// Also validate Mermaid diagrams
			mermaidErrors, err := validateMermaidDiagrams(path)
			if err != nil {
				fileErrors = append(fileErrors, fileValidationError{
					file:  relPath,
					error: fmt.Sprintf("Mermaid validation failed: %v", err),
				})
				totalErrors++
			} else if len(mermaidErrors) > 0 {
				errorMsg := strings.Join(mermaidErrors, "\n  ")
				fileErrors = append(fileErrors, fileValidationError{
					file:  relPath,
					error: fmt.Sprintf("Mermaid errors:\n  %s", errorMsg),
				})
				totalErrors += len(mermaidErrors)
			} else if len(violations) == 0 && len(unknown) == 0 && len(inert) == 0 && len(templateDiags) == 0 && len(boundRefDiags) == 0 && len(paramDiags) == 0 {
				validFiles++
				fmt.Printf("✓ %s\n", relPath)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// Print errors
	if len(fileErrors) > 0 {
		fmt.Printf("\n")
		for _, fe := range fileErrors {
			fmt.Printf("✗ %s:\n", fe.file)
			// Indent error message
			lines := strings.Split(fe.error, "\n")
			for _, line := range lines {
				if line != "" {
					fmt.Printf("  %s\n", line)
				}
			}
			fmt.Printf("\n")
		}
	}

	// Print summary
	separator := "\n" + strings.Repeat("─", 60) + "\n"
	fmt.Print(separator)
	fmt.Println("Summary:")
	fmt.Printf("  Total files: %d\n", totalFiles)
	fmt.Printf("  Valid:       %d\n", validFiles)
	fmt.Printf("  Errors:      %d\n", totalErrors)
	fmt.Printf("\n")

	if totalErrors > 0 {
		fmt.Printf("✗ Validation failed with %d error(s)\n", totalErrors)
		return fmt.Errorf("validation failed")
	}

	fmt.Printf("✓ All checks passed!\n")
	return nil
}

type fileValidationError struct {
	file  string
	error string
}

// validateMermaidDiagrams validates Mermaid diagrams in a markdown file
func validateMermaidDiagrams(filePath string) ([]string, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Extract Mermaid code blocks
	mermaidRegex := regexp.MustCompile("(?s)```mermaid\\n(.+?)\\n```")
	matches := mermaidRegex.FindAllStringSubmatch(string(content), -1)

	if len(matches) == 0 {
		return nil, nil // No Mermaid diagrams found
	}

	var errs []string

	// Create chrome context.
	//
	// --disable-dev-shm-usage is the critical flag on Ubuntu CI runners:
	// the default /dev/shm is 64MB on Docker/Actions runners, Chrome's
	// renderer OOMs trying to allocate shared memory there, and the
	// fallback path manifests as "chrome failed to start: Failed to
	// connect to the bus" (D-Bus negotiation failure after the shm OOM).
	// Switching to /tmp via --disable-dev-shm-usage avoids both.
	//
	// --disable-extensions and --no-first-run shave a second or two off
	// cold-start by skipping the default extension scan and welcome
	// flow Chromium does on a fresh profile.
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("no-first-run", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	// Per-FILE deadline wrapping the whole diagram loop. Without it,
	// a file with many diagrams hitting a sustained Chrome outage could
	// run for N × 92s (3 attempts × 30s + 2 × 1s backoff per diagram)
	// — for 10 diagrams that's ~15 min before the outer CI timeout
	// kills it. 120s comfortably fits one diagram's worst case (92s)
	// plus a second diagram's warm-attempt time, while bounding total
	// wall time even if every diagram exhausts retries. Most files
	// have 0-2 diagrams that complete in seconds.
	//
	// Tradeoff worth knowing: if TWO diagrams both exhaust all retries
	// back-to-back, the second gets ≤28s (120 - 92), shorter than the
	// 30s per-attempt timeout. Its error surfaces as "per-file deadline
	// expired" rather than "after 3 attempts" — same outcome, less
	// triage clarity. If this pattern emerges in real CI logs, the fix
	// is to widen the file budget rather than narrow per-attempt; in
	// practice flakes have been single-diagram so far.
	fileCtx, fileCancel := context.WithTimeout(allocCtx, 120*time.Second)
	defer fileCancel()

	// Create a simple HTML page with Mermaid
	for i, match := range matches {
		mermaidCode := match[1]

		html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.5/dist/mermaid.min.js"></script>
	<script>
		mermaid.initialize({ startOnLoad: true });
	</script>
</head>
<body>
	<div class="mermaid">
%s
	</div>
</body>
</html>
`, mermaidCode)

		// Write temporary HTML file
		tmpFile := fmt.Sprintf("/tmp/mermaid-validate-%d.html", i)
		if err := os.WriteFile(tmpFile, []byte(html), 0644); err != nil {
			return nil, fmt.Errorf("failed to write temp file: %w", err)
		}
		defer os.Remove(tmpFile)

		hasError, runErr := validateOneMermaidDiagramWithRetry(fileCtx, tmpFile)

		if runErr != nil {
			errs = append(errs, fmt.Sprintf("Diagram %d: Failed to validate (%v)", i+1, runErr))
		} else if hasError {
			errs = append(errs, fmt.Sprintf("Diagram %d: Mermaid syntax error detected", i+1))
		}
	}

	return errs, nil
}

// validateOneMermaidDiagramWithRetry runs the chromedp navigation +
// syntax-error check for a single diagram, retrying up to maxAttempts
// times on transient chromedp/websocket failures. Each attempt gets a
// fresh chromedp context with a 30s deadline — the prior attempt's
// context (or its underlying Chrome target) may be dead, so reusing it
// would just hit the same error.
//
// `parentCtx` is the per-FILE deadline (see validateMermaidDiagrams).
// If it expires (context.Canceled or context.DeadlineExceeded on the
// parent) we stop retrying even if the attempt's own error was
// transient — the outer file budget has been spent and any further
// attempts would race the next file's cancellation.
//
// The transient classifier matches the actual flake observed on CI:
// "websocket url timeout reached" (chromedp internal target-connection
// failure) and context.DeadlineExceeded (timeout firing during
// navigation). Non-transient errors (e.g. Chrome failed to launch)
// surface on the first attempt without burning retries.
func validateOneMermaidDiagramWithRetry(parentCtx context.Context, tmpFile string) (bool, error) {
	const (
		maxAttempts    = 3
		attemptTimeout = 30 * time.Second
		backoff        = time.Second
	)

	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// If the per-file budget has already expired, stop. Don't open
		// a new Chrome context only to immediately error on its first
		// step.
		if err := parentCtx.Err(); err != nil {
			return false, fmt.Errorf("per-file deadline expired before diagram could complete (after %d attempt(s)): %w", attempt-1, err)
		}

		hasError, attemptErr := runOneMermaidAttempt(parentCtx, tmpFile, attemptTimeout)
		lastErr = attemptErr

		if lastErr == nil {
			return hasError, nil
		}
		// Check parent BEFORE the transient classifier: if the per-file
		// deadline fired during this attempt, chromedp.Run returns
		// context.DeadlineExceeded (which isTransientChromedpError
		// would classify as retryable). Skip the wasted backoff +
		// next-iteration top-of-loop dance — surface the per-file
		// message immediately so CI logs are unambiguous about which
		// timeout fired.
		if parentErr := parentCtx.Err(); parentErr != nil {
			return false, fmt.Errorf("per-file deadline expired before diagram could complete (after %d attempt(s)): %w", attempt, parentErr)
		}
		if !isTransientChromedpError(lastErr) {
			// Permanent failure (e.g. Chrome won't launch) — don't waste
			// attempts. Surface immediately so the operator gets a fast
			// signal.
			return false, lastErr
		}
		if attempt < maxAttempts {
			// Context-aware backoff: if the per-file deadline fires
			// during the sleep, return directly rather than waking up
			// to the next iteration only to bail at its top-of-loop
			// check. Saves one trip around the loop.
			select {
			case <-time.After(backoff):
			case <-parentCtx.Done():
				return false, fmt.Errorf("per-file deadline expired during backoff (after %d attempt(s)): %w", attempt, parentCtx.Err())
			}
		}
	}

	// Only add the "(per-attempt timeout)" disambiguator when lastErr
	// actually IS a per-attempt context.DeadlineExceeded. If we got
	// here after N transient websocket errors (which don't go through
	// context.DeadlineExceeded), the wrapped error already says
	// "websocket url timeout reached" — distinct enough from the
	// per-file path's "per-file deadline expired" prefix that an
	// extra suffix would be misleading.
	if errors.Is(lastErr, context.DeadlineExceeded) {
		return false, fmt.Errorf("after %d attempts (per-attempt timeout): %w", maxAttempts, lastErr)
	}
	return false, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}

// runOneMermaidAttempt runs a single chromedp Navigate + Evaluate
// against the temp HTML file holding one Mermaid diagram. Fresh
// chromedp context per call (recovers from a dead Chrome target the
// previous attempt may have left in); both cancels are deferred so
// a panic inside chromedp.Run still frees the contexts cleanly. The
// fresh local hasError var per call also obviates the "reset between
// attempts" concern in the caller.
func runOneMermaidAttempt(parentCtx context.Context, tmpFile string, timeout time.Duration) (bool, error) {
	ctx, ctxCancel := chromedp.NewContext(parentCtx)
	defer ctxCancel()
	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	var hasError bool
	err := chromedp.Run(ctx,
		chromedp.Navigate("file://"+tmpFile),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`
			document.body.textContent.includes('Syntax error') ||
			document.body.textContent.includes('Parse error')
		`, &hasError),
	)
	return hasError, err
}

// isTransientChromedpError returns true when err looks like one of the
// known intermittent chromedp/headless-Chrome failure modes that a
// retry with a fresh context can recover from. The two patterns we've
// actually observed in CI:
//
//   - "websocket url timeout reached" — chromedp couldn't (re-)establish
//     the DevTools websocket to the Chrome target within its internal
//     deadline.
//   - context.DeadlineExceeded — our own per-attempt deadline fired
//     during Navigate or Evaluate. Often correlates with Chrome having
//     just been initialised and not yet ready to serve the request.
//
// Anything else (Chrome failed to launch, file write errors, etc.) is
// treated as permanent.
//
// Intentional over-matching: any error whose message contains
// "websocket" or "timeout" (case-insensitive) is classified transient.
// In theory this could trigger retries for a non-transient config
// error like "websocket origin not allowed" — wasting up to 90s before
// surfacing. We accept that trade-off because (a) the real-world
// production flake comes from the websocket layer, and (b) a retried
// config error still surfaces eventually with a clear message. If a
// new error class shows up that's permanently misclassified, narrow
// the matcher to specific chromedp error types rather than
// substring-matching.
func isTransientChromedpError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "websocket") || strings.Contains(msg, "timeout")
}

// validateBlockTemplates runs each of the page's lvt blocks through
// livetemplate.Validate, catching template-syntax and composition errors
// (unclosed {{range}}, unknown functions, unresolved components) that the
// attribute and policy checks cannot see and that otherwise surface only at
// serve time as a block that silently renders nothing. Blocks are checked in
// stable ID order so a generating agent sees the same diagnostics each run.
func validateBlockTemplates(page *tinkerdown.Page, componentSets []*livetemplate.TemplateSet) []string {
	if page == nil {
		return nil
	}
	ids := make([]string, 0, len(page.InteractiveBlocks))
	for id := range page.InteractiveBlocks {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []string
	for _, id := range ids {
		block := page.InteractiveBlocks[id]
		if block == nil {
			continue
		}
		diags, err := livetemplate.Validate(block.Content, livetemplate.WithValidateComponents(componentSets...))
		if err != nil {
			// An infrastructure failure (e.g. a malformed component set) is
			// Tinkerdown's own problem, not the document's — surface it rather
			// than silently pass the block.
			out = append(out, fmt.Sprintf("template validation could not run: %v", err))
			continue
		}
		for _, d := range diags {
			if d.Line > 0 {
				out = append(out, fmt.Sprintf("line %d: %s", d.Line, d.Message))
			} else {
				out = append(out, d.Message)
			}
		}
	}
	return out
}

// unresolvedSourceDiags reports each source a document binds via lvt-source that
// resolves to no declared source — a typo that passes every other check but
// errors at serve ("source not found"). The declared universe mirrors serve's
// getEffectiveSource: the page's own sources (frontmatter + auto-generated,
// carried on page.Config) plus the sources in the tinkerdown.yaml governing the
// file. fileConfig may be nil (a document with no governing config), in which
// case only the page's own sources are declared.
func unresolvedSourceDiags(page *tinkerdown.Page, fileConfig *config.Config) []string {
	if page == nil {
		return nil
	}
	declared := map[string]bool{}
	for name := range page.Config.Sources {
		declared[name] = true
	}
	if fileConfig != nil {
		for name := range fileConfig.Sources {
			declared[name] = true
		}
	}

	var out []string
	for _, name := range page.Refs().Sources { // Refs().Sources is already sorted
		if declared[name] {
			continue
		}
		out = append(out, fmt.Sprintf(
			"source %q is bound via lvt-source but declared nowhere — it will error at serve as \"source not found\"; declared sources: %s",
			name, sourceList(declared)))
	}
	return out
}

// sourceConfigForFile returns the tinkerdown.yaml governing a file's sources: the
// nearest config from the file's directory up to (and including) the validation
// root. Sources live in a per-app config, so a file in a multi-app tree (e.g.
// `validate examples/`) must be checked against its own app's config, not the
// root the walk started at. Cached by the file's directory; nil when no config
// governs the file.
func sourceConfigForFile(path, root string, cache map[string]*config.Config) *config.Config {
	start := filepath.Dir(path)
	if c, ok := cache[start]; ok {
		return c
	}
	var found *config.Config
	for dir := start; ; {
		if hasConfigFile(dir) {
			if c, err := config.LoadFromDir(dir); err == nil {
				found = c
			}
			break
		}
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	cache[start] = found
	return found
}

func hasConfigFile(dir string) bool {
	for _, name := range []string{"tinkerdown.yaml", "lmt.yaml", "livemdtools.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// unsuppliedActionParams reports each :param a SQL action the document invokes
// references but that no form or control supplies — a form omitting a required
// param passes every other check yet errors at serve on the first missing :param
// substitution (SubstituteParams treats an absent key as fatal). :operator is
// excluded: it is server-set, never supplied by the client.
//
// The supplied set is deliberately over-inclusive — every data-* key and named
// form field ANYWHERE in the document, not scoped to the specific invoking form.
// So the check fires only when a param is supplied nowhere at all, which biases
// it toward a safe miss (a param supplied for a different action) over a false
// positive (flagging a param the form actually provides).
func unsuppliedActionParams(page *tinkerdown.Page, fileConfig *config.Config) []string {
	if page == nil || fileConfig == nil {
		return nil
	}
	supplied := suppliedParamNames(page)

	var out []string
	for _, actionName := range page.Refs().Actions { // sorted, built-ins already excluded
		action, ok := fileConfig.Actions[actionName]
		if !ok || action.Kind != "sql" {
			continue
		}
		stmts := action.Statements
		if action.Statement != "" {
			stmts = append([]string{action.Statement}, stmts...)
		}
		for _, param := range source.ReferencedParams(stmts...) {
			if param == "operator" || supplied[param] {
				continue
			}
			out = append(out, fmt.Sprintf(
				"action %q references :%s but no form field or data-* attribute supplies it (it errors at serve: undefined parameter %q)",
				actionName, param, param))
		}
	}
	sort.Strings(out)
	return out
}

// suppliedParamNames collects every param name a document could hand to an
// action: named form fields (<input/select/textarea name=X> → X) and data-*
// attribute keys (data-id → id) anywhere in the markup. data-lvt-* are excluded
// (client-internal hints, never action params). Over-collection is intentional
// and safe — see unsuppliedActionParams.
func suppliedParamNames(page *tinkerdown.Page) map[string]bool {
	out := map[string]bool{}
	scan := func(markup string) {
		if strings.TrimSpace(markup) == "" {
			return
		}
		doc, err := html.Parse(strings.NewReader(markup))
		if err != nil {
			return
		}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode {
				for _, attr := range n.Attr {
					switch {
					case strings.HasPrefix(attr.Key, "data-lvt-"):
						// client-internal, not a param
					case strings.HasPrefix(attr.Key, "data-"):
						out[strings.TrimPrefix(attr.Key, "data-")] = true
					case attr.Key == "name" && (n.Data == "input" || n.Data == "select" || n.Data == "textarea"):
						out[attr.Val] = true
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}
	for _, b := range page.ServerBlocks {
		if b != nil {
			scan(b.Content)
		}
	}
	for _, b := range page.InteractiveBlocks {
		if b != nil {
			scan(b.Content)
		}
	}
	scan(page.StaticHTML)
	return out
}

// sourceList renders a set of source names as a sorted, comma-joined string for
// a diagnostic hint.
func sourceList(m map[string]bool) string {
	if len(m) == 0 {
		return "(none declared)"
	}
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// pageRefs bridges the parser's view of what a document reaches for to the shape the
// policy check consumes. The two are declared separately so internal/config, which
// owns the approved set, need not import the root package.
func pageRefs(page *tinkerdown.Page) config.DocumentRefs {
	refs := page.Refs()
	return config.DocumentRefs{
		Sources:         refs.Sources,
		Actions:         refs.Actions,
		DeclaredSources: refs.DeclaredSources,
		DeclaredActions: refs.DeclaredActions,
	}
}

// configDir returns the directory to look for tinkerdown.yaml in. When validate is
// pointed at a single file, that is the file's directory rather than the file itself.
func configDir(target string) string {
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		return filepath.Dir(target)
	}
	return target
}

// printOperationSummary emits, as JSON, what the documents under dir do with the
// project's approved surface.
//
// JSON because the consumer is a generating agent deciding whether to interrupt its
// operator, not a human reading a terminal. The `privileged` bit carries that decision:
// a console that only reads is not worth a prompt, and a prompt shown for every
// generated page is a prompt nobody reads.
func printOperationSummary(absDir string, manifest *config.Config) error {
	combined := config.DocumentRefs{}
	seen := map[string]bool{}
	add := func(dst *[]string, names []string, kind string) {
		for _, n := range names {
			key := kind + ":" + n
			if seen[key] {
				continue
			}
			seen[key] = true
			*dst = append(*dst, n)
		}
	}

	err := filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipWalkDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		page, perr := tinkerdown.ParseFileInSite(path, absDir)
		if perr != nil {
			// A document that does not parse cannot be described. It has already
			// failed plain `validate`, which is where that is reported.
			return nil
		}
		refs := page.Refs()
		add(&combined.Sources, refs.Sources, "src")
		add(&combined.Actions, refs.Actions, "act")
		add(&combined.DeclaredSources, refs.DeclaredSources, "dsrc")
		add(&combined.DeclaredActions, refs.DeclaredActions, "dact")
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	summary := manifest.Summarize(combined)
	if summary == nil {
		// No generation block: no approved surface, so nothing to review.
		summary = &config.OperationSummary{Operations: []config.Operation{}}
	}
	if summary.Operations == nil {
		summary.Operations = []config.Operation{}
	}

	out, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode summary: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// skipWalkDir reports whether a directory is not worth descending into. Shared so the
// summary walk and the validation walk cannot drift apart on what they consider part
// of the site.
func skipWalkDir(name string) bool {
	if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "vendor", "dist", "build", "target":
		return true
	}
	return false
}
