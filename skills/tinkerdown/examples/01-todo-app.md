---
title: "Todo App"
sources:
  todos: { type: sqlite, db: ./todos.db, table: todos, readonly: false }
---

# Todo App

A simple todo list demonstrating `lvt-source` with a SQLite table for automatic CRUD operations.

**Features demonstrated:**
- `lvt-source` - Bind a container to a SQLite source
- `name="Add"` (on form) - Built-in insert action
- `name="Delete"` (on button) - Built-in delete action
- `lvt-on:click="Toggle"` - Built-in boolean toggle
- `data-id` - Pass the row id with actions
- Conditional rendering with `{{if}}`/`{{else}}`
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="todos">
    <h1>Todo List</h1>

    <!-- Add Todo Form -->
    <form name="Add" lvt-el:reset:on:success>
        <fieldset role="group">
            <input type="text" name="title" required placeholder="What needs to be done?">
            <button type="submit">Add</button>
        </fieldset>
    </form>

    <!-- Todo List -->
    {{if .Data}}
    <table>
        <thead>
            <tr>
                <th>Done</th>
                <th>Task</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
            {{range .Data}}
            <tr>
                <td>
                    <input type="checkbox" {{if .Completed}}checked{{end}} lvt-on:click="Toggle" data-id="{{.Id}}">
                </td>
                <td>{{if .Completed}}<s>{{.Title}}</s>{{else}}{{.Title}}{{end}}</td>
                <td><button name="Delete" data-id="{{.Id}}" >Delete</button></td>
            </tr>
            {{end}}
        </tbody>
    </table>
    <small>{{len .Data}} items total</small>
    {{else}}
    <p><em>No todos yet. Add one above to get started!</em></p>
    {{end}}
</main>
```

## How It Works

1. **Source binding** - `lvt-source="todos"` binds this container to the SQLite `todos` table declared in the frontmatter; its rows load as `.Data`
2. **Add a row** - `name="Add"` on the form is the built-in insert; each input `name` maps to a column, and `lvt-el:reset:on:success` clears the form after a successful insert
3. **Toggle completion** - `lvt-on:click="Toggle"` on the checkbox with `data-id` flips the boolean column for that row
4. **Delete** - `name="Delete"` on the button removes that row from the database

## Prompt to Generate This

> Build a todo app with Livemdtools. I want to add todos, mark them complete with a checkbox, and delete them. Store them in a SQLite source. Use semantic HTML - no CSS classes needed.
