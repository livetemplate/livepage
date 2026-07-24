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
	// Set the operator for :operator substitution (e.g. "WHERE approver = :operator").
	// This ALWAYS overwrites any incoming value: data is the client's action
	// payload, and validateParams does not strip undeclared keys, so a client
	// that sends "operator" in the payload could otherwise attribute an action —
	// an approval written to an audit trail — to an identity it does not own.
	// operator is a reserved, server-set param; a client cannot spoof it.
	if data == nil {
		data = make(map[string]interface{})
	}
	data["operator"] = config.GetOperator()

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
	var (
		b    strings.Builder
		args []interface{}
		last int
		err  error
	)
	scanParams(stmt, func(name string, start, end int) {
		if err != nil {
			return
		}
		value, exists := data[name]
		if !exists {
			err = fmt.Errorf("undefined parameter %q in SQL statement", name)
			return
		}
		b.WriteString(stmt[last:start]) // unchanged text (incl. ::casts, time literals)
		b.WriteByte('?')
		args = append(args, value)
		last = end
	})
	if err != nil {
		return "", nil, err
	}
	b.WriteString(stmt[last:])
	return b.String(), args, nil
}

// scanParams walks stmt and calls fn for each :name placeholder, with the name
// and its [start,end) byte range. It skips :: (postgres casts) and colons not
// followed by a letter (time literals like '12:30:00', a trailing colon). This
// is the single definition of "what is a parameter", shared by substitution
// (SubstituteParams, at runtime) and completeness checking (ReferencedParams,
// used by `tinkerdown validate`), so the two cannot disagree about which names a
// statement requires.
func scanParams(stmt string, fn func(name string, start, end int)) {
	for i := 0; i < len(stmt); {
		if stmt[i] != ':' {
			i++
			continue
		}
		if i+1 < len(stmt) && stmt[i+1] == ':' { // :: postgres cast
			i += 2
			continue
		}
		if i+1 >= len(stmt) || !isParamNameStart(stmt[i+1]) { // bare colon / time literal
			i++
			continue
		}
		end := i + 1
		for end < len(stmt) && isParamNameChar(stmt[end]) {
			end++
		}
		fn(stmt[i+1:end], i, end)
		i = end
	}
}

func isParamNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isParamNameChar(c byte) bool {
	return isParamNameStart(c) || (c >= '0' && c <= '9') || c == '_'
}

// ReferencedParams returns the distinct :name parameters referenced across the
// given statements, in first-seen order — the names a caller must supply for
// SubstituteParams to succeed.
func ReferencedParams(statements ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, stmt := range statements {
		scanParams(stmt, func(name string, _, _ int) {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		})
	}
	return out
}
