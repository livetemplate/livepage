---
title: "Appointment Booking"
sources:
  bookings: { type: sqlite, db: ./bookings.db, table: bookings, readonly: false }
---

# Appointment Booking

A booking system demonstrating date/time inputs and scheduling.

**Features demonstrated:**
- `lvt-source` with a SQLite table
- Date and time inputs
- Select dropdown for services
- Booking confirmation display
- **No CSS classes needed** - PicoCSS styles semantic HTML automatically

```lvt
<main lvt-source="bookings">
    <hgroup>
        <h1>Book an Appointment</h1>
        <p>Schedule your visit with us</p>
    </hgroup>

    <!-- Booking Form -->
    <article>
        <form name="Add" lvt-el:reset:on:success>
            <fieldset role="group">
                <input type="text" name="name" required placeholder="Your name">
                <input type="tel" name="phone" required placeholder="(555) 123-4567">
            </fieldset>

            <input type="email" name="email" required placeholder="Email address">

            <!-- Service Selection -->
            <select name="service" required>
                <option value="">Select a service</option>
                <option value="consultation">Initial Consultation (30 min)</option>
                <option value="followup">Follow-up Visit (15 min)</option>
                <option value="comprehensive">Comprehensive Review (60 min)</option>
                <option value="emergency">Emergency Appointment</option>
            </select>

            <!-- Date and Time -->
            <fieldset role="group">
                <input type="date" name="date" required>
                <select name="time" required>
                    <option value="">Select time</option>
                    <option value="09:00">9:00 AM</option>
                    <option value="09:30">9:30 AM</option>
                    <option value="10:00">10:00 AM</option>
                    <option value="10:30">10:30 AM</option>
                    <option value="11:00">11:00 AM</option>
                    <option value="11:30">11:30 AM</option>
                    <option value="14:00">2:00 PM</option>
                    <option value="14:30">2:30 PM</option>
                    <option value="15:00">3:00 PM</option>
                    <option value="15:30">3:30 PM</option>
                    <option value="16:00">4:00 PM</option>
                </select>
            </fieldset>

            <!-- Notes -->
            <textarea name="notes" rows="3" placeholder="Notes (optional)"></textarea>

            <button type="submit">Book Appointment</button>
        </form>
    </article>

    <!-- Upcoming Bookings -->
    <h2>Upcoming Appointments</h2>

    {{if .Data}}
    {{range .Data}}
    <article>
        <header>
            <strong>{{.Date}} at {{.Time}}</strong>
            <small>{{.Service}}</small>
        </header>
        <p>
            <strong>{{.Name}}</strong><br>
            {{.Email}} | {{.Phone}}
        </p>
        {{if .Notes}}<p><em>Notes: {{.Notes}}</em></p>{{end}}
        <footer>
            <button name="Delete" data-id="{{.Id}}" >Cancel</button>
        </footer>
    </article>
    {{end}}
    {{else}}
    <article>
        <p><em>No appointments scheduled. Book your first appointment above.</em></p>
    </article>
    {{end}}
</main>
```

## How It Works

1. **Source binding** - `lvt-source="bookings"` binds the container to the SQLite `bookings` table; each input `name` maps to a column and rows load as `.Data`
2. **Date input** - `type="date"` shows a native date picker
3. **Time slots** - Use `<select>` with predefined time slots for scheduling
4. **Phone input** - `type="tel"` for phone number formatting
5. **Book / Cancel** - `name="Add"` on the form inserts a booking (`lvt-el:reset:on:success` clears it); `name="Delete"` on a button cancels one

## Prompt to Generate This

> Build an appointment booking system with Livemdtools. Include name, phone, email, service selection, date picker, time slots dropdown, and notes. Store bookings in a SQLite source. Display upcoming bookings in cards with date/time prominently shown. Include a cancel button. Use semantic HTML.
