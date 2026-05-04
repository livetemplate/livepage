package source

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// ExecSource runs a command and parses output as data
type ExecSource struct {
	name      string
	cmd       string
	siteDir   string
	format    string            // "json" (default), "lines", "csv"
	delimiter string            // for csv format, default ","
	env       map[string]string // environment variables (already expanded)
	timeout   time.Duration     // command timeout (default 30s)
}

// NewExecSource creates a new exec source (legacy constructor for backwards compatibility)
func NewExecSource(name, cmd, siteDir string) (*ExecSource, error) {
	if cmd == "" {
		return nil, fmt.Errorf("exec source %q: cmd is required", name)
	}
	return &ExecSource{
		name:      name,
		cmd:       cmd,
		siteDir:   siteDir,
		format:    "json",
		delimiter: ",",
		timeout:   30 * time.Second,
	}, nil
}

// NewExecSourceWithConfig creates a new exec source from config
func NewExecSourceWithConfig(name string, cfg config.SourceConfig, siteDir string) (*ExecSource, error) {
	if cfg.Cmd == "" {
		return nil, fmt.Errorf("exec source %q: cmd is required", name)
	}

	// Default format to json
	format := cfg.Format
	if format == "" {
		format = "json"
	}

	// Default delimiter to comma
	delimiter := cfg.Delimiter
	if delimiter == "" {
		delimiter = ","
	}

	// Default timeout to 30s, but allow config override
	timeout := 30 * time.Second
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil {
			timeout = d
		}
	}

	// Expand environment variables in env map
	env := make(map[string]string)
	for k, v := range cfg.Env {
		env[k] = os.ExpandEnv(v)
	}

	return &ExecSource{
		name:      name,
		cmd:       cfg.Cmd,
		siteDir:   siteDir,
		format:    format,
		delimiter: delimiter,
		env:       env,
		timeout:   timeout,
	}, nil
}

// Name returns the source identifier
func (s *ExecSource) Name() string {
	return s.name
}

// resolveCmdPath turns a relative './path' or '../path' into an absolute
// path under siteDir. Without this, exec.Command's behavior for './'
// commands varies across Go runtimes and platforms (see issue #244):
// the path is sometimes resolved relative to the process CWD before
// cmd.Dir is applied, breaking the user's mental model that
// cmd: "./script.sh" means "the script.sh next to my tinkerdown.yaml".
// Bare names like 'curl' and absolute paths pass through unchanged so
// PATH lookup still works as expected.
func resolveCmdPath(cmdName, siteDir string) string {
	if siteDir == "" {
		return cmdName
	}
	if strings.HasPrefix(cmdName, "./") || strings.HasPrefix(cmdName, "../") {
		return filepath.Join(siteDir, cmdName)
	}
	return cmdName
}

// Fetch executes the command and parses output according to format
func (s *ExecSource) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
	// Parse command - split by spaces (simple parsing)
	parts := strings.Fields(s.cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("exec source %q: empty command", s.name)
	}

	cmdName := resolveCmdPath(parts[0], s.siteDir)
	args := parts[1:]

	// Create command with context and timeout
	timeout := s.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, cmdName, args...)
	cmd.Dir = s.siteDir

	// Set environment: inherit current + add custom
	cmd.Env = os.Environ()
	for k, v := range s.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Execute and capture output
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("exec source %q: command failed: %s\nstderr: %s",
				s.name, err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("exec source %q: %w", s.name, err)
	}

	// Parse output according to format
	return s.parseOutput(output)
}

// parseOutput dispatches to the appropriate parser based on format
func (s *ExecSource) parseOutput(output []byte) ([]map[string]interface{}, error) {
	switch s.format {
	case "lines":
		return s.parseLines(output)
	case "csv":
		return s.parseCSV(output)
	default: // "json" or empty
		return s.parseJSON(output)
	}
}

// parseLines parses output as plain text lines
func (s *ExecSource) parseLines(data []byte) ([]map[string]interface{}, error) {
	content := strings.TrimSpace(string(data))
	if content == "" {
		return []map[string]interface{}{}, nil
	}

	lines := strings.Split(content, "\n")
	results := make([]map[string]interface{}, 0, len(lines))

	index := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		results = append(results, map[string]interface{}{
			"line":  line,
			"index": index,
		})
		index++
	}

	return results, nil
}

// parseCSV parses output as CSV with first row as headers
func (s *ExecSource) parseCSV(data []byte) ([]map[string]interface{}, error) {
	content := strings.TrimSpace(string(data))
	if content == "" {
		return []map[string]interface{}{}, nil
	}

	reader := csv.NewReader(bytes.NewReader([]byte(content)))

	// Set delimiter (must be a single rune)
	if len(s.delimiter) > 0 {
		reader.Comma = rune(s.delimiter[0])
	}

	// Read header row
	headers, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return []map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("exec source %q: failed to read CSV headers: %w", s.name, err)
	}

	// Read data rows
	var results []map[string]interface{}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("exec source %q: failed to read CSV row: %w", s.name, err)
		}

		row := make(map[string]interface{})
		for i, header := range headers {
			if i < len(record) {
				row[header] = record[i]
			}
		}
		results = append(results, row)
	}

	return results, nil
}

// parseJSON handles both array and object JSON responses
func (s *ExecSource) parseJSON(data []byte) ([]map[string]interface{}, error) {
	// Trim whitespace
	data = []byte(strings.TrimSpace(string(data)))

	if len(data) == 0 {
		return []map[string]interface{}{}, nil
	}

	// Try parsing as array first
	var arr []map[string]interface{}
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}

	// Try parsing as single object
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		return []map[string]interface{}{obj}, nil
	}

	// Try parsing as newline-delimited JSON (NDJSON)
	lines := strings.Split(string(data), "\n")
	var results []map[string]interface{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, fmt.Errorf("exec source %q: invalid JSON output: %w", s.name, err)
		}
		results = append(results, obj)
	}

	if len(results) > 0 {
		return results, nil
	}

	return nil, fmt.Errorf("exec source %q: could not parse output as JSON", s.name)
}

// Close is a no-op for exec sources
func (s *ExecSource) Close() error {
	return nil
}

// FetchWithArgs executes the command with custom argument values
// The args map contains argument name -> value pairs that override the defaults
func (s *ExecSource) FetchWithArgs(ctx context.Context, args map[string]string) ([]map[string]interface{}, error) {
	// Parse original command to get executable
	parts := strings.Fields(s.cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("exec source %q: empty command", s.name)
	}

	cmdName := resolveCmdPath(parts[0], s.siteDir)

	// Build new arguments from the provided map
	var newArgs []string
	for name, value := range args {
		// Handle boolean args specially - convert "on" to "true"
		if value == "on" {
			value = "true"
		}
		newArgs = append(newArgs, "--"+name, value)
	}

	// Create command with context and timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, cmdName, newArgs...)
	cmd.Dir = s.siteDir

	// Execute and capture output
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("exec source %q: command failed: %s\nstderr: %s",
				s.name, err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("exec source %q: %w", s.name, err)
	}

	// Parse JSON output
	return s.parseJSON(output)
}

// resolvePath makes a path absolute relative to siteDir
func (s *ExecSource) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.siteDir, path)
}
