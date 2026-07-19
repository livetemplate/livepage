# Tinkerdown

> **v0.2.x — early but tagged. Expect breaking changes between minors.**

**Internal UIs cheap enough to throw away. Describe what you need, get a live app against your real data, delete it when you're done.**

<p align="center">
  <img src="docs/assets/demo.svg" alt="Animated demo showing a markdown file with YAML frontmatter and lvt attributes being served by Tinkerdown into a live interactive task manager with table, status badges, and action buttons" width="960">
</p>

Tinkerdown turns a single markdown file — frontmatter for data sources, prose for content, declarative attributes for interactivity — into a live, themed, URL-routable web app. Built on [LiveTemplate](https://github.com/livetemplate/livetemplate).

## The problem

Internal tooling gets built like a product: one bloated "UI for everyone," a permanent feature backlog, and never enough engineering capacity. Every PM, engineer, and TPM wants their own variation, and the cost per variation is high enough that most are simply never built.

That cost assumption is what changed. An LLM can write a working UI in seconds — but only if the target is a *small, constrained* vocabulary rather than an open-ended framework, and only if it's pointed at data that's already wired up. Tinkerdown is that target: a markdown file, a fixed attribute set, and live data sources. Cheap enough that the answer to "could we get a view for this?" stops being a backlog item.

The unit isn't an app you maintain. It's a UI you generate for one question, use, and delete — or keep, if it turns out you need it every week.

## Why Tinkerdown?

- **A vocabulary an LLM can hit reliably.** Interactivity comes from a fixed set of `lvt-*` attributes, not freeform HTML and JavaScript. Small enough to generate correctly on the first try, consistent across pages, and no generic LLM-output aesthetic.
- **Cheap to throw away.** The admin panel for this sprint. The tracker for that hiring round. The dashboard for the incident retro. A markdown file you delete when the question is answered — not an app anyone has to own.
- **Deterministic before it's live.** `tinkerdown validate` parses a generated file with the real parser and reports errors with line numbers and hints, so a generating agent can self-correct to a clean pass before anything is served.
- **Live data, not a snapshot.** Bind to SQLite, PostgreSQL, REST, GraphQL, JSON, CSV, shell commands, markdown, WASM, or computed sources. The rendered page reflects current state, not a frozen export.
- **One file, co-authorable.** The source stays human- and AI-editable markdown. The rich rendered page is a *result*, not the artifact you have to maintain — so the next change is a text edit, not another full re-prompt.
- **Real URLs.** Every page is linkable, bookmarkable, deep-linkable. No in-memory SPA state hiding behind a single `/`.
- **Git-native and self-hosted.** Plain text in a repo. Version history, search, collaboration, offline access, no subscriptions.
- **Progressive complexity.** Standard markdown → declarative attributes → Go templates. Each step builds on the last without rewriting. See the [Progressive Complexity Guide](docs/guides/progressive-complexity.md).

## Why not just ask Claude for an HTML file?

That works — until you want to edit it. Generated HTML is opaque to hand-editing; the next change means another full re-prompt instead of a five-second text edit. And the data is whatever was true when the file was generated.

Tinkerdown keeps the source you edit (markdown + frontmatter + a small attribute vocabulary) separate from the page you ship (themed HTML, live data, WebSocket reactivity). You — or your agent — change the markdown. Everything else updates.

## Quick Start

```bash
# Install
go install github.com/livetemplate/tinkerdown/cmd/tinkerdown@latest

# Create a new app
tinkerdown new myapp
cd myapp

# Run the app
tinkerdown serve
# Open http://localhost:8080
```

## What You Can Build

Write a markdown file with a YAML source definition and a standard markdown table. Tinkerdown infers that the "Tasks" heading matches the "tasks" source and auto-generates an interactive table with add, edit, and delete:

```markdown
---
title: Task Manager
sources:
  tasks:
    type: sqlite
    db: ./tasks.db
    table: tasks
    readonly: false
---

# Task Manager

## Tasks
| Title | Status | Due Date |
|-------|--------|----------|
```

Run `tinkerdown serve` and get a fully interactive app with database persistence — no HTML needed:

<p align="center">
  <img src="docs/assets/auto-table-demo.png" alt="Screenshot showing an auto-generated expense tracker with data table, edit/delete buttons per row, and an add form — all from a markdown table and YAML source definition" width="720">
</p>

The same primitive scales to other artifacts:

| Artifact | What it looks like |
|---|---|
| **Dashboard / status report** | Frontmatter pulls from PostgreSQL or REST; tables, computed totals, and Mermaid diagrams render inline. ([examples/markdown-data-dashboard](examples/markdown-data-dashboard)) |
| **Literate doc / runnable explainer** | Prose alongside live widgets, real source cited by line range, and your deployed app embedded inline — the doc *is* the working code. ([Literate Authoring guide](docs/guides/literate-docs.md), [examples/literate-counter-include](examples/literate-counter-include)) |
| **Triage / standup board** | Action buttons mutate a shared SQLite or PostgreSQL source; every teammate's tab stays in sync over WebSocket. ([examples/standup-bot](examples/standup-bot), [examples/team-tasks](examples/team-tasks)) |
| **Throwaway admin panel** | Point at a table you already have. Edit/delete/add for free. ([examples/auto-table-sqlite](examples/auto-table-sqlite)) |

**Literate authoring.** When the page itself is a tutorial, the same markdown can show real source *and* run it. The snippet below shows two of the three primitives: `include=` cites line ranges from your `.go` / `.tmpl` source files, and `embed-lvt` drops the running app inline. The third — `show-source` — pairs an inline ` ```lvt ` block with its highlighted template. No copy-pasted snippets to drift out of date.

`````markdown
```go include="./_app/counter.go" lines="5-8"
```

```go include="./_app/counter.go" lines="13-35" highlight="20"
```

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```
`````

A literate doc in the wild: **[livetemplate.fly.dev/recipes/counter](https://livetemplate.fly.dev/recipes/counter/)** — prose, source listings, and a live counter on one page, all from a single markdown file.

See the [Literate Authoring guide](docs/guides/literate-docs.md) for the full primitive set (line ranges, named regions, line highlights, source-link footers) and [examples/literate-counter-include](examples/literate-counter-include) for the canonical pattern.

**Need more control?** Tinkerdown has five complexity tiers. Each builds on the previous — nothing rewrites; start at the lowest tier that works:

- **Tier 0 — pure markdown.** Standard checklists become interactive; toggles and adds persist to the file.
  ```markdown
  - [ ] Buy milk
  - [x] Email Sarah
  ```
- **Tier 1 — markdown + YAML sources.** Frontmatter declares data; markdown tables auto-bind. *(Shown above.)*
- **Tier 2 — `lvt-*` attributes.** Explicit HTML binding when auto-rendering isn't enough — custom forms, confirmation dialogs, datatables, cross-source selects.
  ```html
  <table lvt-source="tasks" lvt-columns="title,status" lvt-datatable lvt-actions="Complete,Delete">
  </table>
  ```
- **Tier 3 — Go templates.** Conditionals, loops, and custom layouts inside ` ```lvt ` blocks; full access to `.Data`, `.Error`, `.Errors`.
  ````markdown
  ```lvt
  {{range .Data}}
  <div class="card"><h3>{{.Title}}</h3></div>
  {{end}}
  ```
  ````
- **Tier 4 — WASM sources.** Write a custom data source in TinyGo when built-ins don't fit; the module exports a `fetch` function and runs server-side.

See the [Progressive Complexity Guide](docs/guides/progressive-complexity.md) for full examples and the escape hatches between tiers.

## Key Features

- **Single-file apps**: Everything in one markdown file with frontmatter
- **9 data sources**: SQLite, JSON, CSV, REST APIs, PostgreSQL, exec scripts, markdown, WASM, computed
- **Auto-rendering**: Tables, selects, and lists generated from data
- **Real-time updates**: WebSocket-powered reactivity
- **Zero config**: `tinkerdown serve` just works
- **Hot reload**: Changes reflect immediately

## Data Sources

Define sources in your page's frontmatter:

```yaml
---
sources:
  tasks:
    type: sqlite
    path: ./tasks.db
    query: SELECT * FROM tasks

  users:
    type: rest
    from: https://api.example.com/users

  config:
    type: json
    path: ./_data/config.json
---
```

| Type | Description | Example |
|------|-------------|---------|
| `sqlite` | SQLite databases | [lvt-source-sqlite-test](examples/lvt-source-sqlite-test) |
| `json` | JSON files | [lvt-source-file-test](examples/lvt-source-file-test) |
| `csv` | CSV files | [lvt-source-file-test](examples/lvt-source-file-test) |
| `rest` | REST APIs | [lvt-source-rest-test](examples/lvt-source-rest-test) |
| `pg` | PostgreSQL | [lvt-source-pg-test](examples/lvt-source-pg-test) |
| `exec` | Shell commands | [lvt-source-exec-test](examples/lvt-source-exec-test) |
| `markdown` | Markdown files | [markdown-data-todo](examples/markdown-data-todo) |
| `wasm` | WASM modules | [lvt-source-wasm-test](examples/lvt-source-wasm-test) |
| `computed` | Derived/aggregated data | [computed-source](examples/computed-source) |

## Auto-Rendering

Generate HTML automatically from data sources:

```html
<!-- Table with actions -->
<table lvt-source="tasks" lvt-columns="title,status" lvt-actions="Edit,Delete">
</table>

<!-- Select dropdown -->
<select lvt-source="categories" lvt-value="id" lvt-label="name">
</select>

<!-- List -->
<ul lvt-source="items" lvt-field="name">
</ul>
```

See [Auto-Rendering Guide](docs/guides/auto-rendering.md) for full details.

## Interactive Attributes

| Attribute | Description |
|-----------|-------------|
| `lvt-source` | Connect element to a data source |
| `name` (on button) | Handle click events |
| `name` (on form) | Handle form submissions |
| `lvt-on:change` | Handle input changes |
| `data-confirm` | Show confirmation dialog before action |
| `data-*` | Pass data with actions |

See [lvt-* Attributes Reference](docs/reference/lvt-attributes.md) for the complete list.

## Configuration

**Recommended:** Configure in frontmatter (single-file apps):

```markdown
---
title: My App
sources:
  tasks:
    type: sqlite
    path: ./tasks.db
    query: SELECT * FROM tasks
styling:
  theme: clean
---
```

**For complex apps:** Use `tinkerdown.yaml` for shared configuration:

```yaml
# tinkerdown.yaml - for multi-page apps with shared sources
server:
  port: 3000
sources:
  shared_data:
    type: rest
    from: ${API_URL}
    cache:
      ttl: 5m
```

See [Configuration Reference](docs/reference/config.md) for when to use each approach.

## AI-Assisted Development

Tinkerdown's surface area is small on purpose: a fixed `lvt-*` attribute vocabulary, frontmatter with a small set of well-defined fields, and a single file that contains the whole app. That gives an LLM very few ways to be wrong.

Describe what you want, and the agent drafts the markdown:

```
Create a task manager with SQLite storage,
a table showing tasks with title/status/due date,
a form to add tasks, and delete buttons on each row.
```

The output is a `.md` file you can read, diff, and hand-edit. No component tree, no build config, no `node_modules` to reason about.

See [AI Generation Guide](docs/guides/ai-generation.md) for tips on Claude Code, Cursor, and other agents.

## Documentation

**Getting Started:**
- [Installation](docs/getting-started/installation.md)
- [Quickstart](docs/getting-started/quickstart.md)
- [Project Structure](docs/getting-started/project-structure.md)

**Guides:**
- [Progressive Complexity](docs/guides/progressive-complexity.md)
- [Data Sources](docs/guides/data-sources.md)
- [Auto-Rendering](docs/guides/auto-rendering.md)
- [Go Templates](docs/guides/go-templates.md)
- [AI Generation](docs/guides/ai-generation.md)
- [Literate Authoring](docs/guides/literate-docs.md)

**Reference:**
- [CLI Commands](docs/reference/cli.md)
- [Frontmatter Options](docs/reference/frontmatter.md)
- [Configuration (tinkerdown.yaml)](docs/reference/config.md)
- [lvt-* Attributes](docs/reference/lvt-attributes.md)

**Planning:**
- [Roadmap](ROADMAP.md)

## Development

```bash
git clone https://github.com/livetemplate/tinkerdown.git
cd tinkerdown
go mod download
go test ./...
go build -o tinkerdown ./cmd/tinkerdown
```

## License

MIT

## Contributing

Contributions welcome! See [ROADMAP.md](ROADMAP.md) for planned features and current priorities.

## Acknowledgements

The framing in this README was sharpened by Thariq Shihipar's [*The Unreasonable Effectiveness of HTML*][source] and the [Hacker News discussion][hn] around it. Tinkerdown is our attempt at a structured answer to the markdown-vs-HTML tension that piece mapped out.

[source]: https://thariqs.github.io/html-effectiveness/
[hn]: https://news.ycombinator.com/item?id=48071940
