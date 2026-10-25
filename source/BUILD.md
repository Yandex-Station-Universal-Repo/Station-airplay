## AirPlay 2 receivers (Station 2 / Mini 3 Pro, 1.3.0)

Pinned source archives are in `third_party/`: Shairport Sync 5.6.2, NQPTP 1.2.9
and FFmpeg 7.1.3. `airplay2-build.json` records source digests and exact Debian
bookworm build package versions/archive digests for ARMhf and ARM64.

Prepare a writable directory with `armhf/sysroot` and `arm64/sysroot`, extracting
the listed Debian development/runtime packages (`dpkg-deb -x`). Provide private
link-only `armhf/stock-libasound.so.2`, `armhf/stock-libdbus-1.so.3` and ARM64
equivalents from the target firmware. Never distribute these captured files.
Install matching GNU cross compilers, autotools, make, pkg-config, xxd, plistutil.

```sh
AP2_BUILD_ROOT=/absolute/build scripts/build-airplay2.sh armhf
AP2_BUILD_ROOT=/absolute/build scripts/build-airplay2.sh arm64
```

The script normalises sysroot symlinks, builds minimal static FFmpeg and crypto,
applies the documented HomePod model patch, and produces stripped binaries in
`BUILD/ABI/output`. Runtime payloads are `runtime/ap2-armhf` and
`runtime/ap2-aarch64`: bin/{shairport-sync,nqptp}, lib/{private loader,libc.so.6,
libm.so.6,libdl.so.2,libpthread.so.0,librt.so.1,libplist-2.0.so.3}. Resolve the
matching Debian sysroot files when assembling these directories. Confirm ELF
NEEDED/GLIBC versions and run each binary with its private loader on the device.
Stock ALSA and D-Bus must resolve from /usr/lib:/lib. No host runtime files or
device dumps should enter the package. Rebuild Go and namespace helper below,
then `python3 scripts/package.py --model station2` or `--model mini3pro`.

## Local build (Go supervisor only)

Requires Go >=1.23 on Mac/Linux; no external Go modules.

## Go supervisor and existing runtime

Build on Linux, or cross-compile Go on Windows with GOOS=linux and run Linux
tests in WSL. The C helper is outside the Go package and never requires cgo.

```sh
cd source
go test -count=1 ./...
go vet ./...
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ../bin/station-airplay .
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ../bin/station-airplay-aarch64 .
cd ..
aarch64-linux-gnu-gcc -static -Os -Wall -Wextra -Werror support/avahi-namespace.c -o bin/avahi-namespace-aarch64
python3 scripts/package.py
```

The launcher selects the supervisor architecture; the supervisor selects the
receiver ABI from the exact YAM model. Station 2 has an AArch64 kernel with an
ARMhf stock audio userspace; Mini 3 Pro uses an AArch64 receiver. Their receiver
source is included in `third_party/shairport-sync-4.3.7.tar.gz`; the only source
historical patch changed `am=ShairportSync` to `am=AudioAccessory1,1`.
Current AirPlay 2 receivers use the configurable `general.model` patch: Station 2
is `AudioAccessory1,1`, Mini 3 Pro is `AudioAccessory5,1`. The original Plum
receiver already advertises `AudioAccessory5,1` and is retained unchanged.
`scripts/build-receivers.sh` documents cross-build flags and accepts separate
matching development sysroots. Use stock-library symlinks in those sysroots
and readelf to verify the final ABI/GLIBC/OpenSSL requirements before deployment.
The bundled native receivers are the previously device-tested binaries; the
cross-build script is a rebuild recipe, not a claim of bit-for-bit reproduction.

`runtime/aarch64` is the minimal Avahi dependency closure from Debian packages,
with an explicit private loader. Package versions, URLs and archive SHA256 are
listed in `third_party/debian-runtime.json`; file SHA256 are recorded separately.
Consult `third_party/README.md` for source/licensing details. Never copy a whole
device filesystem, passwords or private configuration into a contribution.

```sh
cd source
go test -count=1 ./...
go vet ./...
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ../bin/station-airplay .
```

Binaries for Shairport Sync 4.3.7 and NQPTP smi10 were recovered byte-for-byte from verified stable release; rebuilding requires separate ARM hard-float toolchain/glibc 2.28 sysroot and the source/patch history in the full stable archive. The YAM package does not compile these C components on the speaker.
