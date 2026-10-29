#!/bin/bash
# Disposable cross-build only. No speaker/firmware access, no sound generation.
set -euo pipefail
test -f /.dockerenv
ROOT=${STATION_AIRPLAY_ROOT:-/repo}
WORK=${STATION_AIRPLAY_BUILD_ROOT:-/work}
abi=$1
case "$abi" in
 armhf) triplet=arm-linux-gnueabihf; arch=arm; cpu=cortex-a7; extra=-mthumb ;;
 arm64) triplet=aarch64-linux-gnu; arch=aarch64; cpu=cortex-a53; extra= ;;
 *) exit 2;;
esac
out=$WORK/$abi
mkdir -p "$out/prefix"
export CC=$triplet-gcc CXX=$triplet-g++ AR=$triplet-ar
export PKG_CONFIG_LIBDIR=/usr/lib/$triplet/pkgconfig:/usr/share/pkgconfig
if [ -d "$out/deps" ]; then export PKG_CONFIG_LIBDIR="$out/deps/usr/lib/$triplet/pkgconfig:$PKG_CONFIG_LIBDIR"; fi
flags="-Os $extra -ffunction-sections -fdata-sections -ffile-prefix-map=$WORK=. -ffile-prefix-map=$ROOT=."
export CFLAGS="$flags" CXXFLAGS="$flags"
export LDFLAGS="-Wl,--gc-sections -Wl,-z,max-page-size=4096"
cd "$out"
if [ ! -d ffmpeg-7.1.3 ]; then tar -xf "$ROOT/third_party/ffmpeg-7.1.3.tar.xz"; fi
if [ ! -f ffmpeg.done ]; then
 (
 cd ffmpeg-7.1.3
 ./configure --prefix="$out/prefix" --target-os=linux --arch="$arch" --cpu="$cpu" \
 --enable-cross-compile --cross-prefix="$triplet-" --enable-small --disable-debug \
 --disable-everything --disable-programs --disable-doc --disable-network --disable-autodetect \
 --disable-avdevice --disable-avfilter --disable-swscale --enable-avcodec --enable-avformat \
 --enable-avutil --enable-swresample --enable-decoder=aac,alac,pcm_s16be --enable-parser=aac \
 --enable-demuxer=aac,mov --enable-protocol=file,pipe --enable-static --disable-shared \
 --extra-cflags="$flags" --extra-ldflags="$LDFLAGS" > "$out/ffmpeg-configure.log" 2>&1
 make -j2 > "$out/ffmpeg-make.log" 2>&1
 make install > "$out/ffmpeg-install.log" 2>&1
 )
 touch ffmpeg.done
fi
if [ ! -d mbedtls-3.6.7 ]; then python3 -c 'import tarfile,sys; tarfile.open(sys.argv[1]).extractall(".")' "$ROOT/third_party/mbedtls-3.6.7.tar.bz2"; fi
cp "$ROOT/support/mbedtls-config.h" mbedtls-3.6.7/include/mbedtls/mbedtls_config.h
make -C mbedtls-3.6.7/library -j2 libmbedcrypto.a CC="$CC" AR="$AR" CFLAGS="$flags" > "$out/mbed-make.log" 2>&1
cp mbedtls-3.6.7/library/libmbedcrypto.a "$out/prefix/lib/"
cp -r mbedtls-3.6.7/include/mbedtls mbedtls-3.6.7/include/psa "$out/prefix/include/"
export PKG_CONFIG_LIBDIR="$out/prefix/lib/pkgconfig:$PKG_CONFIG_LIBDIR"
export CPPFLAGS="-I$out/prefix/include"
if [ -d "$out/deps" ]; then export CPPFLAGS="$CPPFLAGS -I$out/deps/usr/include -I$out/deps/usr/include/$triplet"; fi
export LDFLAGS="$LDFLAGS -L$out/prefix/lib"
# Keep platform libc/ALSA/D-Bus dynamic, link optional control dependencies
# statically to avoid shipping a second GLib runtime and its module loader tree.
static_flags() {
 for flag in $(pkg-config --libs --static "$@"); do
  case "$flag" in
   -lm|-ldl|-lpthread|-lrt|-lresolv|-ldbus-1|-pthread) printf '%s ' "$flag";;
   -l*) name=${flag#-l}; if [ -f "/usr/lib/$triplet/lib$name.a" ]; then printf '%s ' "/usr/lib/$triplet/lib$name.a"; else printf '%s ' "$flag"; fi;;
   *) printf '%s ' "$flag";;
  esac
 done
}
export glib_CFLAGS="$(pkg-config --cflags gio-unix-2.0)"
export glib_LIBS="$(static_flags gio-unix-2.0)"
if [ -d "$out/deps" ]; then
 export libgcrypt_CFLAGS="-I$out/deps/usr/include -I$out/deps/usr/include/$triplet"
 export libgcrypt_LIBS="$out/deps/usr/lib/$triplet/libgcrypt.a $out/deps/usr/lib/$triplet/libgpg-error.a"
fi
export LIBS="$(static_flags avahi-client) /usr/lib/$triplet/libsodium.a /usr/lib/$triplet/libpopt.a /usr/lib/$triplet/libconfig.a -ldbus-1 -ldl -lm -pthread"
if [ ! -d sps ]; then tar -xf "$ROOT/third_party/shairport-sync-5.6.2.tar.gz"; mv shairport-sync-5.6.2 sps; fi
cd sps
if ! grep -q station_set_stream_gain player.c; then patch -p1 < "$ROOT/support/shared-controls.patch"; fi
if ! grep -q dacp_monitor_ipv4_endpoint_callback dacp.c; then patch -p1 < "$ROOT/support/dacp-ipv4-discovery.patch"; fi
if ! grep -q '"general.model"' shairport.c; then patch -p1 < "$ROOT/support/homepod-model.patch"; fi
if ! grep -q 'cannot derive AirPlay identity' shairport.c; then patch -p1 < "$ROOT/support/mbedtls-ap2.patch"; fi
if ! grep -q station_event_conn_lock ap2_event_receiver.c; then patch -p1 < "$ROOT/support/preplay-event-channel.patch"; fi
autoreconf -fi > "$out/autoreconf.log" 2>&1
./configure --host="$triplet" --with-os=linux --with-alsa --with-metadata --with-avahi \
 --with-ssl=mbedtls --with-airplay-2 --without-soxr --with-dbus-interface \
 --without-mpris-interface --without-systemd-startup --sysconfdir=/etc > "$out/configure.log" 2>&1
make -j2 > "$out/make.log" 2>&1
"$triplet-strip" shairport-sync
cp shairport-sync "$out/shairport-sync"
"$triplet-readelf" -d shairport-sync > "$out/elf-dynamic.txt"
"$triplet-readelf" -V shairport-sync > "$out/elf-versions.txt"
sha256sum "$out/shairport-sync"
