---
title: "Frontmatter default"
lvt_show_source: true
sources:
  items:
    type: markdown
    file: ./_data/items.md
    anchor: "#items"
---

# Frontmatter default on

```lvt interactive
<ul lvt-source="items" id="frontmatter-default-list">
{{range .Data}}<li>{{.Text}}</li>{{end}}
</ul>
```
