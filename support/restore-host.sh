#!/bin/sh
# Restore only the exact legacy boot hook replaced by prepare-host.sh.
set -eu
INIT=/etc/init.d/S99zairplay
BK=/data/yam-airplay-host-backup
fail(){ echo "ERROR: $*" >&2; exit 1; }
[ "$(id -u)" = 0 ] || fail 'root required'
if ps | grep '[y]am-plum-airplay' >/dev/null; then
  fail 'YAM AirPlay is still running: yam stop station-airplay; yam disable station-airplay'
fi
[ -f "$INIT" ] || fail 'boot hook missing; refusing to guess'
grep -Fq '# YAM migration: stop Alice at boot; AirPlay itself is owned by yamd.' "$INIT" || fail 'boot hook has been modified; not restoring automatically'
if [ -f "$BK/S99zairplay.original" ]; then
  mount -o remount,rw / || fail 'cannot make root writable'
  cp -p "$BK/S99zairplay.original" "$INIT" || fail 'cannot restore original'
  mount -o remount,ro / || fail 'cannot make root read-only again'
  echo 'ORIGINAL_AIRPLAY_AUTOSTART_RESTORED; start old AirPlay manually, or reboot.'
elif [ -f "$BK/new-hook" ]; then
  mount -o remount,rw / || fail 'cannot make root writable'
  rm "$INIT" || fail 'cannot remove our hook'
  mount -o remount,ro / || fail 'cannot make root read-only again'
  echo 'YAM_BOOT_HOOK_REMOVED'
else
  fail 'no original-hook backup or new-hook marker; refusing to guess'
fi
# Preserve Avahi policy; another program may still depend on it.
