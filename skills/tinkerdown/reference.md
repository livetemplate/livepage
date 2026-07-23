# Tinkerdown Reference

Complete API reference for building Tinkerdown applications.

## File Structure

A Tinkerdown app is a markdown file with:

```markdown
---
title: "App Title"
---

# Heading

Regular markdown content...

\`\`\`lvt
<!-- Interactive HTML with lvt-* attributes; the block binds a source with lvt-source -->
<div lvt-source="items">
    <form name="Add">
        <input type="text" name="title">
        <button type="submit">Add</button>
    </form>
</div>
\`\`\`
```

### Frontmatter Options

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Page title (appears in browser tab) |
| `type` | string | Page type: `page`, `tutorial` |
| `persist` | string | Storage type: `localstorage` (default: SQLite) |
| `sources` | object | Data sources for `lvt-source` (see below) |
| `styling` | object | Theme configuration (theme, primary_color, font) |
| `blocks` | object | Code block settings (auto_id, id_format, show_line_numbers) |
| `features` | object | Feature flags (hot_reload) |

### Frontmatter Sources (New!)

You can define data sources directly in the frontmatter, making `tinkerdown.yaml` optional for single-file apps:

```yaml
---
title: "My App"
sources:
  users:
    type: json
    file: users.json
  products:
    type: csv
    file: products.csv
  api_data:
    type: rest
    url: https://api.example.com/data
  db_users:
    type: pg
    query: "SELECT * FROM users"
  shell_data:
    type: exec
    cmd: ./get-data.sh
---
```

**Source Types:**
- `json` - JSON file (requires `file`)
- `csv` - CSV file (requires `file`)
- `rest` - REST API (requires `url`)
- `pg` - PostgreSQL query (requires `query`, needs `DATABASE_URL` env var)
- `exec` - Shell command (requires `cmd`)

Frontmatter sources take precedence over `tinkerdown.yaml` sources if both define the same name.

## lvt-* Attributes

### Form Handling

#### `name` (on form)

Handle form submission. A form invokes an action by its `name`; `Add` is the
built-in insert against the block's writable source. The form must sit inside a
block bound with `lvt-source`.

```html
<div lvt-source="contacts">
    <form name="Add">
        <input type="text" name="name" required>
        <input type="email" name="email" required>
        <button type="submit">Submit</button>
    </form>
</div>
```

On submit the form data is:
1. Parsed from the form fields
2. Inserted into the source (a writable `sqlite` source, declared in frontmatter)
3. Available in the template as `.Data`

**Supported field types:**
- `text`, `email`, `url`, `tel` → string
- `number`, `range` → integer
- `checkbox` → boolean
- `textarea` → string
- `date`, `datetime-local` → timestamp

### Click Actions

#### `name` (on button)

Trigger a server action on click.

```html
<button name="Delete" data-id="{{.Id}}">Delete</button>
<button name="Toggle" data-id="{{.Id}}">Toggle</button>
<button name="Refresh">Refresh Data</button>
```

`Add`, `Delete`, `Toggle`, `Update`, and `Refresh` are built-in operations every
writable source provides; any other `name` invokes a named action (in a governed
project it must be an approved action).

### Data Attributes

#### `data-*`

Pass data with click actions. The `*` becomes the parameter name.

```html
<!-- Single value -->
<button name="Delete" data-id="{{.Id}}">Delete</button>

<!-- Multiple values -->
<button name="Move"
        data-id="{{.Id}}"
        data-target="archive">
    Archive
</button>
```

Access in controller:
- `ctx.GetInt("id")` - Get integer value
- `ctx.GetString("target")` - Get string value

### Auto-Persistence

#### `lvt-persist` — removed

> **Removed.** `lvt-persist` no longer does anything (the auto-CRUD it once provided
> was replaced by the `lvt-source` state model). A block using it has no state
> reference and `tinkerdown validate` rejects it. Use [`lvt-source`](#lvt-source):
> declare a writable source, bind it on the block's container, and let the built-in
> `Add`/`Delete` actions insert and remove. That is the form pattern that validates
> and is used throughout the examples.

````markdown
---
sources:
  todos: { type: sqlite, db: ./todos.db, table: todos, readonly: false }
---

```lvt
<div lvt-source="todos">
    <form name="Add" lvt-el:reset:on:success>
        <input type="text" name="title" required>
        <button type="submit">Add Todo</button>
    </form>
    {{range .Data}}
    <li>{{.Title}} <button name="Delete" data-id="{{.Id}}">Delete</button></li>
    {{end}}
</div>
```
````

Rows come from `.Data`; `name="Add"`/`name="Delete"` are built-in operations on the
writable source.

### Data Sources

#### `lvt-source`

Connect to external data. Source must be defined in `tinkerdown.yaml`.

```html
<div lvt-source="users">
    <table>
        {{range .Data}}
        <tr>
            <td>{{.Name}}</td>
            <td>{{.Email}}</td>
        </tr>
        {{end}}
    </table>
</div>
```

**Smart Table Auto-Generation:**

```html
<!-- Auto-generates table headers and rows -->
<table lvt-source="users" lvt-columns="name:Name,email:Email">
</table>
```

**Smart Select Auto-Generation:**

```html
<!-- Auto-generates options -->
<select lvt-source="countries" lvt-value="code" lvt-label="name">
</select>
```

## Source Configuration (tinkerdown.yaml)

### Exec Source (Run Scripts)

Execute any script that outputs JSON.

```yaml
sources:
  users:
    type: exec
    cmd: ./get-users.sh
```

Script should output JSON array:
```json
[
  {"id": 1, "name": "Alice", "email": "alice@example.com"},
  {"id": 2, "name": "Bob", "email": "bob@example.com"}
]
```

### PostgreSQL Source

Query a PostgreSQL database.

```yaml
sources:
  users:
    type: pg
    query: SELECT id, name, email FROM users ORDER BY id
```

Requires `DATABASE_URL` environment variable.

### REST API Source

Fetch data from HTTP endpoints.

```yaml
sources:
  posts:
    type: rest
    url: https://jsonplaceholder.typicode.com/posts
```

With headers:
```yaml
sources:
  github_issues:
    type: rest
    url: https://api.github.com/repos/myorg/myrepo/issues
    headers:
      Authorization: Bearer ${GITHUB_TOKEN}
```

### JSON File Source

Load data from a JSON file.

```yaml
sources:
  config:
    type: json
    file: data/config.json
```

### CSV File Source

Load data from a CSV file.

```yaml
sources:
  sales:
    type: csv
    file: data/sales.csv
```

CSV is automatically parsed with headers as field names.

## Template Syntax

Tinkerdown uses Go templates.

### Variables

```html
{{.Title}}          <!-- Access field -->
{{.User.Name}}      <!-- Nested field -->
{{.Items}}          <!-- Array/slice -->
```

### Conditionals

```html
{{if .Items}}
    <ul>...</ul>
{{else}}
    <p>No items</p>
{{end}}

{{if eq .Status "active"}}Active{{end}}
{{if ne .Count 0}}Has items{{end}}
{{if gt .Count 5}}Many items{{end}}
```

### Loops

```html
{{range .Items}}
    <li>{{.Title}} by {{.Author}}</li>
{{end}}

<!-- With index -->
{{range $i, $item := .Items}}
    <li>{{$i}}: {{$item.Title}}</li>
{{end}}
```

### Built-in Functions

| Function | Example | Description |
|----------|---------|-------------|
| `eq` | `{{if eq .Status "done"}}` | Equal |
| `ne` | `{{if ne .Count 0}}` | Not equal |
| `lt` | `{{if lt .Count 10}}` | Less than |
| `gt` | `{{if gt .Count 5}}` | Greater than |
| `and` | `{{if and .Active .Visible}}` | Logical AND |
| `or` | `{{if or .Admin .Moderator}}` | Logical OR |
| `not` | `{{if not .Deleted}}` | Logical NOT |
| `len` | `{{len .Items}}` | Length of array/string |

## Styling

### Inline Styles

Include `<style>` in your `lvt` block:

```html
\`\`\`lvt
<div class="container">
    <h1>My App</h1>
</div>

<style>
.container {
    max-width: 600px;
    margin: 0 auto;
    padding: 20px;
}
</style>
\`\`\`
```

### Tailwind CSS

Tailwind CSS is included automatically. Use utility classes:

```html
<div class="max-w-xl mx-auto p-6">
    <h1 class="text-2xl font-bold text-gray-800">My App</h1>
    <button class="bg-blue-500 text-white px-4 py-2 rounded">
        Submit
    </button>
</div>
```

## Components

### Datatable

Use the `lvt-columns` attribute to define columns (omit it to auto-discover them):

```html
<table lvt-source="users" lvt-columns="name:Name,email:Email,role:Role">
</table>
```

This generates a **simple** table — plain `<thead>`/`<tbody>`, no sorting or pagination.

For sortable columns and pagination, opt in with `lvt-datatable`:

```html
<table lvt-source="users" lvt-columns="name:Name,email:Email,role:Role" lvt-datatable>
</table>
```

There is no filtering attribute. Filter at the source instead — e.g. a SQL `query` on a
sqlite/postgres source, or a `computed` source derived from another.

### Select Dropdown

Auto-generate options from a data source:

```html
<select lvt-source="countries" lvt-value="code" lvt-label="name">
</select>
```

| Attribute | Description |
|-----------|-------------|
| `lvt-source` | Name of data source |
| `lvt-value` | Field to use as option value |
| `lvt-label` | Field to use as option label |

## Partials

Break large files into reusable components:

```html
{{partial "_header.md"}}

<main>
    <!-- Page content -->
</main>

{{partial "_footer.md"}}
```

Create `_header.md`:
```markdown
---
---

\`\`\`lvt
<header class="bg-gray-800 text-white p-4">
    <h1>My App</h1>
    <nav>...</nav>
</header>
\`\`\`
```

## CLI Reference

### Serve

Run a Tinkerdown application:

```bash
tinkerdown serve myapp.md           # Serve single file
tinkerdown serve ./myapp/           # Serve directory
tinkerdown serve . --port 8080      # Custom port
```

### Build (Coming Soon)

Compile to standalone binary:

```bash
tinkerdown build myapp.md -o myapp
```

## Common Patterns

### CRUD List

```html
<div lvt-source="items">
    <form name="Add">
        <input type="text" name="title" required>
        <button type="submit">Add</button>
    </form>

    {{range .Data}}
    <div>
        {{.Title}}
        <button name="Delete" data-id="{{.Id}}">Delete</button>
    </div>
    {{end}}
</div>
```

### Data Table from API

```yaml
# tinkerdown.yaml
sources:
  users:
    type: rest
    url: https://api.example.com/users
```

```html
<table lvt-source="users" lvt-columns="name:Name,email:Email">
</table>
```

### Form with Validation

```html
<div lvt-source="contacts">
    <form name="Add">
        <input type="text" name="name" required minlength="2">
        <input type="email" name="email" required>
        <input type="tel" name="phone" pattern="[0-9]{10}">
        <button type="submit">Submit</button>
    </form>
</div>
```

HTML5 validation is built-in. Add `required`, `minlength`, `pattern`, etc.

### Conditional Rendering

```html
{{if .Error}}
    <div class="error">{{.Error}}</div>
{{end}}

{{if .Items}}
    <ul>{{range .Items}}<li>{{.Title}}</li>{{end}}</ul>
{{else}}
    <p class="empty">No items yet. Add one above!</p>
{{end}}
```
