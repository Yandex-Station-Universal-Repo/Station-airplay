#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
VERSION=$(sed -n 's/^version = "\([^"]*\)"/\1/p' plugin.toml | head -n 1)
[ -n "$VERSION" ] || { echo 'Missing version in plugin.toml' >&2; exit 1; }
mkdir -p dist
# Keep plugin.toml at the TAR root. Exclude VCS and local state.
tar -czf "dist/station-airplay-${VERSION}.yap" plugin.toml bin lib config LICENSES source support docs/NOTICE.md
printf 'Created %s\n' "dist/station-airplay-${VERSION}.yap"
