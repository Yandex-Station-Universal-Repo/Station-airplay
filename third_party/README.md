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
