#!/bin/bash

# Script de migração para V2 (Semantic Chunking)

echo "🔄 Migrando banco de dados para V2..."

echo "🔄 Migrando banco de dados para V2 (Semantic Columns)..."
psql -h 104.248.219.200 -U postgres -d eva-db -f v2_migration.sql

echo "🔄 Migrando banco de dados para V3 (Full Text Search)..."
psql -h 104.248.219.200 -U postgres -d eva-db -f v3_migration.sql

echo "🔄 Migrando banco de dados para V4 (Conversational Memory)..."
psql -h 104.248.219.200 -U postgres -d eva-db -f v4_migration.sql

if [ $? -eq 0 ]; then
    echo "✅ Migrações concluídas com sucesso!"
    echo "FTS, semântica e memória ativos."
else
    echo "❌ Erro na migração!"
    exit 1
fi
