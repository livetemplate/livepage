---
title: "Saved skills gallery"
---

# Saved skills

Ephemeral UIs are meant to be thrown away — generate one, use it, regenerate from an
up-to-date source of truth when the need returns. But a workflow worth keeping is
captured with `/tinkerdown:save` as a re-runnable skill — on-brand and with **no LLM
generation**. This gallery is the discoverable home for those saved workflows.

```lvt
<main lvt-source="skills">
  {{if .Error}}
  <p><mark>Error: {{.Error}}</mark></p>
  {{else}}
  <table>
    <thead>
      <tr>
        <th>Workflow</th>
        <th>What it stands up</th>
        <th>Say / trigger</th>
        <th>Location</th>
      </tr>
    </thead>
    <tbody>
      {{range .Data}}
      <tr>
        <td><strong>{{.Name}}</strong></td>
        <td>{{.Description}}</td>
        <td>{{.Triggers}}</td>
        <td><code>{{.Path}}</code></td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{end}}
</main>
```

Each workflow's `SKILL.md` — under its **Location** — has the exact stand-up steps:
seed a fixture if the workflow needs pre-loaded rows, then `tinkerdown serve`. Re-running
a saved skill takes seconds because there is nothing to generate.
