---
title: "Per-block show-source"
sources:
  items:
    type: markdown
    file: ./_data/items.md
    anchor: "#items"
---

# Per-block

```lvt interactive show-source
<ul lvt-source="items" id="per-block-list">
{{range .Data}}<li>{{.Text}}</li>{{end}}
</ul>
```
