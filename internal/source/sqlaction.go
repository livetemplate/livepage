package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// RunSQLAction executes a `kind: sql` action's statement(s) against executor.
//
// It is the single place both the UI action path (internal/runtime) and the
// webhook path (internal/server) run a sql action, so the two cannot drift — the
// asymmetry that previously let `:operator` be injected on one path but not the
// other is impossible when both go through here. It:
//
//   - injects the operator identity so `:operator` resolves the same everywhere;
//   - substitutes `:name` placeholders via SubstituteParams;
//   - runs a `statements:` batch atomically via ExecTx (a single `statement:` via
//     Exec). Config validation guarantees exactly one of the two is set.
//
// The caller owns everything around execution — source lookup, timeouts, and any
// post-mutation refresh — this is only the execute step.
func RunSQLAction(ctx context.Context, executor SQLExecutor, action *config.Action, data map[string]interface{}) error {
	// Inject the operator for :operator substitution (e.g. "WHERE approver = :operator").
	if data == nil {
		data = make(map[string]interface{})
	}
	if _, ok := data["operator"]; !ok {
		data["operator"] = config.GetOperator()
	}

	if len(action.Statements) > 0 {
		stmts := make([]SQLStatement, 0, len(action.Statements))
		for _, raw := range action.Statements {
			query, args, err := SubstituteParams(raw, data)
			if err != nil {
				return err
			}
			stmts = append(stmts, SQLStatement{Query: query, Args: args})
		}
		return executor.ExecTx(ctx, stmts)
	}

	query, args, err := SubstituteParams(action.Statement, data)
	if err != nil {
		return err
	}
	_, err = executor.Exec(ctx, query, args...)
	return err
}

// SubstituteParams converts :name placeholders to positional args.
// Input:  "DELETE FROM tasks WHERE id = :id", {"id": "123"}
// Output: "DELETE FROM tasks WHERE id = ?", ["123"]
// Returns an error if a parameter in the statement is not found in data.
//
// Parameter names must start with a letter (a-z, A-Z) and can contain
// letters, digits, and underscores. This avoids false matches on:
// - Time literals like '12:30:00' (digits after colon)
// - Postgres casts like value::text (double colon)
func SubstituteParams(stmt string, data map[string]interface{}) (string, []interface{}, error) {
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
