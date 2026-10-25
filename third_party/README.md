# AirPlay 2 dependencies added in 1.3.0

The complete unmodified upstream source archives for Shairport Sync 5.6.2,
NQPTP 1.2.9 and FFmpeg 7.1.3 are kept here. The only receiver patch is
`support/homepod-model.patch`. Build flags, private sysroot preparation and
runtime assembly are documented in `source/BUILD.md` and
`scripts/build-airplay2.sh`. These sources and build instructions allow
rebuilding/relinking the receiver with modified LGPL libraries.

Source origins: https://github.com/mikebrady/shairport-sync/tree/5.6.2,
https://github.com/mikebrady/nqptp/tree/1.2.9,
https://ffmpeg.org/releases/ffmpeg-7.1.3.tar.xz.
`airplay2-build.json` pins source SHA256 and every Debian build package.
Corresponding Debian source/patches are available at https://sources.debian.org/
under the recorded Source/version, with historic archives at
https://snapshot.debian.org/. `licenses/ap2-*` contains package copyright notices.
Shairport uses GPL-3.0, NQPTP GPL-2.0, the selected FFmpeg configuration LGPL-2.1+;
the distribution notices enumerate the other static and runtime library terms.
No proprietary/captured stock library is redistributed. Shipped libc/libm/loader
and libplist are the recorded Debian packages, isolated per process.

Upstream source archives remain in this repository and are omitted from the
speaker .yap to conserve flash. Keep the source and patch available alongside
any binary redistribution. The legacy Plum provenance caveat below still applies.

## Historical 1.2.0 dependencies

# Added shared-audio dependencies

The new Station 2/Mini 3 Pro profiles retain the previously hardware-tested
Shairport Sync 4.3.7 AirPlay 1 binaries. These are not builds of an AirPlay 2
receiver. Corresponding upstream source is supplied as
`shairport-sync-4.3.7.tar.gz` (https://github.com/mikebrady/shairport-sync/releases/tag/4.3.7),
with the only applied source modification and build flags documented in
`source/BUILD.md` and `scripts/build-receivers.sh`. GPL-3.0 applies to the receiver.
Stock audio/OpenSSL/libc libraries are resolved on each target; they are not
redistributed from the devices. ARMhf expects the Station 2 OpenSSL 1.1 ABI;
AArch64 expects the Mini 3 Pro OpenSSL 3 ABI. Do not assume these binaries work
on other firmware versions.

The isolated AArch64 Avahi runtime was extracted from the verified Debian
packages in `debian-runtime.json`. Source package names/versions are in its
`Source` field (otherwise same as `Package`/`Version`). Exact archive URLs are
`https://deb.debian.org/debian/` plus `Filename`; historical versions can be
retrieved from https://snapshot.debian.org/. Corresponding source and Debian
patches are available through https://sources.debian.org/ at the recorded source
versions. `licenses/` contains the distribution copyright notices. Avahi is
unmodified; the namespace helper is this project's own source in `support/`.

SHA256SUMS.json records each added runtime/receiver/source file, independently of
package release digests. Retain corresponding source, notices and rebuild
instructions when redistributing modified binaries. This metadata covers only
the new profiles; it does not resolve the original Plum binary provenance caveat
in docs/THIRD_PARTY.md.
