---
title: "Status Page"
---

# Service Status Page

A status monitoring page demonstrating exec source for real-time system checks.

**Features demonstrated:**
- `lvt-source` with exec type
- Shell script data provider
- Status indicators
- Auto-refresh capability
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

**Configuration (tinkerdown.yaml):**
```yaml
title: "Service Status"

sources:
  services:
    type: exec
    cmd: ./check-services.sh
  incidents:
    type: sqlite
    db: ./incidents.db
    table: incidents
    readonly: false
```

**check-services.sh:**
```bash
#!/bin/bash
cat <<EOF
[
  {"name": "API Server", "status": "operational", "latency": 45, "uptime": 99.9},
  {"name": "Database", "status": "operational", "latency": 12, "uptime": 99.99},
  {"name": "Cache", "status": "operational", "latency": 3, "uptime": 99.95},
  {"name": "CDN", "status": "degraded", "latency": 120, "uptime": 98.5},
  {"name": "Email Service", "status": "operational", "latency": 230, "uptime": 99.8}
]
EOF
```

```lvt
<main>
    <!-- Header -->
    <header>
        <hgroup>
            <h1>Service Status</h1>
            <p>Real-time system monitoring</p>
        </hgroup>
        <button name="Refresh" class="outline">Refresh</button>
    </header>

    <!-- Overall Status Banner -->
    <article lvt-source="services">
        <p><ins>All Systems Operational</ins> - Last updated: just now</p>
    </article>

    <!-- Services List -->
    <article lvt-source="services">
        <header>Services</header>

        {{if .Error}}
        <p><mark>Error loading services: {{.Error}}</mark></p>
        {{else}}
        <table>
            <thead>
                <tr>
                    <th>Service</th>
                    <th>Status</th>
                    <th>Latency</th>
                    <th>Uptime</th>
                </tr>
            </thead>
            <tbody>
                {{range .Data}}
                <tr>
                    <td><strong>{{.name}}</strong></td>
                    <td>
                        {{if eq .status "operational"}}<ins>{{.status}}</ins>
                        {{else if eq .status "degraded"}}<mark>{{.status}}</mark>
                        {{else}}<del>{{.status}}</del>{{end}}
                    </td>
                    <td>{{if gt .latency 100.0}}<mark>{{.latency}}ms</mark>{{else}}{{.latency}}ms{{end}}</td>
                    <td>{{.uptime}}%</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{end}}
    </article>

</main>
```

## Incident Log

The incident log is a **separate, writable source** — a block gets its state from one
`lvt-source`, so incidents (a SQLite table) live in their own block, distinct from the
read-only exec `services` above.

```lvt
<section lvt-source="incidents">
    <h2>Incident Log</h2>

    <form name="Add" lvt-el:reset:on:success>
        <fieldset role="group">
            <input type="text" name="title" required placeholder="Incident description">
            <select name="severity" required>
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
            </select>
            <button type="submit">Log Incident</button>
        </fieldset>
    </form>

    {{if .Data}}
    {{range .Data}}
    <article>
        <header>
            {{if eq .Severity "critical"}}<mark>{{.Severity}}</mark>
            {{else if eq .Severity "high"}}<mark>{{.Severity}}</mark>
            {{else}}<kbd>{{.Severity}}</kbd>{{end}}
            {{.Title}}
        </header>
        <footer>
            <small>{{.CreatedAt}}</small>
            <button name="Delete" data-id="{{.Id}}">Resolve</button>
        </footer>
    </article>
    {{end}}
    {{else}}
    <p><em>No active incidents</em></p>
    {{end}}
</section>
```

## How It Works

1. **Exec source** - Shell script outputs JSON, Livemdtools parses and displays it
2. **Status indicators** - Use `<ins>` for operational, `<mark>` for warnings, `<del>` for outages
3. **Conditional styling** - `{{if eq .status "operational"}}` for status colors
4. **Incident log** - a separate writable SQLite source (`lvt-source="incidents"`) with the built-in `Add`/`Delete` actions for manual incident tracking
5. **Refresh** - `name="Refresh"` on button reloads data from the script

## Prompt to Generate This

> Build a service status page with Livemdtools. Use an exec source to run a shell script that outputs JSON with service name, status (operational/degraded/outage), latency, and uptime. Show services in a table with status indicators. Add an incident log with severity levels. Use semantic HTML.
