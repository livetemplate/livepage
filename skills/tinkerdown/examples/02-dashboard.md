---
title: "User Dashboard"
sources:
  users:
    type: rest
    url: https://jsonplaceholder.typicode.com/users
---

# User Dashboard

A data dashboard demonstrating `lvt-source` with REST API and table display.

**Features demonstrated:**
- `lvt-source` - Connect to REST API
- Table data display
- Refresh button
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

**Configuration (tinkerdown.yaml):**
```yaml
title: "User Dashboard"

sources:
  users:
    type: rest
    url: https://jsonplaceholder.typicode.com/users
```

```lvt
<main>
    <header>
        <hgroup>
            <h1>User Dashboard</h1>
            <p>Data from external REST API</p>
        </hgroup>
        <button name="Refresh" class="outline">Refresh Data</button>
    </header>

    <!-- Users Table -->
    <article lvt-source="users">
        {{if .Error}}
        <p><mark>Error: {{.Error}}</mark></p>
        {{else}}
        <table>
            <thead>
                <tr>
                    <th>ID</th>
                    <th>Name</th>
                    <th>Email</th>
                    <th>Company</th>
                </tr>
            </thead>
            <tbody>
                {{range .Data}}
                <tr>
                    <td>{{.id}}</td>
                    <td>{{.name}}</td>
                    <td>{{.email}}</td>
                    <td>{{.company.name}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{end}}
    </article>

    <!-- Stats Summary -->
    <section lvt-source="users">
        <table>
            <thead>
                <tr>
                    <th>Total Users</th>
                    <th>Active</th>
                    <th>Companies</th>
                </tr>
            </thead>
            <tbody>
                <tr>
                    <td><strong>{{len .Data}}</strong></td>
                    <td><strong>{{len .Data}}</strong></td>
                    <td><strong>{{len .Data}}</strong></td>
                </tr>
            </tbody>
        </table>
    </section>
</main>
```

## How It Works

1. **Source configuration** - `tinkerdown.yaml` defines the REST API endpoint
2. **Data binding** - `lvt-source="users"` fetches data and makes it available as `.Data`
3. **Template rendering** - Use `{{range .Data}}` to iterate over results
4. **Refresh** - `name="Refresh"` on button reloads data from the API

## Prompt to Generate This

> Build a user dashboard with Livemdtools that fetches data from JSONPlaceholder API. Show a table with ID, name, email, and company. Add stat cards showing total users. Include a refresh button. Use semantic HTML.
