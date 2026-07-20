# lvt-* Attributes Reference

Complete reference for all `lvt-*` attributes.

> **`tinkerdown validate` does not check attribute names.** It verifies that a document
> *parses*; unknown `lvt-*` attributes pass through as ordinary HTML and are silently
> ignored at runtime. A file using a misspelled or superseded attribute validates
> clean and then does nothing. Until vocabulary validation exists, this page — checked
> against `@livetemplate/client` — is the authority. See § Namespace migration if you
> are updating older markup.

## Overview

`lvt-*` attributes add interactivity to HTML elements. They're processed by the LiveTemplate client library.

## Categories

- [Data Binding](#data-binding) - Connect elements to data sources
- [Event Handling](#event-handling) - Respond to user actions
- [UI Directives](#ui-directives) - Control UI behavior
- [Form Handling](#form-handling) - Form-specific attributes

---

## Data Binding

### lvt-source

Bind an element to a data source.

```html
<table lvt-source="tasks">
</table>
```

**Works with:** `<table>`, `<ul>`, `<ol>`, `<select>`

### lvt-columns

Specify columns for auto-rendered tables.

```html
<table lvt-source="tasks" lvt-columns="id,title,status">
</table>

<!-- With custom labels -->
<table lvt-source="tasks" lvt-columns="id:ID,title:Task Title,status:Status">
</table>
```

### lvt-field

Specify field for auto-rendered lists.

```html
<ul lvt-source="users" lvt-field="name">
</ul>
```

### lvt-value / lvt-label

Specify value and label fields for selects.

```html
<select lvt-source="categories" lvt-value="id" lvt-label="name">
</select>
```

### lvt-empty

Empty state message when source has no data.

```html
<table lvt-source="tasks" lvt-empty="No tasks yet">
</table>
```

### lvt-actions

Add action buttons to table rows.

```html
<table lvt-source="tasks" lvt-columns="title,status" lvt-actions="Edit,Delete">
</table>
```

---

## Event Handling

### name (on button)

Handle click events. Use `name` attribute on `<button>` elements to trigger server actions.

```html
<button name="AddTask">Add Task</button>
```

### name (on form)

Handle form submissions. Use `name` attribute on `<form>` elements.

```html
<form name="CreateUser">
  <input name="name" />
  <button type="submit">Create</button>
</form>
```

### lvt-on:change

Handle change events (inputs, selects).

```html
<select lvt-on:change="FilterByCategory">
  <option value="all">All</option>
  <option value="active">Active</option>
</select>
```

### lvt-key

Filter keyboard events by key.

```html
<input lvt-key="Enter" lvt-on:click="Search">
```

> **Removed.** `lvt-click-away` and `lvt-window-{event}` were documented here but are
> not implemented by `@livetemplate/client`. "Click away" survives only as a *lifecycle
> state* — `lvt-el:removeclass:on:click-away` — not as a standalone action attribute.

---

## Data Attributes

### data-*

Pass data with actions.

```html
<button name="Delete" data-id="123">Delete</button>
```

> **Removed.** `lvt-value-*` ("extract values from elements", e.g.
> `lvt-value-name="#nameInput"`) was documented here but is implemented nowhere — not in
> the client, not in Tinkerdown. To send extra values with an action, put them on the
> element as `data-*` attributes, or submit a `<form>` whose named inputs carry them.
>
> Not to be confused with `lvt-value` (no suffix), which is real: it names the value
> field when binding a `<select>` to a source — see § Data Binding.

---

## UI Directives

Visual effects live under the **`lvt-fx:`** namespace.

### lvt-fx:scroll

Control scroll behavior.

```html
<div lvt-fx:scroll="bottom">
  <!-- Auto-scroll to bottom -->
</div>
```

**Values:** `bottom`, `top`, `sticky`

### lvt-fx:highlight

Flash highlight on updates.

```html
<div lvt-fx:highlight>
  Content that highlights when updated
</div>
```

Effects can also be bound to a lifecycle state, e.g. `lvt-fx:highlight:on:success`.

### lvt-fx:animate

Entry animations.

```html
<div lvt-fx:animate="fade">
  Fades in
</div>
```

**Values:** `fade`, `slide`, `scale`

### lvt-autofocus

Auto-focus on visibility.

```html
<input lvt-autofocus>
```

### lvt-focus-trap

Trap focus within an element — Tab and Shift-Tab cycle inside it rather than escaping
to the rest of the page. Useful for modal-like regions.

```html
<div class="modal" lvt-focus-trap>
  Modal content
</div>
```

> **Removed.** `lvt-modal-open` / `lvt-modal-close` were documented here but are not
> implemented by `@livetemplate/client`. Use a native `<dialog>` element for open/close
> semantics; pair it with `lvt-focus-trap` above if you need focus containment.

---

## Form Handling

### lvt-ignore

Skip an element and its entire subtree during DOM diffing. Commonly used to preserve
form values a user is editing, but it applies to any element whose live DOM state the
server should not overwrite. Setting `data-lvt-force-update` on the server's version of
the element bypasses the guard and resumes diffing.

```html
<input name="search" lvt-ignore>
```

### lvt-form:disable-with

Button text during form submission.

```html
<button type="submit" lvt-form:disable-with="Saving...">
  Save
</button>
```

### lvt-form:preserve

Keep a form's field values after its action completes, instead of resetting them.
Set on the `<form>`. Distinct from `lvt-ignore`, which is a general DOM-diffing guard
on any element.

```html
<form name="Search" lvt-form:preserve>
  <input name="q">
</form>
```

### data-confirm

Confirmation dialog before action.

```html
<button name="Delete" data-confirm="Are you sure?">
  Delete
</button>
```

---

## Rate Limiting

Event-handling modifiers live under the **`lvt-mod:`** namespace.

### lvt-mod:throttle

Throttle event handling.

```html
<input lvt-on:change="Search" lvt-mod:throttle="300">
```

### lvt-mod:debounce

Debounce event handling.

```html
<input lvt-on:change="Search" lvt-mod:debounce="300">
```

### lvt-debounce

Distinct from `lvt-mod:debounce`, and still current: overrides the debounce interval
(ms) the client applies to an **auto-wired change binding**, rather than to an explicit
`lvt-on:` handler.

```html
<input name="title" lvt-debounce="500">
```

---

## Lifecycle Hooks

### lvt-el:{method}:on:{state}

Trigger DOM mutations on lifecycle state changes.

```html
<!-- Reset form on success -->
<form lvt-el:reset:on:success>
</form>

<!-- Add class on error -->
<div lvt-el:addClass:on:error="error-state">
</div>
```

Scope to a named action by inserting it before the state — useful when one page has
several actions in flight:

```html
<form lvt-el:reset:on:create-todo:success>
</form>
```

**Available methods:**

| Method | Description |
|--------|-------------|
| `reset` | Reset form |
| `addClass` | Add CSS class |
| `removeClass` | Remove CSS class |
| `toggleClass` | Toggle CSS class |
| `setAttr` | Set an attribute |
| `toggleAttr` | Toggle an attribute |

There is no `disable`/`enable`/`focus`/`blur` method — express those through
`setAttr` / `toggleAttr` (e.g. `lvt-el:setAttr:disabled:on:pending`).

**Available states:**

| State | Description |
|-------|-------------|
| `pending` | Action in progress |
| `success` | Action completed successfully |
| `error` | Action failed |
| `done` | Action settled (either outcome) |

`lvt-el:` methods can target a different element via `data-lvt-target` (`#id` or
`closest:selector`).

---

## Attribute Ownership

### Core LiveTemplate Attributes

These are processed by the `@livetemplate/client` library:

- Event handling: `name` (button/form), `lvt-on:{event}`, `lvt-key`
- Rate limiting: `lvt-mod:throttle`, `lvt-mod:debounce`
- Visual effects: `lvt-fx:scroll`, `lvt-fx:highlight`, `lvt-fx:animate`
- Focus: `lvt-autofocus`
- Forms: `lvt-form:preserve`, `lvt-form:disable-with`, `data-confirm`
- DOM guards: `lvt-ignore`, `lvt-ignore-attrs` (bypass with `data-lvt-force-update`)
- Lifecycle: `lvt-el:{method}:on:{state}` (retarget with `data-lvt-target`)

### Namespace migration

The client groups Tier-2 attributes into namespaces. If you have older markup, these
are the renames:

| Old | Current |
|---|---|
| `lvt-scroll` / `lvt-highlight` / `lvt-animate` | `lvt-fx:scroll` / `lvt-fx:highlight` / `lvt-fx:animate` |
| `lvt-throttle` / `lvt-debounce` | `lvt-mod:throttle` / `lvt-mod:debounce` |
| `lvt-disable-with` | `lvt-form:disable-with` |
| `lvt-preserve` | `lvt-ignore` (general DOM guard) or `lvt-form:preserve` (form values) |
| `lvt-{action}-on:{event}` | `lvt-el:{method}:on:{state}` |
| `lvt-click-away`, `lvt-window-{event}`, `lvt-focus-trap`, `lvt-modal-open`, `lvt-modal-close` | **removed** — not implemented by the client |

**The old names are not shimmed.** Apart from a warn-once shim for `lvt-no-intercept`,
the client does not accept superseded names, so old markup fails silently rather than
warning — and `tinkerdown validate` does not catch it either (see the note at the top
of this page).

### Tinkerdown-Specific Attributes

These are processed by Tinkerdown for auto-rendering:

- Data binding: `lvt-source`, `lvt-columns`, `lvt-field`, `lvt-value`, `lvt-label`
- Display: `lvt-empty`, `lvt-actions`

## Next Steps

- [Auto-Rendering Guide](../guides/auto-rendering.md) - Using auto-rendering
- [Go Templates Guide](../guides/go-templates.md) - Custom layouts
