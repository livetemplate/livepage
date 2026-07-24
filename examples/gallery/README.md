# Saved skills gallery

A dogfooded Tinkerdown app that lists the captured, re-runnable workflows under
`skills/` — the "save it, the need recurs" home for otherwise-ephemeral UIs. It is
itself a plain Tinkerdown app: an `lvt-source` over a CSV, no custom JavaScript.

## Run it

```bash
tinkerdown serve .
```

Open the served URL. Each row is a workflow captured with `/tinkerdown:save`; its
**Location** column points at the `skills/<name>/` dir whose `SKILL.md` carries the
stand-up steps.

## How the listing stays current

No source type reads a directory of `SKILL.md` frontmatter, so the gallery renders a
committed static index (`skills.csv`) rather than scanning `skills/` live (see
`docs/plans/2026-07-09-ephemeral-ui-reframe.md` § M5 Phase 2 for that STOP-gate
decision). `TestSavedSkillsGalleryInSync` asserts the index lists exactly the captured
workflows on disk — every `skills/` dir except the framework-authoring skills
`tinkerdown` and `tinkerdown-save` — so a new capture cannot silently go unlisted.
