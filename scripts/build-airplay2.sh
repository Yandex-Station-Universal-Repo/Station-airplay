#!/bin/bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
P=${AP2_BUILD_ROOT:?Set AP2_BUILD_ROOT to the prepared sysroot/build directory}
abi=$1
for archive in shairport-sync-5.6.2.tar.gz nqptp-1.2.9.tar.gz ffmpeg-7.1.3.tar.xz; do cp "$ROOT/third_party/$archive" "$P/$archive"; done
case "$abi" in
 armhf) triplet=arm-linux-gnueabihf; ffarch=arm; cpu=cortex-a7; ssl=1.1 ;;
 arm64) triplet=aarch64-linux-gnu; ffarch=aarch64; cpu=cortex-a53; ssl=3 ;;
 *) exit 2 ;;
esac
r=$P/$abi/sysroot
w=/tmp/station-airplay-ap2-build/$abi
mkdir -p "$w" "$P/$abi/output"
python3 - "$r" <<'PY'
import pathlib,os,sys
root=pathlib.Path(sys.argv[1])
for p in root.rglob('*'):
 if p.is_symlink():
  target=p.readlink()
  if target.is_absolute():
   replacement=root/str(target).lstrip('/')
   p.unlink();p.symlink_to(os.path.relpath(replacement,p.parent))
PY
# Keep runtime libraries private to the new build sysroot. Never redistribute
# the captured stock libraries; resolve those from the actual device at run time.
cp "$P/$abi/stock-libasound.so.2" "$r/usr/lib/$triplet/libasound.so.2"
cp "$P/$abi/stock-libdbus-1.so.3" "$r/usr/lib/$triplet/libdbus-1.so.3"
ln -sf libasound.so.2 "$r/usr/lib/$triplet/libasound.so"
ln -sf libdbus-1.so.3 "$r/usr/lib/$triplet/libdbus-1.so"
# Use matching stock OpenSSL (1.1 on Station 2, 3 on Mini); never redistribute it.
# The caller supplies the corresponding development headers in this private sysroot.
for lib in crypto ssl; do
 cp "$P/$abi/stock-lib$lib.so.$ssl" "$r/usr/lib/$triplet/lib$lib.so.$ssl"
 ln -sf "lib$lib.so.$ssl" "$r/usr/lib/$triplet/lib$lib.so"
done
for lib in popt config avahi-client avahi-common sodium gcrypt gpg-error uuid; do
 test -f "$r/usr/lib/$triplet/lib$lib.a"
 if test -L "$r/usr/lib/$triplet/lib$lib.so"; then unlink "$r/usr/lib/$triplet/lib$lib.so"; fi
done
export CC="$triplet-gcc --sysroot=$r"
export PKG_CONFIG_SYSROOT_DIR="$r"
export PKG_CONFIG_LIBDIR="$r/usr/lib/$triplet/pkgconfig:$r/usr/lib/pkgconfig:$r/usr/share/pkgconfig"
export CFLAGS="-Os -ffunction-sections -fdata-sections -nostdinc -isystem /usr/lib/gcc-cross/$triplet/13/include -isystem $r/usr/include/$triplet -isystem $r/usr/include -ffile-prefix-map=$w=. -ffile-prefix-map=$P=."
export CPPFLAGS="-I$r/usr/include -I$r/usr/include/$triplet -I$r/usr/include/dbus-1.0 -I$r/usr/lib/$triplet/dbus-1.0/include"
export LDFLAGS="-Wl,--gc-sections -Wl,-z,max-page-size=4096 -L$r/usr/lib/$triplet -L$r/lib/$triplet -L$r/usr/lib -Wl,-rpath-link,$r/usr/lib/$triplet -Wl,-rpath-link,$r/lib/$triplet"
export LIBS='-lavahi-common -lgpg-error -ldbus-1 -ldl -lm -pthread'
if [ ! -d "$w/ffmpeg-7.1.3" ]; then tar -xf "$P/ffmpeg-7.1.3.tar.xz" -C "$w"; fi
if [ ! -f "$w/ffmpeg.done" ]; then
 (
  cd "$w/ffmpeg-7.1.3"
  ./configure --prefix=/usr --target-os=linux --arch="$ffarch" --cpu="$cpu" --enable-cross-compile --cross-prefix="$triplet-" --sysroot="$r" \
   --enable-small --disable-debug --disable-everything --disable-programs --disable-doc --disable-network --disable-autodetect \
   --disable-avdevice --disable-avfilter --disable-swscale --enable-avcodec --enable-avformat --enable-avutil --enable-swresample \
   --enable-decoder=aac,alac,pcm_s16be --enable-parser=aac --enable-demuxer=aac,mov --enable-protocol=file,pipe \
   --enable-static --disable-shared --extra-cflags="$CFLAGS" --extra-ldflags="$LDFLAGS" > "$P/$abi/ffmpeg-configure.log" 2>&1
  make clean > /dev/null
  make -j4 > "$P/$abi/ffmpeg-make.log" 2>&1
  make DESTDIR="$r" install > "$P/$abi/ffmpeg-install.log" 2>&1
 )
 touch "$w/ffmpeg.done"
fi
if [ ! -d "$w/shairport-sync-5.6.2" ]; then tar -xf "$P/shairport-sync-5.6.2.tar.gz" -C "$w"; fi
(
 cd "$w/shairport-sync-5.6.2"
 if ! grep -q '"general.model"' shairport.c; then patch -p1 < "$ROOT/support/homepod-model.patch"; fi
 autoreconf -fi > "$P/$abi/autoreconf.log" 2>&1
 ./configure --host="$triplet" --with-os=linux --with-alsa --with-avahi --with-ssl=openssl --with-airplay-2 \
  --without-soxr --without-dbus-interface --without-mpris-interface --without-systemd-startup --sysconfdir=/etc \
  > "$P/$abi/configure.log" 2>&1
 make clean > /dev/null
 make -j4 CXX="$CC" > "$P/$abi/make.log" 2>&1
 "$triplet-strip" shairport-sync
 cp shairport-sync "$P/$abi/output/"
 "$triplet-readelf" -d shairport-sync | grep NEEDED
)
if [ ! -d "$w/nqptp-1.2.9" ]; then tar -xf "$P/nqptp-1.2.9.tar.gz" -C "$w"; fi
(
 cd "$w/nqptp-1.2.9"
 autoreconf -fi > "$P/$abi/nqptp-autoreconf.log" 2>&1
 LIBS='' ./configure --host="$triplet" --without-systemd-startup > "$P/$abi/nqptp-configure.log" 2>&1
 make -j4 > "$P/$abi/nqptp-make.log" 2>&1
 "$triplet-strip" nqptp
 cp nqptp "$P/$abi/output/"
)
echo "$abi Shairport Sync 5.6.2 and NQPTP 1.2.9 built"
