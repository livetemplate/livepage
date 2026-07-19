# AI-Assisted App Generation

Generate Tinkerdown apps using natural language with AI assistants.

## Overview

Tinkerdown is designed to work seamlessly with AI assistants like Claude Code. You can describe what you want to build in natural language, and the AI will generate a complete working app.

The reason this works is the constraint: interactivity comes from a **fixed `lvt-*` vocabulary** rather than open-ended HTML and JavaScript, so there is a small, well-defined target to hit — and `tinkerdown validate` gives real-parser errors with line numbers and hints, so a generating agent can self-correct to a clean pass before anything is served.

## Using Claude Code Skills

Tinkerdown ships one skill, [`skills/tinkerdown`](../../skills/tinkerdown/), which teaches an assistant the attribute vocabulary, the source types, and the worked examples. With it loaded, you don't invoke a command — you describe what you want in plain language and the assistant writes the markdown.

> **Generation workflow.** The loop today is: describe → the assistant writes `app.md` → `tinkerdown validate app.md` → fix any reported errors → `tinkerdown serve`. A dedicated generate skill that runs this loop end-to-end (including declaring which sources and actions an app is allowed to touch) is in development; see [the ephemeral-UI plan](../plans/2026-07-09-ephemeral-ui-reframe.md). Until it lands, run the steps above.

### Example: Creating an App with Natural Language

Simply describe what you want:

```
Create a task management app with:
- A list of tasks with title, status, and due date
- Ability to add new tasks
- Mark tasks as complete
- Delete tasks
```

The AI will:
1. Create the project structure
2. Set up the SQLite database
3. Generate the markdown with appropriate `lvt-*` attributes
4. Configure sources in frontmatter

## Prompt Tips

### Be Specific About Data

```
Good: "Create a product catalog with name, price, category, and stock quantity"
Better: "Create a product catalog stored in SQLite with fields: name (text), price (decimal), category (dropdown from categories table), stock (integer)"
```

### Describe Interactions

```
Good: "Add a form to create products"
Better: "Add a form to create products with validation, and refresh the table after submission"
```

### Mention Styling Preferences

```
"Use the dark theme"
"Make the table sortable"
"Add action buttons for edit and delete on each row"
```

## Example Prompts

### Task Manager

```
Build a task manager app:
- SQLite database for tasks
- Table showing all tasks with columns: title, priority, status, due_date
- Form to add new tasks
- Click to mark tasks complete
- Delete button on each row
- Use clean theme
```

### API Dashboard

```
Create a dashboard that:
- Fetches user data from https://api.example.com/users
- Displays in a table with name, email, role
- Caches data for 5 minutes
- Has a refresh button
- Shows loading state
```

### System Monitor

```
Build a system monitoring page:
- Show current disk usage (df -h command)
- Show memory usage (free -m command)
- Auto-refresh every 30 seconds
- Display as formatted tables
```

## Generated App Structure

AI-generated apps follow this structure:

```markdown
---
title: Task Manager
description: Manage your daily tasks

sources:
  tasks:
    type: sqlite
    path: ./tasks.db
    query: SELECT * FROM tasks ORDER BY created_at DESC
---

# Task Manager

## All Tasks

<table lvt-source="tasks"
       lvt-columns="title:Task,priority:Priority,status:Status,due_date:Due"
       lvt-actions="Complete,Delete"
       lvt-empty="No tasks yet. Add one below!">
</table>

## Add Task

<form name="AddTask">
  <input name="title" placeholder="Task title" required>
  <select name="priority">
    <option value="low">Low</option>
    <option value="medium">Medium</option>
    <option value="high">High</option>
  </select>
  <input name="due_date" type="date">
  <button type="submit">Add Task</button>
</form>
```

## Best Practices

### 1. Start Simple

Begin with a basic description, then iterate:

```
1. "Create a todo list"
2. "Add a priority field"
3. "Add filtering by status"
```

### 2. Review Generated Code

Always review the generated markdown to understand:
- What sources are configured
- What `lvt-*` attributes are used
- How data flows through the app

### 3. Iterate and Refine

```
"The table looks good, but add a search box to filter tasks"
"Change the theme to dark mode"
"Add a confirmation dialog before deleting"
```

## Troubleshooting AI Generation

### Common Issues

**App doesn't load data:**
- Check that the source path is correct
- Verify the database/file exists
- Check the query syntax

**Interactions don't work:**
- Ensure `lvt-*` attributes are properly formatted
- Check action names match what the handler expects

### Debug Commands

```bash
# Validate the generated app
tinkerdown validate

# Run with debug logging
tinkerdown serve --debug
```

## Next Steps

- [Quickstart Guide](../getting-started/quickstart.md) - Manual app creation
- [Data Sources](data-sources.md) - Understanding source types
- [lvt-* Attributes](../reference/lvt-attributes.md) - All available attributes
