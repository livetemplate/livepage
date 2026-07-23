---
title: "Expense Tracker"
sources:
  expenses: { type: sqlite, db: ./expenses.db, table: expenses, readonly: false }
---

# Expense Tracker

An expense tracking app demonstrating number inputs and calculations.

**Features demonstrated:**
- `lvt-source` with a SQLite table
- Number inputs with currency
- Category selection
- Date tracking
- Table display
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="expenses">
    <h1>Expense Tracker</h1>

    <!-- Add Expense Form -->
    <article>
        <header>Add Expense</header>
        <form name="Add" lvt-el:reset:on:success>
            <fieldset role="group">
                <input type="text" name="description" required placeholder="What did you spend on?">
                <input type="number" name="amount" required min="0.01" step="0.01" placeholder="Amount ($)">
            </fieldset>
            <fieldset role="group">
                <select name="category" required>
                    <option value="">Select category</option>
                    <option value="food">Food & Dining</option>
                    <option value="transport">Transportation</option>
                    <option value="utilities">Utilities</option>
                    <option value="entertainment">Entertainment</option>
                    <option value="shopping">Shopping</option>
                    <option value="healthcare">Healthcare</option>
                    <option value="other">Other</option>
                </select>
                <input type="date" name="expense_date" required>
            </fieldset>
            <button type="submit">Add Expense</button>
        </form>
    </article>

    <!-- Summary -->
    <table>
        <thead>
            <tr>
                <th>Total Expenses</th>
                <th>This Month</th>
                <th>Average</th>
            </tr>
        </thead>
        <tbody>
            <tr>
                <td><strong>{{len .Data}} items</strong></td>
                <td><strong>-</strong></td>
                <td><strong>-</strong></td>
            </tr>
        </tbody>
    </table>

    <!-- Expenses List -->
    <article>
        <header>Recent Expenses</header>

        {{if .Data}}
        <table>
            <thead>
                <tr>
                    <th>Date</th>
                    <th>Description</th>
                    <th>Category</th>
                    <th>Amount</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                {{range .Data}}
                <tr>
                    <td>{{.ExpenseDate}}</td>
                    <td>{{.Description}}</td>
                    <td><kbd>{{.Category}}</kbd></td>
                    <td><strong>${{.Amount}}</strong></td>
                    <td>
                        <button name="Delete" data-id="{{.Id}}" >Delete</button>
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{else}}
        <p><em>No expenses recorded yet. Add your first expense above.</em></p>
        {{end}}
    </article>
</main>
```

## How It Works

1. **Source binding** - `lvt-source="expenses"` binds the container to the SQLite `expenses` table; each input `name` maps to a column and rows load as `.Data`
2. **Currency input** - `type="number"` with `step="0.01"` for decimals
3. **Category display** - Use `<kbd>` tag for visual distinction
4. **Add / Delete** - `name="Add"` on the form inserts an expense (`lvt-el:reset:on:success` clears it); `name="Delete"` removes one

## Prompt to Generate This

> Build an expense tracker with Livemdtools. Let users add expenses with description, amount, category dropdown (food, transport, utilities, etc.), and date. Store them in a SQLite source. Show expenses in a table with category badges. Include a summary row. Use semantic HTML.
