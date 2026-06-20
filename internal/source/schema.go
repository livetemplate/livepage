package source

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// QuerySQLiteSchema opens a SQLite database and returns column info for the
// given table. This is a standalone function for use at parse time, before
// a full SQLiteSource is created.
//
// Returns nil (no error) if the table doesn't exist or the DB can't be opened.
// This allows graceful degradation — auto-tables will use text inputs as fallback.
func QuerySQLiteSchema(dbPath, table, siteDir string) []ColumnInfo {
	if dbPath == "" || table == "" {
		return nil
	}

	if !isValidIdentifier(table) {
		return nil
	}

	// Resolve relative path
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(siteDir, dbPath)
	}

	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil
	}
	defer db.Close()

	// Check table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
	if err != nil {
		return nil
	}

	// Query PRAGMA table_info
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var cid int
		var colName, typeName string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &colName, &typeName, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		// Skip internal columns
		if colName == "id" || colName == "created_at" {
			continue
		}
		columns = append(columns, ColumnInfo{
			Name:     colName,
			Type:     normalizeSQLiteType(typeName),
			Required: notNull == 1,
		})
	}

	return columns
}

// InputTypeForColumn returns the HTML input type for a given column type.
func InputTypeForColumn(colType string) string {
	switch strings.ToLower(colType) {
	case "integer", "real":
		return "number"
	case "boolean":
		return "checkbox"
	case "date":
		return "date"
	case "datetime":
		return "datetime-local"
	default:
		return "text"
	}
}
