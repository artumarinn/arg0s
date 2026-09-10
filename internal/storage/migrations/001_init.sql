CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    name        TEXT,
    project     TEXT,
    created_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    metadata    TEXT
);

CREATE TABLE runs (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES sessions(id),
    prompt        TEXT NOT NULL,
    strategy      TEXT NOT NULL,
    profile       TEXT,
    routing       TEXT,
    context       TEXT,
    status        TEXT NOT NULL,
    verdict       TEXT,
    input_tokens  INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd      REAL DEFAULT 0,
    started_at    INTEGER NOT NULL,
    ended_at      INTEGER,
    error         TEXT
);

CREATE TABLE model_runs (
    id            TEXT PRIMARY KEY,
    run_id        TEXT NOT NULL REFERENCES runs(id),
    model_id      TEXT NOT NULL,
    provider      TEXT NOT NULL,
    role          TEXT NOT NULL,
    input_tokens  INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd      REAL DEFAULT 0,
    latency_ms    INTEGER,
    attempts      INTEGER DEFAULT 1,
    fell_back_from TEXT,
    status        TEXT NOT NULL,
    error         TEXT,
    started_at    INTEGER NOT NULL
);

CREATE TABLE events (
    id         TEXT PRIMARY KEY,
    session_id TEXT,
    run_id     TEXT,
    task_id    TEXT,
    type       TEXT NOT NULL,
    payload    TEXT,
    duration_ms INTEGER,
    timestamp  INTEGER NOT NULL
);

CREATE TABLE artifacts (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id),
    kind       TEXT NOT NULL,
    path       TEXT,
    content    TEXT,
    hash       TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE snapshots (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id),
    round      INTEGER NOT NULL,
    git_ref    TEXT,
    content_hash TEXT NOT NULL,
    files      TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE findings (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id),
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id),
    judge       TEXT NOT NULL,
    model_id    TEXT NOT NULL,
    perspective TEXT,
    severity    TEXT NOT NULL,
    category    TEXT,
    title       TEXT NOT NULL,
    description TEXT,
    evidence    TEXT NOT NULL,
    reasoning   TEXT,
    suggested_fix TEXT,
    confidence  REAL,
    created_at  INTEGER NOT NULL
);

CREATE TABLE ledger (
    id           TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL REFERENCES runs(id),
    snapshot_id  TEXT NOT NULL REFERENCES snapshots(id),
    finding_id   TEXT NOT NULL,
    status       TEXT NOT NULL,
    severity     TEXT NOT NULL,
    judge_a      TEXT,
    judge_b      TEXT,
    independence TEXT,
    action       TEXT,
    action_result TEXT,
    human_verified INTEGER,
    created_at   INTEGER NOT NULL
);

CREATE TABLE memories (
    id            TEXT PRIMARY KEY,
    project       TEXT NOT NULL,
    category      TEXT NOT NULL,
    title         TEXT NOT NULL,
    content       TEXT NOT NULL,
    confidence    REAL DEFAULT 1.0,
    source        TEXT,
    refs          TEXT,
    embedding     BLOB,
    created_at    INTEGER NOT NULL,
    last_verified INTEGER,
    stale         INTEGER DEFAULT 0
);

CREATE TABLE costs_daily (
    date       TEXT PRIMARY KEY,
    cost_usd   REAL NOT NULL DEFAULT 0,
    tokens_in  INTEGER NOT NULL DEFAULT 0,
    tokens_out INTEGER NOT NULL DEFAULT 0,
    runs       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_runs_session   ON runs(session_id, started_at DESC);
CREATE INDEX idx_events_run     ON events(run_id, timestamp);
CREATE INDEX idx_events_type    ON events(type, timestamp DESC);
CREATE INDEX idx_model_runs_run ON model_runs(run_id);
CREATE INDEX idx_findings_run   ON findings(run_id, severity);
CREATE INDEX idx_ledger_run     ON ledger(run_id, status);
CREATE INDEX idx_memories_proj  ON memories(project, category, stale);
