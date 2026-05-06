---
title: "Hide-source override"
lvt_show_source: true
sources:
  items:
    type: markdown
    file: ./_data/items.md
    anchor: "#items"
---

# Override

```lvt interactive hide-source
<ul lvt-source="items" id="hide-override-list">
{{range .Data}}<li>{{.Text}}</li>{{end}}
</ul>
```
