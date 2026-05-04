package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExecSource(t *testing.T) {
	tests := []struct {
		name    string
		srcName string
		cmd     string
		wantErr bool
	}{
		{
			name:    "valid command",
			srcName: "test",
			cmd:     "echo hello",
			wantErr: false,
		},
		{
			name:    "empty command",
			srcName: "test",
			cmd:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := NewExecSource(tt.srcName, tt.cmd, ".")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.srcName, src.Name())
		})
	}
}

func TestExecSourceFetchJSON(t *testing.T) {
	// Create a temp directory with a script that outputs JSON
	tmpDir := t.TempDir()

	// Create a script that outputs JSON array
	scriptPath := filepath.Join(tmpDir, "data.sh")
	scriptContent := `#!/usr/bin/env bash
echo '[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]'
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	src, err := NewExecSource("test", "./data.sh", tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 2)

	assert.Equal(t, float64(1), data[0]["id"])
	assert.Equal(t, "Alice", data[0]["name"])
	assert.Equal(t, float64(2), data[1]["id"])
	assert.Equal(t, "Bob", data[1]["name"])
}

func TestExecSourceFetchSingleObject(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that outputs a single JSON object
	scriptPath := filepath.Join(tmpDir, "single.sh")
	scriptContent := `#!/usr/bin/env bash
echo '{"status":"ok","count":42}'
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	src, err := NewExecSource("test", "./single.sh", tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 1)

	assert.Equal(t, "ok", data[0]["status"])
	assert.Equal(t, float64(42), data[0]["count"])
}

func TestExecSourceFetchNDJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that outputs newline-delimited JSON
	scriptPath := filepath.Join(tmpDir, "ndjson.sh")
	scriptContent := `#!/usr/bin/env bash
echo '{"line":1}'
echo '{"line":2}'
echo '{"line":3}'
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	src, err := NewExecSource("test", "./ndjson.sh", tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 3)

	assert.Equal(t, float64(1), data[0]["line"])
	assert.Equal(t, float64(2), data[1]["line"])
	assert.Equal(t, float64(3), data[2]["line"])
}

func TestExecSourceFetchEmptyOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script with empty output
	scriptPath := filepath.Join(tmpDir, "empty.sh")
	scriptContent := `#!/usr/bin/env bash
echo ""
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	src, err := NewExecSource("test", "./empty.sh", tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestExecSourceFetchInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script with invalid JSON
	scriptPath := filepath.Join(tmpDir, "invalid.sh")
	scriptContent := `#!/usr/bin/env bash
echo 'not valid json'
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	src, err := NewExecSource("test", "./invalid.sh", tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = src.Fetch(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON output")
}

func TestExecSourceFetchCommandFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that exits with error
	scriptPath := filepath.Join(tmpDir, "fail.sh")
	scriptContent := `#!/usr/bin/env bash
exit 1
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	src, err := NewExecSource("test", "./fail.sh", tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = src.Fetch(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command failed")
}

func TestExecSourceClose(t *testing.T) {
	src, err := NewExecSource("test", "echo hello", ".")
	require.NoError(t, err)

	err = src.Close()
	assert.NoError(t, err)
}

// TestExecSourceSecurityFlagDefault verifies the --allow-exec flag is disabled by default
func TestExecSourceSecurityFlagDefault(t *testing.T) {
	// Reset to default state
	config.SetAllowExec(false)
	defer config.SetAllowExec(false)

	// Verify the security flag is disabled by default
	// The actual enforcement happens in the source factory (source.go createSource)
	// which checks config.IsExecAllowed() before creating exec sources.
	// This test verifies the flag defaults to false.
	assert.False(t, config.IsExecAllowed(), "exec should be disabled by default")

	// The constructor (NewExecSourceWithConfig) doesn't check the flag -
	// security is enforced at the factory level to allow testing without the flag.
	cfg := config.SourceConfig{
		Type: "exec",
		Cmd:  "echo hello",
	}
	src, err := NewExecSourceWithConfig("test", cfg, ".")
	require.NoError(t, err, "constructor should work regardless of security flag")
	assert.NotNil(t, src)
}

// TestExecSourceWithAllowExec verifies exec sources work when --allow-exec is set
func TestExecSourceWithAllowExec(t *testing.T) {
	config.SetAllowExec(true)
	defer config.SetAllowExec(false)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\necho '{\"ok\":true}'"), 0755)
	require.NoError(t, err)

	cfg := config.SourceConfig{
		Type: "exec",
		Cmd:  "./test.sh",
	}

	src, err := NewExecSourceWithConfig("test", cfg, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 1)
	assert.Equal(t, true, data[0]["ok"])
}

// TestExecSourceLinesFormat tests parsing output as plain text lines
func TestExecSourceLinesFormat(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "lines.sh")
	scriptContent := `#!/usr/bin/env bash
echo "first line"
echo "second line"
echo "third line"
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	cfg := config.SourceConfig{
		Type:   "exec",
		Cmd:    "./lines.sh",
		Format: "lines",
	}

	src, err := NewExecSourceWithConfig("test", cfg, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 3)

	assert.Equal(t, "first line", data[0]["line"])
	assert.Equal(t, 0, data[0]["index"])
	assert.Equal(t, "second line", data[1]["line"])
	assert.Equal(t, 1, data[1]["index"])
	assert.Equal(t, "third line", data[2]["line"])
	assert.Equal(t, 2, data[2]["index"])
}

// TestExecSourceCSVFormat tests parsing output as CSV with headers
func TestExecSourceCSVFormat(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "csv.sh")
	scriptContent := `#!/usr/bin/env bash
echo "name,age,city"
echo "Alice,30,NYC"
echo "Bob,25,LA"
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	cfg := config.SourceConfig{
		Type:   "exec",
		Cmd:    "./csv.sh",
		Format: "csv",
	}

	src, err := NewExecSourceWithConfig("test", cfg, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 2)

	assert.Equal(t, "Alice", data[0]["name"])
	assert.Equal(t, "30", data[0]["age"])
	assert.Equal(t, "NYC", data[0]["city"])
	assert.Equal(t, "Bob", data[1]["name"])
	assert.Equal(t, "25", data[1]["age"])
	assert.Equal(t, "LA", data[1]["city"])
}

// TestExecSourceCustomDelimiter tests CSV parsing with tab delimiter
func TestExecSourceCustomDelimiter(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "tsv.sh")
	// Tab-separated values
	scriptContent := "#!/usr/bin/env bash\necho -e \"id\\tname\\tvalue\"\necho -e \"1\\tAlpha\\t100\"\necho -e \"2\\tBeta\\t200\"\n"
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	cfg := config.SourceConfig{
		Type:      "exec",
		Cmd:       "./tsv.sh",
		Format:    "csv",
		Delimiter: "\t",
	}

	src, err := NewExecSourceWithConfig("test", cfg, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 2)

	assert.Equal(t, "1", data[0]["id"])
	assert.Equal(t, "Alpha", data[0]["name"])
	assert.Equal(t, "100", data[0]["value"])
}

// TestExecSourceEnvVars tests that environment variables are passed to the command
func TestExecSourceEnvVars(t *testing.T) {
	// Set an env var that will be expanded
	os.Setenv("TEST_EXEC_SECRET", "secret-value-123")
	defer os.Unsetenv("TEST_EXEC_SECRET")

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "env.sh")
	// Script echoes the env vars as JSON
	scriptContent := `#!/usr/bin/env bash
echo "{\"my_var\":\"$MY_VAR\",\"expanded\":\"$EXPANDED_VAR\"}"
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	cfg := config.SourceConfig{
		Type: "exec",
		Cmd:  "./env.sh",
		Env: map[string]string{
			"MY_VAR":       "hello-world",
			"EXPANDED_VAR": "${TEST_EXEC_SECRET}",
		},
	}

	src, err := NewExecSourceWithConfig("test", cfg, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 1)

	assert.Equal(t, "hello-world", data[0]["my_var"])
	assert.Equal(t, "secret-value-123", data[0]["expanded"])
}

// TestExecSourceTimeout tests that custom timeout is respected
func TestExecSourceTimeout(t *testing.T) {
	cfg := config.SourceConfig{
		Type:    "exec",
		Cmd:     "echo test",
		Timeout: "5s",
	}

	src, err := NewExecSourceWithConfig("test", cfg, ".")
	require.NoError(t, err)

	// Verify the timeout was set
	assert.Equal(t, 5*time.Second, src.timeout)
}

// TestExecSourceEmptyLines tests that empty lines are skipped in lines format
func TestExecSourceEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "empty_lines.sh")
	scriptContent := `#!/usr/bin/env bash
echo "line one"
echo ""
echo "line two"
echo ""
echo ""
echo "line three"
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	cfg := config.SourceConfig{
		Type:   "exec",
		Cmd:    "./empty_lines.sh",
		Format: "lines",
	}

	src, err := NewExecSourceWithConfig("test", cfg, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	data, err := src.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, data, 3) // Only non-empty lines

	// Verify lines and sequential indices (not original positions)
	assert.Equal(t, "line one", data[0]["line"])
	assert.Equal(t, 0, data[0]["index"])
	assert.Equal(t, "line two", data[1]["line"])
	assert.Equal(t, 1, data[1]["index"])
	assert.Equal(t, "line three", data[2]["line"])
	assert.Equal(t, 2, data[2]["index"])
}

// Issue #244: exec.Command's behavior for './' and '../' commands
// varied across Go runtimes / platforms — the path was sometimes
// resolved against the process CWD before cmd.Dir applied. Lock the
// "resolve relative to siteDir" semantics here.
func TestResolveCmdPath(t *testing.T) {
	cases := []struct {
		name    string
		cmdName string
		siteDir string
		want    string
	}{
		{"dot-slash joins siteDir", "./script.sh", "/site", "/site/script.sh"},
		{"dotdot-slash joins siteDir", "../parent.sh", "/site/sub", "/site/parent.sh"},
		{"absolute path passes through", "/usr/bin/curl", "/site", "/usr/bin/curl"},
		{"bare name passes through (PATH lookup)", "curl", "/site", "curl"},
		{"empty siteDir is no-op", "./script.sh", "", "./script.sh"},
		{"sub-relative bare path passes through", "scripts/foo.sh", "/site", "scripts/foo.sh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveCmdPath(tc.cmdName, tc.siteDir)
			assert.Equal(t, tc.want, got)
		})
	}
}
