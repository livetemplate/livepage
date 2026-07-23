---
name: tinkerdown
description: Generate throwaway internal UIs as a single markdown file — live data, a fixed attribute vocabulary, no React and no build step.
triggers:
  - tinkerdown
  - internal tool
  - admin dashboard
  - throwaway UI
  - quick dashboard
  - single-file app
  - markdown app
  - no-build app
---

# Tinkerdown: Throwaway Internal UIs in Markdown

Generate a working internal UI as one markdown file: frontmatter declares the data
sources, a fixed set of `lvt-*` attributes declares the interactivity, and
`tinkerdown serve` makes it live. No React, no build step, no CSS classes.

The point is the cost. These are UIs cheap enough to generate for a single question
and delete afterwards — the view someone needs *this week* that would never survive a
sprint-planning conversation. Keep the file if the need recurs; otherwise throw it away.

## When to Use Tinkerdown

Use Tinkerdown for:
- **Internal tools** - Admin dashboards, data viewers, CRUD apps
- **One-off views** - "Show me X grouped by Y" against a table that already exists
- **Prototypes** - Quick interactive demos
- **Personal utilities** - Task managers, trackers, simple apps
- **Data displays** - Tables, forms, dashboards connected to databases/APIs

**Don't use Tinkerdown for:**
- Public-facing marketing sites (use static site generators)
- Apps requiring complex client-side state (use React/Vue)
- Real-time multiplayer games

## The generation workflow

Interactivity comes from a **fixed vocabulary**, not freeform HTML — that constraint is
what makes generated output reliable. Stay inside it (see `reference.md`) rather than
reaching for custom JavaScript.

Follow these steps in order. They exist because each one catches something the next
cannot.

### 1. Read the approved surface, if there is one

If the project has a `tinkerdown.yaml` with a `generation:` block, it declares the
sources and actions you may use — and those are the **only** ones you may use:

```bash
cat tinkerdown.yaml
```

Each approved source and action may carry a `describes:` note saying what it touches.
Use those names as given. **Do not declare your own `sources:` or `actions:` in the
document's frontmatter** — a name outside the approved set fails validation, and
redefining an approved name silently has no effect, because approved definitions are
pinned and yours will be ignored.

No `generation:` block means no approved surface: declare sources in frontmatter as
normal.

### 2. Write the document

**Put `lvt-*` markup inside a ```lvt fence.** Outside one it is ordinary HTML: the page
renders, nothing binds, and no error appears anywhere — not in the browser console, not
in the server log. It simply sits there looking correct. This is the easiest mistake to
make and the hardest to spot.

````markdown
```lvt
<table lvt-source="requests" lvt-columns="id,requester,status"></table>
<button name="approve" data-id="1">Approve</button>
```
````

Use only attributes from `reference.md`. If you need a capability you cannot find
there, check whether it exists before inventing an attribute for it — a plausible
invention is the most common way generated pages fail.

**Binding data and actions — three rules that cause most first-try failures:**

- **State comes from `lvt-source` on the block's container.** `{{range .Data}}` and
  `{{.Field}}` render the rows of the bound source. An interactive block (a form, a
  per-row button) that has no `lvt-source` above it fails validation with *"no state
  reference"* — put the form and its table inside one `<div lvt-source="…">` /
  `<article lvt-source="…">`, as in the Quick Start.
- **`name=` invokes an action.** In a **governed** project (one with a `generation:`
  block) the name on a button or form must be either a built-in operation
  (`Add`/`Delete`/`Toggle`/`Refresh`) or an **approved action name** — a made-up name
  like `name="approve"` fails the policy check unless the manifest approves it. A
  write-form's intake should invoke a governed action (e.g. `name="request-access"`),
  not the built-in `Add` against a writable source, when the manifest keeps status/
  approver fields server-controlled.
- **Confirm-gated buttons need `data-confirm`.** A manifest action's `confirm:` field
  is a *declaration*, not a runtime dialog — it produces no prompt on its own. To ask
  the operator to confirm before a privileged action, put `data-confirm="…"` on the
  button (`<button name="approve-export" data-id="{{.Id}}" data-confirm="Approve?">`).

### 3. Validate, and fix what it reports

```bash
tinkerdown validate app.md
```

This is the gate, and it now checks three separate things:

- **Syntax** — the document parses.
- **Vocabulary** — every `lvt-*` attribute is one something actually implements.
  `unknown attribute "lvt-sortable"` means you invented it; a hint names the
  replacement when there is one.
- **Policy** — every source and action is in the approved set, whether referenced or
  declared.
- **Placement** — no `lvt-*` markup sits outside a ```lvt fence, where it would be
  inert.

Fix and re-run until it passes. Each diagnostic carries a hint naming the approved
alternatives or the correct attribute, so work from those rather than guessing.

**Stop after about five rounds.** If it is still failing, the request likely needs a
capability the vocabulary does not have. Say so rather than continuing to substitute
attributes — a page that validates but does the wrong thing is worse than an honest
"this needs something Tinkerdown does not do."

### 4. Check what the app does, if it is privileged

```bash
tinkerdown validate --summary app-directory
```

Emits JSON on stdout. When `"privileged": true`, the app executes commands, writes
data, or reaches the network — **show the operator the `operations` list and get their
OK before serving.** Each entry carries the manifest's `describes:` note, which is what
makes the list reviewable.

When `privileged` is false the app only reads. Serve it without interrupting anyone; a
prompt on every generated page is a prompt nobody reads.

This command fails if the project config cannot be read. That is deliberate — a policy
gate that cannot see the policy must not report "nothing to review."

### 5. Serve it

```bash
tinkerdown serve <directory>
```

Write the document to a scratch directory and serve that. Ephemeral means nothing is
persisted and the directory is deleted afterwards — not that nothing touches disk. A
directory the operator can re-run and inspect is more useful than one they cannot, and
if the UI turns out to be worth keeping, it already is a file.

Exec sources and actions additionally require `--allow-exec`; that decision belongs to
whoever runs the server, not to the document.

## Quick Start

### 1. Create a markdown file

Create `myapp.md`. An interactive block gets its data from a **source** bound with
`lvt-source` on the block's container — that binding is what gives the block state
(`.Data`); a block with none fails validation with "no state reference". Declare the
source in frontmatter (or use an approved one), then bind it:

```markdown
---
title: "My App"
sources:
  items:
    type: sqlite
    db: ./items.db
    table: items
    readonly: false
---

# My App

\`\`\`lvt
<div lvt-source="items">
    <h2>Add Item</h2>
    <form name="Add" lvt-el:reset:on:success>
        <input type="text" name="title" required>
        <button type="submit">Add</button>
    </form>

    <h2>Items</h2>
    {{if .Data}}
    <ul>
        {{range .Data}}
        <li>
            {{.Title}}
            <button name="Delete" data-id="{{.Id}}">Delete</button>
        </li>
        {{end}}
    </ul>
    {{else}}
    <p>No items yet.</p>
    {{end}}
</div>
\`\`\`
```

The block's rows come from `.Data` (the source's rows), and `name=` on a button or
form invokes an action: `Add`/`Delete`/`Toggle`/`Refresh` are built-in operations
every writable source provides, and any other name invokes a **named action** (see
the next section for governed projects).

### 2. Run it

```bash
tinkerdown serve myapp.md
```

### 3. Open in browser

Navigate to `http://localhost:3000` - your app is running!

## Key Concepts

| Concept | What It Does |
|---------|--------------|
| `lvt-source` | Binds a block to a data source (SQLite, PostgreSQL, REST, CSV, JSON, exec) — the block's rows are `.Data` |
| `name` (on button/form) | Triggers an action: built-in `Add`/`Delete`/`Toggle`/`Refresh`, or an approved named action |
| `data-*` | Passes data with actions (e.g., `data-id="123"`) |
| `data-confirm` | Prompts before an action fires (a manifest `confirm:` is a declaration, not a dialog) |
| frontmatter sources | Define data sources in frontmatter - no `tinkerdown.yaml` needed! |

## Reference

See [reference.md](./reference.md) for complete API documentation:
- File structure and frontmatter
- All `lvt-*` attributes
- Source configuration (pg, rest, csv, json, exec)
- Template syntax (Go templates)
- Components (datatable, dropdown)

## Examples

See [examples/](./examples/) for complete working apps:
1. [Todo App](./examples/01-todo-app.md) - Basic CRUD with `lvt-source`
2. [Dashboard](./examples/02-dashboard.md) - Data display with `lvt-source`
3. [Contact Form](./examples/03-contact-form.md) - Form handling
4. [Blog](./examples/04-blog.md) - Multi-page with partials
5. [Inventory](./examples/05-inventory.md) - PostgreSQL integration
6. [Survey](./examples/06-survey.md) - Multi-step forms
7. [Booking](./examples/07-booking.md) - Date/time handling
8. [Expense Tracker](./examples/08-expense-tracker.md) - CSV import
9. [FAQ](./examples/09-faq.md) - Accordion component
10. [Status Page](./examples/10-status-page.md) - Real-time updates
