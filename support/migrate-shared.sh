#!/bin/sh
# Explicit, reversible migration of the two known standalone installations.
# Run before enabling station-airplay. Does not alter rootfs, Alice or networking.
set -eu
mode=${1:-}
model=${2:-}
case "$model" in
 yandexstation_2) base=/data/local/airplay-station2 ;;
 yandexmini_3_pro) base=/data/local/airplay-mini3pro ;;
 *) echo 'Usage: migrate-shared.sh {apply|rollback} {yandexstation_2|yandexmini_3_pro}' >&2; exit 2 ;;
esac
old=$base/service.sh
backup=$base/service.before-yam.sh
flag=$base/disabled-by-yam
case "$mode" in
 apply)
  [ -f "$old" ] && [ ! -L "$old" ] || { echo 'No regular legacy service; nothing migrated' >&2; exit 1; }
  if [ -e "$backup" ]; then
   grep -q 'station-airplay YAM migration guard' "$old" || { echo 'Conflicting previous backup' >&2; exit 1; }
   [ -f "$flag" ] || { echo 'Previous migration was rolled back; inspect backup first' >&2; exit 1; }
   echo 'Legacy receiver already disabled'; exit 0
  fi
  cp -p "$old" "$backup"
  "$old" stop
  # Both native init and an external keeper still calling start become harmless.
  # Status keeps reporting the old receiver stopped; YAM is the new source of truth.
  {
   echo '#!/bin/sh'
   echo '# station-airplay YAM migration guard'
   echo 'case "${1:-status}" in'
   printf ' start|restart|supervise) [ ! -e "%s" ] || exit 0 ;;\n' "$flag"
   echo 'esac'
   cat "$backup"
  } > "$old.yam-new"
  chmod 755 "$old.yam-new"
  : > "$flag"
  mv "$old.yam-new" "$old"
  echo 'Legacy receiver disabled; now install/enable station-airplay in YAM'
  ;;
 rollback)
  [ -f "$backup" ] && grep -q 'station-airplay YAM migration guard' "$old" || { echo 'No matching migration to restore' >&2; exit 1; }
  # Stop/disable the YAM plugin FIRST; refuse to create a duplicate receiver.
  if pidof shairport-sync shairport-sync-ap2 >/dev/null 2>&1; then
   echo 'Stop/disable the YAM receiver before rollback' >&2; exit 1
  fi
  cp -p "$backup" "$old.restore"
  mv "$old.restore" "$old"
  rm -f "$flag" "$backup"
  "$old" start
  ;;
 *) echo 'Expected apply or rollback' >&2; exit 2 ;;
esac
