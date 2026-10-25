#!/bin/sh
# Build against a legally obtained development sysroot for each target ABI.
# Sysroots must contain matching development headers AND target stock libraries.
# Do not package those device sysroots or personal device files.
set -eu
cd "$(dirname "$0")/.."
project=$(pwd)
for abi in armhf aarch64; do
 case "$abi" in
  armhf) triplet=arm-linux-gnueabihf; sysroot=${SYSROOT_ARMHF:?set SYSROOT_ARMHF} ;;
  aarch64) triplet=aarch64-linux-gnu; sysroot=${SYSROOT_AARCH64:?set SYSROOT_AARCH64} ;;
 esac
 work=$(mktemp -d)
 tar -xf third_party/shairport-sync-4.3.7.tar.gz -C "$work"
 (
  cd "$work/shairport-sync-4.3.7"
  # Preserve the advertised accessory type of the tested receivers.
  sed -i 's/"am=ShairportSync"/"am=AudioAccessory1,1"/g' rtsp.c
  autoreconf -fi
  CC="$triplet-gcc --sysroot=$sysroot"
  export CC
  export CPPFLAGS="-I$sysroot/usr/include -I$sysroot/usr/include/$triplet -I$sysroot/usr/include/dbus-1.0 -I$sysroot/usr/lib/$triplet/dbus-1.0/include"
  export CFLAGS='-Os'
  export LIBS='-lavahi-common -ldbus-1'
  export LDFLAGS="-L$sysroot/usr/lib/$triplet -Wl,-rpath-link,$sysroot/usr/lib/$triplet"
  ./configure --host="$triplet" --with-os=linux --with-pkg-config=no --with-alsa --with-avahi --with-ssl=openssl --without-libdaemon --without-soxr --without-dbus-interface --without-mpris-interface --sysconfdir=/etc
  make -j2 CXX="$CC"
  "$triplet-strip" shairport-sync
  mkdir -p "$project/receivers/$abi"
  cp shairport-sync "$project/receivers/$abi/shairport-sync"
 )
 # Only remove the freshly allocated build directory after validating it.
 case "$work" in /tmp/tmp.*) rm -rf -- "$work" ;; *) echo "Retained build directory: $work" ;; esac
done
