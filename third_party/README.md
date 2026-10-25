# Third-party components

Unmodified upstream source archives are kept here for Shairport Sync 5.6.2,
NQPTP 1.2.9 and FFmpeg 7.1.3. `airplay2-build.json` pins source/package digests;
`debian-runtime.json` records Avahi runtime packages. Origins:
https://github.com/mikebrady/shairport-sync/tree/5.6.2,
https://github.com/mikebrady/nqptp/tree/1.2.9,
https://ffmpeg.org/releases/ffmpeg-7.1.3.tar.xz.

The receiver's only source patch is `support/homepod-model.patch`.
[source/BUILD.md](../source/BUILD.md) and `scripts/build-airplay2.sh` describe
rebuilding/relinking with modified libraries. Source archives stay in Git, off
speaker flash. Keep sources, patch and notices available with redistribution.
Shairport Sync is GPL-3.0, NQPTP GPL-2.0, selected FFmpeg LGPL-2.1+.

`licenses/` keeps each distinct package copyright notice byte-for-byte once.
`licenses/INDEX.json` maps original package/architecture notice names to those
canonical copies; no unique notice text is removed. Exact Debian archive URLs
are `https://deb.debian.org/debian/` + `Filename` in the manifests; corresponding
source/patches are at https://sources.debian.org/ and https://snapshot.debian.org/.
Matching firmware ALSA/D-Bus/OpenSSL files are link inputs only, not distributed.
One common AArch64 libc/loader serves Avahi and receiver; ARMhf has its own.
`SHA256SUMS.json` records the current added runtime/source files.

The original Plum binaries/libraries are retained. The 4.3.7 source archive is
retained for reference; exact Plum rebuild provenance remains subject to the
existing caveat in [THIRD_PARTY.md](../docs/THIRD_PARTY.md).
