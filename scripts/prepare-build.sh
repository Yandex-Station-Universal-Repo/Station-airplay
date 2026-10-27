#!/bin/sh
# Disposable container only. Never run on a speaker or server host.
set -eu
test -f /.dockerenv
export DEBIAN_FRONTEND=noninteractive
dpkg --add-architecture armhf
apt-get update -qq
apt-get install -y --no-install-recommends ca-certificates python3 xz-utils \
 make autoconf automake libtool pkg-config patch qemu-user xxd \
 gcc-arm-linux-gnueabihf g++-arm-linux-gnueabihf libc6-dev-armhf-cross \
 libasound2-dev:armhf libconfig-dev:armhf libpopt-dev:armhf libplist-dev:armhf \
 libsodium-dev:armhf libgcrypt20-dev:armhf uuid-dev:armhf libavahi-client-dev:armhf \
 libglib2.0-dev:armhf libglib2.0-dev-bin libplist-utils
dpkg-query -W > /work/build-packages.tsv
