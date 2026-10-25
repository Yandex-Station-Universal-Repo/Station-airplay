#!/bin/sh
set -eu
command -v adb >/dev/null 2>&1 || exit 1
adb shell sh /data/yam-airplay-restore-host.sh
