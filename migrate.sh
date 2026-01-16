#!/bin/bash

# Script de migração para V2 (Semantic Chunking)
export PGPASSWORD='Debian23@'

echo "🔄 Migrando banco de dados para V2..."

echo "🔄 Migrando banco de dados para V2 (Semantic Columns)..."
PGPASSWORD='Debian23@' psql -h 104.248.219.200 -U postgres -d eva-db -f v2_migration.sql

echo "🔄 Migrando banco de dados para V3 (Full Text Search)..."
PGPASSWORD='Debian23@' psql -h 104.248.219.200 -U postgres -d eva-db -f v3_migration.sql

echo "🔄 Migrando banco de dados para V4 (Conversational Memory)..."
PGPASSWORD='Debian23@' psql -h 104.248.219.200 -U postgres -d eva-db -f v4_migration.sql

echo "🔄 Migrando banco de dados para V5 (Multi-Project Support)..."
PGPASSWORD='Debian23@' psql -h 104.248.219.200 -U postgres -d eva-db -f v5_migration.sql

if [ $? -eq 0 ]; then
    echo "✅ Migrações concluídas com sucesso!"
    echo "FTS, semântica e memória ativos."
else
    echo "❌ Erro na migração!"
    exit 1
fi
