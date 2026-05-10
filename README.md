# Tinkerdown

> **v0.2.x — early but tagged. Expect breaking changes between minors.**

**Markdown is great for writing. HTML is great for showing. Tinkerdown lets you write the first and serve the second — from one file.**

<p align="center">
  <img src="docs/assets/demo.svg" alt="Animated demo showing a markdown file with YAML frontmatter and lvt attributes being served by Tinkerdown into a live interactive task manager with table, status badges, and action buttons" width="960">
</p>

Tinkerdown turns a single markdown file — frontmatter for data sources, prose for content, declarative attributes for interactivity — into a live, themed, URL-routable web app. Built on [LiveTemplate](https://github.com/livetemplate/livetemplate).

## Why Tinkerdown?

- **One file, co-authorable.** The source stays human- and AI-editable markdown. The rich rendered page is a *result*, not the artifact you have to maintain.
- **Declarative, not freeform.** Interactivity comes from a fixed vocabulary of `lvt-*` attributes — predictable for LLMs, consistent across pages, no generic "another Claude page" aesthetic.
- **Live data, not a snapshot.** Bind to SQLite, Postgres, REST, JSON, CSV, shell commands, markdown, WASM, or computed sources. The rendered page reflects current state, not a frozen export.
- **Real URLs.** Every page is linkable, bookmarkable, deep-linkable. No in-memory SPA state hiding behind a single `/`.
- **Progressive complexity.** Standard markdown → declarative attributes → Go templates. Each step builds on the last without rewriting. See the [Progressive Complexity Guide](docs/guides/progressive-complexity.md).
- **Disposable-software friendly.** The admin panel for this sprint. The tracker for that hiring round. The dashboard for the incident retro. Things you'd never scaffold a React app for, but that earn their keep for days or weeks.

## Why not just ask Claude for an HTML file?

That works — until you want to edit it. Generated HTML drifts from the prompt that produced it; the next change is another full re-prompt instead of a five-second text edit. And the data is whatever was true when the file was generated.

Tinkerdown keeps the source you edit (markdown + frontmatter + a small attribute vocabulary) separate from the page you ship (themed HTML, live data, websocket reactivity). You — or your agent — change the markdown. Everything else updates.

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
| **Dashboard / status report** | Frontmatter pulls from Postgres or REST; tables, computed totals, and Mermaid diagrams render inline. ([examples/markdown-data-dashboard](examples/markdown-data-dashboard)) |
| **Interactive explainer** | Prose + live `lvt` widgets + show-source listings, so the doc *is* the working demo. ([examples/literate-counter](examples/literate-counter)) |
| **Triage / standup board** | Action buttons mutate a shared SQLite or Postgres source; every teammate's tab stays in sync over websockets. ([examples/standup-bot](examples/standup-bot), [examples/team-tasks](examples/team-tasks)) |
| **Throwaway admin panel** | Point at a table you already have. Edit/delete/add for free. ([examples/auto-table-sqlite](examples/auto-table-sqlite)) |

**Need more control?** Use HTML attributes for explicit binding:

```html
<table lvt-source="tasks" lvt-columns="title,status" lvt-datatable lvt-actions="Complete,Delete">
</table>
```

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

Tinkerdown's surface area is small on purpose: a fixed `lvt-*` attribute vocabulary, frontmatter that's a YAML schema, and a single file that contains the whole app. That gives an LLM very few ways to be wrong.

Describe what you want:

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

The framing in this README was sharpened by Simon Willison's [post on the unreasonable effectiveness of HTML][simonw] and the [Hacker News discussion][hn] around it. Tinkerdown is our attempt at a structured answer to the markdown-vs-HTML tension that thread mapped out.

[simonw]: https://simonwillison.net/2026/May/8/unreasonable-effectiveness-of-html/
[hn]: https://news.ycombinator.com/item?id=48071940
