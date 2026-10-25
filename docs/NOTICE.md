# Attribution, integrity and support boundary

- `bin/shairport-sync-ap2`: Shairport Sync 4.3.7, original Plum patched binary (`plum-volkeys1`), Mike Brady and contributors, GPL-3.0-or-later (check original upstream `COPYING`). Original 4.3.7 source, patches, build records in `plum-airplay-stable-v1.1.3-full.zip`.
- `bin/nqptp`: NQPTP 1.2.4/smi10, Mike Brady and contributors, license per upstream; from verified stable runtime.
- `bin/avahi-daemon`: Avahi 0.7, upstream https://github.com/avahi/avahi.
- `lib/`: 28 ARMhf libraries from verified stable runtime. Individual notices apply. FFmpeg 4.1.9 stripped audio-only build under LGPL-2.1-or-later configuration. Other libs carry own licenses.
- `bin/station-airplay`, `source`: new adaptation of local `airplayd 1.1.3-plum` metadata/audio/display logic, GPL-3.0-or-later.
- YAM itself is a separate project https://github.com/nyakokitsu/yam. This `.yap` was adapted to its public package format and runtime API; no YAM binaries bundled.

The old C binaries remain intact. This package is not affiliated with Yandex or Apple. No proprietary HomeKit implementation and no hardware-button input handling are included.
