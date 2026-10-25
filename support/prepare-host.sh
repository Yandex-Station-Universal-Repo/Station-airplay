#!/bin/sh
# Opt-in, one-time migration from the old /data/shairport autostart to YAM.
# This is NOT executed by the YAM package. Run explicitly as root.
set -eu
OLD=/data/shairport/airplay.sh
INIT=/etc/init.d/S99zairplay
BK=/data/yam-airplay-host-backup
POLICY=/etc/dbus-1/system.d/avahi-dbus.conf
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
POLICY_SRC="$SCRIPT_DIR/../config/avahi-dbus.conf"
EXPECTED_START='  start) /data/shairport/airplay.sh killquasar; /data/shairport/airplay.sh start ;;'
make_hook() {
  cat <<'HOOK'
#!/bin/sh
# YAM migration: stop Alice at boot; AirPlay itself is owned by yamd.
case "$1" in
  start) /data/shairport/airplay.sh killquasar ;;
  stop) : ;;
esac
HOOK
}
fail(){ echo "ERROR: $*" >&2; exit 1; }
[ "$(id -u)" = 0 ] || fail 'root is required (adb shell root)'
[ -f "$OLD" ] || fail "legacy $OLD is absent; use the manual YAM preparation instructions"
[ -f "$POLICY_SRC" ] || fail "Avahi policy source missing: $POLICY_SRC (install the .yap before preparing)"
[ ! -L "$INIT" ] || fail "refusing to replace symlink $INIT"
[ ! -L "$POLICY" ] || fail "refusing to replace symlink $POLICY"
mkdir -p "$BK"
if [ -f "$INIT" ]; then
  if [ "$(cat "$INIT")" = "$(make_hook)" ]; then
    echo 'YAM boot hook already installed.'
  elif grep -Fq "$EXPECTED_START" "$INIT"; then
    [ ! -e "$BK/S99zairplay.original" ] || fail 'unexpected existing original autostart backup; inspect before retrying'
    cp -p "$INIT" "$BK/S99zairplay.original" || fail 'cannot back up original autostart'
  else
    fail "modified $INIT is not our stable autostart; refusing to overwrite it"
  fi
fi
if [ ! -f "$POLICY" ]; then
  [ ! -e "$POLICY" ] || fail 'unexpected Avahi policy path (not a regular file)'
  cp "$POLICY_SRC" "$BK/avahi-dbus.new" || fail 'cannot stage policy'
fi
# Stop only the *legacy* receiver before changing boot ownership.
sh "$OLD" stop || fail 'failed to stop legacy AirPlay'
# Audio-only mode: Quasar must not simultaneously own ALSA.
sh "$OLD" killquasar || fail 'failed to stop Quasar/Alice'
root_was_rw=no
if mount -o remount,rw /; then
  root_was_rw=yes
else
  fail 'cannot remount root writable for safe migration'
fi
error=0
if [ -f "$INIT" ] && [ "$(cat "$INIT")" != "$(make_hook)" ]; then
  make_hook > "$INIT.yam-new" && chmod 755 "$INIT.yam-new" && mv "$INIT.yam-new" "$INIT" || error=1
fi
if [ ! -f "$INIT" ]; then
  make_hook > "$INIT.yam-new" && chmod 755 "$INIT.yam-new" && mv "$INIT.yam-new" "$INIT" || error=1
  # No legacy hook existed: keep a marker to remove only our addition on revert.
  echo created-by-yam-migration > "$BK/new-hook" || error=1
fi
if [ ! -f "$POLICY" ]; then
  mkdir -p /etc/dbus-1/system.d || error=1
  cp "$POLICY_SRC" "$POLICY" && echo created-by-yam-migration > "$BK/new-policy" || error=1
fi
mount -o remount,ro / || error=1
[ "$error" = 0 ] || fail 'host migration incomplete; check paths and restore manually'
if [ -f "$BK/new-policy" ]; then
  dbus=$(pidof dbus-daemon 2>/dev/null | awk '{print $1}')
  [ -n "$dbus" ] && kill -HUP "$dbus" || echo 'WARNING: could not reload dbus policy; reboot may be required'
fi
echo 'YAM_HOST_PREPARED: legacy AirPlay stopped; Alice off; safe YAM-only boot hook installed.'
echo 'Revert later using restore-host.sh, after disabling the YAM plugin.'
