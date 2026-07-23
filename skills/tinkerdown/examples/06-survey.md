---
title: "Survey Form"
sources:
  responses: { type: sqlite, db: ./survey.db, table: responses, readonly: false }
---

# Customer Survey

A multi-section survey demonstrating radio buttons, select dropdowns, and ratings.

**Features demonstrated:**
- `lvt-source` with a SQLite table
- Radio button groups
- Select dropdowns
- Range/rating inputs
- Multi-field forms
- Results display
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="responses">
    <hgroup>
        <h1>Customer Satisfaction Survey</h1>
        <p>Help us improve by sharing your feedback</p>
    </hgroup>

    <!-- Survey Form -->
    <article>
        <form name="Add" lvt-el:reset:on:success>
            <label>
                Your Name
                <input type="text" name="name" required placeholder="Enter your name">
            </label>

            <label>
                Email
                <input type="email" name="email" required placeholder="you@example.com">
            </label>

            <!-- Overall Satisfaction (Radio) -->
            <fieldset>
                <legend>Overall Satisfaction</legend>
                <label><input type="radio" name="satisfaction" value="very_satisfied"> Very Satisfied</label>
                <label><input type="radio" name="satisfaction" value="satisfied"> Satisfied</label>
                <label><input type="radio" name="satisfaction" value="neutral"> Neutral</label>
                <label><input type="radio" name="satisfaction" value="dissatisfied"> Dissatisfied</label>
            </fieldset>

            <!-- How did you hear about us (Select) -->
            <label>
                How did you hear about us?
                <select name="source">
                    <option value="">Select an option</option>
                    <option value="search">Search Engine</option>
                    <option value="social">Social Media</option>
                    <option value="friend">Friend/Colleague</option>
                    <option value="ad">Advertisement</option>
                    <option value="other">Other</option>
                </select>
            </label>

            <!-- Rating (1-10) -->
            <label>
                Rating (1-10)
                <input type="range" name="rating" min="1" max="10" value="5">
            </label>

            <!-- Comments -->
            <label>
                Additional Comments
                <textarea name="comments" rows="4" placeholder="Tell us more about your experience..."></textarea>
            </label>

            <!-- Would Recommend (Checkbox) -->
            <label>
                <input type="checkbox" name="would_recommend">
                I would recommend this product to others
            </label>

            <button type="submit">Submit Survey</button>
        </form>
    </article>

    <!-- Survey Results -->
    <h2>Survey Responses</h2>

    {{if .Data}}
    {{range .Data}}
    <article>
        <header>
            <strong>{{.Name}}</strong>
            <small>{{.Email}}</small>
        </header>
        <p>
            Satisfaction: <strong>{{.Satisfaction}}</strong> |
            Rating: <strong>{{.Rating}}/10</strong> |
            Source: {{.Source}}
        </p>
        {{if .Comments}}<blockquote>{{.Comments}}</blockquote>{{end}}
        <footer>
            <button name="Delete" data-id="{{.Id}}" >Delete</button>
        </footer>
    </article>
    {{end}}
    {{else}}
    <p><em>No responses yet.</em></p>
    {{end}}
</main>
```

## How It Works

1. **Source binding** - `lvt-source="responses"` binds the container to the SQLite `responses` table; each input `name` maps to a column and rows load as `.Data`
2. **Radio buttons** - Use `name="satisfaction"` with the same name for grouping
3. **Select dropdown** - Use `<select>` with `<option>` elements
4. **Range input** - `type="range"` creates a slider
5. **Add / Delete** - `name="Add"` on the form inserts a response (`lvt-el:reset:on:success` clears it); `name="Delete"` removes one

## Prompt to Generate This

> Build a customer survey with Livemdtools. Include name, email, satisfaction rating (radio buttons), how they heard about us (dropdown), a 1-10 rating slider, comments textarea, and a "would recommend" checkbox. Store responses in a SQLite source and show results in cards. Use semantic HTML.
