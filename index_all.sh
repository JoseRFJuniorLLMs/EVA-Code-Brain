#!/bin/bash

# Script para indexar todos os projetos EVA no Code Brain

echo "🧠 Iniciando indexação de projetos EVA..."

# Possíveis localizações dos projetos
POSSIBLE_DIRS=(
    "/root/EVA"
    "$HOME/EVA"
    "$(dirname "$(pwd)")"
)

EVA_DIR=""

# Encontra o diretório correto
for DIR in "${POSSIBLE_DIRS[@]}"; do
    if [ -d "$DIR" ]; then
        EVA_DIR="$DIR"
        echo "📁 Encontrado: $EVA_DIR"
        break
    fi
done

if [ -z "$EVA_DIR" ]; then
    echo "❌ Diretório EVA não encontrado!"
    echo "Procurei em: ${POSSIBLE_DIRS[@]}"
    exit 1
fi

# Para cada subdiretório
for PROJECT in "$EVA_DIR"/*/ ; do
    if [ -d "$PROJECT" ]; then
        PROJECT_NAME=$(basename "$PROJECT")
        echo ""
        echo "📂 Indexando: $PROJECT_NAME ($PROJECT)"
        cd "$(dirname "$0")"
        ./eva-code-brain -index "$PROJECT"
        echo "✅ $PROJECT_NAME indexado!"
    fi
done

echo ""
echo "🎉 Indexação completa!"
