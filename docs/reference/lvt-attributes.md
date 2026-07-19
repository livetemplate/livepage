# lvt-* Attributes Reference

Complete reference for all `lvt-*` attributes.

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

### lvt-click-away

Trigger action when clicking outside element.

```html
<div lvt-click-away="CloseModal">
  Modal content
</div>
```

### lvt-window-{event}

Handle window-level events.

```html
<div lvt-window-scroll="HandleScroll">
</div>
```

---

## Data Attributes

### data-*

Pass data with actions.

```html
<button name="Delete" data-id="123">Delete</button>
```

### lvt-value-*

Extract values from elements.

```html
<button name="Update" lvt-value-name="#nameInput">
  Update
</button>
```

---

## UI Directives

### lvt-scroll

Control scroll behavior.

```html
<div lvt-scroll="bottom">
  <!-- Auto-scroll to bottom -->
</div>
```

**Values:** `bottom`, `top`, `sticky`

### lvt-highlight

Flash highlight on updates.

```html
<div lvt-highlight>
  Content that highlights when updated
</div>
```

### lvt-animate

Entry animations.

```html
<div lvt-animate="fade">
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

Trap focus within element (for modals).

```html
<div class="modal" lvt-focus-trap>
  Modal content
</div>
```

### lvt-modal-open / lvt-modal-close

Control modals.

```html
<button lvt-modal-open="myModal">Open</button>

<div id="myModal" class="modal">
  <button lvt-modal-close="myModal">Close</button>
</div>
```

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

### lvt-disable-with

Button text during form submission.

```html
<button type="submit" lvt-disable-with="Saving...">
  Save
</button>
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

### lvt-throttle

Throttle event handling.

```html
<input lvt-on:change="Search" lvt-throttle="300">
```

### lvt-debounce

Debounce event handling.

```html
<input lvt-on:change="Search" lvt-debounce="300">
```

---

## Lifecycle Hooks

### lvt-{action}-on:{event}

Trigger actions on lifecycle events.

```html
<!-- Reset form on success -->
<form lvt-el:reset:on:success>
</form>

<!-- Add class on error -->
<div lvt-addClass-on:error="error-state">
</div>
```

**Available actions:**

| Action | Description |
|--------|-------------|
| `reset` | Reset form |
| `addClass` | Add CSS class |
| `removeClass` | Remove CSS class |
| `disable` | Disable element |
| `enable` | Enable element |
| `focus` | Focus element |
| `blur` | Blur element |

**Available events:**

| Event | Description |
|-------|-------------|
| `success` | Action completed successfully |
| `error` | Action failed |
| `loading` | Action in progress |

---

## Attribute Ownership

### Core LiveTemplate Attributes

These are processed by the `@livetemplate/client` library:

- Event handling: `name` (button/form), `lvt-on:change`, `lvt-key`, `lvt-click-away`
- Rate limiting: `lvt-throttle`, `lvt-debounce`
- UI directives: `lvt-scroll`, `lvt-highlight`, `lvt-animate`, `lvt-autofocus`, `lvt-focus-trap`
- Modals: `lvt-modal-open`, `lvt-modal-close`
- Forms: `lvt-ignore`, `lvt-disable-with`, `data-confirm`
- Lifecycle: `lvt-{action}-on:{event}`

### Tinkerdown-Specific Attributes

These are processed by Tinkerdown for auto-rendering:

- Data binding: `lvt-source`, `lvt-columns`, `lvt-field`, `lvt-value`, `lvt-label`
- Display: `lvt-empty`, `lvt-actions`

## Next Steps

- [Auto-Rendering Guide](../guides/auto-rendering.md) - Using auto-rendering
- [Go Templates Guide](../guides/go-templates.md) - Custom layouts
