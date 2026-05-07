---
title: "Linked widgets sharing one source"
sources:
  notes:
    type: markdown
    file: ./_data/notes.md
    anchor: "#notes"
    readonly: false
lvt_show_source: true
---

# Linked widgets

Two `lvt` blocks reading from the same `lvt-source` stay in sync — toggle
a checkbox in either widget and both re-render. The frontmatter sets
`lvt_show_source: true` so each block displays its template above the
live preview without needing the per-block flag.

## A summary view

A compact list, no actions. Useful as an at-a-glance status panel.

```lvt interactive
<ul lvt-source="notes" id="summary" style="list-style: none; padding-left: 0;">
{{range .Data}}
  <li>{{if .Done}}✓ <s>{{.Text}}</s>{{else}}○ {{.Text}}{{end}}</li>
{{end}}
</ul>
```

## An interactive view

The same source, but with click-to-toggle. Clicking here updates the
markdown file on disk; the summary above re-renders automatically.

```lvt interactive
<ul lvt-source="notes" id="interactive" style="list-style: none; padding-left: 0;">
{{range .Data}}
  <li style="display: flex; align-items: center; gap: 8px; padding: 4px 0;">
    <input type="checkbox" {{if .Done}}checked{{end}}
           lvt-on:click="Toggle" data-id="{{.Id}}">
    <span {{if .Done}}style="text-decoration: line-through; opacity: 0.6"{{end}}>{{.Text}}</span>
  </li>
{{end}}
</ul>
```

Both blocks bind to the same `notes` source; tinkerdown wires their
state through one auto-generated server block, so updates triggered by
either widget broadcast to both.
