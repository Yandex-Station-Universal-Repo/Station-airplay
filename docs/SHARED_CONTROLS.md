# Opt-in shared controls: voice, native volume and indication

These controls extend the existing supervisor; they are not a replacement receiver
stack, extra resident manager, log follower or global ALSA/Quasar modification.
They are disabled by default and enabled with `--controls-config`. The original
clock-equipped Plum/Midi renderer is retained. The ARMhf receiver remains
Shairport 5.6.2/AirPlay 2/smi11, with its existing D-Bus DACP interface enabled.
A short SetStreamGain lease is the only receiver extension: it attenuates stream
PCM, never Master/Alice or the sender's remembered volume.

## Reused components

- station-volume get/set/acknowledgement, with compatibility and firmware guards.
  Only a command adapter is added; YAM has no duplicate native-volume protocol.
- Shairport's own RemoteControl methods, not a second DACP implementation.
- Existing YAM authentication, Quasar framing, declared command policy and cancel.
- Existing YAM LED API for announced RGB rings. Station 2's 89x1 framebuffer is
  painted by a model/geometry-gated backend inside the supervisor because YAM has
  no framebuffer LED backend; no raw sysfs or unknown-device writes.
- Standard Go HMAC/SHA256/UDP, replacing private hard-coded peer helper paths.

The catalog audit also found LedArt's station_led.sh (including Station 2
framebuffer and firmware yield), YA Ambilight and Soloist's Spotify bridge. None is
blindly launched as an extra AirPlay manager. Station 2 support uses the same
solid-frame behaviour as the previous package, gated on YAM model
`yandexstation_2` and exact `/sys/class/graphics/fb0/virtual_size` `89,1`; any
mismatch leaves indication off. Soloist's Spotify binary is not an AirPlay
controller.

## Opt in

The package launches the service with `--shared-audio --controls-config
../../airplay-controls.json`, resolved against the plugin directory, so the file
lives in the YAM data directory next to `plugins/`. If that file is missing, the
controls stay off and the receiver still starts as an audio-only service with a
startup warning; point the flag at another absolute path to use a different file.
Example content:

```json
{"voice":true,"native_volume":false,"ducking":true,"leds":false}
```

Voice needs the companion YAM speech bridge and permissions `ipc.speech`, `commands`
with exact `[commands].phrases`. Select the phrases you want, e.g. «пауза»,
«продолжи музыку», «следующий трек», «предыдущий трек». The built-in handler also
recognises documented exact aliases, including «прошлый трек», «дальше», «далее»,
«хватит», «пропусти» и «назад», plus polite lead-ins such as «Алиса», «включи»,
«поставь» и «пожалуйста». Matching stays exact after punctuation and lead-in
normalisation, so «расскажи про прошлый трек» is not claimed. Undeclared phrases
cannot be claimed. No partial ASR, initial snapshot, unkeyed/repeated request or
different sender gets a transport command. A paused-by-voice sender may resume for
30 minutes only while the same sender's remote remains available. Other Alice
intents, including the owner's agent wake phrase, are not intercepted.

`native_volume=true` requires a compatible installed station-volume with
`check-write`. The native level wins at startup. Later phone changes go through
station-volume, read back the actual quantised volume and suppress feedback. Two
stable native changes request sender volume through Shairport; asynchronous replies
are bounded. The receiver ignores its own source attenuation only in this opt-in
mode, after preflight. No mixer writes or `--force` fallback are used. The
station-volume package guards five exact builds: Mini 2, both Lite 2 variants,
Station 2 and the previously verified Midi build. Each build also requires its own
device-tree compatible string; an unknown build or a different board is rejected
before the helper runs.

For optional peer volume, add `group={bind,peers,key_file}`. Bind and each of 1..32
peers must be explicit private IPv4 addresses, no built-in home addresses. Port
43792 is opened only when configured. The key is a separate private 32-byte file,
never embedded in packages or source. On each operation, resolve the own local live
AirPlay service (matching address/name, active flag and group UUID). Same-group
peers only; HMAC, timestamp window, nonce replay/order/range checks, bounded cache,
no rebroadcast of received events. Best effort, not a transactional group volume
protocol; membership/availability may change after the snapshot. No hardware or
real LAN group test has been performed for this candidate.

LED indication is deliberately modest. Sysfs rings use only known YAM RGB ring
counts; Station 2 paints its gated framebuffer with a bright frame while playing, a
dim frame while paused and black on release, matching the previous package. It is
emitted only after confirmed IDLE. On Alice activity the plugin stops painting
without blanking Alice; on explicit idle it can regain indication. Framebuffer
frames are repainted on state change and slowly afterwards in case a native volume
overlay replaced them. Stream duck uses renewed short gain leases, restores on
idle/timeout/exit and expires inside the receiver if control is lost. Clocks are
never touched.

## Field validation — 26 October 2026

The opt-in package was installed one station at a time with a model-specific config:

| Device | YAM model | Firmware | Validated |
|---|---|---|---|
| Mini 2 | `yandexmini_2` | `0.369.1.50.4794659041.20261021.299` | voice, sender buttons, native volume, ring |
| Lite 2 Plum | `yandexplum` | `0.369.1.50.4795647582.20261021.133` | voice, sender buttons, native volume, ring |
| Lite 2 Prune | `yandexplum` | `0.369.1.50.4794654278.20261021.133` | voice, sender buttons, native volume, ring |
| Station 2 | `yandexstation_2` | `0.369.1.50.4794649087.20261021.295` | voice, sender buttons, native volume, framebuffer |

Owner acceptance: music plays over AirPlay; transport works by voice and by the
sender's buttons; phone and station volume stay in sync in both directions;
Station 2 is bright while playing, dim while paused and black after release; the
indication clears when AirPlay stops.

Observed caveat: the first Spotify session after installation needed one reconnect
in the phone app before playback and the AirPlay icons appeared. Later sessions
connected normally. The cause was not isolated and no fix is claimed.

## Limits — do not overstate

- Multiroom with this exact build, cold boot, long soak and repeated reconnect
  cycles were not re-tested by the owner.
- The pilot used `--shared-audio` on Plum, so the legacy default clock/Midi
  renderer path was not regression-tested with the new ARMhf payload.
- Mini 3 Pro AArch64 controls are explicitly rejected until its native control
  build is qualified; its existing audio profile remains.
- Mock tests prove message handling, not audible changes. Submitted remote methods
  in mocks do not prove a real speaker changed volume.
- The packed file is roughly 8 MB; the expanded runtime budget is deliberately
  proposed as 14 MiB instead of 10 MiB, due to static GLib/DACP and its dependency
  closure. This size/maintenance tradeoff requires maintainer review, not a hidden
  test relaxation.
- The LGPL static-link corresponding-source/relink kit for new dependencies must be
  checked before any public binary redistribution. Copyright notices are retained;
  a local candidate is not a public release authorization.
- The new ARMhf receiver still needs legacy Plum/Midi regression testing.

Tests: Go race/vet, controller mocks and a real private D-Bus wire fixture, command
adapter compatibility/token/firmware fixtures, replay/HMAC tests, C gain lease
sanitizers, ARMv7/AArch64 supervisor builds, ARMhf receiver cross-build/QEMU version
and bundled loader resolution. Mock/host results are not speaker acceptance.

## Native rebuild recipe and remaining source audit

Use a disposable Debian bullseye build container, mount the repository as `/repo`
and an empty persistent build directory as `/work`. `scripts/prepare-build.sh`
installs ARMhf cross-development dependencies; `scripts/prepare-armhf-deps.sh`
records/extracts the pinned libgcrypt/libgpg-error dev inputs. Then run
`scripts/build-controls-runtime.sh armhf` without network. The script applies the
published HomePod, Mbed TLS and shared-controls patches to Shairport 5.6.2 and uses
the existing FFmpeg 7.1.3/Mbed TLS 3.6.7 archives. No firmware library is published.
`scripts/collect-runtime.py` stages only missing public ELF dependencies;
`scripts/matching-resolver.py` verifies the exact package libc SHA before adding its
matching resolver (do not mix glibc private ABIs). Inspect the closure and test the
actual packaged loader, not executable bits in a Git checkout.

The completed cross-build used a cached Debian-derived tool image. The cleaned
single-architecture preparation recipe above is included for reviewer reproduction
but has not yet been rebuilt from a pristine base image. Exact dependency source
pinning/relink material, clean-image reproduction and hardware qualification are
publication gates, not completed tests.
