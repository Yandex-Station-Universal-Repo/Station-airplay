#!/bin/sh
# Luna owns supervision; YAM owns the immutable files. No init hooks here.
set -eu
umask 077
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
state=${LUNA_PLUGIN_STATE:?Luna binary host with writable plugin state is required}
case "$state" in /*) ;; *) echo 'LUNA_PLUGIN_STATE must be absolute' >&2; exit 2;; esac
mkdir -p "$state"
config=$state/airplay-controls.json
if [ ! -e "$config" ] && [ ! -L "$config" ]; then
    (set -C; cat "$here/config/airplay-controls-v2.json" > "$config")
fi
exec "$here/bin/station-airplay" --shared-audio --controls-config "$config"
