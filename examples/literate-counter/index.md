---
title: "Literate docs: show the template, run it live"
sources:
  tasks:
    type: markdown
    file: ./_data/tasks.md
    anchor: "#tasks"
    readonly: false
---

# Literate docs: show the template, run it live

This page demonstrates the `show-source` flag on `lvt` blocks. Each
fenced block below is rendered **twice on the page**: once as a
syntax-highlighted code listing so you can read the template, and once
as the live, interactive widget the same template produces.

## A simple list

The template iterates over a markdown data source and renders one
item per task. The `lvt-on:click="Toggle"` directive on the checkbox
sends a `Toggle` action to the source backend, which flips the `Done`
flag and rebroadcasts the data.

```lvt interactive show-source
<ul lvt-source="tasks" style="list-style: none; padding-left: 0;">
{{range .Data}}
  <li style="display: flex; align-items: center; gap: 8px; padding: 4px 0;">
    <input type="checkbox" {{if .Done}}checked{{end}}
           lvt-on:click="Toggle" data-id="{{.Id}}">
    <span {{if .Done}}style="text-decoration: line-through; opacity: 0.6"{{end}}>{{.Text}}</span>
  </li>
{{end}}
</ul>
```

The widget above is reading from `_data/tasks.md`. Edit that file and
refresh — the live list (and the source listing) stay in sync.

## Where Go code fits

You may have noticed there is **no inline Go controller** in this
page. Tinkerdown's `lvt` blocks are wired to *source backends* declared
in the page's frontmatter (here, `type: markdown`). The backend handles
state and actions; the template is the pure view layer.

If you want to write a custom Go controller — a state struct with
hand-written handlers — author it as a separate LiveTemplate app and
embed it into your docs page with the [`embed-lvt`](../../docs/sources/embed-lvt.md)
block. That's the companion to `show-source` for cases that go beyond
data-source binding.

## Turning it off for a single block

When the page enables source display globally via frontmatter
`lvt_show_source: true`, an individual block can opt out with
`hide-source`. That's useful when one widget on a page should be a
demo only, no listing.
