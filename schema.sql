-- DeckFS Schema for D1 (Cloudflare's SQLite) or DuckDB
-- Tracks files, processing status, and enables queries

-- Source files (.dsh)
CREATE TABLE IF NOT EXISTS sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT UNIQUE NOT NULL,           -- R2 object key (path/to/file.dsh)
    bucket TEXT NOT NULL DEFAULT 'input',
    size_bytes INTEGER,
    etag TEXT,                           -- R2 ETag for change detection
    content_hash TEXT,                   -- SHA256 of content
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP                 -- Soft delete
);

CREATE INDEX idx_sources_key ON sources(key);
CREATE INDEX idx_sources_updated ON sources(updated_at);

-- Processing runs
CREATE TABLE IF NOT EXISTS processing_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL REFERENCES sources(id),
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, processing, complete, error
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_ms INTEGER,
    error_message TEXT,
    slide_count INTEGER,
    title TEXT,
    worker_id TEXT,                      -- Which worker instance processed this
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_runs_source ON processing_runs(source_id);
CREATE INDEX idx_runs_status ON processing_runs(status);
CREATE INDEX idx_runs_created ON processing_runs(created_at);

-- Output files (SVGs, manifests)
CREATE TABLE IF NOT EXISTS outputs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES processing_runs(id),
    source_id INTEGER NOT NULL REFERENCES sources(id),
    key TEXT NOT NULL,                   -- R2 object key
    file_type TEXT NOT NULL,             -- svg, manifest, thumbnail
    slide_number INTEGER,                -- NULL for manifest
    size_bytes INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_outputs_run ON outputs(run_id);
CREATE INDEX idx_outputs_source ON outputs(source_id);
CREATE INDEX idx_outputs_key ON outputs(key);

-- File versions (for history/rollback)
CREATE TABLE IF NOT EXISTS versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL REFERENCES sources(id),
    version_number INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    etag TEXT,
    size_bytes INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_id, version_number)
);

CREATE INDEX idx_versions_source ON versions(source_id);

-- Tags for organization
CREATE TABLE IF NOT EXISTS tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    color TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS source_tags (
    source_id INTEGER NOT NULL REFERENCES sources(id),
    tag_id INTEGER NOT NULL REFERENCES tags(id),
    PRIMARY KEY (source_id, tag_id)
);

-- Watch patterns (which paths to monitor)
CREATE TABLE IF NOT EXISTS watch_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern TEXT NOT NULL,               -- Glob pattern like "presentations/*.dsh"
    enabled BOOLEAN DEFAULT TRUE,
    config_json TEXT,                    -- Processing config overrides
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Useful views

-- Latest processing status for each source
CREATE VIEW IF NOT EXISTS v_source_status AS
SELECT 
    s.id,
    s.key,
    s.size_bytes,
    s.updated_at AS source_updated,
    pr.status,
    pr.slide_count,
    pr.title,
    pr.completed_at AS last_processed,
    pr.duration_ms,
    pr.error_message
FROM sources s
LEFT JOIN processing_runs pr ON pr.id = (
    SELECT id FROM processing_runs 
    WHERE source_id = s.id 
    ORDER BY created_at DESC 
    LIMIT 1
)
WHERE s.deleted_at IS NULL;

-- Files needing reprocessing (source updated after last successful run)
CREATE VIEW IF NOT EXISTS v_needs_processing AS
SELECT 
    s.id,
    s.key,
    s.updated_at AS source_updated,
    pr.completed_at AS last_processed
FROM sources s
LEFT JOIN processing_runs pr ON pr.id = (
    SELECT id FROM processing_runs 
    WHERE source_id = s.id AND status = 'complete'
    ORDER BY created_at DESC 
    LIMIT 1
)
WHERE s.deleted_at IS NULL
  AND (pr.completed_at IS NULL OR s.updated_at > pr.completed_at);

-- Processing stats
CREATE VIEW IF NOT EXISTS v_processing_stats AS
SELECT 
    DATE(created_at) as date,
    COUNT(*) as total_runs,
    SUM(CASE WHEN status = 'complete' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as failed,
    AVG(duration_ms) as avg_duration_ms,
    SUM(slide_count) as total_slides
FROM processing_runs
GROUP BY DATE(created_at)
ORDER BY date DESC;
