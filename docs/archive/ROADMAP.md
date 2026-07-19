# Tinkerdown Roadmap: Micro Apps & Tooling Platform

## Vision

Transform Tinkerdown into a **full-featured platform for building micro apps and internal tooling** using markdown as the primary interface.

**Target Users:**
- Developers building internal tools, dashboards, and admin panels
- Technical writers creating interactive documentation
- Teams needing quick data-driven apps without full-stack complexity
- AI systems generating functional apps from natural language

---

## For AI Assistants: How to Use This Roadmap

This roadmap is designed to be followed and updated by LLMs. Here's how to work with it:

### Selecting a Task
1. Check **Current Sprint** below for active work
2. If empty, pick from **Quick Wins** section or **Implementation Priorities**
3. Find the corresponding **Detailed Implementation Plan** at the end of this document

### Git Workflow (Required)

**IMPORTANT: Always use Pull Requests for changes. Never commit directly to main.**

1. **Create a worktree** for the feature:
   ```bash
   git worktree add .worktrees/<feature-name> -b feature/<feature-name>
   cd .worktrees/<feature-name>
   ```

2. **Make changes** in the worktree (not in main)

3. **Create a PR** when ready:
   ```bash
   git push -u origin feature/<feature-name>
   gh pr create --title "feat: <description>" --body "..."
   ```

4. **Wait for review/approval** before merging

5. **Cleanup** after merge:
   ```bash
   cd /path/to/main/repo
   git worktree remove .worktrees/<feature-name>
   git pull origin main
   ```

### Executing a Task
1. **Create a worktree** (see Git Workflow above)
2. Read the implementation plan for the task
3. Follow the **Implementation Steps** in order
4. Run the **E2E Tests** specified (or write them if missing)
5. Update **Documentation** as specified
6. Update **Examples** as specified
7. **Create a PR** for review

### After Completing a Task
1. Mark the checkbox as done: `- [ ]` → `- [x]`
2. Update **Current Sprint** section below (move to completed, add next task)
3. **Create PR** with message format: `feat(<phase>): <description>`
4. If implementation plan needs updates based on learnings, update it
5. **Wait for PR approval** before considering task complete

### Checkbox Legend
- `- [ ]` = Not started
- `- [~]` = In progress (add your name/date: `- [~] @claude 2025-01-15`)
- `- [x]` = Completed

### Common Commands
```bash
# Run all tests
go test ./...

# Run specific test
go test ./... -v -run TestName

# Start dev server
tinkerdown serve examples/your-example

# Validate markdown
tinkerdown validate examples/your-example

# Build for production
go build -o tinkerdown ./cmd/tinkerdown
```

### Commit Message Format
```
feat(phase-N): brief description

- Detail 1
- Detail 2

Refs: #issue-number (if applicable)
```

---

## Current Sprint

> **Instructions:** Keep this section updated with current work. Maximum 3 active tasks.

### In Progress
<!-- Add tasks here when starting work -->
_No tasks currently in progress_

### Recently Completed
<!-- Move completed tasks here, keep last 5 -->
1. **3.1 Enhanced CLI Scaffolding** - Completed 2026-01-01 (PR #24)
   - Added `--template` flag with 7 templates: basic, tutorial, todo, dashboard, form, api-explorer, wasm-source
   - Template descriptions and `--list` flag for discovery
   - Custom delimiters (`[[` `]]`) to avoid conflicts with Tinkerdown runtime syntax
   - 19 E2E tests covering all templates and edge cases
   - Full documentation in template READMEs

2. **3.6B Create Documentation Structure** - Completed 2025-12-31 (PR #20)
   - Created `docs/getting-started/` with installation, quickstart, project-structure
   - Created `docs/guides/` with auto-rendering, data-sources, go-templates, styling, debugging, deployment
   - Created `docs/reference/` with cli, config, frontmatter, lvt-attributes
   - Created `docs/sources/` with docs for all source types (sqlite, rest, exec, json, csv, markdown, wasm, graphql)

3. **4.1 GraphQL Source** - Completed 2025-12-31 (PR #22)
   - Query file support (.graphql files)
   - Variable substitution with environment expansion
   - Result path extraction for nested responses (`result_path` config)
   - Authentication via headers (same as REST)
   - Retry/circuit breaker support (inherits from Phase 2.1)
   - E2E test with public Countries API
   - Full documentation in `docs/sources/graphql.md`

4. **2.3 Multi-page WebSocket Support** - Completed 2025-12-31 (PR #19)
   - Accept page identifier via query param (`?page=/path`)
   - Route WebSocket messages to correct page's state
   - Handle page transitions gracefully
   - Clean up state on page navigation

5. **2.2 Source Caching Layer** - Completed 2025-12-30 (PR #18)
   - Cache configuration per source (`ttl`, `strategy`)
   - In-memory cache with TTL expiration
   - Two strategies: `simple` and `stale-while-revalidate`
   - Automatic cache invalidation on writes
   - Background revalidation with proper cancellation

### Next Up
<!-- Queue of next 3-5 tasks to tackle -->
1. 4.2 MongoDB Source
2. 4.3 Source Composition
3. 3.3 Debug Mode & Logging
4. 3.2 Expanded Validation

---

## Priority Framework

| Level | Criteria | Focus |
|-------|----------|-------|
| **P0** | Blocks core functionality or causes data loss | Immediate |
| **P1** | High-impact features enabling new use cases | Near-term |
| **P2** | Developer experience and productivity | Medium-term |
| **P3** | Polish, optimization, and edge cases | Ongoing |

---

## Version Milestones

| Phase | Target Version | Key Deliverables |
|-------|---------------|------------------|
| Phase 1 | **v0.2.0** | Auto-rendering tables, lists, selects |
| Phase 2 | **v0.3.0** | Error handling, caching, multi-page WebSocket |
| Phase 3 | **v0.4.0** | CLI templates, validation, WASM SDK, docs cleanup |
| Phase 4 | **v0.5.0** | GraphQL, MongoDB, source composition |
| Phase 5 | **v1.0.0** | Auth, rate limiting, health endpoints, security hardening |
| Phase 6 | **v1.1.0** | Component library, pagination, themes |
| Phase 7 | **v1.2.0** | Multi-user broadcasting, scheduled tasks, API mode |
| Phase 8 | **v1.3.0** | Bundle optimization, accessibility, comprehensive tests |

> **Note:** v1.0.0 marks production readiness. Earlier versions are for development/testing.

---

## Feature Dependencies

```
Phase 1 (Auto-Rendering)
└── No dependencies - can start immediately

Phase 2 (Stability)
├── 2.1 Error Handling ← None
├── 2.2 Source Caching ← 2.1 (needs error types)
└── 2.3 Multi-Page WS ← None

Phase 3 (Developer Experience)
├── 3.1 CLI Templates ← Phase 1 (uses auto-rendering)
├── 3.2 Validation ← None
├── 3.3 Debug Mode ← None
├── 3.4 Hot Reload ← None
├── 3.5 WASM SDK ← None
└── 3.6 Docs Cleanup ← None

Phase 4 (Data Ecosystem)
├── 4.1 GraphQL ← 2.1, 2.2 (error handling, caching)
├── 4.2 MongoDB ← 2.1, 2.2
├── 4.3 Source Composition ← 2.1 (needs error handling)
├── 4.4 Webhook Source ← 2.3 (WebSocket support)
└── 4.5 S3 Source ← 2.1, 2.2

Phase 5 (Production Readiness)
├── 5.1 Authentication ← None (core middleware)
├── 5.2 Rate Limiting ← None
├── 5.3 Health Endpoints ← 2.1 (error states)
├── 5.4 Graceful Shutdown ← 2.3 (WebSocket tracking)
├── 5.5 Single Binary ← None
└── 5.6 Security ← 5.1 (CSRF needs session)

Phase 6 (UI & Components)
├── 6.1 Components ← Phase 1 (uses auto-rendering)
├── 6.2 Pagination ← 2.2 (uses caching)
├── 6.3 Sorting/Filter ← 6.2 (pagination)
├── 6.4 Themes ← None
└── 6.5 UX Improvements ← None

Phase 7 (Advanced Features)
├── 7.1 Broadcasting ← 2.3 (multi-page WS)
├── 7.2 Scheduled Tasks ← 2.1 (error handling)
├── 7.3 API Mode ← 5.1 (authentication)
├── 7.4 Offline Support ← None
└── 7.5 WASM Marketplace ← 3.5 (WASM SDK)

Phase 8 (Polish)
└── All items ← Phases 1-7
```

---

## Compatibility Guarantees

### Semantic Versioning
This project follows [Semantic Versioning 2.0.0](https://semver.org/):
- **MAJOR** (1.x.x): Breaking changes to markdown syntax, config format, or CLI
- **MINOR** (x.1.x): New features, backward-compatible
- **PATCH** (x.x.1): Bug fixes, backward-compatible

### Backward Compatibility Promises

**Go Templates (Always Supported):**
- Existing `{{range}}`, `{{if}}`, `{{.field}}` syntax will always work
- Auto-rendering is additive; manual templates are never required
- No breaking changes to template helpers

**Configuration (tinkerdown.yaml):**
- New fields are optional with sensible defaults
- Deprecated fields emit warnings for 2 minor versions before removal
- Breaking config changes only in major versions

**lvt-* Attributes:**
- Core attributes (`name` on button/form, `lvt-on:change`, etc.) are stable
- New attributes don't affect existing ones
- Deprecated attributes work for 2 minor versions with warnings

### Migration Support
- Breaking changes include migration guide in release notes
- `tinkerdown migrate` command for automated config updates (future)
- Deprecation warnings in CLI output

---

## Phase 1: Auto-Rendering & Go Template Ergonomics (P0)

### Value Proposition
> "Go templates you already know. Less boilerplate for common patterns."

Go's `html/template` is a known quantity—mustache syntax is universal, conditionals are straightforward. The main pain point is repetitive `{{range}}` loops for data display. This phase solves that with auto-rendering, not a new DSL.

### Design Principles

```
┌─────────────────────────────────────────────────────────────┐
│                    What you write                            │
├─────────────────────────────────────────────────────────────┤
│  Go templates              │  Auto-rendering               │
│  {{range .tasks}}          │  lvt-source="tasks"           │
│  {{if .done}}              │  lvt-columns="done,text"      │
│  {{.field}}                │  lvt-empty="No items"         │
├─────────────────────────────────────────────────────────────┤
│  Full control, any layout  │  Common patterns only         │
│  Custom when needed        │  Tables, lists, selects       │
└─────────────────────────────────────────────────────────────┘
```

**The 80/20 rule:** Auto-rendering handles 80% of data display (tables, lists, selects). Go templates handle 100% of custom layouts.

### Current Attribute Ownership

**Already in `@livetemplate/client` (core):**

*Event Handling:*
- `name` (button/form), `lvt-on:change` - Event handlers
- `lvt-click-away` - Click outside detection
- `lvt-key` - Keyboard key filtering
- `lvt-throttle`, `lvt-debounce` - Rate limiting
- `lvt-window-{event}` - Window-level events

*Lifecycle & Reactive:*
- `lvt-el:{method}:on:{state}` - Lifecycle hooks (reset, addClass, toggleAttr, etc.)
- `data-confirm` - Confirmation dialogs
- `data-*`, `lvt-value-*` - Data extraction
- `lvt-ignore` - Skip an element and its subtree during DOM diffing
- `lvt-disable-with` - Button text during submit

*UI Directives:*
- `lvt-scroll` - Scroll behavior (bottom, top, sticky)
- `lvt-highlight` - Flash highlight animation
- `lvt-animate` - Entry animations (fade, slide, scale)
- `lvt-autofocus` - Auto-focus on visibility
- `lvt-focus-trap` - Modal focus trapping
- `lvt-modal-open`, `lvt-modal-close` - Modal controls

**Tinkerdown-specific (auto-rendering):**
- `lvt-source` - Data source binding
- `lvt-columns`, `lvt-actions` - Auto-table generation
- `lvt-value`, `lvt-label` - Select field mapping
- `lvt-empty` - Empty state message

**⚠️ Cleanup: Remove duplicates from Tinkerdown client**
- `name` (button/form), `lvt-on:change` handlers already in core

---

### 1.1 Auto-Rendering Tables

**Problem:** Data tables require verbose `{{range}}` loops with manual header/row generation.

**Solution:** `lvt-source` on `<table>` auto-generates the complete structure:

```html
<!-- Instead of this -->
<table>
  <thead><tr><th>Done</th><th>Task</th><th>Priority</th></tr></thead>
  <tbody>
    {{range .tasks}}
    <tr>
      <td>{{if .done}}✓{{end}}</td>
      <td>{{.text}}</td>
      <td>{{.priority}}</td>
    </tr>
    {{end}}
  </tbody>
</table>

<!-- Write this -->
<table lvt-source="tasks" lvt-columns="done,text,priority">
</table>
```

**Features:**
- `lvt-columns="field,field"` - Which fields to display
- `lvt-columns="field:Label"` - Custom column headers
- `lvt-actions="Delete,Toggle"` - Action buttons column
- `lvt-empty="No tasks yet"` - Empty state message

**Work Required:**
- [ ] Parse `lvt-columns` attribute (field or field:Label format)
- [ ] Generate `<thead>` with column labels
- [ ] Generate `<tbody>` with `{{range .Data}}` iteration
- [ ] Support `lvt-actions` for action button columns
- [ ] Handle empty state with `lvt-empty`

**Impact:** Data tables in one line instead of 15+

---

### 1.2 Auto-Rendering Lists

**Problem:** Lists require `{{range}}` boilerplate.

**Solution:** `lvt-source` on `<ul>` or `<ol>` auto-generates list items:

```html
<!-- Instead of this -->
<ul>
  {{range .items}}
  <li>{{.name}}</li>
  {{end}}
</ul>

<!-- Write this -->
<ul lvt-source="items" lvt-field="name">
</ul>
```

**Features:**
- `lvt-field="name"` - Which field to display (default: entire object)
- `lvt-empty="No items"` - Empty state

**Work Required:**
- [ ] Detect `<ul lvt-source>` and `<ol lvt-source>` patterns
- [ ] Generate `<li>` elements with field content
- [ ] Support `lvt-empty` for empty state

**Impact:** Simple lists without range loops

---

### 1.3 Auto-Rendering Selects

**Problem:** Select dropdowns need `{{range}}` to populate options.

**Solution:** `lvt-source` on `<select>` auto-populates options (already partially implemented):

```html
<!-- Instead of this -->
<select name="category">
  {{range .categories}}
  <option value="{{.id}}">{{.name}}</option>
  {{end}}
</select>

<!-- Write this -->
<select lvt-source="categories" lvt-value="id" lvt-label="name" name="category">
</select>
```

**Work Required:**
- [ ] Verify current implementation works
- [ ] Document `lvt-value` and `lvt-label` attributes
- [ ] Add `lvt-empty` for "Select..." placeholder

**Impact:** Form selects without range loops

---

### 1.4 When to Use Go Templates

Auto-rendering is for common patterns only. Use Go templates when you need:

```html
<!-- Custom card layouts -->
{{range .tasks}}
<div class="card {{if .done}}completed{{end}}">
  <h3>{{.text}}</h3>
  <span class="priority priority-{{.priority}}">{{.priority}}</span>
  <button name="Delete" data-id="{{.id}}">Delete</button>
</div>
{{end}}

<!-- Conditional rendering -->
{{if .error}}
<div class="error">{{.error}}</div>
{{end}}

<!-- Nested data -->
{{range .orders}}
<div class="order">
  <h3>Order #{{.id}}</h3>
  {{range .items}}
  <div class="item">{{.name}} x{{.quantity}}</div>
  {{end}}
</div>
{{end}}
```

**Rule of thumb:** If it's a table, list, or select → use auto-rendering. If it's custom → use Go templates.

---

## Phase 2: Stability & Performance (P0)

### Value Proposition
> "Make what exists work reliably in production"

### 2.1 Data Source Error Handling
**Files:** `internal/source/*.go`, `internal/runtime/state.go`

**Current State:** Errors can crash or hang; no retry logic; silent failures possible.

**Work Required:**
- [x] Unified error types for all sources
- [x] Retry with exponential backoff for transient failures
- [x] Circuit breaker for repeatedly failing sources
- [ ] User-friendly error messages in templates (`.Error` field rendering)
- [x] Timeout configuration per source

**Impact:** Production reliability for data-driven apps

---

### 2.2 Source Caching Layer
**Current State:** Every page view refetches all data.

**Work Required:**
- [x] Cache configuration per source:
  ```yaml
  sources:
    users:
      type: rest
      url: https://api.example.com/users
      cache:
        ttl: 5m
        strategy: stale-while-revalidate
  ```
- [x] In-memory cache with TTL
- [x] Cache invalidation on write operations
- [ ] Manual cache clear via `Refresh` action

**Impact:** 10x faster page loads; reduced API costs; better UX

---

### 2.3 Multi-Page WebSocket Support ✅
**Files:** `internal/server/server.go`

**Current State:** ~~WebSocket handler only serves first route - multi-page sites limited.~~ Completed in PR #19.

**Work Required:**
- [x] Accept page identifier via query param or path
- [x] Route WebSocket messages to correct page's state
- [x] Handle page transitions gracefully
- [x] Clean up state on page navigation

**Impact:** Enables documentation sites with interactive examples on every page

---

## Phase 3: Developer Experience (P1)

### Value Proposition
> "Reduce time from idea to working app by 10x"

### 3.1 Enhanced CLI Scaffolding
**Files:** `cmd/tinkerdown/commands/new.go`

**Current State:** `new` command creates minimal template only.

**Work Required:**
- [ ] Add `--template` flag with options:
  - `basic` - Minimal with one source
  - `todo` - SQLite CRUD with toggle/delete
  - `dashboard` - Multi-source data display
  - `form` - Contact form with SQLite persistence
  - `api-explorer` - REST source with refresh
  - `cli-wrapper` - Exec source with argument form
  - `wasm-source` - Template for building custom WASM sources
- [ ] Generate sample data files for each template
- [ ] Include inline documentation comments

**Impact:** 5-minute start to working prototype

---

### 3.2 Expanded Validation
**Files:** `cmd/tinkerdown/commands/validate.go`

**Current State:** Only validates markdown parsing and Mermaid syntax.

**Work Required:**
- [ ] Validate source references exist in config
- [ ] Check `lvt-*` attributes have valid values
- [ ] Verify source types are valid (exec, pg, rest, json, csv, markdown, sqlite, wasm)
- [ ] Warn on unused source definitions
- [ ] Validate WASM module paths exist
- [ ] Type-check common template patterns

**Impact:** Catch errors at write-time, not runtime

---

### 3.3 Debug Mode & Logging
**Files:** `internal/server/server.go`

**Work Required:**
- [ ] `--debug` / `--verbose` CLI flags
- [ ] Structured JSON logging option
- [ ] Request correlation IDs
- [ ] WebSocket message logging (with sensitive data redaction)
- [ ] State change logging
- [ ] Source fetch timing

**Impact:** 10x faster debugging of production issues

---

### 3.4 Hot Reload for Configuration
**Current State:** Config changes require server restart.

**Work Required:**
- [ ] Watch `tinkerdown.yaml` for changes
- [ ] Reload sources without dropping WebSocket connections
- [ ] Notify connected clients of config reload
- [ ] Support frontmatter changes via file watcher

**Impact:** Faster iteration on source configuration

---

### 3.5 WASM Source Development Kit
**New Feature** - Critical for ecosystem growth

**Work Required:**
- [ ] `tinkerdown wasm init <name>` - Scaffold new WASM source
- [ ] `tinkerdown wasm build` - Compile TinyGo source to WASM
- [ ] `tinkerdown wasm test` - Test WASM module locally
- [ ] Documentation for WASM interface contract
- [ ] Example sources: GitHub API, REST APIs

**Impact:** Enable community source contributions

---

### 3.6 Documentation Cleanup & Consolidation

**Current State:** Phase A (cleanup) completed. Redundant files removed, product docs archived.

**Audit of Current Docs (after Phase A cleanup):**
```
Root level:
├── README.md            # Keep - entry point
└── ROADMAP.md           # Keep - feature planning

docs/ (current structure):
├── auto-rendering.md           # Keep - active user documentation
├── archive/                    # Archived product/marketing docs
│   ├── internal-tools-saas.md
│   ├── launch-page.md
│   └── pmf-one-file-ai-builder.md
└── plans/                      # Keep - design documents

skills/tinkerdown/ (AI reference):
├── SKILL.md             # Keep - skill definition
├── reference.md         # Keep - AI reference
└── examples/            # Keep - example apps
```

**Work Required:**

**Phase A: Cleanup Redundant Docs**
- [ ] Review `PROGRESS.md` - merge relevant content into ROADMAP, delete
- [ ] Review `UX_IMPROVEMENTS.md` - merge into ROADMAP Phase 6, delete
- [ ] Review `docs/implementation-plan.md` - archive or delete if outdated
- [ ] Move marketing/product docs to separate location or archive

**Phase B: Create User-Facing Documentation Structure**
```
docs/
├── getting-started/
│   ├── installation.md        # CLI install, prerequisites
│   ├── quickstart.md          # First app in 5 minutes
│   └── project-structure.md   # File layout, conventions
├── guides/
│   ├── data-sources.md        # All source types with examples
│   ├── auto-rendering.md      # lvt-source tables, lists, selects
│   ├── go-templates.md        # Template syntax, helpers
│   ├── styling.md             # Themes, CSS customization
│   ├── deployment.md          # Production deployment guide
│   └── debugging.md           # Debug mode, logging, troubleshooting
├── reference/
│   ├── cli.md                 # All CLI commands and flags
│   ├── config.md              # tinkerdown.yaml schema
│   ├── lvt-attributes.md      # All lvt-* attributes reference
│   └── frontmatter.md         # Page frontmatter options
├── sources/
│   ├── sqlite.md              # SQLite source reference
│   ├── rest.md                # REST API source reference
│   ├── exec.md                # Exec source reference
│   ├── graphql.md             # GraphQL source (when added)
│   ├── wasm.md                # WASM source authoring guide
│   └── ...                    # Other sources
└── plans/                     # Keep design documents
    └── *.md
```

**Phase C: Create Missing Guides**
- [ ] `docs/getting-started/quickstart.md` - 5-minute first app
- [ ] `docs/guides/auto-rendering.md` - Tables, lists, selects with lvt-source
- [ ] `docs/guides/data-sources.md` - Overview of all source types
- [ ] `docs/reference/lvt-attributes.md` - Complete attribute reference
- [ ] `docs/reference/config.md` - tinkerdown.yaml schema reference

**Phase D: Keep Examples and Docs in Sync**
- [ ] Each feature in docs links to working example
- [ ] Each example has inline comments explaining concepts
- [ ] Examples serve as E2E test fixtures

**Impact:** Clear learning path; reduced confusion; maintainable documentation

---

## Phase 4: Data Ecosystem (P1)

### Value Proposition
> "Connect to any data source in minutes, not days"

### 4.1 GraphQL Source ✅
**Work Required:**
- [x] New source type: `graphql`
- [x] Config: `url`, `query`, `variables`
- [x] Authentication headers
- [x] Auto-flatten nested response (via `result_path`)
- [ ] Support for mutations via write operations (future enhancement)

**Impact:** Modern API ecosystem support

---

### 4.2 MongoDB Source
**Work Required:**
- [ ] New source type: `mongodb`
- [ ] Config: `uri`, `database`, `collection`, `filter`
- [ ] Pure Go driver (no cgo)
- [ ] CRUD operations support

**Impact:** NoSQL database support

---

### 4.3 Source Composition
**New Feature**

**Work Required:**
- [ ] Computed sources that transform other sources:
  ```yaml
  sources:
    users:
      type: rest
      url: https://api.example.com/users
    active_users:
      type: computed
      from: users
      filter: "status == 'active'"
      sort: "name asc"
  ```
- [ ] Expression language: [CEL (Common Expression Language)](https://github.com/google/cel-spec)
  - Simple, safe, fast evaluation
  - Go implementation available: `github.com/google/cel-go`
  - Examples: `status == 'active'`, `age > 18 && role in ['admin', 'editor']`
- [ ] Join sources on common fields
- [ ] Aggregation (count, sum, avg)

**Impact:** Complex data apps without custom code

---

### 4.4 Webhook Source
**Work Required:**
- [ ] Source that receives HTTP POST
- [ ] Store latest N events
- [ ] Trigger UI update on new data
- [ ] Optional signature verification (Stripe, GitHub)

**Impact:** Real-time integrations (webhooks, events)

---

### 4.5 S3/Cloud Storage Source
**Work Required:**
- [ ] New source type: `s3`
- [ ] List objects, read JSON/CSV files
- [ ] Support for GCS, Azure Blob (compatible APIs)
- [ ] Credentials via environment variables

**Impact:** Cloud-native data access

---

## Phase 5: Production Readiness (P1)

### Value Proposition
> "Deploy with confidence to real users"

### 5.1 Authentication Middleware
**Work Required:**
- [ ] Built-in auth strategies:
  - API key (header-based)
  - Basic auth
  - OAuth2 (Google, GitHub)
  - Custom JWT validation
- [ ] Per-page auth requirements in frontmatter:
  ```yaml
  auth: required
  # or
  auth:
    provider: github
    allowed_orgs: [mycompany]
  ```
- [ ] User context available in templates (`{{.User.Email}}`)

**Impact:** Secure internal tools; multi-user apps

---

### 5.2 Request Rate Limiting
**Work Required:**
- [ ] Per-IP rate limiting
- [ ] Per-source rate limiting (protect external APIs)
- [ ] Configurable limits per endpoint
- [ ] Graceful 429 responses with retry-after

**Impact:** Protection against abuse; resource management

---

### 5.3 Health & Metrics Endpoints
**Work Required:**
- [ ] `/health` - Basic liveness check
- [ ] `/ready` - Readiness including source connectivity
- [ ] `/metrics` - Prometheus-compatible metrics:
  - Request count/latency by route
  - WebSocket connection count
  - Source fetch latency/error rates
  - WASM execution time

**Impact:** Kubernetes-ready deployment; observability

---

### 5.4 Graceful Shutdown
**Work Required:**
- [ ] Track in-flight requests
- [ ] Drain WebSocket connections
- [ ] Complete pending source operations
- [ ] Close WASM runtimes cleanly
- [ ] Configurable shutdown timeout

**Impact:** Zero-downtime deployments

---

### 5.5 Single Binary Distribution
**Work Required:**
- [ ] Embed client assets in Go binary
- [ ] `tinkerdown build <dir>` command producing standalone binary
- [ ] Cross-compilation support (linux, darwin, windows)
- [ ] Docker image generation

**Impact:** Simple deployment; Docker images <50MB

---

### 5.6 Security Hardening

**Work Required:**

**CSRF Protection:**
- [ ] Generate CSRF tokens for all forms
- [ ] Validate tokens on POST/PUT/DELETE requests
- [ ] Auto-inject token into forms with `name` attribute
- [ ] Cookie-based double-submit pattern for WebSocket actions

**Content Security Policy (CSP):**
- [ ] Default restrictive CSP headers
- [ ] Configurable CSP via `tinkerdown.yaml`:
  ```yaml
  security:
    csp:
      default-src: "'self'"
      script-src: "'self' 'unsafe-inline'"  # Required for inline event handlers
      style-src: "'self' 'unsafe-inline'"
      connect-src: "'self' ws: wss:"
  ```
- [ ] Nonce-based script loading for stricter environments

**Input Sanitization:**
- [ ] HTML escape all template output by default (Go templates do this)
- [ ] Validate source configuration values at startup
- [ ] Sanitize file paths in `exec` source to prevent path traversal
- [ ] SQL parameterization enforced in SQLite source (already implemented)
- [ ] Validate and sanitize REST API URL templates

**WASM Resource Limits:**
- [ ] Memory limits per WASM module (default: 64MB)
- [ ] CPU time limits per execution (default: 30s)
- [ ] Configuration via source definition:
  ```yaml
  sources:
    custom_source:
      type: wasm
      module: ./custom.wasm
      limits:
        memory: 128MB
        timeout: 60s
  ```
- [ ] Graceful termination on limit exceeded
- [ ] Logging of resource usage for debugging

**Impact:** Secure deployment for internal tools; defense in depth

---

## Phase 6: UI & Components (P2)

### Value Proposition
> "Beautiful apps without CSS expertise"

### 6.1 Component Library
**Work Required:**
- [ ] Chart component (line, bar, pie via Chart.js or similar)
- [ ] Modal/dialog component with `lvt-modal`
- [ ] Toast notifications for action feedback
- [ ] File upload with drag-and-drop
- [ ] Tree view for hierarchical data
- [ ] Tabs component
- [ ] Accordion/collapsible sections

**Impact:** Rich UIs without custom code

---

### 6.2 Built-in Pagination
**Current State:** Must render all data; large datasets slow.

**Work Required:**
- [ ] `lvt-paginate="20"` attribute on containers
- [ ] Auto-generate prev/next controls
- [ ] Server-side pagination for sources
- [ ] URL-based page state for bookmarkability

**Impact:** Apps handling 10k+ records

---

### 6.3 Built-in Sorting & Filtering
**Work Required:**
- [ ] `lvt-sortable` attribute on tables
- [ ] `lvt-filter="field"` for search input
- [ ] Client-side for small datasets (<1000 rows)
- [ ] Server-side for large datasets

**Impact:** Usable data tables out of the box

---

### 6.4 Theme System Expansion
**Current State:** Only "clean" theme fully implemented.

**Work Required:**
- [ ] Complete "dark" and "minimal" themes
- [ ] Custom theme via CSS variables:
  ```yaml
  styling:
    theme: custom
    primary_color: "#007bff"
    background: "#1a1a2e"
  ```
- [ ] Dark mode toggle component
- [ ] Per-page theme override in frontmatter

**Impact:** Brand customization; accessibility

---

### 6.5 UX Improvements

**Navigation Enhancements:**
- [ ] Sticky table of contents in right sidebar (auto-generated from headings)
- [ ] Previous/Next page navigation buttons at bottom of content
- [ ] Sidebar collapse/expand toggle for small screens
- [ ] Active page indicator with visual highlighting
- [ ] Breadcrumb navigation for deep hierarchies

**Code Block Improvements:**
- [ ] Copy-to-clipboard button on all code blocks
- [ ] Syntax highlighting for 20+ languages
- [ ] Line numbers (optional, configurable)
- [ ] Code block titles/filenames

**Loading & Feedback:**
- [ ] Loading skeleton screens during data fetch
- [ ] Smooth transitions between page states
- [ ] Toast notifications for action success/failure
- [ ] Progress indicators for long operations

**Search Enhancements:**
- [ ] Extended search result previews (120+ characters)
- [ ] Keyboard navigation in search results (arrow keys, Enter)
- [ ] Search result highlighting in content
- [ ] Recent searches history

**Mobile Experience:**
- [ ] Responsive sidebar with swipe gestures
- [ ] Touch-friendly interactive elements
- [ ] Optimized table scrolling on mobile

**Impact:** Professional documentation sites with excellent user experience

---

## Phase 7: Advanced Features (P2)

### Value Proposition
> "Handle complex real-world scenarios"

### 7.1 Multi-User State Broadcasting
**Work Required:**
- [ ] Shared state mode for collaborative apps:
  ```yaml
  sources:
    tasks:
      type: sqlite
      broadcast: true  # Sync across all connected clients
  ```
- [ ] Broadcast state changes to all connected clients
- [ ] Conflict resolution strategies (last-write-wins, merge)
- [ ] Presence indicators (who's viewing)

**Impact:** Real-time collaborative tools

---

### 7.2 Scheduled Tasks
**Work Required:**
- [ ] Cron-like syntax in config:
  ```yaml
  schedules:
    refresh_data:
      cron: "*/5 * * * *"
      source: external_api
      action: Refresh
  ```
- [ ] Background execution without user connection
- [ ] Error notifications via webhook

**Impact:** Data refresh without user interaction

---

### 7.3 API Endpoint Mode
**Work Required:**
- [ ] Expose sources as REST endpoints:
  ```yaml
  api:
    enabled: true
    prefix: /api/v1
    sources:
      - name: tasks
        methods: [GET, POST, PUT, DELETE]
  ```
- [ ] OpenAPI spec generation
- [ ] API key authentication

**Impact:** Backend services from markdown definitions

---

### 7.4 Offline Support
**Work Required:**
- [ ] Service worker for asset caching
- [ ] Offline indicator component
- [ ] Queue actions when offline
- [ ] Sync when reconnected
- [ ] Conflict resolution UI

**Impact:** Reliable mobile/field usage

---

### 7.5 WASM Source Marketplace
**Work Required:**
- [ ] Registry of community WASM sources
- [ ] `tinkerdown source add <name>` command
- [ ] Versioning and updates
- [ ] Security review process
- [ ] Documentation site for source authors

**Impact:** Rich ecosystem without core development

---

## Phase 8: Polish & Optimization (P3)

### Value Proposition
> "Production-grade performance and reliability"

### 8.1 Bundle Size Optimization
**Current State:** Client bundle includes Monaco for code blocks.

**Work Required:**
- [ ] Lazy load heavy components (Monaco, charts)
- [ ] Tree-shake unused features
- [ ] Alternative lightweight code viewer
- [ ] Target <200KB initial bundle

---

### 8.2 Accessibility Audit
**Work Required:**
- [ ] WCAG 2.1 AA compliance
- [ ] Full keyboard navigation
- [ ] Screen reader testing
- [ ] ARIA attributes for all components
- [ ] Focus management on page transitions
- [ ] Color contrast validation

---

### 8.3 Performance Profiling
**Work Required:**
- [ ] Built-in performance tracing
- [ ] Slow source warnings (>500ms)
- [ ] Memory usage monitoring for WASM
- [ ] WebSocket message size optimization
- [ ] Template rendering performance

---

### 8.4 Comprehensive Test Suite
**Work Required:**
- [ ] Unit tests for all sources (including SQLite, WASM)
- [ ] Integration tests for GenericState action dispatch
- [ ] Cross-browser E2E tests
- [ ] Performance regression tests
- [ ] WASM source contract tests

---

## Implementation Priorities

```
Priority Order (based on user impact and dependencies):

HIGH IMPACT, LOW EFFORT (Quick Wins)
├── 1.1 Auto-rendering tables (lvt-source + lvt-columns)
├── 1.3 Document existing select auto-rendering
├── 3.6A Docs cleanup (delete redundant files)
├── 3.3 Debug mode CLI flag
├── 3.2 Source reference validation
└── 3.1 CLI templates (todo, dashboard)

HIGH IMPACT, MEDIUM EFFORT (Core Features)
├── 1.2 Auto-rendering lists
├── 2.2 Source caching
├── 2.1 Error handling improvements
├── 3.6B-C Docs structure + guides
├── 3.5 WASM source dev kit
├── 5.3 Health endpoints
└── 4.1 GraphQL source

HIGH IMPACT, HIGH EFFORT (Major Features)
├── 5.1 Authentication
├── 7.1 Multi-user broadcasting
├── 5.5 Single binary distribution
└── 7.3 API endpoint mode

MEDIUM IMPACT (Nice to Have)
├── 6.1 Chart component (future)
├── 4.3 Source composition
├── 7.2 Scheduled tasks
└── 8.1-8.4 Polish items
```

---

## Success Metrics

### Phase 1-2 Complete
**Functionality:**
- [ ] All example apps work reliably (currently 14 in `examples/`)
- [ ] Zero crashes on source errors (graceful degradation)
- [ ] New app scaffolded in <1 minute
- [ ] 90% of config errors caught at validation time

**Testing Requirements:**
- [ ] E2E tests for auto-rendering components (table, list, select)
- [ ] Integration tests for error handling and retry logic
- [ ] Test coverage ≥70% for new code

### Phase 3-4 Complete
**Functionality:**
- [ ] Apps load <500ms with caching
- [ ] 8+ data source types available
- [ ] Apps deployable to Kubernetes with health checks
- [ ] Auth-protected internal tools working

**Testing Requirements:**
- [ ] E2E tests for each new data source type
- [ ] Integration tests for caching layer
- [ ] Security tests for authentication middleware
- [ ] Performance benchmarks for source operations
- [ ] Test coverage ≥80% for core modules

### Phase 5-6 Complete
**Functionality:**
- [ ] 10+ reusable components
- [ ] 10k row tables render smoothly
- [ ] Real-time collaborative demo working
- [ ] REST API generation from sources

**Testing Requirements:**
- [ ] E2E browser tests for all UI components
- [ ] Cross-browser testing (Chrome, Firefox, Safari)
- [ ] Accessibility tests (axe-core integration)
- [ ] Load tests for pagination and large datasets
- [ ] Test coverage ≥85% for component library

### Phase 7-8 Complete
**Functionality:**
- [ ] <200KB initial bundle size
- [ ] WCAG 2.1 AA compliant
- [ ] Production-ready security hardening

**Testing Requirements:**
- [ ] Security audit tests (CSRF, CSP validation)
- [ ] WASM resource limit tests
- [ ] Performance regression test suite
- [ ] Full accessibility audit with manual testing
- [ ] Test coverage ≥95% on core (unit + integration + E2E)

---

## Quick Wins (Start Immediately)

These high-impact, low-effort items can be tackled immediately. See referenced sections for full details.

**Auto-Rendering (Highest Priority):**
1. **Auto-table rendering** → See [Phase 1, Section 1.1](#11-auto-rendering-tables)
2. **Auto-list rendering** → See [Phase 1, Section 1.2](#12-auto-rendering-lists)
3. **Document existing `<select lvt-source>`** → See [Phase 1, Section 1.3](#13-auto-rendering-selects)

**Developer Experience:**
4. **Add `--debug` flag** → See [Phase 3, Section 3.3](#33-debug-mode--logging)
5. **CLI templates** → See [Phase 3, Section 3.1](#31-enhanced-cli-scaffolding)
6. **Source reference validation** → See [Phase 3, Section 3.2](#32-expanded-validation)

**Documentation Cleanup:**
7. **Delete redundant docs** → See [Phase 3, Section 3.6 Phase A](#36-documentation-cleanup--consolidation)
8. **Create docs structure** → See [Phase 3, Section 3.6 Phase B](#36-documentation-cleanup--consolidation)
9. **Write quickstart guide** → See [Phase 3, Section 3.6 Phase C](#36-documentation-cleanup--consolidation)

---

## Contributing

> **TODO:** Create `CONTRIBUTING.md` with development setup and contribution guidelines.

Each feature should have:
1. Design document before implementation
2. Tests covering new functionality
3. Documentation updates
4. Example apps demonstrating usage

---

# Detailed Implementation Plans

> **For AI Assistants:** Each implementation plan follows this structure:
> - **Prerequisites**: What must exist before starting
> - **Files to Modify**: Exact paths to change
> - **Implementation Steps**: Ordered tasks with checkboxes
> - **Verification**: How to confirm it works
> - **Definition of Done**: Exit criteria for completion

---

## Phase 1: Auto-Rendering - Implementation Plan

### 1.1 Auto-Rendering Tables

**Prerequisites:**
- [ ] Understand current `page.go` structure (read `internal/page/page.go`)
- [ ] Review existing lvt-source handling (grep for `lvt-source` in codebase)

**Files to Modify:**
- `internal/page/page.go` - Add table auto-generation logic
- `internal/page/page_test.go` - Unit tests for parsing

#### Implementation Steps

**Step 1: Enhance table auto-generation (`page.go`)**
```go
// Extend autoGenerateTableTemplate() to handle lvt-columns parsing
// Input: <table lvt-source="tasks" lvt-columns="done:Done,text:Task,priority:Priority">
// Output: Full datatable template with headers and row iteration

func autoGenerateTableTemplate(source, columns, actions, empty string) string {
    // Parse columns: "done,text,priority" or "done:Done,text:Task"
    // Generate thead with labels
    // Generate tbody with {{range .Data}} iteration
    // Add action buttons if lvt-actions specified
    // Add empty state if lvt-empty specified
}
```

- [ ] Parse `lvt-columns="field,field"` simple format
- [ ] Parse `lvt-columns="field:Label,field:Label"` with custom headers
- [ ] Generate `<thead>` with column labels (field name or custom label)
- [ ] Generate `<tbody>` with `{{range .Data}}` iteration
- [ ] Support `lvt-actions="Delete,Toggle"` for action button columns
- [ ] Handle empty state with `lvt-empty="No items yet"`

**Step 2: Detect and transform table elements**
- [ ] Find `<table lvt-source="x">` during HTML processing
- [ ] Extract `lvt-columns`, `lvt-actions`, `lvt-empty` attributes
- [ ] Replace element content with generated Go template

#### E2E Tests

**File:** `auto_table_e2e_test.go`

```go
func TestAutoTableBasic(t *testing.T) {
    // Test: <table lvt-source="x" lvt-columns="a,b"> generates table with headers and rows
}

func TestAutoTableCustomLabels(t *testing.T) {
    // Test: lvt-columns="done:Done,text:Task" uses custom labels
}

func TestAutoTableWithActions(t *testing.T) {
    // Test: lvt-actions="Delete,Toggle" adds action buttons column
}

func TestAutoTableEmptyState(t *testing.T) {
    // Test: lvt-empty="No data" shows when source returns empty array
}

func TestAutoTableWithData(t *testing.T) {
    // Test: Table renders actual data from source
}
```

#### Documentation Updates

- [ ] `docs/auto-rendering.md` - Complete auto-rendering guide
- [ ] Update `docs/lvt-source.md` with table examples

#### Example Updates

- [ ] `examples/auto-table/` - Table with lvt-source and lvt-columns
- [ ] Update `examples/lvt-source-sqlite-test/` to demonstrate auto-table

#### Verification

Run these commands to verify the implementation:
```bash
# Unit tests
go test ./internal/page/... -v -run TestAutoTable

# E2E tests
go test ./... -v -run TestAutoTable

# Manual verification
cd examples/auto-table && tinkerdown serve
# Open browser, verify table renders with headers and data
```

#### Definition of Done

- [ ] All checkboxes above are checked
- [ ] All E2E tests pass
- [ ] Example app runs without errors
- [ ] Documentation updated
- [ ] Code reviewed/committed
- [ ] Update **Current Sprint** section in this file

---

### 1.2 Auto-Rendering Lists

**Prerequisites:**
- [ ] 1.1 Auto-Rendering Tables completed (shares parsing logic)

**Files to Modify:**
- `internal/page/page.go` - Add list auto-generation logic

#### Implementation Steps

**Step 1: Add list auto-generation (`page.go`)**
```go
func autoGenerateListTemplate(source, field, empty string) string {
    // Input: <ul lvt-source="items" lvt-field="name">
    // Output: <ul>{{range .Data}}<li>{{.name}}</li>{{end}}</ul>
    // If no field specified, use {{.}} for simple values
}
```

- [ ] Detect `<ul lvt-source>` and `<ol lvt-source>` patterns
- [ ] Support `lvt-field="name"` to display specific field
- [ ] Default to `{{.}}` if no field specified (for string arrays)
- [ ] Handle empty state with `lvt-empty`

#### E2E Tests

```go
func TestAutoListBasic(t *testing.T) {
    // Test: <ul lvt-source="items"> generates list items
}

func TestAutoListWithField(t *testing.T) {
    // Test: lvt-field="name" displays specific field
}

func TestAutoListEmptyState(t *testing.T) {
    // Test: lvt-empty shows when source is empty
}
```

#### Documentation Updates

- [ ] Add list examples to `docs/auto-rendering.md`

#### Example Updates

- [ ] `examples/auto-list/` - List rendering demo

#### Verification
```bash
go test ./... -v -run TestAutoList
cd examples/auto-list && tinkerdown serve
```

#### Definition of Done
- [ ] All E2E tests pass
- [ ] Example runs without errors
- [ ] Update **Current Sprint** section

---

### 1.3 Auto-Rendering Selects

#### Implementation Steps

**Step 1: Verify and document select auto-population**
- [ ] Test current `<select lvt-source>` implementation
- [ ] Ensure `lvt-value` and `lvt-label` work correctly
- [ ] Add `lvt-empty` for placeholder option ("Select...")

#### E2E Tests

```go
func TestAutoSelectBasic(t *testing.T) {
    // Test: <select lvt-source="x" lvt-value="id" lvt-label="name"> populates options
}

func TestAutoSelectPlaceholder(t *testing.T) {
    // Test: lvt-empty="Select..." adds placeholder option
}
```

#### Documentation Updates

- [ ] Document select auto-rendering in `docs/auto-rendering.md`

---

## Phase 2: Stability & Performance - Implementation Plan

### 2.1 Data Source Error Handling

#### Implementation Steps

**Step 1: Create unified error types (`internal/source/errors.go`)**
```go
type SourceError struct {
    Source    string
    Operation string
    Err       error
    Retryable bool
}

type ConnectionError struct { ... }
type TimeoutError struct { ... }
type ValidationError struct { ... }
```

**Step 2: Implement retry logic (`internal/source/retry.go`)**
- [ ] Exponential backoff: 100ms, 200ms, 400ms, max 3 retries
- [ ] Only retry on `Retryable` errors
- [ ] Log retry attempts

**Step 3: Implement circuit breaker (`internal/source/circuit.go`)**
- [ ] Open circuit after 5 failures in 1 minute
- [ ] Half-open after 30 seconds
- [ ] Close on successful request

**Step 4: Add timeout configuration**
```yaml
sources:
  slow_api:
    type: rest
    url: https://slow.api.com
    timeout: 30s  # Default: 10s
```

#### E2E Tests

**File:** `source_error_handling_e2e_test.go`

```go
func TestSourceRetryOnTransientError(t *testing.T) {
    // Test: Source retries on network error
}

func TestSourceCircuitBreaker(t *testing.T) {
    // Test: Circuit opens after repeated failures
}

func TestSourceTimeout(t *testing.T) {
    // Test: Custom timeout is respected
}

func TestErrorDisplayInTemplate(t *testing.T) {
    // Test: {{.Error}} renders user-friendly message
}
```

#### Documentation Updates

- [ ] `docs/error-handling.md` - Error handling guide
- [ ] Document timeout and retry configuration

#### Example Updates

- [ ] `examples/error-handling/` - Demo of error states and recovery

---

### 2.2 Source Caching Layer

#### Implementation Steps

**Step 1: Create cache interface (`internal/cache/cache.go`)**
```go
type Cache interface {
    Get(key string) ([]map[string]interface{}, bool)
    Set(key string, data []map[string]interface{}, ttl time.Duration)
    Invalidate(key string)
}

type MemoryCache struct { ... }
```

**Step 2: Add cache configuration parsing**
```yaml
sources:
  users:
    type: rest
    cache:
      ttl: 5m
      strategy: stale-while-revalidate
```

**Step 3: Integrate cache into source fetching**
- [ ] Check cache before fetch
- [ ] Store result after successful fetch
- [ ] Invalidate on write operations (Add, Update, Delete)

**Step 4: Implement stale-while-revalidate**
- [ ] Return stale data immediately
- [ ] Fetch fresh data in background
- [ ] Update cache when fresh data arrives

#### E2E Tests

**File:** `source_caching_e2e_test.go`

```go
func TestCacheHit(t *testing.T) {
    // Test: Second request uses cached data
}

func TestCacheTTL(t *testing.T) {
    // Test: Cache expires after TTL
}

func TestCacheInvalidationOnWrite(t *testing.T) {
    // Test: Add/Delete invalidates cache
}

func TestStaleWhileRevalidate(t *testing.T) {
    // Test: Returns stale data while fetching fresh
}
```

#### Documentation Updates

- [ ] `docs/caching.md` - Caching configuration and strategies
- [ ] Add performance tuning section

#### Example Updates

- [ ] Update REST example with caching enabled

---

### 2.3 Multi-Page WebSocket Support

#### Implementation Steps

**Step 1: Add page identifier to WebSocket connection**
- [ ] Accept `?page=/path/to/page` query parameter
- [ ] Route messages to correct page's GenericState
- [ ] Track active connections per page

**Step 2: Handle page transitions**
- [ ] Clean up state when client disconnects
- [ ] Support client-side page navigation without reload

**Step 3: Connection management**
- [ ] Limit connections per page (configurable)
- [ ] Clean up stale connections

#### E2E Tests

**File:** `multipage_websocket_e2e_test.go`

```go
func TestMultiPageWebSocket(t *testing.T) {
    // Test: Two pages have independent WebSocket connections
}

func TestPageTransition(t *testing.T) {
    // Test: Navigating between pages maintains correct state
}

func TestConnectionCleanup(t *testing.T) {
    // Test: Disconnected clients are cleaned up
}
```

#### Documentation Updates

- [ ] Update WebSocket documentation with multi-page support
- [ ] Add architecture diagram

#### Example Updates

- [ ] `examples/docs-site/` - Verify multi-page interactive examples work

---

## Phase 3: Developer Experience - Implementation Plan

### 3.1 Enhanced CLI Scaffolding

#### Implementation Steps

**Step 1: Create template definitions (`cmd/tinkerdown/templates/`)**
```
templates/
├── basic/
│   ├── index.md
│   └── tinkerdown.yaml
├── todo/
│   ├── index.md
│   ├── _data/tasks.md
│   └── tinkerdown.yaml
├── dashboard/
│   ├── index.md
│   └── tinkerdown.yaml
├── form/
│   └── ...
├── api-explorer/
│   └── ...
└── wasm-source/
    ├── source.go
    ├── Makefile
    └── README.md
```

**Step 2: Implement `--template` flag**
```go
// cmd/tinkerdown/commands/new.go
func newCmd() *cobra.Command {
    var template string
    cmd.Flags().StringVar(&template, "template", "basic", "Template: basic, todo, dashboard, form, api-explorer, wasm-source")
}
```

**Step 3: Template file generation**
- [ ] Copy template files to target directory
- [ ] Replace placeholders with project name
- [ ] Generate sample data files

#### E2E Tests

**File:** `cli_scaffolding_e2e_test.go`

```go
func TestNewBasicTemplate(t *testing.T) {
    // Test: tinkerdown new myapp creates basic structure
}

func TestNewTodoTemplate(t *testing.T) {
    // Test: tinkerdown new myapp --template=todo creates todo app
}

func TestNewWasmSourceTemplate(t *testing.T) {
    // Test: tinkerdown new mysource --template=wasm-source creates WASM scaffolding
}

func TestTemplateRuns(t *testing.T) {
    // Test: Generated app starts without errors
}
```

#### Documentation Updates

- [ ] `docs/cli.md` - Document all CLI commands and flags
- [ ] `docs/templates.md` - Template reference
- [ ] Update getting started with template examples

#### Example Updates

- [ ] Templates become canonical examples
- [ ] Link from docs to template source

---

### 3.2 Expanded Validation

#### Implementation Steps

**Step 1: Add source reference validation**
```go
// cmd/tinkerdown/commands/validate.go
func validateSourceReferences(page *tinkerdown.Page, config *config.Config) []ValidationError {
    // Check each lvt-source="name" has matching config entry
}
```

**Step 2: Add attribute validation**
- [ ] Validate `lvt-*` attribute values are well-formed
- [ ] Check source types are valid
- [ ] Warn on unused sources

**Step 3: Add WASM path validation**
- [ ] Check WASM module files exist
- [ ] Validate WASM interface (has required exports)

#### E2E Tests

**File:** `validation_e2e_test.go`

```go
func TestValidateMissingSource(t *testing.T) {
    // Test: Error when lvt-source references undefined source
}

func TestValidateInvalidSourceType(t *testing.T) {
    // Test: Error on invalid source type
}

func TestValidateUnusedSource(t *testing.T) {
    // Test: Warning on defined but unused source
}

func TestValidateMissingWasmFile(t *testing.T) {
    // Test: Error when WASM file doesn't exist
}
```

#### Documentation Updates

- [ ] `docs/validation.md` - Validation rules reference
- [ ] Add validation to CI/CD guide

---

### 3.3 Debug Mode & Logging

#### Implementation Steps

**Step 1: Add CLI flags**
```go
// cmd/tinkerdown/commands/serve.go
var debug bool
var verbose bool
cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
cmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")
```

**Step 2: Implement structured logging**
```go
// internal/server/logging.go
type Logger struct {
    level LogLevel
    json  bool
}

func (l *Logger) Info(msg string, fields ...Field) { ... }
func (l *Logger) Debug(msg string, fields ...Field) { ... }
```

**Step 3: Add request correlation IDs**
- [ ] Generate unique ID per request
- [ ] Include in all log messages for request
- [ ] Pass to WebSocket messages

#### E2E Tests

```go
func TestDebugFlag(t *testing.T) {
    // Test: --debug enables debug output
}

func TestVerboseLogging(t *testing.T) {
    // Test: --verbose shows detailed logs
}
```

#### Documentation Updates

- [ ] `docs/debugging.md` - Debugging guide
- [ ] Add troubleshooting section

---

### 3.4 Hot Reload for Configuration

#### Implementation Steps

**Step 1: Watch config files**
- [ ] Use fsnotify to watch `tinkerdown.yaml`
- [ ] Debounce rapid changes (100ms)

**Step 2: Reload sources**
- [ ] Parse new config
- [ ] Recreate affected sources
- [ ] Keep WebSocket connections alive

**Step 3: Notify clients**
- [ ] Send reload message via WebSocket
- [ ] Client triggers re-fetch of data

#### E2E Tests

```go
func TestConfigHotReload(t *testing.T) {
    // Test: Changing config reloads sources without restart
}

func TestConfigReloadKeepsConnections(t *testing.T) {
    // Test: WebSocket stays connected during reload
}
```

---

### 3.5 WASM Source Development Kit

#### Implementation Steps

**Step 1: Implement `tinkerdown wasm init`**
```bash
tinkerdown wasm init github-issues
# Creates:
# - source.go (TinyGo template)
# - Makefile
# - README.md
# - test/
```

**Step 2: Implement `tinkerdown wasm build`**
```bash
tinkerdown wasm build
# Runs: tinygo build -o source.wasm -target=wasi source.go
```

**Step 3: Implement `tinkerdown wasm test`**
```bash
tinkerdown wasm test
# Loads WASM, calls fetch(), validates output
```

#### E2E Tests

```go
func TestWasmInit(t *testing.T) {
    // Test: wasm init creates correct structure
}

func TestWasmBuild(t *testing.T) {
    // Test: wasm build produces valid WASM
}

func TestWasmTest(t *testing.T) {
    // Test: wasm test validates module
}
```

#### Documentation Updates

- [ ] `docs/wasm-sources.md` - Complete WASM source authoring guide
- [ ] `docs/wasm-interface.md` - WASM interface specification
- [ ] `docs/wasm-examples.md` - Example sources walkthrough

#### Example Updates

- [ ] `examples/wasm-github-source/` - GitHub API WASM source
- [ ] `examples/wasm-rest-source/` - REST API WASM source

---

## Phase 4: Data Ecosystem - Implementation Plan

### 4.1 GraphQL Source

#### Implementation Steps

**Step 1: Create GraphQL source (`internal/source/graphql.go`)**
```go
type GraphQLSource struct {
    name     string
    url      string
    query    string
    variables map[string]interface{}
    headers   map[string]string
}

func (s *GraphQLSource) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    // Execute GraphQL query, flatten response
}
```

**Step 2: Add configuration parsing**
```yaml
sources:
  issues:
    type: graphql
    url: https://api.github.com/graphql
    query: |
      query($owner: String!, $repo: String!) {
        repository(owner: $owner, name: $repo) {
          issues(first: 10) {
            nodes { title, state, createdAt }
          }
        }
      }
    variables:
      owner: "livetemplate"
      repo: "tinkerdown"
    headers:
      Authorization: "Bearer ${GITHUB_TOKEN}"
```

**Step 3: Response flattening**
- [ ] Extract array from nested response path
- [ ] Handle pagination (cursor-based)

#### E2E Tests

```go
func TestGraphQLSource(t *testing.T) {
    // Test with mock GraphQL server
}

func TestGraphQLVariables(t *testing.T) {
    // Test variable substitution
}

func TestGraphQLAuth(t *testing.T) {
    // Test header authentication
}
```

#### Documentation Updates

- [ ] `docs/sources/graphql.md` - GraphQL source reference

#### Example Updates

- [ ] `examples/lvt-source-graphql-test/` - GraphQL source demo

---

### 4.2 MongoDB Source

#### Implementation Steps

**Step 1: Create MongoDB source (`internal/source/mongodb.go`)**
- [ ] Use official Go driver (go.mongodb.org/mongo-driver)
- [ ] Support find, insert, update, delete operations

**Step 2: Configuration**
```yaml
sources:
  products:
    type: mongodb
    uri: ${MONGODB_URI}
    database: myapp
    collection: products
    filter: { "status": "active" }
```

#### E2E Tests

```go
func TestMongoDBSource(t *testing.T) {
    // Test with embedded MongoDB or container
}

func TestMongoDBWrite(t *testing.T) {
    // Test CRUD operations
}
```

---

### 4.3 Source Composition

#### Implementation Steps

**Step 1: Create computed source (`internal/source/computed.go`)**
```go
type ComputedSource struct {
    name   string
    from   string          // Source to compute from
    filter string          // Filter expression
    sort   string          // Sort expression
    registry *SourceRegistry
}
```

**Step 2: Expression parser**
- [ ] Parse filter expressions: `status == 'active'`
- [ ] Parse sort expressions: `name asc, createdAt desc`
- [ ] Support basic operators: `==`, `!=`, `>`, `<`, `contains`

**Step 3: Aggregation**
```yaml
sources:
  stats:
    type: computed
    from: orders
    aggregate:
      total: sum(amount)
      count: count()
      avg: avg(amount)
```

#### E2E Tests

```go
func TestComputedFilter(t *testing.T) {
    // Test: filter expression works
}

func TestComputedSort(t *testing.T) {
    // Test: sort expression works
}

func TestComputedAggregate(t *testing.T) {
    // Test: aggregation functions work
}
```

---

## Phase 5: Production Readiness - Implementation Plan

### 5.1 Authentication Middleware

#### Implementation Steps

**Step 1: Create auth middleware (`internal/server/auth.go`)**
```go
type AuthMiddleware struct {
    providers map[string]AuthProvider
}

type AuthProvider interface {
    Authenticate(r *http.Request) (*User, error)
}

// Implementations:
type APIKeyAuth struct { ... }
type BasicAuth struct { ... }
type OAuth2Auth struct { ... }
type JWTAuth struct { ... }
```

**Step 2: Frontmatter configuration**
```yaml
auth: required
# or
auth:
  provider: github
  allowed_orgs: [mycompany]
  allowed_users: [admin]
```

**Step 3: User context in templates**
- [ ] Add `User` to template data: `{{.User.Email}}`
- [ ] Support user-specific data filtering

#### E2E Tests

```go
func TestAPIKeyAuth(t *testing.T) {
    // Test: API key authentication works
}

func TestOAuth2Flow(t *testing.T) {
    // Test: OAuth2 login flow
}

func TestUnauthorizedRedirect(t *testing.T) {
    // Test: Unauthenticated users redirected to login
}

func TestUserContextInTemplate(t *testing.T) {
    // Test: {{.User.Email}} renders correctly
}
```

#### Documentation Updates

- [ ] `docs/authentication.md` - Auth configuration guide
- [ ] `docs/security.md` - Security best practices

#### Example Updates

- [ ] `examples/authenticated-app/` - App with login

---

### 5.2 Health & Metrics Endpoints

#### Implementation Steps

**Step 1: Add health endpoints**
```go
// GET /health - Liveness
// GET /ready - Readiness (includes source connectivity)
// GET /metrics - Prometheus format
```

**Step 2: Implement metrics collection**
- [ ] Request count/latency by route
- [ ] WebSocket connection count
- [ ] Source fetch latency/error rates
- [ ] WASM execution time

#### E2E Tests

```go
func TestHealthEndpoint(t *testing.T) {
    // Test: /health returns 200
}

func TestReadyEndpoint(t *testing.T) {
    // Test: /ready checks source connectivity
}

func TestMetricsEndpoint(t *testing.T) {
    // Test: /metrics returns Prometheus format
}
```

---

### 5.3 Single Binary Distribution

#### Implementation Steps

**Step 1: Embed client assets**
```go
//go:embed assets/*
var assets embed.FS
```

**Step 2: Implement `tinkerdown build` command**
```bash
tinkerdown build ./myapp -o myapp-server
# Produces standalone binary with embedded markdown and assets
```

**Step 3: Cross-compilation**
```bash
tinkerdown build ./myapp --platform=linux/amd64,darwin/arm64,windows/amd64
```

#### E2E Tests

```go
func TestBuildCommand(t *testing.T) {
    // Test: build produces working binary
}

func TestBuiltBinaryRuns(t *testing.T) {
    // Test: Built binary serves app correctly
}
```

---

### 5.4 Security Hardening

#### Implementation Steps

**Step 1: CSRF Protection (`internal/server/csrf.go`)**
```go
type CSRFMiddleware struct {
    tokenStore TokenStore
}

func (m *CSRFMiddleware) GenerateToken(sessionID string) string {
    // Generate secure random token
    // Store in session-scoped map
}

func (m *CSRFMiddleware) ValidateToken(r *http.Request) bool {
    // Check X-CSRF-Token header or form field
    // Compare with stored token for session
}
```

- [ ] Generate CSRF tokens per session
- [ ] Auto-inject token into forms via template function
- [ ] Validate on all state-changing requests
- [ ] WebSocket: Use double-submit cookie pattern

**Step 2: Content Security Policy (`internal/server/security.go`)**
```go
func CSPMiddleware(config CSPConfig) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Security-Policy", config.String())
            next.ServeHTTP(w, r)
        })
    }
}
```

- [ ] Default restrictive CSP for new projects
- [ ] Configuration via `tinkerdown.yaml`
- [ ] Nonce generation for inline scripts (optional)

**Step 3: Input Sanitization**
- [ ] Validate source configuration at startup
- [ ] Path traversal prevention in `exec` source
- [ ] URL validation in `rest` source
- [ ] Ensure SQL parameterization (verify existing implementation)

**Step 4: WASM Resource Limits (`internal/source/wasm.go`)**
```go
type WASMConfig struct {
    MemoryLimit int64         // Default: 64MB
    Timeout     time.Duration // Default: 30s
}

func (s *WASMSource) executeWithLimits(ctx context.Context) ([]byte, error) {
    ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
    defer cancel()

    // Set memory limit via wazero configuration
    // Monitor execution, cancel if exceeded
}
```

- [ ] Parse limits from source configuration
- [ ] Integrate with wazero runtime limits
- [ ] Log resource usage for debugging
- [ ] Graceful error on limit exceeded

#### E2E Tests

```go
func TestCSRFProtection(t *testing.T) {
    // Test: POST without token returns 403
}

func TestCSRFTokenAutoInjection(t *testing.T) {
    // Test: Forms automatically include CSRF token
}

func TestCSPHeaders(t *testing.T) {
    // Test: Response includes correct CSP headers
}

func TestPathTraversalPrevention(t *testing.T) {
    // Test: exec source rejects ../../../etc/passwd
}

func TestWASMMemoryLimit(t *testing.T) {
    // Test: WASM module terminated when exceeding memory
}

func TestWASMTimeout(t *testing.T) {
    // Test: WASM execution cancelled after timeout
}
```

#### Documentation Updates

- [ ] `docs/security.md` - Comprehensive security guide
- [ ] Add security checklist to deployment guide
- [ ] Document WASM sandboxing model

#### Example Updates

- [ ] `examples/secure-app/` - Demo with all security features enabled

---

## Phase 6: UI & Components - Implementation Plan

### 6.1 Component Library

#### Implementation Steps

**Step 1: Chart component**
- [ ] Integrate Chart.js or lightweight alternative
- [ ] `<chart type="line" lvt-source="metrics" x="date" y="value">`
- [ ] Support line, bar, pie charts

**Step 2: Modal component**
- [ ] `lvt-modal-open="modal-id"` and `lvt-modal-close="modal-id"`
- [ ] Keyboard support (Escape to close)
- [ ] Focus trapping

**Step 3: Toast notifications**
- [ ] Auto-show on action success/error
- [ ] `lvt-toast-on:success="Item saved!"`

**Step 4: File upload**
- [ ] Drag-and-drop zone
- [ ] Progress indicator
- [ ] Integration with sources

#### E2E Tests

```go
func TestChartComponent(t *testing.T) {
    // Test: Chart renders from source data
}

func TestModalComponent(t *testing.T) {
    // Test: Modal opens/closes correctly
}

func TestToastNotification(t *testing.T) {
    // Test: Toast appears on action
}

func TestFileUpload(t *testing.T) {
    // Test: File upload works
}
```

---

### 6.2 Built-in Pagination

#### Implementation Steps

**Step 1: Add pagination attribute**
```html
<div lvt-source="users" lvt-paginate="20">
  <!-- Auto-generates prev/next controls -->
</div>
```

**Step 2: Server-side pagination**
- [ ] Add `page` and `per_page` to source queries
- [ ] Track current page in state
- [ ] Generate pagination controls

#### E2E Tests

```go
func TestPagination(t *testing.T) {
    // Test: Pagination controls work
}

func TestPaginationStatePreserved(t *testing.T) {
    // Test: Page number survives refresh
}
```

---

## Phase 7: Advanced Features - Implementation Plan

### 7.1 Multi-User State Broadcasting

#### Implementation Steps

**Step 1: Add broadcast configuration**
```yaml
sources:
  tasks:
    type: sqlite
    broadcast: true
```

**Step 2: Implement broadcasting**
- [ ] Track all WebSocket connections per source
- [ ] On state change, broadcast to all connections
- [ ] Exclude sender from broadcast

**Step 3: Conflict resolution**
- [ ] Last-write-wins (default)
- [ ] Optional merge strategy

#### E2E Tests

```go
func TestBroadcastToOtherClients(t *testing.T) {
    // Test: Change in one client appears in another
}

func TestConflictResolution(t *testing.T) {
    // Test: Concurrent edits handled correctly
}
```

---

### 7.2 Scheduled Tasks

#### Implementation Steps

**Step 1: Add scheduler (`internal/scheduler/scheduler.go`)**
```go
type Scheduler struct {
    jobs []Job
}

type Job struct {
    Cron   string
    Source string
    Action string
}
```

**Step 2: Configuration**
```yaml
schedules:
  refresh_data:
    cron: "*/5 * * * *"
    source: external_api
    action: Refresh
```

**Step 3: Background execution**
- [ ] Run jobs without active WebSocket connection
- [ ] Log execution results
- [ ] Webhook notification on error

#### E2E Tests

```go
func TestScheduledTask(t *testing.T) {
    // Test: Task runs at scheduled time
}

func TestScheduledTaskError(t *testing.T) {
    // Test: Error notification works
}
```

---

### 7.3 API Endpoint Mode

#### Implementation Steps

**Step 1: Add API configuration**
```yaml
api:
  enabled: true
  prefix: /api/v1
  sources:
    - name: tasks
      methods: [GET, POST, PUT, DELETE]
```

**Step 2: Generate REST endpoints**
- [ ] GET /api/v1/tasks - List
- [ ] POST /api/v1/tasks - Create
- [ ] PUT /api/v1/tasks/:id - Update
- [ ] DELETE /api/v1/tasks/:id - Delete

**Step 3: OpenAPI spec generation**
- [ ] `tinkerdown api spec` generates openapi.yaml
- [ ] Swagger UI endpoint

#### E2E Tests

```go
func TestAPIEndpoints(t *testing.T) {
    // Test: REST endpoints work
}

func TestOpenAPISpec(t *testing.T) {
    // Test: Spec generation works
}
```

---

## Phase 8: Polish & Optimization - Implementation Plan

### 8.1 Bundle Size Optimization

#### Implementation Steps

- [ ] Lazy load Monaco editor only for WASM blocks
- [ ] Lazy load Chart.js only when charts present
- [ ] Tree-shake unused LiveTemplate features
- [ ] Target <200KB initial bundle

#### Metrics

```go
func TestBundleSize(t *testing.T) {
    // Test: Bundle is under 200KB
}
```

---

### 8.2 Accessibility Audit

#### Implementation Steps

- [ ] Run axe-core automated tests
- [ ] Manual screen reader testing
- [ ] Keyboard navigation testing
- [ ] Color contrast validation

#### E2E Tests

```go
func TestKeyboardNavigation(t *testing.T) {
    // Test: All interactive elements keyboard accessible
}

func TestARIAAttributes(t *testing.T) {
    // Test: Components have correct ARIA
}
```

---

### 8.3 Comprehensive Test Suite

#### Test Organization

```
tests/
├── unit/
│   ├── source/          # Source unit tests
│   ├── runtime/         # GenericState tests
│   └── parser/          # Parser tests
├── integration/
│   ├── websocket/       # WebSocket integration
│   └── sources/         # Source integration
├── e2e/
│   ├── chromedp/        # Browser E2E tests
│   └── api/             # API E2E tests
└── performance/
    ├── benchmark/       # Benchmarks
    └── regression/      # Performance regression
```

#### Coverage Targets

- [ ] Unit tests: 90% coverage
- [ ] Integration tests: All source types
- [ ] E2E tests: All examples run successfully
- [ ] Performance: No regressions >10%

---

## Testing Standards

### E2E Test Template

```go
func TestFeatureName(t *testing.T) {
    // Setup
    srv := server.New("examples/feature-test")
    go srv.Start(":0")
    defer srv.Shutdown()

    // Wait for server
    waitForServer(t, srv.Addr())

    // Run browser tests with chromedp
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()

    var result string
    err := chromedp.Run(ctx,
        chromedp.Navigate(srv.URL()),
        chromedp.WaitVisible(".expected-element"),
        chromedp.Text(".result", &result),
    )

    require.NoError(t, err)
    assert.Equal(t, "expected", result)
}
```

### Documentation Template

Each feature documentation should include:

1. **Overview** - What the feature does
2. **Quick Start** - Minimal working example
3. **Configuration** - All config options
4. **Examples** - Multiple use cases
5. **Troubleshooting** - Common issues

### Example Template

Each example should include:

1. `index.md` - Main page with interactive demo
2. `README.md` - Explanation of what the example demonstrates
3. `tinkerdown.yaml` - Configuration (if needed)
4. `_data/` - Sample data files (if needed)
5. Working E2E test in `*_e2e_test.go`

---

## 3.6 Documentation Cleanup - Implementation Plan

### Phase A: Cleanup Redundant Docs

#### Step 1: Audit and Merge PROGRESS.md
- [x] Review `PROGRESS.md` content
- [x] Migrate any uncaptured items to ROADMAP.md (none needed - already captured)
- [x] Delete `PROGRESS.md`

#### Step 2: Audit and Merge UX_IMPROVEMENTS.md
- [x] Review `UX_IMPROVEMENTS.md` content
- [x] Verify all items are in Phase 6 (UI & Components)
- [x] Delete `UX_IMPROVEMENTS.md`

#### Step 3: Audit docs/ folder
- [x] Review `docs/implementation-plan.md` - deleted (superseded by ROADMAP.md)
- [x] Review `docs/internal-tools-saas.md` - archived to `docs/archive/`
- [x] Review `docs/launch-page.md` - archived to `docs/archive/`
- [x] Review `docs/pmf-one-file-ai-builder.md` - archived to `docs/archive/`

### Phase B: Create Documentation Structure

#### Step 1: Create folder structure
```bash
mkdir -p docs/getting-started
mkdir -p docs/guides
mkdir -p docs/reference
mkdir -p docs/sources
# Keep docs/plans/ as-is
```

#### Step 2: Create placeholder files
```bash
# Getting Started
touch docs/getting-started/installation.md
touch docs/getting-started/quickstart.md
touch docs/getting-started/project-structure.md

# Guides
touch docs/guides/data-sources.md
touch docs/guides/auto-rendering.md
touch docs/guides/go-templates.md
touch docs/guides/styling.md
touch docs/guides/deployment.md
touch docs/guides/debugging.md

# Reference
touch docs/reference/cli.md
touch docs/reference/config.md
touch docs/reference/lvt-attributes.md
touch docs/reference/frontmatter.md

# Sources (one per source type)
touch docs/sources/sqlite.md
touch docs/sources/rest.md
touch docs/sources/exec.md
touch docs/sources/json.md
touch docs/sources/csv.md
touch docs/sources/markdown.md
touch docs/sources/wasm.md
```

### Phase C: Write Priority Documentation

#### Priority 1: Getting Started (enables new users)
1. **`docs/getting-started/quickstart.md`**
   - Install tinkerdown CLI
   - Create first app with `tinkerdown new`
   - Run with `tinkerdown serve`
   - Make first interactive change

2. **`docs/getting-started/installation.md`**
   - Prerequisites (Go 1.21+)
   - Installation methods (go install, binary, brew)
   - Verify installation

3. **`docs/getting-started/project-structure.md`**
   - File layout explanation
   - `tinkerdown.yaml` purpose
   - `_data/` folder convention
   - Frontmatter options

#### Priority 2: Core Guides (enables productive use)
1. **`docs/guides/auto-rendering.md`**
   - Tables with `lvt-source` + `lvt-columns`
   - Lists with `lvt-source` + `lvt-field`
   - Selects with `lvt-source` + `lvt-value` + `lvt-label`
   - When to use Go templates instead

2. **`docs/guides/data-sources.md`**
   - Overview of all source types
   - Quick examples for each
   - Links to detailed source docs

#### Priority 3: Reference (enables self-service)
1. **`docs/reference/lvt-attributes.md`**
   - Complete list of all `lvt-*` attributes
   - Which are in core vs. Tinkerdown-specific
   - Examples for each

2. **`docs/reference/config.md`**
   - Full `tinkerdown.yaml` schema
   - All source type configurations
   - Environment variable substitution

### Phase D: Ongoing Documentation Maintenance

#### Per-Feature Documentation Checklist
When implementing a new feature, update:
- [ ] Relevant guide in `docs/guides/`
- [ ] Reference documentation in `docs/reference/`
- [ ] Example in `examples/`
- [ ] Link from example to docs and vice versa

#### Documentation Review Triggers
- Before each release: review all docs for accuracy
- After breaking changes: update migration notes
- Quarterly: audit for outdated content
