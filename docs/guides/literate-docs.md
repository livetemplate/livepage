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

## What `show-source` is not for

`show-source` displays templates. If your goal is to demonstrate a
*custom Go controller* — a state struct, hand-written action handlers,
business logic — that lives in a separate LiveTemplate app. Author the
app as a Go program, deploy it (locally for dev or alongside your docs
in production), and use the [`embed-lvt`](../sources/embed-lvt.md)
block to weave its live UI into your docs page.

The two flags compose cleanly:

- `show-source` is for documenting the template *layer*.
- `embed-lvt` is for documenting the *runtime*.

## A complete example

See [`examples/literate-counter/`](../../examples/literate-counter/index.md)
for a runnable page that uses `show-source` to document a simple todo
template bound to a markdown data source.
