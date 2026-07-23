// Package runtime provides in-process state handling for lvt-source blocks,
// replacing the previous plugin-based compilation approach.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode"

	"github.com/livetemplate/lvt/components/datatable"

	"github.com/livetemplate/tinkerdown/internal/cache"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/source"
	"github.com/livetemplate/tinkerdown/internal/wasm"
)

// Store is the interface for state objects that can handle actions.
// All state types (GenericState for lvt-source) implement this interface.
type Store interface {
	HandleAction(action string, data map[string]interface{}) error
	// Close releases resources. Optional - returns nil if not implemented.
	Close() error
}

// GenericState holds runtime state for any source type.
// It replaces the code-generated State structs that were previously compiled as plugins.
type GenericState struct {
	// Common fields (JSON-serializable for templates)
	Data   []map[string]interface{} `json:"data"`
	Error  string                   `json:"error,omitempty"`
	Errors map[string]string        `json:"errors,omitempty"`

	// Datatable field - used when source is rendered in a table element
	Table *datatable.DataTable `json:"table,omitempty"`

	// Cache metadata for UI display
	CacheInfo *cache.CacheInfo `json:"cache_info,omitempty"`

	// Inline edit state - tracks which row is being edited (empty = none)
	EditingID string `json:"editingId,omitempty"`

	// Exec-specific fields
	Output     string `json:"output,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Duration   int64  `json:"duration,omitempty"`
	Status     string `json:"status,omitempty"`
	Command    string `json:"command,omitempty"`
	Args       []Arg  `json:"args,omitempty"`
	Executable string `json:"executable,omitempty"`

	// Private runtime fields (not serialized)
	source       source.Source
	sourceCfg    config.SourceConfig
	sourceType   string
	sourceName   string
	siteDir      string
	elementType  string   // "table", "select", or "div"
	tableColumns []string // columns for datatable rendering
	activeFilter string   // current filter expression (empty = show all)
	mu           sync.RWMutex

	// Page-level configuration for custom actions.
	// These fields are configured via SetPageConfig during initialization only
	// and must be treated as read-only for the lifetime of the GenericState
	// once any action execution has begun. SetPageConfig must not be called
	// concurrently with action handling.
	actions  map[string]*config.Action          // Custom actions declared in frontmatter
	registry func(string) (source.Source, bool) // Lookup function for sources (for SQL actions)
}

// Arg represents an exec source argument
type Arg struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Value       string `json:"value,omitempty"`
}

// NewGenericState creates a new state for the given source configuration.
// This replaces the plugin compilation step - state is created directly in-process.
func NewGenericState(name string, cfg config.SourceConfig, siteDir, currentFile string) (*GenericState, error) {
	return NewGenericStateWithMetadata(name, cfg, siteDir, currentFile, nil)
}

// NewGenericStateForComputed creates a state for a computed source.
// sourceLookup is used to resolve the parent source by name. If nil, the parent
// is created from config using createSource.
func NewGenericStateForComputed(name string, cfg config.SourceConfig, siteDir, currentFile string, metadata map[string]string, sourceLookup func(string) (source.Source, bool)) (*GenericState, error) {
	if cfg.From == "" {
		return nil, fmt.Errorf("computed source %q: 'from' field is required", name)
	}

	// Try sourceLookup first (from WebSocket handler's source registry)
	var parentSrc source.Source
	if sourceLookup != nil {
		var ok bool
		parentSrc, ok = sourceLookup(cfg.From)
		if !ok {
			return nil, fmt.Errorf("computed source %q: parent source %q not found", name, cfg.From)
		}
		// Verify parent is not itself a computed source (chaining not supported)
		if _, isComputed := parentSrc.(*source.ComputedSource); isComputed {
			return nil, fmt.Errorf("computed source %q: chaining computed sources is not supported (parent %q is also computed)", name, cfg.From)
		}
	}

	// Build a minimal registry with the parent source for NewComputedSource
	reg := source.NewEmptyRegistry()
	if parentSrc != nil {
		reg.Set(cfg.From, parentSrc)
	}

	computedSrc, err := source.NewComputedSource(name, cfg, reg)
	if err != nil {
		return nil, err
	}

	s := &GenericState{
		source:     computedSrc,
		sourceCfg:  cfg,
		sourceType: cfg.Type,
		sourceName: name,
		siteDir:    siteDir,
		Errors:     make(map[string]string),
	}

	if metadata != nil {
		s.elementType = metadata["lvt-element"]
	}

	// Initial fetch
	if err := s.refresh(); err != nil {
		s.Error = err.Error()
	}

	return s, nil
}

// NewGenericStateWithMetadata creates a new state with block metadata for datatable support.
// Metadata should include "lvt-element" ("table", "select", or "div") and "lvt-columns" for tables.
func NewGenericStateWithMetadata(name string, cfg config.SourceConfig, siteDir, currentFile string, metadata map[string]string) (*GenericState, error) {
	// For computed sources, use the specialized constructor
	if cfg.Type == "computed" {
		return NewGenericStateForComputed(name, cfg, siteDir, currentFile, metadata, nil)
	}

	// Create the underlying source using the existing factory
	src, err := createSource(name, cfg, siteDir, currentFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create source %q: %w", name, err)
	}

	s := &GenericState{
		source:     src,
		sourceCfg:  cfg,
		sourceType: cfg.Type,
		sourceName: name,
		siteDir:    siteDir,
		Errors:     make(map[string]string),
	}

	// Parse metadata for element type and columns
	if metadata != nil {
		s.elementType = metadata["lvt-element"]
		if columns := metadata["lvt-columns"]; columns != "" {
			// Parse "name:Name,email:Email" format
			for _, pair := range strings.Split(columns, ",") {
				parts := strings.SplitN(pair, ":", 2)
				if len(parts) > 0 {
					s.tableColumns = append(s.tableColumns, parts[0])
				}
			}
		}
	}

	// Set exec-specific fields if applicable
	if cfg.Type == "exec" {
		s.Command = cfg.Cmd
		s.Status = "ready"
		// Parse command-line arguments for form rendering
		s.Args = parseExecArgs(cfg.Cmd)
		// If manual mode, don't auto-fetch
		if cfg.Manual {
			return s, nil
		}
	}

	// Initial data fetch
	if err := s.refresh(); err != nil {
		s.Error = err.Error()
	}

	return s, nil
}

// SetPageConfig configures page-level settings for custom actions.
// actions is the map of custom actions declared in frontmatter.
// registry is a lookup function to find sources by name (for SQL actions).
func (s *GenericState) SetPageConfig(actions map[string]*config.Action, registry func(string) (source.Source, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = actions
	s.registry = registry
}

// createSource creates a source from config (mirrors source.createSource)
func createSource(name string, cfg config.SourceConfig, siteDir, currentFile string) (source.Source, error) {
	switch cfg.Type {
	case "exec":
		if !config.IsExecAllowed() {
			return nil, &source.ValidationError{
				Source: name,
				Field:  "type",
				Reason: "exec sources are disabled by default for security. Use --allow-exec flag to enable.",
			}
		}
		return source.NewExecSourceWithConfig(name, cfg, siteDir)
	case "pg":
		return source.NewPostgresSource(name, cfg.Query, cfg.Options)
	case "rest":
		return source.NewRestSourceWithConfig(name, cfg)
	case "json":
		return source.NewJSONFileSource(name, cfg.File, siteDir)
	case "csv":
		return source.NewCSVFileSource(name, cfg.File, siteDir, cfg.Options)
	case "markdown":
		return source.NewMarkdownSource(name, cfg.File, cfg.Anchor, siteDir, currentFile, cfg.IsReadonly())
	case "sqlite":
		return source.NewSQLiteSource(name, cfg.DB, cfg.Table, siteDir, cfg.IsReadonly())
	case "wasm":
		return wasm.NewWasmSource(name, cfg.Path, siteDir, cfg.Options)
	case "graphql":
		return source.NewGraphQLSource(name, cfg, siteDir)
	case "computed":
		// Computed sources are handled specially — they need a parent source.
		// The parent is created by the caller and passed via sourceLookup.
		// See NewGenericStateForComputed.
		return nil, fmt.Errorf("computed sources must be created via NewGenericStateForComputed")
	default:
		return nil, fmt.Errorf("unsupported source type: %s", cfg.Type)
	}
}

// HandleAction dispatches an action to the appropriate handler.
// This replaces the reflection-based dispatch used in generated plugins.
// builtinActions are action names the runtime handles intrinsically for any
// source, independent of the manifest — source affordances (refresh a view,
// add/delete a row) rather than custom, governed actions. The policy lint must
// not treat them as unapproved: they are governed by whether the source itself
// is approved and writable. Kept in sync with the HandleAction switch below.
var builtinActions = map[string]bool{
	"refresh": true, "run": true, "filter": true, "edit": true, "canceledit": true,
	"add": true, "toggle": true, "delete": true, "update": true,
}

// builtinActionPrefixes are the datatable action families (Sort_X, NextPage_X,
// PrevPage_X). Used only by HandleAction's dispatch to route framework-generated
// datatable actions — deliberately NOT part of IsBuiltinAction (see below).
var builtinActionPrefixes = []string{"sort", "nextpage", "prevpage"}

// IsBuiltinAction reports whether name is one of the runtime's built-in source
// affordances (add/delete/toggle/refresh/…) that every writable source handles,
// as opposed to a custom, governed action. Exact-match, case-insensitive.
//
// It feeds the policy lint (refs.go), so the datatable dispatch prefixes
// (sort/nextpage/prevpage) are intentionally excluded: those actions are
// framework-generated and never appear in authored markup, and a prefix match
// would classify a custom governed action like "sort-by-priority" as built-in
// and silently drop it from the approved-action check — a governance hole.
func IsBuiltinAction(name string) bool {
	return builtinActions[strings.ToLower(name)]
}

func (s *GenericState) HandleAction(action string, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear previous errors
	s.Errors = make(map[string]string)
	s.Error = ""

	// Normalize action to lowercase for matching
	actionLower := strings.ToLower(action)

	switch actionLower {
	case "refresh":
		return s.refresh()
	case "run":
		return s.runExec(data)
	case "filter":
		return s.handleFilter(data)
	case "edit":
		return s.handleEdit(data)
	case "canceledit":
		s.EditingID = ""
		return nil
	case "add", "toggle", "delete", "update":
		err := s.handleWriteAction(action, data)
		if err == nil {
			s.EditingID = ""
		}
		return err
	default:
		// A custom (governed) action wins over the datatable-prefix fallback, so an
		// approved action whose name starts with sort/nextpage/prevpage actually
		// runs — keeping "the policy lint approved it" and "it executes" in sync.
		// Framework-generated datatable actions (Sort_<col>, NextPage, PrevPage) are
		// never manifest actions, so they still fall through to handleDatatableAction.
		if customAction, ok := s.actions[action]; ok {
			return s.executeCustomAction(customAction, data)
		}

		// Datatable actions (Sort_X, NextPage_X, PrevPage_X).
		for _, p := range builtinActionPrefixes {
			if strings.HasPrefix(actionLower, p) {
				return s.handleDatatableAction(action, data)
			}
		}

		return fmt.Errorf("unknown action %q", action)
	}
}

// GetState returns the current state for template rendering.
// This replaces the RPC GetState() call.
func (s *GenericState) GetState() (json.RawMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s)
}

// GetStateAsInterface returns the state as an interface{} for template rendering.
// This is used by the WebSocket handler to render templates.
func (s *GenericState) GetStateAsInterface() (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a map representation for template compatibility
	stateBytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	var rawMap map[string]interface{}
	if err := json.Unmarshal(stateBytes, &rawMap); err != nil {
		return nil, err
	}

	// Apply active filter to data if present
	if s.activeFilter != "" {
		filteredData := s.GetFilteredData()
		rawMap["data"] = filteredData
	}

	// Process state map to add titlecase keys for template access
	// This allows templates to use both {{.data}} and {{.Data}}
	return processStateMap(rawMap), nil
}

// processStateMap processes a map to add titlecase keys alongside lowercase keys.
// This allows templates to use both {{.data}} and {{.Data}}, {{.status}} and {{.Status}}.
// It also processes nested maps and slices recursively.
func processStateMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		var processedValue interface{}

		switch val := v.(type) {
		case map[string]interface{}:
			processedValue = processMapValues(val)
		case []interface{}:
			newSlice := make([]interface{}, len(val))
			for i, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					newSlice[i] = processMapValues(itemMap)
				} else {
					newSlice[i] = convertNumber(item)
				}
			}
			processedValue = newSlice
		case []map[string]interface{}:
			// Handle slice of maps directly (e.g., from GetFilteredData)
			newSlice := make([]interface{}, len(val))
			for i, item := range val {
				newSlice[i] = processMapValues(item)
			}
			processedValue = newSlice
		case float64:
			processedValue = convertNumber(val)
		default:
			processedValue = v
		}

		// Keep original key (e.g., "status", "data", "items")
		result[k] = processedValue

		// Also add titlecased key if different (e.g., "Status", "Data", "Items")
		// This allows templates to use either {{.status}} or {{.Status}}
		if len(k) > 0 {
			titleKey := strings.ToUpper(k[:1]) + k[1:]
			if titleKey != k {
				result[titleKey] = processedValue
			}
		}
	}
	return result
}

// processMapValues recursively processes map values to add titlecase keys.
func processMapValues(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		var processedValue interface{}

		switch val := v.(type) {
		case map[string]interface{}:
			processedValue = processMapValues(val)
		case []interface{}:
			newSlice := make([]interface{}, len(val))
			for i, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					newSlice[i] = processMapValues(itemMap)
				} else {
					newSlice[i] = convertNumber(item)
				}
			}
			processedValue = newSlice
		case float64:
			processedValue = convertNumber(val)
		default:
			processedValue = v
		}

		// Keep original key
		result[k] = processedValue

		// Also add PascalCase key if different (converts snake_case to PascalCase)
		if len(k) > 0 {
			pascalKey := snakeToPascal(k)
			if pascalKey != k {
				result[pascalKey] = processedValue
			}
		}
	}
	return result
}

// snakeToPascal converts snake_case to PascalCase.
// Examples: "assigned_to" -> "AssignedTo", "id" -> "Id", "my_field_name" -> "MyFieldName"
func snakeToPascal(s string) string {
	if s == "" {
		return s
	}

	var result strings.Builder
	capitalizeNext := true

	for _, c := range s {
		if c == '_' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			result.WriteRune(unicode.ToUpper(c))
			capitalizeNext = false
		} else {
			result.WriteRune(c)
		}
	}

	return result.String()
}

// convertNumber converts float64 values that are whole numbers to int.
// JSON unmarshals all numbers as float64, but int is often more useful.
func convertNumber(v interface{}) interface{} {
	if f, ok := v.(float64); ok {
		if f == float64(int(f)) {
			return int(f)
		}
	}
	return v
}

// Close releases any resources held by the source.
func (s *GenericState) Close() error {
	if s.source != nil {
		return s.source.Close()
	}
	return nil
}

// refresh fetches data from the source
func (s *GenericState) refresh() error {
	if s.source == nil {
		// No source to refresh - this is valid for actions without a source binding
		return nil
	}

	ctx := context.Background()
	data, err := s.source.Fetch(ctx)
	if err != nil {
		s.Error = err.Error()
		return err
	}

	s.Data = data
	s.Error = ""

	// Populate CacheInfo if source supports it
	if provider, ok := s.source.(source.CacheInfoProvider); ok {
		s.CacheInfo = provider.GetCacheInfo()
	}

	// Build DataTable if this is a table element
	if s.elementType == "table" {
		s.Table = s.buildDataTable()
	}

	return nil
}

// handleEdit sets the editing ID to the row being edited.
// The template checks EditingID to show inputs vs display text.
func (s *GenericState) handleEdit(data map[string]interface{}) error {
	id, ok := data["id"]
	if !ok || id == nil {
		return fmt.Errorf("Edit action requires an 'id' parameter")
	}
	s.EditingID = fmt.Sprintf("%v", id)
	return nil
}

// handleFilter applies a filter expression to the data.
// Filter expressions use the same syntax as computed expressions:
//   - "done" - filter rows where done is truthy
//   - "not done" - filter rows where done is falsy
//   - "status = active" - filter rows where status equals "active"
//   - "assigned_to = operator" - filter by operator identity (variable substitution)
//   - "" (empty) - show all data (no filter)
func (s *GenericState) handleFilter(data map[string]interface{}) error {
	// Extract filter expression from data
	filter := ""
	if f, ok := data["filter"].(string); ok {
		filter = f
	}

	// Substitute "operator" variable with actual operator value
	// This enables filters like "assigned_to = operator" to work with the --operator flag
	if strings.Contains(filter, "operator") {
		operator := s.getOperator()
		filter = strings.ReplaceAll(filter, "operator", operator)
	}

	// Store the active filter
	s.activeFilter = filter

	// Re-render with filtered data
	// Note: The actual filtering happens in GetFilteredData which is called during render
	return nil
}

// GetFilteredData returns the data with the active filter applied.
// This is used by the template renderer to show filtered results.
// Note: This method must be called while holding at least a read lock,
// or from within a method that already holds the lock.
func (s *GenericState) GetFilteredData() []map[string]interface{} {
	if s.activeFilter == "" {
		return s.Data
	}

	// Parse the filter expression
	where, err := parseFilterExpression(s.activeFilter)
	if err != nil {
		// Log parse error for debugging, return all data as fallback
		fmt.Fprintf(os.Stderr, "Warning: failed to parse filter expression %q: %v\n", s.activeFilter, err)
		return s.Data
	}

	// Apply filter
	return filterDataByWhere(s.Data, where)
}

// parseFilterExpression parses a simple filter expression.
// Supports:
//   - "field" -> field is truthy
//   - "not field" -> field is falsy
//   - "field = value" -> field equals value
//   - "field != value" -> field not equals value
func parseFilterExpression(input string) (*WhereClause, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	// Check for "not field" pattern (negated truthy check)
	if strings.HasPrefix(strings.ToLower(input), "not ") {
		field := strings.TrimSpace(input[4:])
		return &WhereClause{
			Field:    field,
			Operator: "=",
			Value:    false,
		}, nil
	}

	// Use the existing parseWhereClause from expr.go
	return parseWhereClause(input)
}

// filterDataByWhere filters data rows based on a where clause.
// Uses getFieldValue from expr.go for consistent field lookup behavior.
func filterDataByWhere(data []map[string]interface{}, where *WhereClause) []map[string]interface{} {
	if where == nil {
		return data
	}

	var result []map[string]interface{}
	for _, row := range data {
		val := getFieldValue(row, where.Field)
		if matchesCondition(val, where.Operator, where.Value) {
			result = append(result, row)
		}
	}
	return result
}

// buildDataTable creates a datatable.DataTable from the current Data
func (s *GenericState) buildDataTable() *datatable.DataTable {
	if len(s.Data) == 0 {
		return nil
	}

	// Determine columns - use explicit columns or auto-discover from first row
	var columns []datatable.Column
	if len(s.tableColumns) > 0 {
		for _, col := range s.tableColumns {
			label := col
			if len(label) > 0 {
				label = strings.ToUpper(label[:1]) + label[1:]
			}
			columns = append(columns, datatable.Column{
				ID:       col,
				Label:    label,
				Sortable: true,
			})
		}
	} else {
		// Auto-discover columns from first row
		for key := range s.Data[0] {
			// Skip internal fields starting with uppercase (title-cased duplicates)
			if len(key) > 0 && key[0] >= 'A' && key[0] <= 'Z' {
				continue
			}
			label := key
			if len(label) > 0 {
				label = strings.ToUpper(label[:1]) + label[1:]
			}
			columns = append(columns, datatable.Column{
				ID:       key,
				Label:    label,
				Sortable: true,
			})
		}
	}

	// Build rows
	var rows []datatable.Row
	for i, item := range s.Data {
		data := make(map[string]any)
		for _, col := range columns {
			if val, ok := item[col.ID]; ok {
				data[col.ID] = val
			} else {
				// Try titlecase key
				titleKey := col.ID
				if len(titleKey) > 0 {
					titleKey = strings.ToUpper(titleKey[:1]) + titleKey[1:]
				}
				if val, ok := item[titleKey]; ok {
					data[col.ID] = val
				}
			}
		}
		// Generate row ID
		rowID := fmt.Sprintf("row-%d", i)
		if id, ok := item["id"]; ok {
			rowID = fmt.Sprintf("%v", id)
		} else if id, ok := item["Id"]; ok {
			rowID = fmt.Sprintf("%v", id)
		}
		rows = append(rows, datatable.Row{ID: rowID, Data: data})
	}

	return datatable.New(s.sourceName, datatable.WithColumns(columns), datatable.WithRows(rows))
}

// parseExecArgs parses command-line arguments from a command string.
// It extracts --flag value pairs and infers types from values.
// Example: "./script.sh --name World --count 3 --verbose true"
// Returns Args with Name, Label, Type, and Value set.
func parseExecArgs(cmd string) []Arg {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}

	var args []Arg

	// Skip the executable (first part)
	for i := 1; i < len(parts); i++ {
		part := parts[i]

		// Check for --flag or -flag
		if !strings.HasPrefix(part, "-") {
			continue
		}

		// Extract flag name (remove leading dashes)
		name := strings.TrimLeft(part, "-")
		if name == "" {
			continue
		}

		// Get the value (next part if available and doesn't start with -)
		value := ""
		if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
			i++
			value = parts[i]
		}

		// Infer type from value
		argType := "string"
		if value == "true" || value == "false" {
			argType = "bool"
		} else if isNumeric(value) {
			argType = "number"
		}

		// Create label from name (capitalize first letter)
		label := name
		if len(label) > 0 {
			label = strings.ToUpper(label[:1]) + label[1:]
		}

		args = append(args, Arg{
			Name:    name,
			Label:   label,
			Type:    argType,
			Value:   value,
			Default: value,
		})
	}

	return args
}

// isNumeric checks if a string represents a number
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	// Allow optional leading minus sign
	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	hasDecimal := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			if hasDecimal {
				return false
			}
			hasDecimal = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
