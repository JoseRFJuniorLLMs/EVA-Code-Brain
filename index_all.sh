#!/bin/bash

# Script para indexar todos os projetos EVA no Code Brain

echo "🧠 Iniciando indexação de projetos EVA..."

# Lista de projetos para indexar
PROJECTS=(
    "/root/EVA-Mind"
    "/root/EVA-back"
    "/root/EVA-Mobile"
    "/root/EVA-Familia"
    "/root/EVA-Markov"
    "/root/EVA-Code-Brain"
)

# Para cada projeto
for PROJECT in "${PROJECTS[@]}"; do
    if [ -d "$PROJECT" ]; then
        echo ""
        echo "📂 Indexando: $PROJECT"
        cd /root/EVA-Code-Brain
        ./eva-code-brain -index "$PROJECT"
        echo "✅ $PROJECT indexado!"
    else
        echo "⚠️  Projeto não encontrado: $PROJECT"
    fi
done

echo ""
echo "🎉 Indexação completa!"
echo ""
echo "Agora você pode perguntar sobre qualquer código dos projetos indexados."
