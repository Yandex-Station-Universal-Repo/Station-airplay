## Local build (Go supervisor only)

Requires Go >=1.23 on Mac/Linux; no external Go modules.

## Multi-model package (1.2.0)

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
patch changes the advertised `am=ShairportSync` to `am=AudioAccessory1,1`.
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
