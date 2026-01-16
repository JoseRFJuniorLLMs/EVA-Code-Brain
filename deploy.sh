#!/bin/bash

# Configuration
APP_NAME="eva-code-brain"
PORT=8088

echo "[DEPLOY] Iniciando deploy do $APP_NAME..."

# 1. Atualizar código (AGORA AUTOMÁTICO)
echo "[DEPLOY] Atualizando repositório..."
git pull origin main

# 1.5. Rodar migrações (CRÍTICO)
echo "[DEPLOY] Rodando migrações de banco de dados..."
chmod +x migrate.sh
./migrate.sh

# 2. Matar processo antigo
echo "[DEPLOY] Parando processos antigos..."
pkill -f "$APP_NAME" || echo "Nenhum processo anterior encontrado."

# 3. Build da aplicação
echo "[DEPLOY] Compilando aplicação..."
go build -o $APP_NAME .
if [ $? -ne 0 ]; then
    echo "[ERRO] Falha no build!"
    exit 1
fi

# 4. Iniciar servidor
echo "[DEPLOY] Iniciando servidor na porta $PORT..."
nohup ./$APP_NAME > server.log 2>&1 &
PID=$!

echo "[DEPLOY] Servidor rodando com PID: $PID"
echo "[DEPLOY] Logs em server.log"
