#!/bin/bash
# Minimal shared ARMhf runtime; use the same prepared sysroot as build-airplay2.sh.
set -euo pipefail
export PATH=/usr/bin:/bin
R=$(cd "$(dirname "$0")/.." && pwd)
r=${AP2_BUILD_ROOT:?Set AP2_BUILD_ROOT}/armhf/sysroot
w=${AP2_RUNTIME_WORK:-/tmp/station-airplay-runtime}
mkdir -p "$w"
for archive in dbus-1.16.2.tar.xz mbedtls-3.6.7.tar.bz2; do tar -xf "$R/third_party/$archive" -C "$w"; done
flags="-Os -mthumb -ffunction-sections -fdata-sections -nostdinc -isystem /usr/lib/gcc-cross/arm-linux-gnueabihf/13/include -isystem $r/usr/include/arm-linux-gnueabihf -isystem $r/usr/include"
cp "$R/support/mbedtls-config.h" "$w/mbedtls-3.6.7/include/mbedtls/mbedtls_config.h"
make -C "$w/mbedtls-3.6.7/library" clean
make -C "$w/mbedtls-3.6.7/library" -j4 libmbedcrypto.a CC="arm-linux-gnueabihf-gcc --sysroot=$r" AR=arm-linux-gnueabihf-ar CFLAGS="$flags"
cp "$w/mbedtls-3.6.7/library/libmbedcrypto.a" "$r/usr/lib/arm-linux-gnueabihf/"
cp -r "$w/mbedtls-3.6.7/include/mbedtls" "$w/mbedtls-3.6.7/include/psa" "$r/usr/include/"
export PKG_CONFIG_SYSROOT_DIR="$r"
export PKG_CONFIG_LIBDIR="$r/usr/lib/arm-linux-gnueabihf/pkgconfig:$r/usr/lib/pkgconfig"
cmake -S "$w/dbus-1.16.2" -B "$w/dbus-build" \
 -DCMAKE_SYSTEM_NAME=Linux -DCMAKE_SYSTEM_PROCESSOR=arm \
 -DCMAKE_C_COMPILER=arm-linux-gnueabihf-gcc -DCMAKE_CXX_COMPILER=arm-linux-gnueabihf-g++ \
 -DCMAKE_SYSROOT="$r" -DCMAKE_C_FLAGS="$flags" -DCMAKE_SHARED_LINKER_FLAGS=-Wl,--gc-sections \
 -DCMAKE_FIND_ROOT_PATH="$r" -DCMAKE_FIND_ROOT_PATH_MODE_LIBRARY=ONLY -DCMAKE_FIND_ROOT_PATH_MODE_INCLUDE=ONLY \
 -DCMAKE_DISABLE_FIND_PACKAGE_X11=TRUE -DCMAKE_DISABLE_FIND_PACKAGE_GLIB2=TRUE \
 -DEXPAT_LIBRARY="$R/lib/libexpat.so.1" -DEXPAT_INCLUDE_DIR=/usr/include \
 -DDBUS_SESSION_SOCKET_DIR=/tmp -DENABLE_SYSTEMD=OFF -DDBUS_WITH_GLIB=OFF -DDBUS_BUILD_X11=OFF \
 -DDBUS_BUILD_TESTS=OFF -DDBUS_ENABLE_VERBOSE_MODE=OFF -DDBUS_ENABLE_XML_DOCS=OFF
cmake --build "$w/dbus-build" --target dbus-1 -j4
arm-linux-gnueabihf-strip "$w/dbus-build/lib/libdbus-1.so.3"
cp -L "$w/dbus-build/lib/libdbus-1.so.3" "$R/lib/libdbus-1.so.3"
arm-linux-gnueabihf-gcc --sysroot="$r" $flags -Wl,--gc-sections -s "$R/support/avahi-namespace.c" -o "$R/bin/avahi-namespace"
