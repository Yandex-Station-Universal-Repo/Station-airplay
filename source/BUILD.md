## Local build (Go supervisor only)

Requires Go >=1.23 on Mac/Linux; no external Go modules.

```sh
cd source
go test -count=1 ./...
go vet ./...
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ../bin/station-airplay .
```

Binaries for Shairport Sync 4.3.7 and NQPTP smi10 were recovered byte-for-byte from verified stable release; rebuilding requires separate ARM hard-float toolchain/glibc 2.28 sysroot and the source/patch history in the full stable archive. The YAM package does not compile these C components on the speaker.
