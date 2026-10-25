# Build

Go >=1.23, Python >=3.8; Linux/WSL for tests. One universal YAM package.

```sh
cd source
go test ./...
go vet ./...
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o ../bin/station-airplay .
cd ..
upx --best --lzma bin/station-airplay # UPX 5.2.1; only the static supervisor
upx -t bin/station-airplay
python3 scripts/package.py
```

The checked-in executable is packed; `upx -d` restores it for inspection. Packing
needs no installed decompressor on speakers. Test actual execution and
`os.Executable()`-based package resolution on both kernels after changing packers.

## Native components

Prepare `AP2_BUILD_ROOT/{armhf,arm64}/sysroot` with the development packages pinned
in `third_party/airplay2-build.json`. Host tools: GCC/G++13 ARMhf cross compilers,
AArch64 GCC, CMake, autotools, make and libexpat1-dev headers. Captured firmware
ALSA/D-Bus are private link inputs, plus OpenSSL3 for AArch64; never redistribute
them. Then:

```sh
scripts/build-runtime.sh
scripts/build-airplay2.sh armhf
scripts/build-airplay2.sh arm64
```

`build-runtime.sh` builds D-Bus1.16.2 without systemd/X11 and static Mbed TLS3.6.7
crypto with `support/mbedtls-config.h`. Shairport uses RSA/AES/MD5/SHA, while
pairing/AEAD remain in libsodium. The Mbed patch fixes an unconditional OpenSSL
MD5 call in AirPlay2 identity generation and removes unused TLS/X509 link probes.
It preserves the same digest bytes; it does not implement an AirPlay protocol.
`homepod-model.patch` selects the same identity in Bonjour and `/info`.

Copy stripped ARMhf outputs to `bin/shairport-sync-ap2` and `bin/nqptp`, AArch64
receiver to `bin/shairport-sync-ap2-aarch64`. Both use Shairport5.6.2/FFmpeg7.1.3;
one ARMhf NQPTP1.2.9/smi11 serves both ABIs. Keep one ARMhf libc2.36/loader/libm/
libplist set in `lib`, plus the necessary original Avahi0.7/ALSA dependencies.
AArch64 loads firmware libc/ALSA/OpenSSL/D-Bus; only libplist is in `lib/aarch64`.
Station2 explicitly preloads firmware ALSA for its shared dmix. Legacy Light/Midi
keep their existing audio/renderers, using the common current ARMhf engine.

After rebuilding, inspect `readelf -d/-V`, actual loader resolution and library
closure. Preserve all selected codecs and legal notices. Check unit tests,
metadata/clock/RGB, lifecycle/crash recovery, native audio and multiroom; new
Light/Midi runtime upgrades require hardware validation by their owners.
