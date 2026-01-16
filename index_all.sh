#!/bin/bash

# Script para indexar todos os projetos EVA no Code Brain

echo "🧠 Iniciando indexação de projetos EVA..."

# Diretório base
EVA_DIR="/root/EVA"

# Verifica se o diretório existe
if [ ! -d "$EVA_DIR" ]; then
    echo "❌ Diretório $EVA_DIR não encontrado!"
    exit 1
fi

# Para cada subdiretório em /root/EVA/
for PROJECT in "$EVA_DIR"/*/ ; do
    if [ -d "$PROJECT" ]; then
        PROJECT_NAME=$(basename "$PROJECT")
        echo ""
        echo "📂 Indexando: $PROJECT_NAME"
        cd /root/EVA-Code-Brain
        ./eva-code-brain -index "$PROJECT"
        echo "✅ $PROJECT_NAME indexado!"
    fi
done

echo ""
echo "🎉 Indexação completa!"
echo ""
echo "Agora você pode perguntar sobre qualquer código dos projetos indexados."
