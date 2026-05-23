package commands

import (
	"context"
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
)

// ValidateCommand implements the validate command.
func ValidateCommand(args []string) error {
	// Parse arguments
	dir := "."
	if len(args) > 0 {
		dir = args[0]
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

	fmt.Printf("🔍 Validating tinkerdown files in: %s\n\n", absDir)

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
		_, err = tinkerdown.ParseFileInSite(path, absDir)
		if err != nil {
			// Collect error
			fileErrors = append(fileErrors, fileValidationError{
				file:  relPath,
				error: err.Error(),
			})
			totalErrors++
		} else {
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
			} else {
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
