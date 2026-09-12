CREATE TABLE symbols (
    id          TEXT PRIMARY KEY,
    project     TEXT NOT NULL,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,   -- function|method|type|interface|var|const|package
    file        TEXT NOT NULL,
    line_start  INTEGER NOT NULL,
    line_end    INTEGER NOT NULL,
    signature   TEXT,
    doc         TEXT,
    language    TEXT,
    indexed_at  INTEGER NOT NULL
);

CREATE TABLE edges (
    from_symbol TEXT NOT NULL REFERENCES symbols(id),
    to_symbol   TEXT NOT NULL REFERENCES symbols(id),
    kind        TEXT NOT NULL,   -- calls|implements|embeds|references|imports
    file        TEXT,
    line        INTEGER,
    PRIMARY KEY (from_symbol, to_symbol, kind, file, line)
);

CREATE INDEX idx_symbols_name    ON symbols(project, name);
CREATE INDEX idx_symbols_file    ON symbols(project, file);
CREATE INDEX idx_edges_from      ON edges(from_symbol);
CREATE INDEX idx_edges_to        ON edges(to_symbol);
