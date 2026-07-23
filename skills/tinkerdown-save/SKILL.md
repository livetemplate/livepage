---
name: tinkerdown-save
description: Capture an ephemeral Tinkerdown UI you just generated into a durable, re-runnable Claude Code skill — so a workflow worth keeping re-runs in seconds without regenerating. Invoked on request as /tinkerdown:save; opt-in, never automatic.
triggers:
  - save this UI
  - keep this app
  - persist this workflow
  - make this reusable
  - turn this into a skill
  - /tinkerdown:save
---

# /tinkerdown:save — capture an ephemeral UI as a reusable skill

Generated UIs are **ephemeral by default** — use them, throw them away, regenerate
from an up-to-date source of truth when the need returns. But some workflows recur.
When the user decides one is worth keeping, this skill distils the just-completed
generation session into a durable, re-runnable skill, so it stands up again in
seconds with **no LLM generation at all**.

## When to use

Only when **the user asks to keep / save / persist** a UI generated earlier in the
conversation. This is opt-in and on request — never fire it automatically after a
generation. Ephemeral is the default; capture is the exception.

Do not use it to save an app that does not yet work: the thing being captured must
already `tinkerdown validate` clean and serve. Capture preserves a working artifact;
it does not fix a broken one.

## What to extract from the session

Look back over the conversation for the working UI the user produced and gather:

1. **The `app.md`** — the final, validated document that was served.
2. **The manifest** — the `tinkerdown.yaml` it ran against (its approved sources and
   named actions), if there was one. A plain app with sources in frontmatter has no
   separate manifest; note that.
3. **The fixtures / data setup** — the schema + seed (a `seed.sql`, a `.db`, or the
   steps that created the data). Synthetic/sample data, not anything real.
4. **The stand-up steps** — how it was served: seeding the database, the
   `tinkerdown serve …` command, any `--operator` / `--allow-exec` flags.
5. **The intent** — what the UI is *for*, in one line. This becomes the skill's
   `description` and `triggers`.

## How to produce the skill

Create `skills/<kebab-name>/` at the repo's top level (alongside the other skill
directories) and write a `SKILL.md` with:

- **Frontmatter:** `name` (kebab-case, derived from the app's title), a one-line
  `description` (what it stands up + that it re-runs without generation), and 3–6
  `triggers` (the phrases someone would say when they need this workflow).
- **When to use** — the situation this workflow serves; when to regenerate instead.
- **What it does** — the UI's behavior in a few bullets.
- **Run it** — the exact stand-up steps (see below), copy-pasteable.

### Writing the "Run it" steps — the part that is easy to get wrong

The blind test of this skill showed the stand-up steps are where a captured skill
breaks, not the markup. Get these three right:

- **Seeding is a real step, and `tinkerdown serve` does not do it.** Serving starts
  the server; it never applies `seed.sql`. A writable `sqlite` source *does*
  auto-create its table on first write, so an app works **empty** with no seed at
  all — seed only when the workflow needs pre-loaded rows (a queue to review, a
  catalog). When you do seed, do not assume `sqlite3` is installed (it often is not).
  Give a portable command:

  ```bash
  # if the sqlite3 CLI is available:
  sqlite3 data/app.db < seed.sql
  # otherwise (python3 is almost always present):
  python3 -c "import sqlite3; sqlite3.connect('data/app.db').executescript(open('seed.sql').read())"
  ```

- **Decide fresh-vs-persistent and say so.** A demo re-seeds fresh each run; a workflow
  the user keeps ("we do this every day") must **persist** — so make `seed.sql`
  idempotent (`CREATE TABLE IF NOT EXISTS`) and seed **only if the database does not
  already exist**, so a re-run keeps prior data instead of erroring on the existing
  table or wiping it. State which model the skill uses.

- **Flags follow the app, not habit.** Add `--operator you@example.com` only if the
  app writes an operator identity (a governed action referencing `:operator`, e.g. an
  audit trail). Add `--allow-exec` only if the app uses an `exec` source or action.
  Otherwise omit both. (`serve` always has *some* operator identity — the flag only
  sets it.)

**Bundle vs. point — the rule that avoids duplication:**

- **Bundle** the artifacts into `skills/<name>/` (`app.md`, `tinkerdown.yaml`,
  `seed.sql`) when the UI was generated in a scratch/ephemeral directory that will be
  thrown away — the skill is then their only home, so copies are not duplication.
- **Point** at the existing path instead (do **not** copy) when the artifacts already
  live at a stable, committed location in the repo (e.g. under `examples/`). Copying a
  committed source of truth just creates two files that must stay identical — which is
  drift waiting to happen. Reference the committed path from the skill.

## Verify before finishing (required)

A captured skill nobody trusts is worse than none. Confirm:

1. `tinkerdown validate <the skill's app.md, or the path it points at>` passes clean.
2. Every path the `SKILL.md` references actually exists.
3. The stand-up steps are complete — a fresh reader could seed and serve from them
   alone, with nothing implicit.

If validation fails, the underlying app was not actually working — fix that first (or
tell the user it is not ready to save), rather than capturing a broken artifact.

## Example of a captured skill

[`skills/pii-access-approval`](../pii-access-approval/) is a worked result of this
capability: the PII / data-export approval console, captured as a re-runnable skill.
Its artifacts already live under `examples/`, so it **points** at them rather than
copying — the "committed source" case above.
