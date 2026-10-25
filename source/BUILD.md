# Build

Go >=1.23; no external Go modules. Run Linux tests in Linux/WSL.

```sh
cd source
go test ./...
go vet ./...
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ../bin/station-airplay .
cd ..
python3 scripts/package.py
```

One static ARMv7 supervisor runs on all three supported kernels (ARM compatibility
was checked on Station 2 and Mini 3 Pro). New profiles must verify this before
extending compatibility; kernel architecture does not determine the audio ABI.

## AirPlay 2 receiver

`scripts/build-airplay2.sh armhf|arm64` builds Shairport Sync 5.6.2 and NQPTP 1.2.9
with matching smi11, using pinned upstream archives in `third_party/` and
`AP2_BUILD_ROOT/{armhf,arm64}/sysroot` with the development packages recorded in
`third_party/airplay2-build.json`. Matching firmware ALSA, D-Bus and OpenSSL are
private link inputs (`stock-lib*.so.*`); they are never redistributed. Station 2
requires OpenSSL 1.1 headers/libs, Mini 3 Pro OpenSSL 3. FFmpeg 7.1.3 is built with
AAC/ALAC/PCM decoders only, `--enable-small`, section GC, no network/programs/docs.
Keep these sources and the model patch available when redistributing binaries.

Copy the stripped outputs to `runtime/ap2-{armhf,aarch64}/bin`. ARMhf retains its
private Debian libc/loader/libm/libplist under `runtime/ap2-armhf/lib`.
`runtime/aarch64/lib` holds the newer common libc/loader/libm used by Avahi,
namespace helper and the AArch64 receiver/timer, plus libplist and the small compatibility DSOs from the receiver
sysroot. Other Avahi dependencies are pinned in `third_party/debian-runtime.json`.
Do not copy the second libc/loader  into the package.
The original Plum binaries and libraries remain unchanged.

Build the small namespace helper against the AArch64 receiver development sysroot:

```sh
aarch64-linux-gnu-gcc --sysroot="$AP2_BUILD_ROOT/arm64/sysroot" -Os -ffunction-sections -fdata-sections -Wl,--gc-sections -Wl,-z,max-page-size=4096 -s support/avahi-namespace.c -o bin/avahi-namespace-aarch64
```

It is started with the common private loader, not the stock firmware loader.
Check `readelf -d/-V`, loader resolution and actual playback on both devices.
`python3 scripts/package.py` produces one universal YAM package/ID. Optional
`--model` filters are local flash-constrained builds, not separate registry plugins.
Packages contain runtime + notices; source, build scripts and docs stay in Git.
