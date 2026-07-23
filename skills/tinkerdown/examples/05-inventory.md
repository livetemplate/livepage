---
title: "Inventory Manager"
sources:
  products: { type: sqlite, db: ./inventory.db, table: products, readonly: false }
---

# Inventory Manager

An inventory system demonstrating `lvt-source` with a SQLite table.

**Features demonstrated:**
- `lvt-source` with a SQLite table
- Number inputs
- CRUD operations
- Conditional styling for low stock
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="products">
    <h1>Inventory Manager</h1>

    <!-- Add Product Form -->
    <article>
        <header>Add Product</header>
        <form name="Add" lvt-el:reset:on:success>
            <fieldset role="group">
                <input type="text" name="name" required placeholder="Product name">
                <input type="text" name="sku" required placeholder="SKU-001">
            </fieldset>
            <fieldset role="group">
                <input type="number" name="quantity" required min="0" value="0" placeholder="Quantity">
                <input type="number" name="price" required min="0" step="0.01" placeholder="Price ($)">
            </fieldset>
            <button type="submit">Add Product</button>
        </form>
    </article>

    <!-- Products Table -->
    <article>
        <header>
            <span>Products</span>
            <button name="Refresh" class="outline">Refresh</button>
        </header>

        {{if .Data}}
        <table>
            <thead>
                <tr>
                    <th>Name</th>
                    <th>SKU</th>
                    <th>Quantity</th>
                    <th>Price</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                {{range .Data}}
                <tr>
                    <td>{{.Name}}</td>
                    <td><code>{{.Sku}}</code></td>
                    <td>{{if lt .Quantity 10}}<mark>{{.Quantity}}</mark>{{else}}{{.Quantity}}{{end}}</td>
                    <td>${{.Price}}</td>
                    <td>
                        <button name="Delete" data-id="{{.Id}}" >Delete</button>
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{else}}
        <p><em>No products in inventory. Add one above!</em></p>
        {{end}}
    </article>
</main>
```

## How It Works

1. **Source binding** - `lvt-source="products"` binds the container to the SQLite `products` table declared in the frontmatter; its rows load as `.Data`
2. **Number inputs** - `type="number"` with `min`, `step` attributes
3. **Low stock warning** - Conditional styling with `{{if lt .Quantity 10}}` using `<mark>` tag
4. **Add / Delete / Refresh** - `name="Add"` on the form inserts a row, `name="Delete"` removes one, and `name="Refresh"` reloads the table

## Prompt to Generate This

> Build an inventory manager with Livemdtools. Store products in a SQLite source. Show a table with name, SKU, quantity, price. Highlight low stock (under 10). Include add/delete functionality and a refresh button. Use semantic HTML.
