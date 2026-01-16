-- Tabelas para Memória Conversacional

CREATE TABLE IF NOT EXISTS conversation_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT, -- Pode ser NULL se não tiver login
    title TEXT,   -- Título da conversa (opcional)
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS conversation_messages (
    id SERIAL PRIMARY KEY,
    session_id UUID REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL, -- 'user', 'assistant', 'system', 'tool'
    content TEXT NOT NULL,
    tool_calls JSONB,   -- Para armazenar chamadas de tools
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_session_id ON conversation_messages(session_id);
