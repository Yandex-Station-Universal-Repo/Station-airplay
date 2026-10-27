# Components and provenance

Pinned sources: Shairport Sync5.6.2 (GPL3), NQPTP1.2.9 (GPL2), FFmpeg7.1.3
(selected LGPL2.1+ codecs), Mbed TLS3.6.7 (Apache2 OR GPL2+) and D-Bus1.16.2
(GPL2+ OR AFL2.1). Archives/hashes are in this directory; corresponding
build commands and patches are in [BUILD.md](../source/BUILD.md) and `support/`.

Sources: https://github.com/mikebrady/shairport-sync/tree/5.6.2,
https://github.com/mikebrady/nqptp/tree/1.2.9,
https://ffmpeg.org/releases/ffmpeg-7.1.3.tar.xz,
https://github.com/Mbed-TLS/mbedtls/releases/tag/mbedtls-3.6.7,
https://dbus.freedesktop.org/releases/dbus/dbus-1.16.2.tar.xz.

`airplay2-build.json` records development packages; `SHA256SUMS.json` hashes the
current binaries/libraries/notices/sources. `licenses/` retains unique notices.
Original identical architecture copies were deduplicated without losing text.
Source archives/build/docs remain in Git rather than the speaker payload.
Firmware libraries are private link inputs, never redistributed.

Original Avahi0.7 and selected legacy ARMhf libraries are reused, not duplicated;
their pre-existing rebuild-provenance limitations remain in [THIRD_PARTY.md](../docs/THIRD_PARTY.md).
UPX5.2.1 packs only the static Go supervisor; no UPX runtime/tool is bundled.

`bin/dbus-daemon` and `bin/dbus-send` (Mini, 1.4.3) are D-Bus1.16.2 from the pinned
archive with the `build-runtime.sh` options, cross-compiled by zig cc0.16 for
`arm-linux-gnueabihf` (glibc symbols <=2.34, no RUNPATH, `-ffile-prefix-map`).
They load the packaged `libdbus-1`/`libexpat`/libc. `build-runtime.sh` rebuilds
them with the GCC sysroot.

## Shared controls runtime (proposed)

The controls PR adds an ARMhf Shairport Sync 5.6.2 receiver built from the pinned
archive in this directory with the published HomePod, Mbed TLS and shared-controls
patches in `support/`. `controls-runtime-build.json` records the exact build inputs
and its experimental status. `scripts/build-controls-runtime.sh` rebuilds it;
`scripts/collect-runtime.py` stages only missing public ELF dependencies and
`scripts/matching-resolver.py` checks the packaged libc SHA before adding its
matching resolver.

The ten added ARMhf libraries (avahi-client, blkid, config, mount, pcre2-8, popt,
resolv, selinux, sodium, uuid) are the reviewed Debian buster set that matches the
upstream avahi-common/core ABI; their notices are under
`LICENSES/controls-runtime`. The upstream D-Bus, Avahi and Expat binaries are
unchanged by this PR. ARM64 controls remain unqualified, and the LGPL static-link
corresponding-source/relink kit for new dependencies is a separate publication
gate, not implied by this review build.
