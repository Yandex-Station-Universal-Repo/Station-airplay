#!/bin/sh
# Static supervisor selection; device/ABI selection is validated inside Go.
set -eu
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
unset LD_LIBRARY_PATH
case "$(uname -m)" in
 aarch64) exec "$base/station-airplay-aarch64" "$@" ;;
 armv7l|armv8l) exec "$base/station-airplay" "$@" ;;
 *) echo 'station-airplay: unsupported CPU architecture' >&2; exit 1 ;;
esac
