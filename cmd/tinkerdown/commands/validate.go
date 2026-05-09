package commands

import (
	"context"
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

	var errors []string

	// Create chrome context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

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

		// Check for errors
		var hasError bool
		err = chromedp.Run(ctx,
			chromedp.Navigate("file://"+tmpFile),
			chromedp.Sleep(2*time.Second),
			chromedp.Evaluate(`
				document.body.textContent.includes('Syntax error') ||
				document.body.textContent.includes('Parse error')
			`, &hasError),
		)

		if err != nil {
			errors = append(errors, fmt.Sprintf("Diagram %d: Failed to validate (%v)", i+1, err))
		} else if hasError {
			errors = append(errors, fmt.Sprintf("Diagram %d: Mermaid syntax error detected", i+1))
		}
	}

	return errors, nil
}
