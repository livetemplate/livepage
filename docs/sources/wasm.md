# WASM Source

Create custom data sources using WebAssembly.

## Configuration

```yaml
sources:
  custom:
    type: wasm
    path: ./custom.wasm
```

## Options

| Option | Required | Description |
|--------|----------|-------------|
| `type` | Yes | Must be `wasm` |
| `path` | Yes | Path to the `.wasm` module file |
| `options` | No | Init config passed to the module (a map of string values) |

## Read-only vs. writable

Writability is derived from the module itself, not the manifest: a module that
exports a `write` function (see *Required Interface* below) is **writable** — it
supports `add`, `update`, and `delete` — while a module that exports only `fetch` is
**read-only** (list only). A write against a read-only module is refused with a clear
error, never silently dropped.

A writable WASM source has full parity across both entry points — the live server and
the CLI:

```bash
# list (read) works for any WASM source
tinkerdown cli app.md list custom

# add/update/delete work only if the module exports `write`
tinkerdown cli app.md add custom --title="New item"
```

## Examples

### Basic Usage

```yaml
sources:
  github_issues:
    type: wasm
    path: ./sources/github.wasm
    options:
      repo: livetemplate/tinkerdown
```

### With Credentials

```yaml
sources:
  external_data:
    type: wasm
    path: ./sources/external-api.wasm
    options:
      api_key: ${EXTERNAL_API_KEY}
      resource_id: ${EXTERNAL_RESOURCE_ID}
```

## Writing WASM Sources

WASM sources are written in Go and compiled with TinyGo.

### Required Interface

```go
package main

import "encoding/json"

// Config is passed from tinkerdown.yaml
type Config struct {
    Repo string `json:"repo"`
}

// Fetch is called to get data
//
//export fetch
func fetch(configPtr *byte, configLen int) (dataPtr *byte, dataLen int) {
    // Parse config
    config := parseConfig(configPtr, configLen)

    // Fetch your data
    data := fetchData(config)

    // Return JSON array
    return toJSON(data)
}

// Write is called for mutations
//
//export write
func write(action string, dataPtr *byte, dataLen int) error {
    // Handle write operations
    return nil
}

func main() {}
```

### Compilation

```bash
tinygo build -o mysource.wasm -target=wasi source.go
```

## WASM SDK

Use the Tinkerdown WASM SDK to simplify development:

```bash
# Initialize a new WASM source
tinkerdown wasm init mysource

# Build the WASM module
tinkerdown wasm build

# Test the module
tinkerdown wasm test
```

## Source Structure

```
mysource/
├── source.go          # Main source implementation
├── go.mod             # Go module
├── Makefile           # Build commands
├── README.md          # Documentation
└── test/
    └── source_test.go # Tests
```

## Example: GitHub Issues

```go
package main

import (
    "encoding/json"
    "io"
    "net/http"
)

type Config struct {
    Repo  string `json:"repo"`
    Token string `json:"token"`
}

type Issue struct {
    Number int    `json:"number"`
    Title  string `json:"title"`
    State  string `json:"state"`
}

//export fetch
func fetch(configPtr *byte, configLen int) (dataPtr *byte, dataLen int) {
    config := parseConfig(configPtr, configLen)

    url := "https://api.github.com/repos/" + config.Repo + "/issues"
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("Authorization", "Bearer "+config.Token)

    resp, _ := http.DefaultClient.Do(req)
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)

    var issues []Issue
    json.Unmarshal(body, &issues)

    return toJSON(issues)
}
```

## Resource Limits

WASM modules run with resource limits:

```yaml
sources:
  custom:
    type: wasm
    path: ./custom.wasm
    limits:
      memory: 64MB      # Default: 64MB
      timeout: 30s      # Default: 30s
```

## Security

WASM sources run in a sandboxed environment:

- No filesystem access (except allowed paths)
- No network access (except through provided HTTP client)
- Memory and CPU limits enforced

## Community Sources

Browse community-contributed sources:

- GitHub API source
- REST API sources
- Database sources

## Full Example

```yaml
# tinkerdown.yaml
sources:
  github_issues:
    type: wasm
    path: ./sources/github.wasm
    options:
      repo: livetemplate/tinkerdown
      token: ${GITHUB_TOKEN}
    cache:
      ttl: 5m
```

```html
<!-- index.md -->
<h2>Open Issues</h2>
<table lvt-source="github_issues" lvt-columns="number,title,state">
</table>
```

## Next Steps

- [Data Sources Guide](../guides/data-sources.md) - Overview of all sources
