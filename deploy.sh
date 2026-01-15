#!/bin/bash

# Configuration
APP_NAME="eva-code-brain"
PORT=8088

echo "[DEPLOY] Iniciando deploy do $APP_NAME..."

# 1. Atualizar código (opcional, remova o comentário se quiser que ele faça pull)
# git pull origin main

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
