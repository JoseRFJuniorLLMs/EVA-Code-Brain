-- v6: Advanced Memory System
-- Adds semantic search, metadata tracking, and auto-summarization capabilities

-- 1. Add embedding column to messages for semantic search
ALTER TABLE conversation_messages 
ADD COLUMN IF NOT EXISTS embedding vector(768);

-- 2. Add metadata column for structured context tracking
ALTER TABLE conversation_messages 
ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}';

-- 3. Create conversation summaries table
CREATE TABLE IF NOT EXISTS conversation_summaries (
    id SERIAL PRIMARY KEY,
    session_id UUID REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    summary TEXT NOT NULL,
    message_range_start INTEGER NOT NULL, -- First message ID in this summary
    message_range_end INTEGER NOT NULL,   -- Last message ID in this summary
    created_at TIMESTAMP DEFAULT NOW()
);

-- 4. Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_summaries_session ON conversation_summaries(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_embedding ON conversation_messages USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX IF NOT EXISTS idx_messages_metadata ON conversation_messages USING gin (metadata);

-- 5. Add updated_at to sessions for tracking activity
ALTER TABLE conversation_sessions 
ADD COLUMN IF NOT EXISTS last_message_at TIMESTAMP DEFAULT NOW();

-- 6. Create function to auto-update last_message_at
CREATE OR REPLACE FUNCTION update_session_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE conversation_sessions 
    SET last_message_at = NOW() 
    WHERE id = NEW.session_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 7. Create trigger to update session timestamp on new message
DROP TRIGGER IF EXISTS trigger_update_session_timestamp ON conversation_messages;
CREATE TRIGGER trigger_update_session_timestamp
    AFTER INSERT ON conversation_messages
    FOR EACH ROW
    EXECUTE FUNCTION update_session_timestamp();
