-- Add Full Text Search (FTS) column
-- We use 'generated always' to keep it in sync with content automatically
ALTER TABLE project_codebase 
ADD COLUMN IF NOT EXISTS content_fts tsvector 
GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

-- Create GIN index for fast keyword search
CREATE INDEX IF NOT EXISTS idx_content_fts ON project_codebase USING GIN (content_fts);

-- Function to search using RRF (Reciprocal Rank Fusion) is implemented in application logic,
-- but we rely on simple queries here.
