package runtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/security"
	"github.com/livetemplate/tinkerdown/internal/source"
)

func TestSubstituteParams(t *testing.T) {
	tests := []struct {
		name      string
		stmt      string
		data      map[string]interface{}
		wantQuery string
		wantArgs  []interface{}
		wantErr   bool
	}{
		{
			name:      "single param",
			stmt:      "DELETE FROM tasks WHERE id = :id",
			data:      map[string]interface{}{"id": 123},
			wantQuery: "DELETE FROM tasks WHERE id = ?",
			wantArgs:  []interface{}{123},
		},
		{
			name:      "multiple params",
			stmt:      "UPDATE tasks SET done = :done WHERE id = :id",
			data:      map[string]interface{}{"id": 456, "done": true},
			wantQuery: "UPDATE tasks SET done = ? WHERE id = ?",
			wantArgs:  []interface{}{true, 456},
		},
		{
			name:      "no params",
			stmt:      "DELETE FROM tasks WHERE done = 1",
			data:      map[string]interface{}{},
			wantQuery: "DELETE FROM tasks WHERE done = 1",
			wantArgs:  []interface{}{},
		},
		{
			name:      "param with underscore",
			stmt:      "SELECT * FROM users WHERE created_at < :cutoff_date",
			data:      map[string]interface{}{"cutoff_date": "2024-01-01"},
			wantQuery: "SELECT * FROM users WHERE created_at < ?",
			wantArgs:  []interface{}{"2024-01-01"},
		},
		{
			name:    "missing param returns error",
			stmt:    "DELETE FROM tasks WHERE id = :id",
			data:    map[string]interface{}{},
			wantErr: true,
		},
		{
			name:      "explicit nil value is allowed",
			stmt:      "UPDATE tasks SET notes = :notes WHERE id = :id",
			data:      map[string]interface{}{"id": 1, "notes": nil},
			wantQuery: "UPDATE tasks SET notes = ? WHERE id = ?",
			wantArgs:  []interface{}{nil, 1},
		},
		{
			name:      "colon in time literal is preserved",
			stmt:      "SELECT * FROM events WHERE start_time > '12:30:00' AND id = :id",
			data:      map[string]interface{}{"id": 42},
			wantQuery: "SELECT * FROM events WHERE start_time > '12:30:00' AND id = ?",
			wantArgs:  []interface{}{42},
		},
		{
			name:      "postgres style cast preserved",
			stmt:      "SELECT value::text FROM data WHERE id = :id",
			data:      map[string]interface{}{"id": 1},
			wantQuery: "SELECT value::text FROM data WHERE id = ?",
			wantArgs:  []interface{}{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotArgs, err := source.SubstituteParams(tt.stmt, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("substituteParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("substituteParams() query = %q, want %q", gotQuery, tt.wantQuery)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Errorf("substituteParams() args len = %d, want %d", len(gotArgs), len(tt.wantArgs))
				return
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Errorf("substituteParams() args[%d] = %v, want %v", i, gotArgs[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name    string
		action  *config.Action
		data    map[string]interface{}
		wantErr bool
	}{
		{
			name: "all required params present",
			action: &config.Action{
				Params: map[string]config.ParamDef{
					"id":   {Required: true},
					"name": {Required: true},
				},
			},
			data:    map[string]interface{}{"id": 1, "name": "test"},
			wantErr: false,
		},
		{
			name: "missing required param",
			action: &config.Action{
				Params: map[string]config.ParamDef{
					"id": {Required: true},
				},
			},
			data:    map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "empty required param",
			action: &config.Action{
				Params: map[string]config.ParamDef{
					"name": {Required: true},
				},
			},
			data:    map[string]interface{}{"name": ""},
			wantErr: true,
		},
		{
			name: "optional param missing",
			action: &config.Action{
				Params: map[string]config.ParamDef{
					"optional": {Required: false},
				},
			},
			data:    map[string]interface{}{},
			wantErr: false,
		},
		{
			name: "no params defined",
			action: &config.Action{
				Params: nil,
			},
			data:    map[string]interface{}{"anything": "value"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateParams(tt.action, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandTemplate(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		data    map[string]interface{}
		want    string
		wantErr bool
	}{
		{
			name: "no template",
			text: "https://api.example.com/users",
			data: map[string]interface{}{},
			want: "https://api.example.com/users",
		},
		{
			name: "simple substitution",
			text: "https://api.example.com/users/{{.id}}",
			data: map[string]interface{}{"id": 123},
			want: "https://api.example.com/users/123",
		},
		{
			name: "json body",
			text: `{"text": "Task: {{.task}}"}`,
			data: map[string]interface{}{"task": "Buy groceries"},
			want: `{"text": "Task: Buy groceries"}`,
		},
		{
			name: "invalid template",
			text: "{{.broken",
			data: map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandTemplate(tt.text, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("expandTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsExecAllowed(t *testing.T) {
	// Save original state and restore after test
	origState := config.IsExecAllowed()
	defer config.SetAllowExec(origState)

	// Test default (should be disabled)
	config.SetAllowExec(false)
	if config.IsExecAllowed() {
		t.Error("IsExecAllowed() should be false by default")
	}

	// Test enabled
	config.SetAllowExec(true)
	if !config.IsExecAllowed() {
		t.Error("IsExecAllowed() should be true after SetAllowExec(true)")
	}

	// Test disabled again
	config.SetAllowExec(false)
	if config.IsExecAllowed() {
		t.Error("IsExecAllowed() should be false after SetAllowExec(false)")
	}
}

func TestExecuteCustomAction_UnknownKind(t *testing.T) {
	state := &GenericState{}
	action := &config.Action{Kind: "unknown"}

	err := state.executeCustomAction(action, nil)
	if err == nil {
		t.Error("executeCustomAction() should error on unknown action kind")
	}
	if err.Error() != "unknown action kind: unknown" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExecuteExecAction_Disabled(t *testing.T) {
	// Save original state and restore after test
	origState := config.IsExecAllowed()
	defer config.SetAllowExec(origState)

	config.SetAllowExec(false)

	state := &GenericState{}
	action := &config.Action{Kind: "exec", Cmd: "echo hello"}

	err := state.executeExecAction(action, nil)
	if err == nil {
		t.Error("executeExecAction() should error when exec is disabled")
	}
	if err.Error() != "exec actions disabled (use --allow-exec flag)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSanitizeExecCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantErr string
	}{
		{
			name:    "valid command",
			cmd:     "echo hello",
			wantErr: "",
		},
		{
			name:    "empty command",
			cmd:     "   ",
			wantErr: "exec command is empty after templating",
		},
		{
			name:    "null byte",
			cmd:     "echo hello\x00world",
			wantErr: "exec command contains null byte",
		},
		{
			name:    "semicolon",
			cmd:     "echo hello; rm -rf /",
			wantErr: "exec command contains disallowed shell characters",
		},
		{
			name:    "pipe",
			cmd:     "cat file | grep secret",
			wantErr: "exec command contains disallowed shell characters",
		},
		{
			name:    "ampersand",
			cmd:     "command1 && command2",
			wantErr: "exec command contains disallowed shell characters",
		},
		{
			name:    "backtick",
			cmd:     "echo `whoami`",
			wantErr: "exec command contains disallowed shell characters",
		},
		{
			name:    "dollar sign",
			cmd:     "echo $HOME",
			wantErr: "exec command contains disallowed shell characters",
		},
		{
			name:    "redirect",
			cmd:     "echo secret > /etc/passwd",
			wantErr: "exec command contains disallowed shell characters",
		},
		{
			name:    "newline",
			cmd:     "echo hello\nrm -rf /",
			wantErr: "exec command contains disallowed shell characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sanitizeExecCommand(tt.cmd)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("sanitizeExecCommand() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("sanitizeExecCommand() expected error containing %q", tt.wantErr)
				} else if err.Error() != tt.wantErr {
					t.Errorf("sanitizeExecCommand() error = %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		// Valid URLs
		{
			name:    "valid https url",
			url:     "https://api.example.com/endpoint",
			wantErr: "",
		},
		{
			name:    "valid http url",
			url:     "http://api.example.com/endpoint",
			wantErr: "",
		},
		// Invalid schemes
		{
			name:    "file scheme",
			url:     "file:///etc/passwd",
			wantErr: "URL scheme must be http or https",
		},
		{
			name:    "ftp scheme",
			url:     "ftp://files.example.com/file.txt",
			wantErr: "URL scheme must be http or https",
		},
		// Localhost
		{
			name:    "localhost",
			url:     "http://localhost/admin",
			wantErr: "requests to localhost are not allowed",
		},
		{
			name:    "localhost with port",
			url:     "http://localhost:8080/admin",
			wantErr: "requests to localhost are not allowed",
		},
		// Loopback IPs
		{
			name:    "127.0.0.1",
			url:     "http://127.0.0.1/admin",
			wantErr: "requests to loopback addresses are not allowed",
		},
		{
			name:    "127.0.0.1 with port",
			url:     "http://127.0.0.1:8080/admin",
			wantErr: "requests to loopback addresses are not allowed",
		},
		{
			name:    "ipv6 loopback",
			url:     "http://[::1]/admin",
			wantErr: "requests to loopback addresses are not allowed",
		},
		// Private networks
		{
			name:    "10.x.x.x",
			url:     "http://10.0.0.1/admin",
			wantErr: "requests to private network addresses are not allowed",
		},
		{
			name:    "172.16.x.x",
			url:     "http://172.16.0.1/admin",
			wantErr: "requests to private network addresses are not allowed",
		},
		{
			name:    "192.168.x.x",
			url:     "http://192.168.1.1/admin",
			wantErr: "requests to private network addresses are not allowed",
		},
		// Link-local
		{
			name:    "link-local metadata endpoint",
			url:     "http://169.254.169.254/latest/meta-data",
			wantErr: "requests to link-local addresses are not allowed",
		},
		// Unspecified
		{
			name:    "0.0.0.0",
			url:     "http://0.0.0.0/admin",
			wantErr: "requests to unspecified addresses are not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPURL(tt.url)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateHTTPURL() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("validateHTTPURL() expected error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("validateHTTPURL() error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestExecuteSQLAction_NoRegistry(t *testing.T) {
	state := &GenericState{
		registry: nil, // No registry configured
	}
	action := &config.Action{Kind: "sql", Source: "db", Statement: "DELETE FROM tasks"}

	err := state.executeSQLAction(action, nil)
	if err == nil {
		t.Error("executeSQLAction() should error when registry is nil")
	}
	if err.Error() != "source registry not configured" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExecuteSQLAction_SourceNotFound(t *testing.T) {
	state := &GenericState{
		registry: func(name string) (source.Source, bool) {
			return nil, false
		},
	}

	action := &config.Action{Kind: "sql", Source: "missing", Statement: "DELETE FROM tasks"}

	err := state.executeSQLAction(action, nil)
	if err == nil {
		t.Error("executeSQLAction() should error when source not found")
	}
	if err.Error() != `source "missing" not found` {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestHandleAction_CustomActionWinsOverDatatablePrefix pins the dispatch
// precedence: a custom (governed) action whose name starts with a datatable
// prefix (sort/nextpage/prevpage) must be dispatched as the custom action, not
// routed to the datatable handler — otherwise an approved action the policy lint
// allowed could never actually run. With a nil registry the sql action fails
// distinctively ("source registry not configured"), proving it reached
// executeCustomAction rather than handleDatatableAction.
func TestHandleAction_CustomActionWinsOverDatatablePrefix(t *testing.T) {
	state := &GenericState{
		actions: map[string]*config.Action{
			"sort-by-priority": {Kind: "sql", Source: "db", Statement: "UPDATE t SET x = 1"},
		},
		registry: nil,
	}

	err := state.HandleAction("sort-by-priority", nil)
	if err == nil || err.Error() != "source registry not configured" {
		t.Errorf("custom action with a datatable-prefix name was not dispatched as custom: got %v", err)
	}
}

func TestExecuteHTTPAction(t *testing.T) {
	// Bypass SSRF validation for mock server tests
	security.SetTestBypassSSRF(true)
	t.Cleanup(func() { security.SetTestBypassSSRF(false) })

	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		action         *config.Action
		data           map[string]interface{}
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "successful POST request",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("expected POST, got %s", r.Method)
				}
				w.WriteHeader(http.StatusOK)
			},
			action:  &config.Action{Kind: "http", Method: "POST"},
			wantErr: false,
		},
		{
			name: "successful GET request",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("expected GET, got %s", r.Method)
				}
				w.WriteHeader(http.StatusOK)
			},
			action:  &config.Action{Kind: "http", Method: "GET"},
			wantErr: false,
		},
		{
			name: "template expansion in URL",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/users/42" {
					t.Errorf("expected /users/42, got %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusOK)
			},
			action:  &config.Action{Kind: "http", Method: "GET"},
			data:    map[string]interface{}{"id": 42},
			wantErr: false,
		},
		{
			name: "JSON body with template",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
				}
				w.WriteHeader(http.StatusOK)
			},
			action:  &config.Action{Kind: "http", Method: "POST", Body: `{"message": "{{.msg}}"}`},
			data:    map[string]interface{}{"msg": "hello"},
			wantErr: false,
		},
		{
			name: "server returns 500 error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("internal error"))
			},
			action:         &config.Action{Kind: "http", Method: "POST"},
			wantErr:        true,
			wantErrContain: "HTTP 500",
		},
		{
			name: "server returns 404 error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("not found"))
			},
			action:         &config.Action{Kind: "http", Method: "GET"},
			wantErr:        true,
			wantErrContain: "HTTP 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			// Set URL with template expansion if needed
			url := server.URL
			if tt.name == "template expansion in URL" {
				url = server.URL + "/users/{{.id}}"
			}
			tt.action.URL = url

			state := &GenericState{}
			err := state.executeHTTPAction(tt.action, tt.data)

			if tt.wantErr {
				if err == nil {
					t.Error("executeHTTPAction() expected error, got nil")
				} else if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("executeHTTPAction() error = %q, want to contain %q", err.Error(), tt.wantErrContain)
				}
			} else {
				if err != nil {
					t.Errorf("executeHTTPAction() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestExecuteHTTPAction_BodySizeLimit(t *testing.T) {
	// Bypass SSRF validation for this test
	security.SetTestBypassSSRF(true)
	t.Cleanup(func() { security.SetTestBypassSSRF(false) })

	state := &GenericState{}

	// Create a body that exceeds 1MB
	largeBody := strings.Repeat("x", 1<<20+1) // 1MB + 1 byte

	action := &config.Action{
		Kind:   "http",
		URL:    "http://example.com/api",
		Method: "POST",
		Body:   largeBody,
	}

	err := state.executeHTTPAction(action, nil)
	if err == nil {
		t.Error("executeHTTPAction() expected body size error, got nil")
	} else if !strings.Contains(err.Error(), "request body too large") {
		t.Errorf("executeHTTPAction() error = %q, want to contain 'request body too large'", err.Error())
	}
}

func TestExecuteHTTPAction_SSRFProtection(t *testing.T) {
	state := &GenericState{}

	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{
			name:    "localhost blocked",
			url:     "http://localhost:8080/admin",
			wantErr: "requests to localhost are not allowed",
		},
		{
			name:    "private IP blocked",
			url:     "http://192.168.1.1/admin",
			wantErr: "requests to private network addresses are not allowed",
		},
		{
			name:    "metadata endpoint blocked",
			url:     "http://169.254.169.254/latest/meta-data",
			wantErr: "requests to link-local addresses are not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &config.Action{Kind: "http", URL: tt.url, Method: "GET"}
			err := state.executeHTTPAction(action, nil)
			if err == nil {
				t.Error("executeHTTPAction() expected SSRF error, got nil")
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("executeHTTPAction() error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestExecuteExecAction_Enabled(t *testing.T) {
	// Save original state and restore after test
	origState := config.IsExecAllowed()
	defer config.SetAllowExec(origState)

	config.SetAllowExec(true)

	state := &GenericState{
		siteDir: "/tmp", // Use /tmp as working directory
	}
	action := &config.Action{Kind: "exec", Cmd: "echo hello"}

	err := state.executeExecAction(action, nil)
	if err != nil {
		t.Errorf("executeExecAction() with --allow-exec should succeed, got error: %v", err)
	}
}

func TestExecuteExecAction_WithTemplateExpansion(t *testing.T) {
	// Save original state and restore after test
	origState := config.IsExecAllowed()
	defer config.SetAllowExec(origState)

	config.SetAllowExec(true)

	state := &GenericState{
		siteDir: "/tmp",
	}
	action := &config.Action{Kind: "exec", Cmd: "echo {{.message}}"}
	data := map[string]interface{}{"message": "hello world"}

	err := state.executeExecAction(action, data)
	if err != nil {
		t.Errorf("executeExecAction() with template expansion should succeed, got error: %v", err)
	}
}

func TestExecuteExecAction_CommandSanitization(t *testing.T) {
	// Save original state and restore after test
	origState := config.IsExecAllowed()
	defer config.SetAllowExec(origState)

	config.SetAllowExec(true)

	state := &GenericState{
		siteDir: "/tmp",
	}

	tests := []struct {
		name    string
		cmd     string
		data    map[string]interface{}
		wantErr string
	}{
		{
			name:    "command injection via template blocked",
			cmd:     "echo {{.msg}}",
			data:    map[string]interface{}{"msg": "hello; rm -rf /"},
			wantErr: "exec command contains disallowed shell characters",
		},
		{
			name:    "null byte injection blocked",
			cmd:     "echo {{.msg}}",
			data:    map[string]interface{}{"msg": "hello\x00world"},
			wantErr: "exec command contains null byte",
		},
		{
			name:    "pipe injection blocked",
			cmd:     "echo {{.msg}}",
			data:    map[string]interface{}{"msg": "hello | cat /etc/passwd"},
			wantErr: "exec command contains disallowed shell characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &config.Action{Kind: "exec", Cmd: tt.cmd}
			err := state.executeExecAction(action, tt.data)
			if err == nil {
				t.Error("executeExecAction() expected sanitization error, got nil")
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("executeExecAction() error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestHandleEditAction(t *testing.T) {
	s := &GenericState{}

	// Edit sets EditingID
	err := s.HandleAction("Edit", map[string]interface{}{"id": "42"})
	if err != nil {
		t.Fatalf("HandleAction(Edit) failed: %v", err)
	}
	if s.EditingID != "42" {
		t.Errorf("expected EditingID '42', got %q", s.EditingID)
	}

	// CancelEdit clears EditingID
	err = s.HandleAction("CancelEdit", nil)
	if err != nil {
		t.Fatalf("HandleAction(CancelEdit) failed: %v", err)
	}
	if s.EditingID != "" {
		t.Errorf("expected EditingID empty after CancelEdit, got %q", s.EditingID)
	}
}

func TestHandleEditNumericID(t *testing.T) {
	s := &GenericState{}

	// Edit with numeric ID (common for SQLite auto-increment)
	err := s.HandleAction("Edit", map[string]interface{}{"id": 7})
	if err != nil {
		t.Fatalf("HandleAction(Edit) failed: %v", err)
	}
	if s.EditingID != "7" {
		t.Errorf("expected EditingID '7', got %q", s.EditingID)
	}
}
