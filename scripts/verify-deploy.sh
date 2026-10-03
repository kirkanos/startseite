#!/usr/bin/env bash
#
# Prueft nach einem Deploy, ob der Dienst wirklich laeuft.
#
# "systemctl start" kehrt zurueck, sobald der Startbefehl abgesetzt ist - nicht,
# wenn die Container laufen. Ohne diese Pruefung meldet die Pipeline auch dann
# Erfolg, wenn ein Image fehlt oder ein Container sofort wieder abstuerzt.
#
# Laeuft auf dem Server, die Pipeline reicht es per ssh durch:
#
#   ssh server "bash -s -- <service> [sekunden]" < scripts/verify-deploy.sh
#
# Erfolg heisst: die systemd-Unit ist aktiv, alle Container laufen, und wer
# einen Healthcheck hat, meldet "healthy" - mehrmals hintereinander, damit ein
# Container in der Neustart-Schleife nicht zufaellig durchrutscht.
set -uo pipefail

SERVICE=${1:?Aufruf: verify-deploy.sh <service> [sekunden]}
TIMEOUT=${2:-300}
INTERVAL=5
STABIL=3

cd "/services/$SERVICE" || exit 1

zustand() {
  docker compose ps --all --format '{{.Service}}|{{.State}}|{{.Health}}' 2>/dev/null
}

bericht() {
  echo "--- systemd ---" >&2
  systemctl status --no-pager --lines 0 "$SERVICE" >&2 || true
  journalctl -u "$SERVICE" --no-pager -n 20 >&2 || true
  echo "--- Container ---" >&2
  zustand >&2 || true
  echo "--- Letzte Protokollzeilen ---" >&2
  docker compose logs --tail 30 2>&1 | tail -60 >&2 || true
}

echo "== Warte auf $SERVICE (bis zu ${TIMEOUT}s)"
ende=$((SECONDS + TIMEOUT))
gut=0
while :; do
  aktiv=$(systemctl is-active "$SERVICE" 2>/dev/null)
  ausgabe=$(zustand)

  # Container ohne eigenen Healthcheck melden ein leeres Feld; fuer die zaehlt
  # allein, dass sie laufen.
  offen=$(awk -F'|' '$2 != "running" || ($3 != "" && $3 != "healthy") { printf "%s(%s%s) ", $1, $2, ($3 != "" ? "," $3 : "") }' <<< "$ausgabe")
  [[ -z $ausgabe ]] && offen="(keine Container) "
  [[ $aktiv != active ]] && offen="systemd:$aktiv $offen"

  if [[ -z $offen ]]; then
    gut=$((gut + 1))
    if (( gut >= STABIL )); then
      echo "   systemd aktiv, alle Container laufen:"
      tr "|" " " <<< "$ausgabe" | sed "s/^/     /"
      echo "== Deploy bestaetigt"
      exit 0
    fi
  else
    gut=0
    echo "   noch nicht bereit: $offen"
  fi

  if (( SECONDS >= ende )); then
    echo "Zeitueberschreitung nach ${TIMEOUT}s. Nicht bereit: $offen" >&2
    bericht
    exit 1
  fi

  sleep "$INTERVAL"
done
