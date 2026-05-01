CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memories (
    id              BIGSERIAL PRIMARY KEY,
    session_id      TEXT NOT NULL,
    turn_index      INT,
    memory_type     TEXT NOT NULL,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    source          TEXT,
    token_count     INT NOT NULL,
    source_tokens   INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    embedding       vector(384),

    search_vector   tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', title), 'A') ||
        setweight(to_tsvector('english', content), 'B')
    ) STORED
);

CREATE INDEX IF NOT EXISTS idx_memories_search ON memories USING GIN (search_vector);

CREATE INDEX IF NOT EXISTS idx_memories_embedding ON memories USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX IF NOT EXISTS idx_memories_session ON memories (session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories (memory_type, created_at DESC);
