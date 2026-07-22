package source

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// SQLiteSource provides read/write access to SQLite tables.
// It implements WritableSource for Add, Update, Delete operations.
type SQLiteSource struct {
	name     string
	db       *sql.DB
	table    string
	dbPath   string
	readonly bool
	siteDir  string

	// Schema tracking
	columns      []string
	mu           sync.RWMutex
	hasSchema    bool
	hasCreatedAt bool // true if table has a created_at column (for ORDER BY)
}

// NewSQLiteSource creates a new SQLite source
func NewSQLiteSource(name, dbPath, table, siteDir string, readonly bool) (*SQLiteSource, error) {
	if table == "" {
		return nil, fmt.Errorf("sqlite source %q: table name is required", name)
	}

	// Validate table name (prevent SQL injection)
	if !isValidIdentifier(table) {
		return nil, fmt.Errorf("sqlite source %q: invalid table name %q", name, table)
	}

	// Default database path
	if dbPath == "" {
		dbPath = "./tinkerdown.db"
	}

	// Resolve relative path
	if !strings.HasPrefix(dbPath, "/") {
		dbPath = siteDir + "/" + dbPath
	}

	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("sqlite source %q: failed to open database: %w", name, err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite source %q: failed to connect: %w", name, err)
	}

	s := &SQLiteSource{
		name:     name,
		db:       db,
		table:    table,
		dbPath:   dbPath,
		readonly: readonly,
		siteDir:  siteDir,
	}

	// Try to discover existing schema
	s.discoverSchema()

	return s, nil
}

// Name returns the source identifier
func (s *SQLiteSource) Name() string {
	return s.name
}

// Fetch retrieves all records from the table
func (s *SQLiteSource) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.hasSchema {
		// Table doesn't exist yet, return empty
		return []map[string]interface{}{}, nil
	}

	query := fmt.Sprintf("SELECT * FROM %s", s.table)
	if s.hasCreatedAt {
		query += " ORDER BY created_at DESC"
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite source %q: fetch failed: %w", s.name, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

// Close releases the database connection
func (s *SQLiteSource) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// IsReadonly returns whether the source is read-only
func (s *SQLiteSource) IsReadonly() bool {
	return s.readonly
}

// Exec executes a SQL statement with the given arguments.
// This implements the SQLExecutor interface for custom action support.
// Returns the number of rows affected.
func (s *SQLiteSource) Exec(ctx context.Context, query string, args ...interface{}) (int64, error) {
	if s.readonly {
		return 0, fmt.Errorf("sqlite source %q is read-only", s.name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite source %q: exec failed: %w", s.name, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite source %q: failed to get rows affected: %w", s.name, err)
	}

	return rowsAffected, nil
}

// ExecTx runs the given statements in a single transaction. Either every
// statement commits, or the first error rolls all of them back — so an action
// that must change state and append an audit record does both or neither. The
// source's write mutex is held for the whole transaction, matching Exec's
// single-writer discipline against SQLite's one-writer lock.
func (s *SQLiteSource) ExecTx(ctx context.Context, stmts []SQLStatement) error {
	if s.readonly {
		return fmt.Errorf("sqlite source %q is read-only", s.name)
	}
	if len(stmts) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite source %q: begin transaction: %w", s.name, err)
	}

	for i, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt.Query, stmt.Args...); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("sqlite source %q: statement %d failed (%w); rollback also failed: %v", s.name, i+1, err, rbErr)
			}
			return fmt.Errorf("sqlite source %q: statement %d failed, transaction rolled back: %w", s.name, i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite source %q: commit transaction: %w", s.name, err)
	}
	return nil
}

// WriteItem performs write operations (add, update, delete)
func (s *SQLiteSource) WriteItem(ctx context.Context, action string, data map[string]interface{}) error {
	if s.readonly {
		return fmt.Errorf("sqlite source %q is read-only", s.name)
	}

	switch action {
	case "add":
		return s.addItem(ctx, data)
	case "update":
		return s.updateItem(ctx, data)
	case "delete":
		return s.deleteItem(ctx, data)
	case "toggle":
		return s.toggleItem(ctx, data)
	default:
		return fmt.Errorf("sqlite source %q: unknown action %q", s.name, action)
	}
}

// addItem inserts a new record
func (s *SQLiteSource) addItem(ctx context.Context, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure table exists with schema from data
	if err := s.ensureTable(data); err != nil {
		return err
	}

	// Filter out special fields
	fields := filterDataFields(data)
	if len(fields) == 0 {
		return fmt.Errorf("no fields to insert")
	}

	// Build INSERT statement
	columns := make([]string, 0, len(fields))
	placeholders := make([]string, 0, len(fields))
	values := make([]interface{}, 0, len(fields))

	for col, val := range fields {
		if !isValidIdentifier(col) {
			continue
		}
		columns = append(columns, col)
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		s.table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	_, err := s.db.ExecContext(ctx, query, values...)
	return err
}

// updateItem updates an existing record by ID
func (s *SQLiteSource) updateItem(ctx context.Context, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := getID(data)
	if !ok {
		return fmt.Errorf("update requires 'id' field")
	}

	fields := filterDataFields(data)
	delete(fields, "id")

	if len(fields) == 0 {
		return fmt.Errorf("no fields to update")
	}

	// Build UPDATE statement
	setClauses := make([]string, 0, len(fields))
	values := make([]interface{}, 0, len(fields)+1)

	for col, val := range fields {
		if !isValidIdentifier(col) {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", col))
		values = append(values, val)
	}
	values = append(values, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?",
		s.table,
		strings.Join(setClauses, ", "))

	_, err := s.db.ExecContext(ctx, query, values...)
	return err
}

// deleteItem removes a record by ID
func (s *SQLiteSource) deleteItem(ctx context.Context, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := getID(data)
	if !ok {
		return fmt.Errorf("delete requires 'id' field")
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", s.table)
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// toggleItem toggles a boolean column (defaults to "done") for a record by ID
func (s *SQLiteSource) toggleItem(ctx context.Context, data map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := getID(data)
	if !ok {
		return fmt.Errorf("toggle requires 'id' field")
	}

	// Get column to toggle (default: "done")
	column := "done"
	if col, ok := data["column"].(string); ok && col != "" {
		column = col
	}

	if !isValidIdentifier(column) {
		return fmt.Errorf("invalid column name: %s", column)
	}

	// Use SQL to toggle the value: 0 -> 1, non-zero -> 0
	query := fmt.Sprintf("UPDATE %s SET %s = CASE WHEN %s = 0 OR %s IS NULL THEN 1 ELSE 0 END WHERE id = ?",
		s.table, column, column, column)

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("toggle failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no record found with id: %v", id)
	}

	return nil
}

// ensureTable creates the table if it doesn't exist, based on data fields
func (s *SQLiteSource) ensureTable(data map[string]interface{}) error {
	if s.hasSchema {
		return nil
	}

	// Build CREATE TABLE from data fields
	var columnDefs []string
	columnDefs = append(columnDefs, "id INTEGER PRIMARY KEY AUTOINCREMENT")

	for col, val := range data {
		if col == "id" || !isValidIdentifier(col) {
			continue
		}
		sqlType := inferSQLType(val)
		columnDefs = append(columnDefs, fmt.Sprintf("%s %s", col, sqlType))
		s.columns = append(s.columns, col)
	}

	columnDefs = append(columnDefs, "created_at DATETIME DEFAULT CURRENT_TIMESTAMP")

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)",
		s.table,
		strings.Join(columnDefs, ", "))

	if _, err := s.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	s.hasSchema = true
	s.hasCreatedAt = true
	return nil
}

// discoverSchema checks if the table exists and reads its columns
func (s *SQLiteSource) discoverSchema() {
	query := fmt.Sprintf("SELECT name FROM sqlite_master WHERE type='table' AND name=?")
	var name string
	err := s.db.QueryRow(query, s.table).Scan(&name)
	if err != nil {
		// Table doesn't exist
		return
	}

	// Get column names
	colQuery := fmt.Sprintf("PRAGMA table_info(%s)", s.table)
	rows, err := s.db.Query(colQuery)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typeName string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == "created_at" {
			s.hasCreatedAt = true
			continue
		}
		if name != "id" {
			s.columns = append(s.columns, name)
		}
	}

	if err := rows.Err(); err != nil {
		return
	}

	s.hasSchema = true
}

// Schema returns column information for the table, implementing SchemaProvider.
// Excludes internal columns (id, created_at).
func (s *SQLiteSource) Schema(ctx context.Context) ([]ColumnInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.hasSchema {
		return nil, nil
	}

	colQuery := fmt.Sprintf("PRAGMA table_info(%s)", s.table)
	rows, err := s.db.QueryContext(ctx, colQuery)
	if err != nil {
		return nil, fmt.Errorf("sqlite source %q: schema query failed: %w", s.name, err)
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var cid int
		var name, typeName string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		// Skip internal columns
		if name == "id" || name == "created_at" {
			continue
		}
		columns = append(columns, ColumnInfo{
			Name:     name,
			Type:     normalizeSQLiteType(typeName),
			Required: notNull == 1,
		})
	}

	return columns, nil
}

// normalizeSQLiteType converts SQLite type affinity to a normalized type string.
func normalizeSQLiteType(sqlType string) string {
	sqlType = strings.ToUpper(strings.TrimSpace(sqlType))
	switch {
	case strings.Contains(sqlType, "INT"):
		return "integer"
	case strings.Contains(sqlType, "REAL") || strings.Contains(sqlType, "FLOA") || strings.Contains(sqlType, "DOUB"):
		return "real"
	case strings.Contains(sqlType, "BOOL"):
		return "boolean"
	case strings.Contains(sqlType, "DATE") && !strings.Contains(sqlType, "DATETIME"):
		return "date"
	case strings.Contains(sqlType, "DATETIME") || strings.Contains(sqlType, "TIMESTAMP"):
		return "datetime"
	default:
		return "text"
	}
}

// Helper functions

// sqliteBusyTimeoutMs is how long a connection waits for a lock held by another
// writer before returning SQLITE_BUSY. SQLite's default is 0 (fail immediately),
// which makes any concurrent access — e.g. a reader querying while an action's
// write is committing — flake with "database is locked". A non-zero timeout lets
// the brief write window drain and the query succeed.
const sqliteBusyTimeoutMs = 5000

// sqliteDSN builds the modernc.org/sqlite connection string for a database file
// path, attaching a busy_timeout pragma so concurrent access waits for locks
// instead of failing immediately. Applies to every SQLite connection the library
// opens, regardless of source or example.
func sqliteDSN(dbPath string) string {
	return fmt.Sprintf("%s?_pragma=busy_timeout(%d)", dbPath, sqliteBusyTimeoutMs)
}

func isValidIdentifier(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i, c := range name {
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

func inferSQLType(val interface{}) string {
	switch val.(type) {
	case int, int32, int64, float64, float32:
		return "INTEGER"
	case bool:
		return "INTEGER" // SQLite uses INTEGER for boolean
	case time.Time:
		return "DATETIME"
	default:
		return "TEXT"
	}
}

func filterDataFields(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range data {
		// Skip internal/special fields
		if strings.HasPrefix(k, "_") {
			continue
		}
		result[k] = v
	}
	return result
}

func getID(data map[string]interface{}) (interface{}, bool) {
	if id, ok := data["id"]; ok {
		return id, true
	}
	if id, ok := data["Id"]; ok {
		return id, true
	}
	if id, ok := data["ID"]; ok {
		return id, true
	}
	return nil, false
}
