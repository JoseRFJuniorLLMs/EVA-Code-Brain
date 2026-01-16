#!/bin/bash

# Setup completo do EVA Code Brain

echo "🚀 Setup do EVA Code Brain"
echo ""

# 1. Criar schema no banco
echo "📊 Criando tabelas no banco de dados..."
psql -h 104.248.219.200 -U postgres -d eva-db -f schema.sql

if [ $? -ne 0 ]; then
    echo "❌ Erro ao criar schema!"
    echo "Tente manualmente:"
    echo "psql -h 104.248.219.200 -U postgres -d eva-db < schema.sql"
    exit 1
fi

echo "✅ Tabelas criadas!"
echo ""

# 2. Instalar Ollama (se não estiver instalado)
echo "🦙 Verificando Ollama..."
if ! command -v ollama &> /dev/null; then
    echo "Instalando Ollama..."
    curl -fsSL https://ollama.com/install.sh | sh
    echo "✅ Ollama instalado!"
else
    echo "✅ Ollama já instalado!"
fi

# 3. Baixar modelo de embedding
echo ""
echo "📥 Baixando modelo de embedding..."
ollama pull nomic-embed-text
echo "✅ Modelo baixado!"

# 4. Atualizar .env
echo ""
echo "⚙️  Configurando .env..."
if ! grep -q "USE_OLLAMA_EMBED" .env; then
    echo "" >> .env
    echo "# Ollama Embeddings" >> .env
    echo "USE_OLLAMA_EMBED=true" >> .env
    echo "OLLAMA_EMBED_MODEL=nomic-embed-text" >> .env
    echo "OLLAMA_HOST=http://localhost:11434" >> .env
    echo "✅ .env atualizado!"
else
    echo "✅ .env já configurado!"
fi

echo ""
echo "🎉 Setup completo!"
echo ""
echo "Agora rode: ./index_all.sh"
