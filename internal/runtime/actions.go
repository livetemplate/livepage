package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/security"
	"github.com/livetemplate/tinkerdown/internal/source"
)

// buildCommandString rebuilds the command string from executable and current arg values
func buildCommandString(origCmd string, args []Arg) string {
	// Get executable from original command
	parts := strings.Fields(origCmd)
	if len(parts) == 0 {
		return origCmd
	}
	executable := parts[0]

	// Build new command with current arg values
	cmdParts := []string{executable}
	for _, arg := range args {
		cmdParts = append(cmdParts, "--"+arg.Name, arg.Value)
	}
	return strings.Join(cmdParts, " ")
}

// runExec handles the Run action for exec sources
func (s *GenericState) runExec(data map[string]interface{}) error {
	if s.sourceType != "exec" {
		return fmt.Errorf("Run action only valid for exec sources")
	}

	execSrc, ok := s.source.(*source.ExecSource)
	if !ok {
		return fmt.Errorf("invalid exec source")
	}

	s.Status = "running"

	ctx := context.Background()
	var result []map[string]interface{}
	var err error

	// Check if form data was submitted
	if len(data) > 0 {
		// Update Args with submitted values
		for i := range s.Args {
			if val, ok := data[s.Args[i].Name]; ok {
				valStr := fmt.Sprintf("%v", val)
				// Convert checkbox "on" to "true"
				if valStr == "on" {
					valStr = "true"
				}
				s.Args[i].Value = valStr
			}
		}

		// Build args map from submitted data
		argsMap := make(map[string]string)
		for k, v := range data {
			argsMap[k] = fmt.Sprintf("%v", v)
		}

		// Update the Command string to show current argument values
		s.Command = buildCommandString(s.sourceCfg.Cmd, s.Args)

		// Execute with custom arguments
		result, err = execSrc.FetchWithArgs(ctx, argsMap)
	} else {
		// No form data - use default command
		result, err = execSrc.Fetch(ctx)
	}

	if err != nil {
		s.Status = "error"
		s.Error = err.Error()
		return err
	}

	s.Data = result
	s.Status = "success"
	s.Error = ""
	return nil
}

// handleWriteAction handles Add, Toggle, Delete, Update actions for writable sources
func (s *GenericState) handleWriteAction(action string, data map[string]interface{}) error {
	writable, ok := s.source.(source.WritableSource)
	if !ok {
		return fmt.Errorf("source %q does not support write operations", s.sourceName)
	}

	if writable.IsReadonly() {
		return fmt.Errorf("source %q is read-only", s.sourceName)
	}

	// Resolve template expressions in action data (e.g., {{timestamp}}, {{today}}, {{.operator}})
	// This enables auto-filling timestamps and operator identity on form submission
	resolver := NewDefaultResolver(s.getOperator())
	resolvedData, err := resolver.ResolveMap(data)
	if err != nil {
		s.Error = err.Error()
		return fmt.Errorf("failed to resolve template expressions: %w", err)
	}

	// Delegate to the source's WriteItem
	ctx := context.Background()
	if err := writable.WriteItem(ctx, strings.ToLower(action), resolvedData); err != nil {
		s.Error = err.Error()
		return err
	}

	// Refresh data after write
	return s.refresh()
}

// getOperator returns the current operator identity from config.
func (s *GenericState) getOperator() string {
	return config.GetOperator()
}

// handleDatatableAction handles Sort, NextPage, PrevPage actions
func (s *GenericState) handleDatatableAction(action string, data map[string]interface{}) error {
	// Parse action pattern: Sort_<id>, NextPage_<id>, PrevPage_<id>
	// or just Sort, NextPage, PrevPage (without suffix)
	actionLower := strings.ToLower(action)

	var baseAction string
	if strings.HasPrefix(actionLower, "sort") {
		baseAction = "sort"
	} else if strings.HasPrefix(actionLower, "nextpage") {
		baseAction = "nextpage"
	} else if strings.HasPrefix(actionLower, "prevpage") {
		baseAction = "prevpage"
	} else {
		return fmt.Errorf("unknown datatable action: %s", action)
	}

	switch baseAction {
	case "sort":
		// Get column from data
		column, ok := data["column"].(string)
		if !ok {
			// Try to get from action suffix
			if strings.Contains(action, "_") {
				parts := strings.SplitN(action, "_", 2)
				if len(parts) == 2 {
					column = parts[1]
				}
			}
		}
		if column == "" {
			return fmt.Errorf("sort requires column parameter")
		}
		return s.sortData(column)

	case "nextpage":
		// For now, pagination is handled client-side or via datatable component
		// This is a placeholder for future implementation
		return nil

	case "prevpage":
		return nil

	default:
		return fmt.Errorf("unknown datatable action: %s", baseAction)
	}
}

// sortData sorts the data by the given column
func (s *GenericState) sortData(column string) error {
	if len(s.Data) == 0 {
		return nil
	}

	// Check if column exists
	if _, ok := s.Data[0][column]; !ok {
		return fmt.Errorf("column %q not found in data", column)
	}

	// Toggle between ascending and descending based on current order
	ascending := true
	if len(s.Data) >= 2 {
		first := s.Data[0][column]
		last := s.Data[len(s.Data)-1][column]
		ascending = compareValues(first, last) > 0 // If already desc, make it asc
	}

	// Use sort.Slice for O(n log n) performance
	sort.Slice(s.Data, func(i, j int) bool {
		a := s.Data[i][column]
		b := s.Data[j][column]
		cmp := compareValues(a, b)
		if ascending {
			return cmp < 0
		}
		return cmp > 0
	})

	return nil
}

// compareValues compares two interface{} values for sorting
func compareValues(a, b interface{}) int {
	// Handle nil
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Try string comparison
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		return strings.Compare(aStr, bStr)
	}

	// Try numeric comparison
	aNum := toFloat64(a)
	bNum := toFloat64(b)
	if aNum < bNum {
		return -1
	}
	if aNum > bNum {
		return 1
	}
	return 0
}

// toFloat64 converts an interface{} to float64
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	default:
		return 0
	}
}

// executeCustomAction dispatches a custom action declared in frontmatter.
func (s *GenericState) executeCustomAction(action *config.Action, data map[string]interface{}) error {
	// Validate required params
	if err := validateParams(action, data); err != nil {
		return err
	}

	switch action.Kind {
	case "sql":
		return s.executeSQLAction(action, data)
	case "http":
		return s.executeHTTPAction(action, data)
	case "exec":
		return s.executeExecAction(action, data)
	default:
		return fmt.Errorf("unknown action kind: %s", action.Kind)
	}
}

// validateParams checks that required parameters are present.
func validateParams(action *config.Action, data map[string]interface{}) error {
	for name, def := range action.Params {
		val, exists := data[name]
		if def.Required {
			// Missing key or nil value is always treated as missing
			if !exists || val == nil {
				return fmt.Errorf("required parameter %q is missing", name)
			}
			// For string values, also treat empty string as missing
			if str, ok := val.(string); ok && str == "" {
				return fmt.Errorf("required parameter %q is missing", name)
			}
		}
	}
	return nil
}

// executeSQLAction executes a SQL action against a source.
func (s *GenericState) executeSQLAction(action *config.Action, data map[string]interface{}) error {
	if s.registry == nil {
		return fmt.Errorf("source registry not configured")
	}

	// Look up the source
	src, ok := s.registry(action.Source)
	if !ok {
		return fmt.Errorf("source %q not found", action.Source)
	}

	// Check if source supports SQL execution
	executor, ok := src.(source.SQLExecutor)
	if !ok {
		return fmt.Errorf("source %q does not support SQL execution", action.Source)
	}

	// Inject operator into data for :operator parameter substitution
	// This enables SQL actions like "WHERE assigned_to = :operator" to work
	if data == nil {
		data = make(map[string]interface{})
	}
	if _, exists := data["operator"]; !exists {
		data["operator"] = s.getOperator()
	}

	// Execute with a timeout to avoid hanging indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A statements batch runs atomically: substitute each, then hand the whole
	// set to ExecTx so a state change and its audit record commit or roll back
	// together. Config validation guarantees exactly one of Statement/Statements.
	if len(action.Statements) > 0 {
		stmts := make([]source.SQLStatement, 0, len(action.Statements))
		for _, raw := range action.Statements {
			query, args, err := substituteParams(raw, data)
			if err != nil {
				s.Error = err.Error()
				return err
			}
			stmts = append(stmts, source.SQLStatement{Query: query, Args: args})
		}
		if err := executor.ExecTx(ctx, stmts); err != nil {
			s.Error = err.Error()
			return err
		}
		return s.refresh()
	}

	// Substitute parameters in the single SQL statement.
	query, args, err := substituteParams(action.Statement, data)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	if _, err := executor.Exec(ctx, query, args...); err != nil {
		s.Error = err.Error()
		return err
	}

	// Refresh data after mutation
	return s.refresh()
}

// substituteParams converts :name placeholders to positional args.
// Input:  "DELETE FROM tasks WHERE id = :id", {"id": "123"}
// Output: "DELETE FROM tasks WHERE id = ?", ["123"]
// Returns an error if a parameter in the statement is not found in data.
//
// Parameter names must start with a letter (a-z, A-Z) and can contain
// letters, digits, and underscores. This avoids false matches on:
// - Time literals like '12:30:00' (digits after colon)
// - Postgres casts like value::text (double colon)
func substituteParams(stmt string, data map[string]interface{}) (string, []interface{}, error) {
	var args []interface{}
	result := stmt

	// Find all :name patterns and replace with ?
	// Process in a way that handles overlapping names correctly
	for {
		// Find the next :name pattern
		idx := strings.Index(result, ":")
		if idx == -1 {
			break
		}

		// Skip double colons (postgres cast syntax like ::text)
		if idx+1 < len(result) && result[idx+1] == ':' {
			result = result[:idx] + "\x00DOUBLECOLON\x00" + result[idx+2:]
			continue
		}

		// Check if next character is a letter (parameter names must start with letter)
		if idx+1 >= len(result) {
			// Colon at end of string, not a parameter
			result = result[:idx] + "\x00COLON\x00" + result[idx+1:]
			continue
		}

		firstChar := result[idx+1]
		if !((firstChar >= 'a' && firstChar <= 'z') || (firstChar >= 'A' && firstChar <= 'Z')) {
			// Not a valid parameter (starts with digit, symbol, etc.)
			// This handles time literals like '12:30:00'
			result = result[:idx] + "\x00COLON\x00" + result[idx+1:]
			continue
		}

		// Extract the parameter name (alphanumeric and underscore)
		endIdx := idx + 1
		for endIdx < len(result) {
			c := result[endIdx]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
				endIdx++
			} else {
				break
			}
		}

		paramName := result[idx+1 : endIdx]
		paramValue, exists := data[paramName]
		if !exists {
			return "", nil, fmt.Errorf("undefined parameter %q in SQL statement", paramName)
		}
		args = append(args, paramValue)
		result = result[:idx] + "?" + result[endIdx:]
	}

	// Restore markers
	result = strings.ReplaceAll(result, "\x00DOUBLECOLON\x00", "::")
	result = strings.ReplaceAll(result, "\x00COLON\x00", ":")

	return result, args, nil
}

// validateHTTPURL delegates to the shared security package.
func validateHTTPURL(rawURL string) error {
	return security.ValidateHTTPURL(rawURL)
}

// executeHTTPAction executes an HTTP request.
func (s *GenericState) executeHTTPAction(action *config.Action, data map[string]interface{}) error {
	// Expand template expressions in URL and body
	urlStr, err := expandTemplate(action.URL, data)
	if err != nil {
		return fmt.Errorf("failed to expand URL template: %w", err)
	}

	// Validate URL for SSRF protection
	if err := validateHTTPURL(urlStr); err != nil {
		return fmt.Errorf("URL validation failed: %w", err)
	}

	var body string
	if action.Body != "" {
		body, err = expandTemplate(action.Body, data)
		if err != nil {
			return fmt.Errorf("failed to expand body template: %w", err)
		}
	}

	// Limit request body size to 1MB to prevent memory exhaustion
	const maxBodySize = 1 << 20 // 1MB
	if len(body) > maxBodySize {
		return fmt.Errorf("request body too large: %d bytes (max %d)", len(body), maxBodySize)
	}

	// Default method is POST
	method := action.Method
	if method == "" {
		method = "POST"
	}

	// Create HTTP request
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, urlStr, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set Content-Type for JSON body if it looks like JSON
	if body != "" && req.Header.Get("Content-Type") == "" {
		trimmedBody := strings.TrimSpace(body)
		if len(trimmedBody) > 0 {
			firstChar := trimmedBody[0]
			if firstChar == '{' || firstChar == '[' {
				req.Header.Set("Content-Type", "application/json")
			}
		}
	}

	// Execute request with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		s.Error = err.Error()
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)
		if readErr != nil {
			errMsg += fmt.Sprintf(" (failed to read response body: %v)", readErr)
		} else if len(bodyBytes) > 0 {
			errMsg += ": " + string(bodyBytes[:min(len(bodyBytes), 200)])
		}
		s.Error = errMsg
		return fmt.Errorf("%s", errMsg)
	}

	// Success - refresh data if this block has a source
	return s.refresh()
}

// sanitizeExecCommand validates a command string for shell safety.
// It rejects characters commonly used for command chaining/injection.
func sanitizeExecCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("exec command is empty after templating")
	}

	// Reject null bytes (can be used to truncate strings in some contexts)
	if strings.ContainsRune(cmd, '\x00') {
		return fmt.Errorf("exec command contains null byte")
	}

	// Reject characters used for shell metacharacters and command chaining
	if strings.ContainsAny(cmd, "&;|$><`\\\n\r") {
		return fmt.Errorf("exec command contains disallowed shell characters")
	}

	return nil
}

// executeExecAction executes a shell command.
func (s *GenericState) executeExecAction(action *config.Action, data map[string]interface{}) error {
	// Check if exec is allowed
	if !config.IsExecAllowed() {
		return fmt.Errorf("exec actions disabled (use --allow-exec flag)")
	}

	// Expand template expressions in command
	cmdStr, err := expandTemplate(action.Cmd, data)
	if err != nil {
		return fmt.Errorf("failed to expand command template: %w", err)
	}

	// Validate command for shell safety
	if err := sanitizeExecCommand(cmdStr); err != nil {
		return err
	}

	// Create command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = s.siteDir

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err = cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("command failed: %v", err)
		if stderr.Len() > 0 {
			errMsg += ": " + stderr.String()
		}
		s.Error = errMsg
		return fmt.Errorf("%s", errMsg)
	}

	// Success - refresh data
	return s.refresh()
}

// expandTemplate expands Go template expressions in a string.
func expandTemplate(text string, data map[string]interface{}) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}

	tmpl, err := template.New("action").Parse(text)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
