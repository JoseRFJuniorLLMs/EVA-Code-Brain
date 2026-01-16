-- Add new columns for Semantic Chunking
ALTER TABLE project_codebase ADD COLUMN IF NOT EXISTS chunk_type TEXT;
ALTER TABLE project_codebase ADD COLUMN IF NOT EXISTS symbol_name TEXT;
ALTER TABLE project_codebase ADD COLUMN IF NOT EXISTS start_line INTEGER;
ALTER TABLE project_codebase ADD COLUMN IF NOT EXISTS end_line INTEGER;
ALTER TABLE project_codebase ADD COLUMN IF NOT EXISTS context_info TEXT;

-- Create index for symbol search
CREATE INDEX IF NOT EXISTS idx_symbol_name ON project_codebase(symbol_name);
