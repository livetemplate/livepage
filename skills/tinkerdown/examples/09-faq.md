---
title: "FAQ Page"
sources:
  faqs: { type: sqlite, db: ./faqs.db, table: faqs, readonly: false }
---

# FAQ Page

A frequently asked questions page demonstrating collapsible sections.

**Features demonstrated:**
- `lvt-source` with a SQLite table
- Accordion-style FAQ items using `<details>`
- Add new FAQ entries
- Category organization
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="faqs">
    <hgroup>
        <h1>Frequently Asked Questions</h1>
        <p>Find answers to common questions below</p>
    </hgroup>

    <!-- Add FAQ Form (Admin) -->
    <details>
        <summary>+ Add New FAQ</summary>
        <article>
            <form name="Add" lvt-el:reset:on:success>
                <label>
                    Question
                    <input type="text" name="question" required placeholder="Enter the question">
                </label>
                <label>
                    Answer
                    <textarea name="answer" required rows="4" placeholder="Enter the answer"></textarea>
                </label>
                <label>
                    Category
                    <select name="category" required>
                        <option value="general">General</option>
                        <option value="billing">Billing</option>
                        <option value="technical">Technical</option>
                        <option value="account">Account</option>
                        <option value="shipping">Shipping</option>
                    </select>
                </label>
                <button type="submit">Add FAQ</button>
            </form>
        </article>
    </details>

    <!-- FAQ List -->
    {{if .Data}}
    {{range .Data}}
    <details>
        <summary>
            {{.Question}}
            <kbd>{{.Category}}</kbd>
        </summary>
        <p>{{.Answer}}</p>
        <button name="Delete" data-id="{{.Id}}" >Delete FAQ</button>
    </details>
    {{end}}

    <small>{{len .Data}} FAQ entries</small>
    {{else}}
    <article>
        <p><em>No FAQs yet. Click "Add New FAQ" above to create your first entry.</em></p>
    </article>
    {{end}}
</main>
```

## How It Works

1. **Source binding** - `lvt-source="faqs"` binds the container to the SQLite `faqs` table; each input `name` maps to a column and rows load as `.Data`
2. **Accordion** - HTML `<details>` and `<summary>` elements create collapsible sections
3. **Category badges** - Use `<kbd>` tag for visual category labels
4. **Add / Delete** - `name="Add"` on the form inserts a FAQ (`lvt-el:reset:on:success` clears it); `name="Delete"` removes one
5. **Native behavior** - No JavaScript needed for expand/collapse

## Prompt to Generate This

> Build an FAQ page with Livemdtools. Use HTML details/summary for collapsible Q&A sections. Store FAQs in a SQLite source. Include a hidden admin form to add new FAQs with question, answer, and category. Show category badges. Include delete buttons. Use semantic HTML.
