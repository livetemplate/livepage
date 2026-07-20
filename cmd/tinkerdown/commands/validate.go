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
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/config"
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
		if !strings.HasPrefix(arg, "-") {
			dir = arg
		}
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
		// A malformed config is the config's own problem and is reported by serve;
		// validate should still check every document's syntax rather than refusing
		// to run. Policy checks are skipped for this run.
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

	err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			name := d.Name()
			// Skip hidden directories (starting with . or _)
			if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			// Skip common non-documentation directories
			skipDirs := []string{"node_modules", "vendor", "dist", "build", "target", ".git"}
			for _, skip := range skipDirs {
				if name == skip {
					return filepath.SkipDir
				}
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
			// Policy: does this document stay inside the project's approved surface?
			violations := manifest.CheckPolicy(pageRefs(page))
			for _, v := range violations {
				fileErrors = append(fileErrors, fileValidationError{
					file:  relPath,
					error: v.Error(),
				})
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
			} else if len(violations) == 0 {
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
			name := d.Name()
			if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
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
