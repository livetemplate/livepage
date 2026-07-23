---
name: pii-access-approval
description: Stand up the PII / data-export access-approval console in seconds — a governed request queue with bounded, audited Approve/Deny. A saved, re-runnable workflow; no LLM generation needed.
triggers:
  - PII access approval
  - data-export approval
  - access request queue
  - approve access requests
  - sensitive data export console
---

# PII / Data-Export Access Approval — saved workflow

This is a **captured, re-runnable workflow**: the reference console for approving
requests to export sensitive data. It removes the real friction of *Slack-ping
someone with prod access → they hand-run a query → weak audit trail*. Because the
whole app already exists as a single markdown file plus a manifest, standing it up
takes seconds and needs **no LLM generation** — you run it, you don't regenerate it.

This is the "keep it — the need recurs" path of the ephemeral-UI model: the
individual UI is throwaway, but a workflow worth keeping is captured as a skill and
re-run directly.

## When to use

Use this when someone needs to review and approve requests for access to sensitive
data — a PII export, a scoped prod-DB read — with a durable, auditable decision
trail. If the team's data or fields differ, treat the served console as the starting
point and edit `app.md` (it is a single, human-editable file), or regenerate a
variant with the `/tinkerdown` skill against your own manifest.

## What it does

- A **pending-requests queue** with the full decision context (requester, dataset,
  bounded scope, justification, TTL, sensitivity).
- **Approve** → a bounded, server-authoritative export (the row cap is read from the
  request row, not the client) **plus** a durable audit record, committed atomically.
- **Deny** → records the decision; grants nothing.
- **Governed writes only**: the UI-bound sources are read-only; every mutation goes
  through a named, audited action, so a requester cannot forge a decision.

## Run it

The artifacts live at [`examples/pii-access-approval/`](../../examples/pii-access-approval/)
— the workspace manifest (`tinkerdown.yaml`), the console (`app.md`), and the
synthetic fixtures (`seed.sql`). Stand it up:

```bash
cd examples/pii-access-approval

# 1. Build the fixture database from the seed script (synthetic data only).
mkdir -p data
sqlite3 data/access.db < seed.sql
# ...or, if the sqlite3 CLI isn't installed:
# python3 -c "import sqlite3; sqlite3.connect('data/access.db').executescript(open('seed.sql').read())"

# 2. Serve. --operator is the approver identity written into the audit trail
#    (this app writes it into the audit records, so the flag matters here).
tinkerdown serve . --operator you@corp.example
```

Open the served URL, review a pending request's scope, and click **Approve** or
**Deny**. The request leaves the *Pending* view and a row appears in the audit trail.

## How it works

See [`examples/pii-access-approval/README.md`](../../examples/pii-access-approval/README.md)
for the full walkthrough — the manifest's approved surface, the atomic multi-statement
actions, the server-authoritative bounded export, and the governed-writes design. The
console's whole framework leg (parse → first render → live WebSocket) is ~17 ms, so the
time cost is the human's review, not the runtime.
