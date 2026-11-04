#!/usr/bin/env bash
# Supervisor del bot (macOS / Linux): compila bot/bin/bot y lo vuelve a lanzar cada vez que termina.
# Si el bot está caído más de 30 s se pierde la partida por ausencia: este script lo evita.
#
# Uso, desde cualquier carpeta:   scripts/run-bot.sh
# Variables: las del entorno y, si existe, las de .env en la raíz (ver .env.example). Nunca se imprimen.
# Ctrl+C detiene el bot y el supervisor.
set -u

cd "$(dirname "$0")/.." || exit 1 # raíz del repositorio
ROOT=$(pwd)
BIN="$ROOT/bot/bin/bot"

if [ -f .env ]; then
  set -a # todo lo que se defina a continuación se exporta al entorno del bot
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

echo "supervisor: compilando $BIN" >&2
(cd bot && go build -o "$BIN" ./cmd/bot) || exit 1

stop=0
pid=""
# trap: al recibir Ctrl+C (INT) o TERM, marca stop y le pasa la señal al bot para que cierre prolijo.
trap 'stop=1; [ -n "$pid" ] && kill -INT "$pid" 2>/dev/null' INT TERM

restarts=0
while [ "$stop" -eq 0 ]; do
  "$BIN" &
  pid=$!
  wait "$pid"
  code=$?
  if [ "$stop" -eq 1 ]; then
    wait "$pid" 2>/dev/null # espera a que el bot termine de cerrar
    break
  fi
  restarts=$((restarts + 1))
  echo "$(date '+%Y-%m-%dT%H:%M:%S') supervisor: el bot terminó con código $code; reinicio #$restarts en 1 s" >&2
  sleep 1
done
echo "supervisor: detenido después de $restarts reinicios" >&2
