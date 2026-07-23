---
title: "Contact Form"
sources:
  contacts: { type: sqlite, db: ./contacts.db, table: contacts, readonly: false }
---

# Contact Form

A contact form demonstrating form handling with multiple field types.

**Features demonstrated:**
- `lvt-source` with a SQLite table
- Text, email, textarea, checkbox inputs
- Form validation (HTML5)
- Table display of submissions
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="contacts">
    <h1>Contact Us</h1>

    <!-- Contact Form -->
    <article>
        <form name="Add" lvt-el:reset:on:success>
            <label>
                Name
                <input type="text" name="name" required minlength="2" placeholder="Your name">
            </label>

            <label>
                Email
                <input type="email" name="email" required placeholder="you@example.com">
            </label>

            <label>
                Subject
                <input type="text" name="subject" required placeholder="What's this about?">
            </label>

            <label>
                Message
                <textarea name="message" required rows="4" placeholder="Your message..."></textarea>
            </label>

            <label>
                <input type="checkbox" name="subscribe">
                Subscribe to our newsletter
            </label>

            <button type="submit">Send Message</button>
        </form>
    </article>

    <!-- Submissions List -->
    <h2>Recent Submissions</h2>

    {{if .Data}}
    <table>
        <thead>
            <tr>
                <th>Name</th>
                <th>Email</th>
                <th>Subject</th>
                <th>Newsletter</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
            {{range .Data}}
            <tr>
                <td>{{.Name}}</td>
                <td>{{.Email}}</td>
                <td>{{.Subject}}</td>
                <td>{{if .Subscribe}}Yes{{else}}No{{end}}</td>
                <td>
                    <button name="Delete" data-id="{{.Id}}" >Delete</button>
                </td>
            </tr>
            {{end}}
        </tbody>
    </table>
    {{else}}
    <p><em>No submissions yet.</em></p>
    {{end}}
</main>
```

## How It Works

1. **Source binding** - `lvt-source="contacts"` binds the container to the SQLite `contacts` table declared in the frontmatter; its rows load as `.Data`
2. **Form fields** - Each input `name` maps to a column in the `contacts` table
3. **Field types** - `text`, `email`, `textarea`, `checkbox` are all supported
4. **Validation** - Use HTML5 attributes like `required`, `minlength`, `type="email"`
5. **Add / Delete** - `name="Add"` on the form inserts a row (`lvt-el:reset:on:success` clears the form); `name="Delete"` on a button removes that row

## Prompt to Generate This

> Build a contact form with Livemdtools. Include name, email, subject, message, and a newsletter checkbox. Store submissions in a SQLite source and show them in a table with delete buttons. Use form validation. Use semantic HTML.
