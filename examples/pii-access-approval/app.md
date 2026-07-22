---
title: PII / Data-Export Access Approval
sidebar: false
---

# PII / Data-Export Access Approval

Pending requests for access to sensitive data. Review the **scope** before you act:
**Approve** runs a bounded export and writes a durable audit record; **Deny** records the
decision. No access is granted without a trail, and no request is decided twice.

## [Pending] status = pending | [All] | [Approved] status = approved | [Denied] status = denied

```lvt
<article lvt-source="access_requests">
  {{if .Error}}
  <p><mark>Error: {{.Error}}</mark></p>
  {{else}}
  <figure>
  <table>
    <thead>
      <tr>
        <th scope="col">Requester</th>
        <th scope="col">Dataset</th>
        <th scope="col">Scope (bounded)</th>
        <th scope="col">Justification</th>
        <th scope="col">TTL</th>
        <th scope="col">Sensitivity</th>
        <th scope="col">Status</th>
        <th scope="col">Decision</th>
      </tr>
    </thead>
    <tbody>
      {{range .Data}}
      <tr data-key="{{.Id}}">
        <td><kbd>{{.Requester}}</kbd><br><small>{{.Team}}</small></td>
        <td><code>{{.Dataset}}</code></td>
        <td><small>{{.Scope}}</small></td>
        <td>{{.Reason}}{{if .Ticket}} <small>({{.Ticket}})</small>{{end}}</td>
        <td>{{.Ttl}}</td>
        <td><mark>{{.Sensitivity}}</mark></td>
        <td>
          {{if eq .Status "approved"}}<ins>Approved</ins>
          {{else if eq .Status "denied"}}<del>Denied</del>
          {{else}}<em>Pending</em>{{end}}
        </td>
        <td>
          {{if eq .Status "pending"}}
          <button name="approve-export" data-id="{{.Id}}"
                  data-confirm="Approve {{.Requester}}'s bounded export of {{.Dataset}} and write the audit record?">Approve</button>
          <button name="deny-request" data-id="{{.Id}}" class="secondary"
                  data-confirm="Deny {{.Requester}}'s request for {{.Dataset}}?">Deny</button>
          {{else}}
          <small>by {{.Approver}}</small>
          {{end}}
        </td>
      </tr>
      {{else}}
      <tr><td colspan="8"><em>No requests in this view.</em></td></tr>
      {{end}}
    </tbody>
  </table>
  </figure>
  {{end}}

  <footer>
    <details>
      <summary>Request access</summary>
      <form name="Add" lvt-el:reset:on:success>
        <input name="requester" placeholder="you@company.com" required>
        <input name="team" placeholder="Team">
        <input name="dataset" value="orders_pii" required>
        <label>Row cap <input name="row_cap" type="number" value="500" min="1" required></label>
        <input name="scope" placeholder="SELECT name,email FROM orders_pii LIMIT 500" required>
        <input name="reason" placeholder="Business justification" required>
        <input name="ttl" value="24h">
        <input name="ticket" placeholder="TICKET-123">
        <input name="sensitivity" value="PII">
        <input type="hidden" name="status" value="pending">
        <button type="submit">Request access</button>
      </form>
    </details>
  </footer>
</article>
```

## Recent decisions (audit) {#audit}

Every approve and deny appends here. This is the durable, auditable trail — who accessed
which dataset, when, why, and for how long. Click **Refresh** to pull the latest rows.

```lvt
<article lvt-source="audit_log">
  <button name="Refresh" class="secondary">↻ Refresh audit trail</button>
  <table>
    <thead>
      <tr>
        <th scope="col">When</th>
        <th scope="col">Approver</th>
        <th scope="col">Requester</th>
        <th scope="col">Dataset</th>
        <th scope="col">Decision</th>
        <th scope="col">Scope</th>
      </tr>
    </thead>
    <tbody>
      {{range .Data}}
      <tr data-key="{{.Id}}">
        <td>{{.Ts}}</td>
        <td>{{.Approver}}</td>
        <td>{{.Requester}}</td>
        <td><code>{{.Dataset}}</code></td>
        <td>{{if eq .Decision "approved"}}<ins>approved</ins>{{else}}<del>denied</del>{{end}}</td>
        <td><small>{{.Scope}}</small></td>
      </tr>
      {{else}}
      <tr><td colspan="6"><em>No decisions yet.</em></td></tr>
      {{end}}
    </tbody>
  </table>
</article>
```

## Requestable datasets {#datasets}

The approved catalog an approver can grant against (read-only).

```lvt
<table lvt-source="datasets"
       lvt-columns="name:Dataset,sensitivity:Sensitivity,description:Description"
       lvt-empty="No datasets.">
</table>
```
