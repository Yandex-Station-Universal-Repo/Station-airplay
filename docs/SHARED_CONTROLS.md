# Opt-in shared controls: voice, native volume and indication

These controls extend the existing supervisor; they are not a replacement receiver
stack, extra resident manager, log follower or global ALSA/Quasar modification.
They are disabled by default and enabled with `--controls-config`. The original
clock-equipped Plum/Midi renderer is retained. The ARMhf receiver remains
Shairport 5.6.2/AirPlay 2/smi11, with its existing D-Bus DACP interface enabled.
A short SetStreamGain lease is the only receiver extension: it attenuates stream
PCM, never Master/Alice or the sender's remembered volume.

## Runtime host authentication (Luna and legacy YAM)

The current Luna host supplies `LUNA_API_URL` and `LUNA_API_TOKEN`. AirPlay
uses that complete pair and sends **only** `X-Luna-Token`. When neither Luna
value is set, a complete `YAM_API_URL` / `YAM_API_TOKEN` pair selects the legacy
host and **only** `X-YAM-Token`.

Incomplete Luna credentials fail closed rather than falling back or combining
families. If both families are present, their endpoints (ignoring trailing
slashes) and tokens must agree; matching aliases prefer Luna. No request is
retried with another authentication header after an error. Both modes keep the
local/own-interface HTTP check, bypass environment proxies and refuse redirects.
Both `LUNA_API_*` and `YAM_API_*` are removed from audio/Avahi child environments.

This is runtime API compatibility, **not** completed YAM v2 package support.
YAM v2 is a separate package manager; it does not supply these runtime credentials
or replace Luna's speech/LED host. Packaging `.yampkg`, binary-plugin discovery,
service lifecycle and v2 station-volume dependency resolution are separate work.
The existing `.yap` and relative controls/dependency layout are unchanged here.

## YAM v2 native-volume dependency (explicit opt-in)

Set `"native_volume":true,"volume_dependency":"station-volume"` only when
running as a leased YAM binary plugin on a Luna host that implements
`GET /api/runtime/:id/dependencies/:alias`. The runtime descriptor must declare:

```toml
[dependencies]
station-volume = "station.volume"
```

The YAM source package must also declare its package-level dependency on
`station/volume` and the host capability `luna-binary-dependencies-v1`.
The alias and namespace shown here are the proposed integration layout, not
an assertion that these packages are already published in a registry.

In this mode AirPlay asks its authenticated Luna API for an enabled,
compatible, package-owned oneshot shell launcher **before every** check/get/set
and readback. It does not inspect `enabled.json`, `plugin.toml` or sibling
directories to find station-volume. Luna's caller lease prevents YAM from
removing the resolved files while the invocation is active; stop all leased
plugins before a YAM transaction.

The returned station-volume launcher must be plain `sh`, with absolute
contained paths and no fixed args or library override. The original shell
guard still implements `check-write` and guards every `set`; no `--force`,
direct native binary call or alternate protocol is introduced. Existing
timeouts, output caps, child process-group cancellation and readback stay in
effect. A disabled/missing provider or API error fails closed, with **no**
legacy filesystem fallback. Omit `volume_dependency` for the unchanged legacy
`.yap` sibling-directory resolver. Setting an alias requires native volume.

This adapter alone does not install `.yampkg`, bootstrap Luna, change existing
user config or qualify hardware. Packaging and migration are separate work.

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

## Volume handoff (1.4.12)

With native_volume enabled, a newly active AirPlay session inherits the current
station level. The service sends that level to the same live sender and waits
up to 3 seconds for acknowledgement; it never applies stale remembered phone
volume to Alice during this handover. A missing control channel defers the
handover without changing native volume. Afterwards existing bidirectional
phone/station synchronization applies. Disconnect and service stop do not
restore an earlier level: Alice retains the final native level used for AirPlay.

This is control-level synchronization, not a claim of measured loudness equality
between sources or an atomic audio/volume transaction before the first sample.
Live sender acknowledgement, reconnect and multiroom behaviour require hardware
acceptance separately from mock regression tests.

## Attachment and Station 2 indication (1.4.13)

The existing receiver GetControlStatus method reports its principal AirPlay 2
connection from initial SETUP, not only an audio play session. Use its live
event-channel/client identity to handle attachment before RECORD/audio; do not
infer attachment from retained classic DACP metadata. Volume and LEDs start
when the receiver reports that live connection, still respecting fresh Alice
idle and volume-overlay gates. This does not promise zero delay from the iOS UI
tap: protocol setup, the 500 ms control tick and speech readiness still apply.

Station 2 has a shared 89x1 framebuffer with no verified exclusive ownership
API. Do not stop platformd or race the stock renderer at a higher frame rate.
The backend reads current content, skips redundant writes and yields to foreign
frames, including a foreign clear. Off/shutdown clears only a matching frame
that this instance painted. It retries after a quiet window; native indication
has priority. A continuously active vendor animation may keep AirPlay indication
hidden. This is cooperative contention mitigation, not a proven exclusive
foreground overlay, and needs live visual acceptance on Station 2.
