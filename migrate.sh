#!/bin/bash

# Script de migração para V2 (Semantic Chunking)

echo "🔄 Migrando banco de dados para V2..."

psql -h 104.248.219.200 -U postgres -d eva-db -f v2_migration.sql

if [ $? -eq 0 ]; then
    echo "✅ Migração concluída com sucesso!"
    echo "Novas colunas adicionadas: chunk_type, symbol_name, start_line, end_line"
else
    echo "❌ Erro na migração!"
    exit 1
fi
