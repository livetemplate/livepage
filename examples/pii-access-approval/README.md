# PII / Data-Export Access Approval

The M1 reference app: a console for approving requests to export sensitive data.
It removes real, high-stakes friction — today that loop is *Slack-ping someone
with prod access → they hand-run a query → weak or absent audit trail*. Here an
approver sees the full decision context, and **Approve** runs a **bounded** export
and writes a **durable audit record** atomically; **Deny** records the decision.
Nothing touches production and no data is real — `orders_pii` is synthetic.

## What it demonstrates

- **A workspace manifest** (`tinkerdown.yaml`) declaring the *approved* sources and
  actions an LLM may wire up (the `generation:` block), plus `describes:` metadata
  that surfaces in the operation summary an operator reviews before serving.
- **Atomic multi-statement actions** (`statements:` + `ExecTx`): Approve runs a
  bounded export **and** appends its audit row **and** flips the status — all or
  nothing. A partial success (access granted, audit missing) is impossible.
- **Server-authoritative boundedness**: the button sends only the request `id`;
  the export's row cap and dataset are read from the request row via subqueries,
  so a tampered client cannot widen the export.

## Run it

```bash
# 1. Build the fixture database from the seed script (synthetic data only).
mkdir -p data
sqlite3 data/access.db < seed.sql

# 2. Serve. --operator is the approver identity written into the audit trail.
tinkerdown serve . --operator you@corp.example
```

Then open the served URL, review a pending request's scope, and click **Approve**
or **Deny**. Watch the request leave the *Pending* tab and a row appear in the
audit trail (refresh the audit section to see it).

## Files

- `tinkerdown.yaml` — the workspace manifest (approved surface + actions).
- `app.md` — the console (the golden generated artifact; hand-authored here as the
  target `/tinkerdown` should be able to produce).
- `seed.sql` — schema + synthetic fixtures. `data/access.db` is built from it and
  is gitignored (it is a generated artifact, not source).
