-- Migração V5: Suporte a múltiplos projetos
-- Adiciona coluna de nome do projeto para evitar que arquivos com nomes iguais se sobrescrevam

ALTER TABLE project_codebase ADD COLUMN IF NOT EXISTS project_name TEXT;

-- Índice para busca rápida por projeto
CREATE INDEX IF NOT EXISTS idx_project_name ON project_codebase(project_name);

-- Atualiza a tabela de metadados se necessário
-- (Já existe, mas vamos garantir)
CREATE TABLE IF NOT EXISTS project_metadata (
    project_name TEXT PRIMARY KEY,
    root_path TEXT,
    total_files INTEGER,
    total_chunks INTEGER,
    last_indexed TIMESTAMP
);
