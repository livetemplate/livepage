# Literate documentation with `show-source`

Tinkerdown's ` ```lvt ` blocks render templates as live, interactive
widgets. By default the *source* of the template is hidden — you only
see what it renders. For documentation pages that's often inverted from
what you want: readers learn by reading the code that produced what
they're looking at.

The `show-source` flag pairs the rendered widget with a
syntax-highlighted listing of the very same template, so a single
authored block becomes both the example *and* the explanation.

## Per-block opt-in

Add `show-source` to the fence info of any ` ```lvt ` block:

````markdown
```lvt interactive show-source
<ul lvt-source="tasks">
  {{range .Data}}<li>{{.Text}}</li>{{end}}
</ul>
```
````

Renders:

1. A standard code block with the template body, highlighted as
   `language-html`.
2. The live `tinkerdown-interactive-block` widget, exactly as if
   `show-source` were absent.

Both are wrapped in a `tinkerdown-lvt-demo` container so they read as
one demo unit.

## Page-level default

When most blocks on a page should show their source, set the default in
frontmatter and omit per-block flags:

```yaml
---
title: "Templating reference"
lvt_show_source: true
sources:
  tasks: { type: markdown, file: ./_data/tasks.md }
---
```

Individual blocks can still opt out with `hide-source`:

````markdown
```lvt interactive hide-source
<!-- This widget runs but its source isn't displayed -->
```
````

## Documenting Go controllers and external files: `include="..."`

`show-source` is for templates that live entirely inside the markdown
page. To document the *Go side* of a real LiveTemplate app — a state
struct, hand-written action handlers, business logic — author those
in regular `.go` files (the same files your deployed app uses) and
cite line ranges from them in markdown:

````markdown
The state — pure data, cloned per session:

```go include="./_app/counter.go" lines="5-8"
```

The handler that mutates it on action:

```go include="./_app/counter.go" lines="13-19"
```
````

Tinkerdown reads the file at render time and emits a normal
syntax-highlighted code block — same `<pre><code class="language-go">`
shape Prism already styles. The `include="..."` and `lines="..."`
attributes are stripped from the rendered fence info so the DOM
stays clean.

### Path resolution and confinement

Paths are resolved relative to the markdown file's directory, then
canonicalized (symlinks followed) and confined to that directory's
tree. Paths that escape (`../../etc/passwd`) are rejected with a
warning and the original empty fence passes through, so the page
still renders.

### Whole-file include

Omit `lines=` to include the entire file. Useful for short templates
or config snippets:

````markdown
```html include="./_app/counter.tmpl"
```
````

### Auto-dedent

The longest common leading whitespace across non-blank lines is
stripped, so a snippet pulled from inside a function body renders
flush-left without mangling its relative indentation.

### Hot reload

The file watcher tracks every included file. Edit `_app/counter.go`
while the page is open and the snippets refresh — handy for keeping
docs in sync with code as you iterate.

### Multiple ranges in one block

Comma-separate the `lines=` value to pull several ranges from one
file in a single block. Tinkerdown joins them with a
language-appropriate ellipsis comment (e.g. `// ...` for Go,
`# ...` for Python/YAML, `<!-- ... -->` for HTML), so the rendered
snippet visually signals the omitted middle.

````markdown
```go include="./_app/counter.go" lines="5-8,13-22"
```
````

### Named regions

For snippets that survive line-number drift, mark spans in the
source file with `>>> region:NAME` / `<<< region:NAME` markers
inside any single-line comment style. The `region="..."` fence
attribute extracts the lines strictly between matching markers
(the markers themselves are excluded from the rendered snippet).

In the source file:

```go
// >>> region:state
type Counter struct{ Count int }
// <<< region:state
```

In the markdown:

````markdown
```go include="./_app/counter.go" region="state"
```
````

`region=` and `lines=` are mutually exclusive — if both are set,
`region=` wins and `lines=` is dropped with a warning. The footer
link uses the region's resolved line range.

### Highlight specific lines

`highlight="N-M"` overlays a visual emphasis on the cited lines
within the snippet. Format mirrors Prism's `data-line` attribute:
single line (`"3"`), single range (`"3-5"`), or multiple
ranges/lines (`"3-5,8,12-14"`).

````markdown
```go include="./_app/counter.go" lines="13-22" highlight="20"
```
````

This works alongside `lines=` and `region=`. **Line numbers in
`highlight=` are file-absolute** — they match the gutter numbers
shown in the rendered snippet (which match the source file's line
numbers). For a `lines="13-22"` slice, `highlight="20"` emphasises
file line 20, which appears as line 20 in the rendered gutter.
Author with the editor open; copy line numbers directly.

### Source-link footer

Each `include=` block automatically renders a small footer link to
the cited file at the cited line range, like `counter.go:13-35`,
when the page declares its repo coordinates in frontmatter:

```yaml
---
source_repo: "https://github.com/yourorg/yourrepo"
source_path: "examples/literate-counter/index.md"
---
```

`source_repo` is the repo URL; `source_path` is the page's path
within the repo (used to derive the included files' repo-relative
locations). The link's git ref defaults to the running tinkerdown
binary's release version (set via build-time ldflags), so released
docs always link at the matching tag. For `dev` builds the ref
falls back to `main`. Override per-page with `source_ref:` in
frontmatter — useful when a page documents code from a specific
commit or tag.

If `source_repo` is unset the footer is silently omitted; includes
still work, just without the link.

## Pairing with `embed-lvt` for the literate flow

`include=` shows the source; [`embed-lvt`](../sources/embed-lvt.md)
shows the same code running. Together you get the literate
"read it, see it run" experience without writing any new runtime:
the deployed app is whatever you already have, and the docs page
just cites the real files.

````markdown
```go include="./_app/counter.go" lines="5-8"
```

```go include="./_app/counter.go" lines="13-19"
```

```html include="./_app/counter.tmpl"
```

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```
````

The reader sees four code blocks (state, handler, template, running
widget) — all sourced from real, deployable files. No drift between
"what the tutorial says" and "what production does."

## Complete examples

- [`examples/literate-counter/`](../../examples/literate-counter/index.md)
  — `show-source` on an inline `lvt` block bound to a markdown
  data source. No external files; the template lives entirely in
  the page.
- [`examples/literate-counter-include/`](../../examples/literate-counter-include/index.md)
  — `include=` with line ranges from a real `_app/counter.go` plus
  an `embed-lvt` block running it.
- [`examples/literate-linked-include/`](../../examples/literate-linked-include/index.md)
  — two `embed-lvt` blocks with shared `session=` value, sharing
  one upstream session across page regions.
