---
title: "Track A and Track B together"
sources:
  tasks:
    type: markdown
    file: ./_data/tasks.md
    anchor: "#tasks"
    readonly: false
lvt_show_source: true
---

# Mixed: literate `lvt` + remote `embed-lvt` on one page

Both block types coexist on the same page without conflict. The
literate `lvt` block uses tinkerdown's shared WebSocket (handler
multiplexes by `blockID`); the `embed-lvt` block holds its own
`LiveTemplateClient` pointed at the upstream's WebSocket. They don't
see each other.

## Literate block (Track A)

Backed by a markdown data source — no Go controller, just template +
data binding.

```lvt interactive
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

## Remote embed (Track B)

A separately deployed counter app, fetched server-side and inlined.
The auto-proxy hooks up both the HTTP fetch and the WebSocket upgrade
without a `tinkerdown.yaml` route entry.

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```

## What's happening

Two completely independent runtimes powering UI on the same page:

| Track A (`lvt`) | Track B (`embed-lvt`) |
|---|---|
| Runtime: tinkerdown's lvt-source backend | Runtime: a deployed LiveTemplate Go program |
| WebSocket: shared (`/ws?page=...`) | WebSocket: dedicated (auto-proxied to upstream) |
| State lives in: the markdown data file | State lives in: the upstream app's session |
| Add custom Go controller? No (use Track B) | Yes — that's its whole point |

> **Note:** This example assumes the upstream counter setup from
> [`embed-counter`](../embed-counter/) — start the counter on
> `:9090` first, then `tinkerdown serve examples/mixed-tracks`.
