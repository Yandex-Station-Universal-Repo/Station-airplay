#!/bin/sh
set -eu
test -f /.dockerenv
mkdir -p /work/armhf/debs /work/armhf/deps
cd /work/armhf/debs
apt-get download libgcrypt20-dev:armhf=1.8.7-6 libgpg-error-dev:armhf=1.38-2
sha256sum ./*.deb > SHA256SUMS
for file in ./*.deb; do dpkg-deb -x "$file" /work/armhf/deps; done
