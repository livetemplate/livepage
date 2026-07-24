package commands

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/source"
	"github.com/livetemplate/tinkerdown/internal/wasm"
)

// defaultTimeout is the default context timeout for CLI operations
const defaultTimeout = 30 * time.Second

// maxColumnWidth is the maximum width for table columns before truncation
const maxColumnWidth = 50

// CLICommand implements the cli command for CRUD operations on sources.
// Usage: tinkerdown cli <file.md|directory> <action> <source> [flags]
func CLICommand(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: tinkerdown cli <file.md|directory> <action> <source> [flags]\n\n" +
			"Actions:\n" +
			"  list      List items from a source\n" +
			"  add       Add a new item to a source\n" +
			"  update    Update an existing item by ID\n" +
			"  delete    Delete an item by ID\n\n" +
			"Flags:\n" +
			"  --format=<table|json|csv>  Output format (default: table)\n" +
			"  --filter=<field=value>     Filter results\n" +
			"  --id=<id>                  Item ID for update/delete\n" +
			"  --<field>=<value>          Field values for add/update\n" +
			"  -y, --yes                  Skip confirmation prompts\n\n" +
			"Examples:\n" +
			"  tinkerdown cli app.md list tasks\n" +
			"  tinkerdown cli app.md list tasks --format=json\n" +
			"  tinkerdown cli app.md add tasks --text=\"New task\"\n" +
			"  tinkerdown cli app.md update tasks --id=abc123 --done=true\n" +
			"  tinkerdown cli app.md delete tasks --id=abc123 -y")
	}

	// Parse base arguments
	pathArg := args[0]
	action := args[1]
	sourceName := args[2]
	flags := args[3:]

	// Parse flags
	opts := parseFlags(flags)

	// Resolve path and load config
	absPath, err := filepath.Abs(pathArg)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Determine directory - if path is a file, use its directory
	var dir string
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", pathArg)
	}
	if info.IsDir() {
		dir = absPath
	} else {
		dir = filepath.Dir(absPath)
	}

	// Load config
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if source exists in config
	if cfg.Sources == nil {
		return fmt.Errorf("no sources defined in config")
	}
	srcCfg, ok := cfg.Sources[sourceName]
	if !ok {
		available := make([]string, 0, len(cfg.Sources))
		for name := range cfg.Sources {
			available = append(available, name)
		}
		sort.Strings(available)
		return fmt.Errorf("source %q not found. Available sources: %s", sourceName, strings.Join(available, ", "))
	}

	// Create the source directly based on type
	src, err := createCLISource(sourceName, srcCfg, dir, absPath)
	if err != nil {
		return fmt.Errorf("failed to create source: %w", err)
	}
	defer func() {
		if err := src.Close(); err != nil {
			log.Printf("warning: failed to close source: %v", err)
		}
	}()

	// Create context with timeout to prevent hanging operations
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	switch action {
	case "list":
		return cliList(ctx, src, opts)
	case "add":
		return cliAdd(ctx, src, opts)
	case "update":
		return cliUpdate(ctx, src, opts)
	case "delete":
		return cliDelete(ctx, src, opts)
	default:
		return fmt.Errorf("unknown action: %s. Valid actions: list, add, update, delete", action)
	}
}

// cliOptions holds parsed command-line flags
type cliOptions struct {
	format string                 // Output format: table, json, csv
	filter string                 // Filter expression: field=value
	id     string                 // Item ID for update/delete
	yes    bool                   // Skip confirmation
	fields map[string]interface{} // Field values for add/update
}

// parseFlags parses command-line flags
func parseFlags(args []string) cliOptions {
	opts := cliOptions{
		format: "table",
		fields: make(map[string]interface{}),
	}

	for _, arg := range args {
		if arg == "-y" || arg == "--yes" {
			opts.yes = true
			continue
		}

		if !strings.HasPrefix(arg, "--") {
			continue
		}

		// Remove leading --
		arg = strings.TrimPrefix(arg, "--")

		// Split on first =
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case "format":
			// Validate format value
			switch value {
			case "table", "json", "csv":
				opts.format = value
			default:
				// Invalid format - keep default and log warning
				log.Printf("warning: invalid format %q, using default 'table'", value)
			}
		case "filter":
			opts.filter = value
		case "id":
			opts.id = value
		default:
			// Parse value as appropriate type
			opts.fields[key] = parseValue(value)
		}
	}

	return opts
}

// parseValue attempts to parse a string value into an appropriate Go type
func parseValue(s string) interface{} {
	// Try boolean
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Check if it contains a decimal point - if so, parse as float
	// This prevents "123.0" from being parsed as int64
	if strings.Contains(s, ".") {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}

	// Try integer (only for values without decimal points)
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}

	// Try float as fallback (handles scientific notation like "1e5")
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Return as string
	return s
}

// createCLISource creates a source for CLI use
func createCLISource(name string, cfg config.SourceConfig, siteDir, currentFile string) (source.Source, error) {
	switch cfg.Type {
	case "sqlite":
		return source.NewSQLiteSource(name, cfg.DB, cfg.Table, siteDir, cfg.IsReadonly())
	case "json":
		return source.NewJSONFileSource(name, cfg.File, siteDir)
	case "csv":
		return source.NewCSVFileSource(name, cfg.File, siteDir, cfg.Options)
	case "markdown":
		return source.NewMarkdownSource(name, cfg.File, cfg.Anchor, siteDir, currentFile, cfg.IsReadonly())
	case "rest":
		return source.NewRestSourceWithConfig(name, cfg)
	case "pg":
		return source.NewPostgresSourceWithConfig(name, cfg.Query, cfg.Options, cfg)
	case "graphql":
		return source.NewGraphQLSource(name, cfg, siteDir)
	case "wasm":
		// WASM has full CLI parity with `serve`: a module that exports `write` gets
		// add/update/delete, and a read-only one (no `write` export) lists only —
		// NewWasmSource derives writability from the module (WasmSource.IsReadonly),
		// so the cliAdd/cliUpdate/cliDelete readonly guard applies uniformly.
		return wasm.NewWasmSource(name, cfg.Path, siteDir, cfg.Options)
	// exec is intentionally not constructed here: it is a read-oriented command
	// runner gated by --allow-exec, not a CRUD store, so it has no place in the
	// `tinkerdown cli` add/update/delete surface. This omission is deliberate.
	default:
		return nil, fmt.Errorf("unsupported source type for CLI: %s", cfg.Type)
	}
}

// cliList fetches and displays items from a source
func cliList(ctx context.Context, src source.Source, opts cliOptions) error {
	data, err := src.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch data: %w", err)
	}

	// Apply filter if specified
	if opts.filter != "" {
		data = applyFilter(data, opts.filter)
	}

	if len(data) == 0 {
		// For JSON format, output empty array; for others, output message
		switch opts.format {
		case "json":
			fmt.Println("[]")
		case "csv":
			// For CSV, just output headers (no data rows)
			fmt.Println("No items found.")
		default:
			fmt.Println("No items found.")
		}
		return nil
	}

	switch opts.format {
	case "json":
		return outputJSON(data)
	case "csv":
		return outputCSV(data)
	default:
		return outputTable(data)
	}
}

// applyFilter filters data based on field=value expression
func applyFilter(data []map[string]interface{}, filter string) []map[string]interface{} {
	if filter == "" {
		return data
	}

	// Parse filter: field=value or field!=value
	var field, value string
	var negate bool

	if idx := strings.Index(filter, "!="); idx >= 0 {
		field = filter[:idx]
		value = filter[idx+2:]
		negate = true
	} else if idx := strings.Index(filter, "="); idx >= 0 {
		field = filter[:idx]
		value = filter[idx+1:]
	} else {
		return data // Invalid filter (no operator found)
	}

	// Validate field name is not empty
	if field == "" {
		return data // Invalid filter (empty field name)
	}

	// Parse the filter value to enable type-aware comparison
	parsedValue := parseValue(value)

	result := make([]map[string]interface{}, 0)
	for _, item := range data {
		itemValue, exists := item[field]
		if !exists {
			if negate {
				result = append(result, item)
			}
			continue
		}

		// Compare using both parsed value and string representation for flexibility
		var matches bool
		if itemValue == parsedValue {
			matches = true
		} else {
			// Handle boolean comparison (SQLite stores bools as 0/1)
			if boolVal, isBool := parsedValue.(bool); isBool {
				switch v := itemValue.(type) {
				case int64:
					matches = (boolVal && v == 1) || (!boolVal && v == 0)
				case float64:
					matches = (boolVal && v == 1) || (!boolVal && v == 0)
				case int:
					matches = (boolVal && v == 1) || (!boolVal && v == 0)
				default:
					// Fallback to string comparison
					itemStr := fmt.Sprintf("%v", itemValue)
					matches = itemStr == value
				}
			} else {
				// Fallback to string comparison for mixed types
				itemStr := fmt.Sprintf("%v", itemValue)
				matches = itemStr == value
			}
		}

		if negate {
			matches = !matches
		}

		if matches {
			result = append(result, item)
		}
	}

	return result
}

// outputJSON outputs data as JSON
func outputJSON(data []map[string]interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// outputCSV outputs data as CSV
func outputCSV(data []map[string]interface{}) error {
	if len(data) == 0 {
		return nil
	}

	// Get all columns from all rows
	columnSet := make(map[string]bool)
	for _, row := range data {
		for col := range row {
			columnSet[col] = true
		}
	}

	// Sort columns for consistent output
	columns := make([]string, 0, len(columnSet))
	for col := range columnSet {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// Write CSV
	w := csv.NewWriter(os.Stdout)

	// Header
	if err := w.Write(columns); err != nil {
		return err
	}

	// Rows
	for _, row := range data {
		record := make([]string, len(columns))
		for i, col := range columns {
			if val, ok := row[col]; ok {
				record[i] = fmt.Sprintf("%v", val)
			}
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}

	// Flush and check for errors
	w.Flush()
	return w.Error()
}

// truncateString truncates a string to maxColumnWidth with ellipsis if needed
func truncateString(s string) string {
	if len(s) > maxColumnWidth {
		return s[:maxColumnWidth-3] + "..."
	}
	return s
}

// outputTable outputs data as a formatted table
func outputTable(data []map[string]interface{}) error {
	if len(data) == 0 {
		return nil
	}

	// Get all columns from all rows
	columnSet := make(map[string]bool)
	for _, row := range data {
		for col := range row {
			columnSet[col] = true
		}
	}

	// Sort columns with id first
	columns := make([]string, 0, len(columnSet))
	for col := range columnSet {
		columns = append(columns, col)
	}
	sort.Slice(columns, func(i, j int) bool {
		// Put "id" first
		if columns[i] == "id" {
			return true
		}
		if columns[j] == "id" {
			return false
		}
		return columns[i] < columns[j]
	})

	// Calculate column widths
	widths := make(map[string]int)
	for _, col := range columns {
		widths[col] = len(col)
	}
	for _, row := range data {
		for col, val := range row {
			str := truncateString(fmt.Sprintf("%v", val))
			if len(str) > widths[col] {
				widths[col] = len(str)
			}
		}
	}

	// Print header
	var header strings.Builder
	var separator strings.Builder
	for i, col := range columns {
		if i > 0 {
			header.WriteString(" | ")
			separator.WriteString("-+-")
		}
		header.WriteString(fmt.Sprintf("%-*s", widths[col], col))
		separator.WriteString(strings.Repeat("-", widths[col]))
	}
	fmt.Println(header.String())
	fmt.Println(separator.String())

	// Print rows
	for _, row := range data {
		var line strings.Builder
		for i, col := range columns {
			if i > 0 {
				line.WriteString(" | ")
			}
			val := ""
			if v, ok := row[col]; ok {
				val = truncateString(fmt.Sprintf("%v", v))
			}
			line.WriteString(fmt.Sprintf("%-*s", widths[col], val))
		}
		fmt.Println(line.String())
	}

	fmt.Printf("\n%d item(s)\n", len(data))
	return nil
}

// cliAdd adds a new item to a source
func cliAdd(ctx context.Context, src source.Source, opts cliOptions) error {
	writable, ok := src.(source.WritableSource)
	if !ok {
		return fmt.Errorf("source does not support write operations")
	}

	if writable.IsReadonly() {
		return fmt.Errorf("source is read-only")
	}

	if len(opts.fields) == 0 {
		return fmt.Errorf("no fields provided. Use --field=value to specify fields")
	}

	if err := writable.WriteItem(ctx, "add", opts.fields); err != nil {
		return fmt.Errorf("failed to add item: %w", err)
	}

	fmt.Println("Item added successfully.")
	return nil
}

// cliUpdate updates an existing item
func cliUpdate(ctx context.Context, src source.Source, opts cliOptions) error {
	writable, ok := src.(source.WritableSource)
	if !ok {
		return fmt.Errorf("source does not support write operations")
	}

	if writable.IsReadonly() {
		return fmt.Errorf("source is read-only")
	}

	if opts.id == "" {
		return fmt.Errorf("--id is required for update")
	}

	if len(opts.fields) == 0 {
		return fmt.Errorf("no fields to update. Use --field=value to specify fields")
	}

	// Add ID to fields (parse ID to appropriate type for comparison)
	data := make(map[string]interface{})
	for k, v := range opts.fields {
		data[k] = v
	}
	data["id"] = parseValue(opts.id)

	if err := writable.WriteItem(ctx, "update", data); err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	fmt.Printf("Item %s updated successfully.\n", opts.id)
	return nil
}

// cliDelete deletes an item
func cliDelete(ctx context.Context, src source.Source, opts cliOptions) error {
	writable, ok := src.(source.WritableSource)
	if !ok {
		return fmt.Errorf("source does not support write operations")
	}

	if writable.IsReadonly() {
		return fmt.Errorf("source is read-only")
	}

	if opts.id == "" {
		return fmt.Errorf("--id is required for delete")
	}

	// Confirmation prompt unless -y is provided
	if !opts.yes {
		fmt.Printf("Are you sure you want to delete item %s? [y/N] ", opts.id)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			// Default to cancel on read error for safety
			fmt.Println("\nDelete cancelled (failed to read input).")
			return nil
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Delete cancelled.")
			return nil
		}
	}

	if err := writable.WriteItem(ctx, "delete", map[string]interface{}{
		"id": parseValue(opts.id),
	}); err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	fmt.Printf("Item %s deleted successfully.\n", opts.id)
	return nil
}
