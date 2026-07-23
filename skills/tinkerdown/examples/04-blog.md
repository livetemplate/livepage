---
title: "Simple Blog"
sources:
  posts: { type: sqlite, db: ./posts.db, table: posts, readonly: false }
---

# Simple Blog

A blog demonstrating `lvt-source` with a SQLite table for posts.

**Features demonstrated:**
- `lvt-source` for blog posts
- Textarea for long content
- Date display
- Delete functionality
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="posts">
    <!-- Header -->
    <hgroup>
        <h1>My Blog</h1>
        <p>Thoughts and ideas</p>
    </hgroup>

    <!-- New Post Form -->
    <article>
        <header>Write a Post</header>
        <form name="Add" lvt-el:reset:on:success>
            <input type="text" name="title" required placeholder="Post title">
            <textarea name="content" required rows="6" placeholder="Write your post content here..."></textarea>
            <input type="text" name="author" placeholder="Your name (optional)">
            <button type="submit">Publish Post</button>
        </form>
    </article>

    <!-- Posts List -->
    {{if .Data}}
    {{range .Data}}
    <article>
        <header>
            <hgroup>
                <h2>{{.Title}}</h2>
                <p>{{if .Author}}By {{.Author}} - {{end}}{{.CreatedAt}}</p>
            </hgroup>
        </header>
        <p>{{.Content}}</p>
        <footer>
            <button name="Delete" data-id="{{.Id}}" >Delete Post</button>
        </footer>
    </article>
    {{end}}
    {{else}}
    <article>
        <p><em>No posts yet. Write your first post above!</em></p>
    </article>
    {{end}}
</main>
```

## How It Works

1. **Source binding** - `lvt-source="posts"` binds the container to the SQLite `posts` table declared in the frontmatter; its rows load as `.Data`
2. **Add a post** - `name="Add"` on the form inserts a row; each input `name` (title, content, author) maps to a column, and `lvt-el:reset:on:success` clears the form after publishing
3. **Long content** - `<textarea>` is stored as text in SQLite
4. **Delete** - `name="Delete"` on a button removes that post

## Prompt to Generate This

> Build a simple blog with Livemdtools. Let users write posts with a title, content, and optional author name. Store posts in a SQLite source. Display posts in a card layout with timestamps. Include delete buttons. Use semantic HTML.
