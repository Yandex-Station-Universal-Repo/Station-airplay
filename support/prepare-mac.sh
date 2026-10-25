#!/bin/sh
# Run on Mac after: yam install station-airplay-1.1.0.yap
set -eu
command -v adb >/dev/null 2>&1 || { echo 'adb not found' >&2; exit 1; }
adb shell id
adb shell sh /opt/yam/plugins/station-airplay/support/prepare-host.sh
