# Configuration Reference (tinkerdown.yaml)

Reference for `tinkerdown.yaml` - the **optional** configuration file for complex apps.

> **Recommendation:** For most apps, configure sources directly in [frontmatter](frontmatter.md). Use `tinkerdown.yaml` only for shared configuration across multiple pages or complex setups.

## When to Use tinkerdown.yaml

| Use Case | Recommendation |
|----------|----------------|
| Single-page app | Use frontmatter |
| Simple multi-page app | Use frontmatter per page |
| Shared sources across pages | Use `tinkerdown.yaml` |
| Complex caching strategies | Use `tinkerdown.yaml` |
| Server settings (port, host) | Use `tinkerdown.yaml` |
| Secrets via environment variables | Use `tinkerdown.yaml` |

## File Location

Place `tinkerdown.yaml` in your app's root directory:

```
myapp/
├── tinkerdown.yaml    # Optional
├── index.md
└── ...
```

## Full Schema

```yaml
# Server settings (can't be in frontmatter)
server:
  port: 8080
  host: localhost

# REST API (optional)
api:
  enabled: true
  auth:
    api_key: ${API_KEY}           # Legacy single key (coexists with keys:)
    header_name: X-API-Key        # Default; set to "Authorization" for Bearer tokens
    keys:
      - name: reader
        key: ${READ_KEY}
        permissions: [read]
  cors:
    origins: ["http://localhost:3000"]
  rate_limit:
    requests_per_second: 10
    burst: 20
    max_tracked_ips: 10000

# Global styling (can also be per-page in frontmatter)
styling:
  theme: clean  # clean, dark, minimal

# Shared data sources
sources:
  source_name:
    type: sqlite|rest|exec|json|csv|markdown|wasm
    # Type-specific options...
    cache:
      ttl: 5m
      strategy: simple|stale-while-revalidate
    timeout: 10s
```

## Server Configuration

Server settings **must** be in `tinkerdown.yaml` (not available in frontmatter):

```yaml
server:
  port: 8080           # Server port (default: 8080)
  host: localhost      # Server host (default: localhost)
```

## API Configuration

The optional `api:` block enables a REST API for programmatic access to your app's data sources.

```yaml
api:
  enabled: true        # Enable REST API endpoints (default: false)
```

> **Important:** If `api.auth` is omitted while `api.enabled: true`, all API requests are allowed through without authentication.

### Authentication

Configure `api.auth` to require API key authentication on all API endpoints.

#### Legacy single key

The simplest setup — one key with full permissions (read, write, delete):

```yaml
api:
  enabled: true
  auth:
    api_key: ${API_KEY}
```

#### Multi-key with permissions

For finer-grained access, define named keys with specific permissions:

```yaml
api:
  enabled: true
  auth:
    keys:
      - name: dashboard
        key: ${DASHBOARD_KEY}
        permissions: [read]
      - name: admin
        key: ${ADMIN_KEY}
        permissions: [read, write, delete]
```

Available permissions:

| Permission | Allowed HTTP methods |
|------------|---------------------|
| `read`     | GET, HEAD           |
| `write`    | POST, PUT, PATCH    |
| `delete`   | DELETE              |

> **Note:** `OPTIONS` requests bypass permission checks entirely to support CORS preflight.

If `permissions` is omitted for a named key, the key can authenticate but has no permissions — all method-level checks will be denied.

Both formats can coexist — the legacy `api_key` is treated as a key named "default" with full permissions.

#### Custom header

By default, all keys (both legacy and named) are sent via the `X-API-Key` header. To use Bearer token authentication instead:

```yaml
api:
  enabled: true
  auth:
    api_key: ${API_KEY}
    header_name: Authorization  # Expects "Authorization: Bearer <token>"
```

When using `Authorization`, clients send `Authorization: Bearer <token>` and the middleware strips the `Bearer ` prefix before matching. Set `api_key` (or `keys[].key`) to the **raw token value**, not the full `Bearer ...` string.

> **Secure default:** If any key (`api_key` or `keys[].key`) references an environment variable that is **not set**, authentication is still treated as **enabled**. The expanded key is empty, so no request can match it and all API requests are rejected. Auth is never silently disabled by a missing env var.

### CORS

Configure allowed origins for cross-origin API requests:

```yaml
api:
  enabled: true
  cors:
    origins:
      - "http://localhost:3000"
      - "https://myapp.example.com"
```

Use `"*"` to allow all origins (not recommended for production with authenticated APIs).

### Rate Limiting

Protect API endpoints with per-IP rate limiting:

```yaml
api:
  enabled: true
  rate_limit:
    requests_per_second: 10   # Per IP (default: 10; supports floats, e.g. 0.5)
    burst: 20                 # Max requests in a spike before rate kicks in (default: 20)
    max_tracked_ips: 10000    # Max unique IPs tracked; LRU eviction (default: 10000)
```

## Styling Configuration

Can be in frontmatter or `tinkerdown.yaml`. Config file applies globally:

```yaml
styling:
  theme: clean         # Theme name (default: clean)
  # Options: clean, dark, minimal
```

## Source Configuration

Sources in `tinkerdown.yaml` are available to **all pages**. Page-specific sources should go in frontmatter.

### Common Options

```yaml
sources:
  example:
    type: <source_type>    # Required: sqlite, rest, graphql, exec, json, csv, markdown, wasm
    cache:                 # Optional: caching configuration
      ttl: 5m              # Time-to-live
      strategy: simple     # simple or stale-while-revalidate
    timeout: 10s           # Optional: request timeout
```

### SQLite Source

```yaml
sources:
  tasks:
    type: sqlite
    path: ./data.db
    query: SELECT * FROM tasks
```

### REST Source

```yaml
sources:
  users:
    type: rest
    from: https://api.example.com/users
    method: GET
    headers:
      Authorization: Bearer ${API_TOKEN}
```

### GraphQL Source

```yaml
sources:
  issues:
    type: graphql
    from: https://api.github.com/graphql
    query_file: queries/issues.graphql  # Path to .graphql file
    variables:                           # Optional query variables
      owner: livetemplate
      repo: tinkerdown
    result_path: repository.issues.nodes # Dot-path to extract array
    options:
      auth_header: "Bearer ${GITHUB_TOKEN}"
```

GraphQL-specific options:

| Option | Required | Description |
|--------|----------|-------------|
| `query_file` | Yes | Path to `.graphql` file (relative to app directory) |
| `variables` | No | Map of query variables (supports `${ENV_VAR}` expansion) |
| `result_path` | Yes | Dot-notation path to extract array from response |
| `options.auth_header` | No | Authorization header value |

### Exec Source

```yaml
sources:
  system_info:
    type: exec
    command: uname -a
```

### JSON Source

```yaml
sources:
  config:
    type: json
    path: ./_data/config.json
```

### CSV Source

```yaml
sources:
  products:
    type: csv
    path: ./_data/products.csv
    delimiter: ","
    header: true
```

### Markdown Source

```yaml
sources:
  posts:
    type: markdown
    path: ./_data/posts/
    glob: "*.md"
```

### WASM Source

```yaml
sources:
  custom:
    type: wasm
    module: ./custom.wasm
    config:
      api_key: ${API_KEY}
```

## Caching Configuration

Caching is where `tinkerdown.yaml` shines - complex cache strategies:

```yaml
sources:
  api_data:
    type: rest
    from: https://api.example.com/data
    cache:
      ttl: 5m                  # Cache duration
      strategy: stale-while-revalidate  # Background refresh
```

### Cache Strategies

| Strategy | Description |
|----------|-------------|
| `simple` | Return cached data until TTL expires |
| `stale-while-revalidate` | Return stale data immediately, refresh in background |

## Environment Variables

Use `${VAR_NAME}` syntax for secrets - a key reason to use `tinkerdown.yaml`:

```yaml
sources:
  api:
    type: rest
    from: ${API_URL}
    headers:
      Authorization: Bearer ${API_TOKEN}
```

## Validation

Validate your configuration:

```bash
tinkerdown validate
```

## Example: Complex Multi-Page App

When you have shared authentication, caching, and multiple pages:

```yaml
# tinkerdown.yaml
server:
  port: 3000

styling:
  theme: dark

sources:
  # Shared auth - used by all pages
  current_user:
    type: rest
    from: ${AUTH_API}/me
    headers:
      Authorization: Bearer ${AUTH_TOKEN}
    cache:
      ttl: 10m
      strategy: stale-while-revalidate

  # Shared data with aggressive caching
  products:
    type: rest
    from: https://api.example.com/products
    cache:
      ttl: 1h
      strategy: stale-while-revalidate

  # Shared database
  orders:
    type: sqlite
    path: ./data/orders.db
    query: SELECT * FROM orders
```

Then each page uses these sources:

```markdown
---
title: Dashboard
# No need to redefine sources - they come from tinkerdown.yaml
---

# Dashboard

Welcome, {{.current_user.name}}!

<table lvt-source="orders" lvt-columns="id,product,status">
</table>
```

## Priority: Frontmatter vs Config File

When the same source is defined in both places:

1. **Frontmatter wins** for that page
2. Config file provides defaults

### With a `generation:` block

A project that declares a [`generation:` block](#the-generation-block) gains a third, highest tier:

1. **Approved definitions are pinned** — a page cannot redefine a source or action named in `generation.sources` / `generation.actions`
2. Frontmatter
3. Config file defaults

This exists because a page's frontmatter can declare its own sources and actions. Without pinning, a generated page could *reference* an approved name while *defining* that name as something else — passing any check that reasons about names alone. Pinning makes an approved name mean one thing regardless of what the page says. An attempted redefinition is ignored and logged, so it is visible rather than silent.

Approval also makes site-level **actions** reachable. Ordinarily a page can only invoke actions declared in its own frontmatter; naming an action in `generation.actions` opts it in. Actions written for schedules or webhooks stay unreachable from pages unless approved.

Projects without a `generation:` block are unaffected — the two-tier rule above applies exactly as before.

## The `generation:` block

Declares which of this project's sources and actions an LLM may wire up when generating an app against it. Its presence is what turns a `tinkerdown.yaml` into a *manifest*.

```yaml
sources:
  requests:
    describes: "Pending PII access requests awaiting approval"
    type: sqlite
    db: ./requests.db
    table: requests

actions:
  approve-request:
    describes: "Grants scoped, time-boxed access and writes an audit record"
    kind: sql
    source: requests
    statement: "UPDATE requests SET status = 'approved' WHERE id = :id"
    confirm: "Approve this access request?"
    params:
      id:
        type: number
        required: true

generation:
  sources: [requests]
  actions: [approve-request]
  style_guide: ./style-guide.md   # optional
```

| Field | Purpose |
|---|---|
| `sources` | Source names a generated app may bind to. Each must exist in `sources:`. |
| `actions` | Action names a generated app may invoke. Each must exist in `actions:`. |
| `style_guide` | Optional path to a markdown file describing house style — tone, layout, preferred components. Without it, generation falls back to the project theme and PicoCSS defaults. |

The `describes:` field on a source or action carries no runtime behavior. It is the human-readable summary an operator reads when reviewing what a generated app actually does — "the pending PII access requests queue" rather than a table name.

### Atomic multi-statement actions (`statements:`)

A `kind: sql` action normally carries a single `statement:`. When an action must do several things that have to succeed or fail together — change state **and** append an audit record, say — use a `statements:` list instead; they run in one transaction (all commit, or the first error rolls all of them back):

```yaml
approve-export:
  kind: sql
  source: access_requests
  statements:
    - "INSERT INTO exports (...) SELECT ... FROM orders_pii LIMIT (SELECT row_cap FROM access_requests WHERE id = :id)"
    - "INSERT INTO audit_log (...) SELECT ... FROM access_requests WHERE id = :id"
    - "UPDATE access_requests SET status = 'approved' WHERE id = :id"
```

An action must set exactly one of `statement`/`statements` — both, or neither, is a config error caught at load. Keep values the client controls out of the batch where they matter: reading a row cap from the row (`LIMIT (SELECT row_cap FROM …)`) rather than from a button parameter keeps a scoped operation scoped even against a tampered client.

**`:operator` is reserved and server-set.** Every `kind: sql` action can reference `:operator`; it is always the server-side operator identity (from `--operator`), and any `operator` value in the client's action payload is ignored. This is deliberate — it lets a statement like `… SET approver = :operator` write a trustworthy identity to an audit trail that a client cannot spoof. Do not use `operator` as a client-supplied parameter name.

## Next Steps

- [Frontmatter Reference](frontmatter.md) - Recommended configuration approach
- [Data Sources Guide](../guides/data-sources.md) - Using sources
