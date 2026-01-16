#!/bin/bash

# Script de migração para V2 (Semantic Chunking)

echo "🔄 Migrando banco de dados para V2..."

echo "🔄 Migrando banco de dados para V2 (Semantic Columns)..."
psql -h 104.248.219.200 -U postgres -d eva-db -f v2_migration.sql

echo "🔄 Migrando banco de dados para V3 (Full Text Search)..."
psql -h 104.248.219.200 -U postgres -d eva-db -f v3_migration.sql

if [ $? -eq 0 ]; then
    echo "✅ Migrações concluídas com sucesso!"
    echo "FTS e semântica ativos."
else
    echo "❌ Erro na migração!"
    exit 1
fi
