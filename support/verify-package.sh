#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
[ -f plugin.toml ] || { echo 'missing plugin.toml' >&2; exit 1; }
[ -x bin/station-airplay ] || { echo 'missing new ARM supervisor' >&2; exit 1; }
[ -x bin/shairport-sync-ap2 ] || exit 1
[ -x bin/nqptp ] || exit 1
[ -x bin/avahi-daemon ] || exit 1
[ ! -e bin/airplayd ] || { echo 'old hardware daemon is forbidden' >&2; exit 1; }
! grep -R -n -E '/dev/input|/sys/class/leds' source/service.go source/metadata.go | grep -v '^.*//' || { echo 'direct device access forbidden' >&2; exit 1; }
echo 'YAM_SOURCE_CHECK_OK'
