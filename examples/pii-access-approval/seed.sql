-- Fixtures for the PII / data-export access-approval console.
--
-- NOTHING here is real. Every name, email and amount in orders_pii is
-- fabricated ("*.test" is a reserved non-routable TLD). This makes the export
-- job genuinely run against "PII" while touching no real person's data.
--
-- Build the database:  sqlite3 data/access.db < seed.sql

CREATE TABLE access_requests (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    requester   TEXT NOT NULL,
    team        TEXT,
    dataset     TEXT NOT NULL,
    row_cap     INTEGER NOT NULL,
    scope       TEXT,
    reason      TEXT,
    ticket      TEXT,
    ttl         TEXT,
    sensitivity TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    approver    TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts         TEXT,
    approver   TEXT,
    requester  TEXT,
    dataset    TEXT,
    decision   TEXT,
    scope      TEXT,
    reason     TEXT,
    ttl        TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE datasets (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    sensitivity TEXT,
    description TEXT
);

CREATE TABLE orders_pii (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    name   TEXT,
    email  TEXT,
    amount REAL
);

CREATE TABLE exports (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id  INTEGER,
    exported_at TEXT,
    name        TEXT,
    email       TEXT
);

-- Pending requests, each with the full context an approver needs to decide.
INSERT INTO access_requests (requester, team, dataset, row_cap, scope, reason, ticket, ttl, sensitivity, status) VALUES
  ('dana@corp.example', 'Support',   'orders_pii', 3, 'SELECT name,email FROM orders_pii LIMIT 3', 'Chargeback dispute for customer #8842', 'SUP-1201', '24h', 'PII·GDPR', 'pending'),
  ('ravi@corp.example', 'Analytics', 'orders_pii', 5, 'SELECT name,email FROM orders_pii LIMIT 5', 'GDPR data-subject export request',      'DSR-77',   '48h', 'PII·GDPR', 'pending'),
  ('mei@corp.example',  'Fraud',     'orders_pii', 2, 'SELECT name,email FROM orders_pii LIMIT 2', 'Investigate suspicious order pattern',  'FRD-338',  '12h', 'PII·PCI',  'pending');

INSERT INTO datasets (name, sensitivity, description) VALUES
  ('orders_pii',  'PII·GDPR', 'Customer names + emails attached to orders'),
  ('users_email', 'PII',      'User email addresses'),
  ('payments',    'PCI',      'Card payment metadata (tokenized)');

-- Decoy queue, used only by the frontmatter-shadowing test: a page that tries to
-- redefine the approved `access_requests` source to point here must be ignored,
-- because approval pins the source against page-level redefinition. Its single
-- row is deliberately distinguishable from the approved queue's rows.
CREATE TABLE decoy_requests (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    requester   TEXT NOT NULL,
    team        TEXT,
    dataset     TEXT NOT NULL,
    row_cap     INTEGER NOT NULL,
    scope       TEXT,
    reason      TEXT,
    ticket      TEXT,
    ttl         TEXT,
    sensitivity TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    approver    TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO decoy_requests (requester, team, dataset, row_cap, scope, reason, ticket, ttl, sensitivity, status) VALUES
  ('shadow-decoy@evil.example', 'Shadow', 'everything', 999999, 'SELECT * FROM everything', 'shadow injection', 'SHADOW-1', '999h', 'NONE', 'pending');

-- Synthetic PII: eight rows — more than any request''s row cap — so a bounded
-- export''s LIMIT genuinely bites rather than returning the whole table.
INSERT INTO orders_pii (name, email, amount) VALUES
  ('Ada Fictional',   'ada@example.test',  120.50),
  ('Ben Placeholder', 'ben@example.test',   38.00),
  ('Cid Synthetic',   'cid@example.test',  500.25),
  ('Dot Notreal',     'dot@example.test',   12.99),
  ('Eve Sample',      'eve@example.test',  340.00),
  ('Fox Dummy',       'fox@example.test',   88.40),
  ('Gil Madeup',      'gil@example.test',  210.10),
  ('Hana Faux',       'hana@example.test',  45.00);
