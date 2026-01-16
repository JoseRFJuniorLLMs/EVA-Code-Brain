-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Table to store code chunks
CREATE TABLE IF NOT EXISTS project_codebase (
    id SERIAL PRIMARY KEY,
    file_path TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding vector(768), -- Dimension for text-embedding-004
    chunk_type TEXT,
    symbol_name TEXT,
    start_line INTEGER,
    end_line INTEGER,
    context_info TEXT,
    last_modified_hash TEXT,
    file_size INTEGER,
    language TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Index for similarity search (HNSW)
CREATE INDEX IF NOT EXISTS idx_codebase_embedding 
ON project_codebase USING hnsw (embedding vector_cosine_ops);

-- Table to store project metadata
CREATE TABLE IF NOT EXISTS project_metadata (
    id SERIAL PRIMARY KEY,
    project_name TEXT UNIQUE NOT NULL,
    root_path TEXT,
    total_files INTEGER DEFAULT 0,
    total_chunks INTEGER DEFAULT 0,
    total_size BIGINT DEFAULT 0,
    last_indexed TIMESTAMP DEFAULT NOW()
);
