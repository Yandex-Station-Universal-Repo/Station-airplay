# Shared audio profiles

| YAM model | Native audio ABI | Shared PCM | Icon |
|---|---|---|---|
| `yandexstation_2` | ARMhf | `plug:dmix_spk` | HomePod |
| `yandexmini_3_pro` | AArch64 | `speakers` | HomePod mini |
| `yandexplum`, `plum` (Лайт 2) | ARMhf | `plug:dmix_spk` | HomePod mini |
| `yandexplum`, `plum`, `--shared-audio` | ARMhf | `speakers` | HomePod mini |
| `yandexmini_2` (Мини 2) | ARMhf | `default` | HomePod mini |
| `yandexmicro` (Лайт) | ARMhf | `plug:dmix_spk` | HomePod mini |
| `yandexmini`, `YandexStationMini` (Мини) | ARMhf | `softvol` | HomePod mini |

Test firmware: Station 2 `0.369.1.50.4794649087.20261021.295`, Mini 3 Pro
`0.369.1.50.4795647264.20261021.107`; YAM 0.1.13. Both kernels execute the single
static ARMv7 supervisor. Audio uses 48 kHz/S16 stereo and retains Alice.
Both mixers reject 44.1 kHz when resampling is disabled in the new receiver.
Station 2, Mini 3 Pro and Lite do not touch clock or RGB. Light 2 plays through the same shared mixer and, while the service is running, YAM holds its segment clock and 3 LEDs. The hostname maind already publishes is `YandexLite-gen2-<device id>`.

## Mini 2 and the Lite 2 shared-audio opt-in

Mini 2 (`yandexmini_2`) uses the firmware `default` PCM, which already mixes
through Master softvol/dmix next to Alice. It is an ARMhf profile on an AArch64
kernel and does not take clock or RGB.

Light 2 already shares `plug:dmix_spk` with Alice and, while the service runs,
YAM holds its segment clock and 3 LEDs. `--shared-audio` selects an audio-only
profile instead: shared PCM `speakers`, no clock and no RGB. Clock capability is
not a reliable selector — both tested Lite 2 variants advertise segment clocks —
so the audio-only profile is chosen by the explicit option, or automatically when
YAM reports no segment clock. Unknown models still fail closed.

Do not replace system ALSA, patch Quasar, change rootfs or use an exclusive
hardware PCM. One supervisor/native engine lifecycle remains owned by YAM: no
per-model second manager, former private receiver service or old runtime
directory is called. Rooms are global YAM metadata, not this option; see
[ROOMS.md](ROOMS.md).

### Actual software validation, 26 October 2026

| Hardware | Firmware | Validated |
|---|---|---|
| Mini 2 | `0.369.1.50.4794659041.20261021.299` | ARMhf load, start/stop, port retirement, RTSP/discovery, room name |
| Lite 2 Plum | `0.369.1.50.4795647582.20261021.133` | `--shared-audio`, Alice process retained, same checks |
| Lite 2 Prune (YAM reports `yandexplum`) | `0.369.1.50.4794654278.20261021.133` | same checks; no invented Prune model/capabilities |
| Station 2 | `0.369.1.50.4794649087.20261021.295` | same checks, existing Station 2 profile unchanged |

Local build used Shairport 5.6.2/smi11 and NQPTP 1.2.9/smi11. The packed ARMv7
supervisor ran on all four AArch64 kernels with ARMhf userspace. `/info` returned
the profile's HomePod/HomePod mini model. After disable, owned children and
timing/RTSP ports retired; re-enable recovered discovery. Native configs/library
hashes, Alice/Jarvis process identities, Master and read-only rootfs stayed
unchanged. Room metadata persisted through a daemon restart, independently of the
package.

**This is not listening acceptance of this commit.** The binary in this PR is
rebuilt from the rebased source; the listening checks above belong to the pilot
build. Real buffered music, Alice during playback, volume, multiroom, reconnect
and phone icon caching still need an owner run on the exact package built here.
No cold reboot was performed. Neither RTSP200 nor an advertised model proves
audio, synchronization, lossless or 24-bit output.

## Install / rollback

Build per [BUILD.md](../source/BUILD.md), install the universal `.yap` through YAM,
then enable `station-airplay`. With closed WAN, transfer the package from the PC.
Do not change network/OTA policy or system ALSA/D-Bus/accounts.

Stop any previous receiver and its autostart first. For the previously tested
`/data/local/airplay-station2` and `/data/local/airplay-mini3pro` layouts, the
repository contains `support/migrate-shared.sh apply MODEL` / `rollback MODEL`;
read it before applying. It preserves backups and guards the old launcher.
YAM stop and disable are separate operations: disable+stop before reinstalling or
restoring an older package. Keep the previous tested package for rollback.

## Validation / limits

The 1.3.1 baseline passed real iPhone buffered AAC/48 kHz playback on both and
simultaneously, 331 seconds continuous observation, receiver/timer crash recovery,
stop/start and protected-state comparisons. The owner reported no audible dropouts
or noticeable desync. These results do not automatically validate a later rebuild.

Known baseline issue: reconnection may take 20–25 seconds, Mini first then Station 2;
earlier iOS connection errors and intermittent disappearance remain unexplained.
One event-channel RTSP 400 was received from iPhone before a successful retry.
The sender is jailbroken with tweaks; its contribution has not been isolated.
No measured cause or fix is claimed. Lossless/surround input, 24-bit DAC output,
Plum hardware and a full reboot after the latest package are unverified.

Lite (`yandexmicro`), firmware `0.369.1.46.4778262442.20261013.297`, YAM latest
release (2026-10): 1.4.1 runs the static ARMv7 supervisor on the AArch64 kernel,
uses the ARMhf receiver with firmware ALSA preloaded on `plug:dmix_spk`, publishes
via the stock `yandexmicro-<serial>` host. Owner confirmed audible iPhone playback
with Alice still running. The YAM installer adds no boot hook on Lite; with an
owner-added `/etc/init.d/S99zyamd` (start-stop-daemon, same command as the
installer) yamd and the plugin came back after two reboots. Long-run stability and
multiroom are unverified.

Mini (`yandexmini`, Amlogic AXG, Linkplay-based firmware
`1.305.4.16.3237824040.20250721`), BetterYAM 0.1.18: 1.4.3 runs the static ARMv7
supervisor on the AArch64 4.9 kernel and the ARMhf receiver with the firmware
ALSA from `/system/vendor/quasar/libs` preloaded. Quasar plays on `default` =
plug -> softvol `Master` -> asym -> dmix `dmixer` (hw:0,0, 48 kHz S16_LE);
`/etc/asound.conf` is stale and the live config comes from `/tmp/alsa/asoundrc`.
The receiver opens `softvol` directly. Every `plug:` PCM fails in the receiver
with `Unable to set hw parameters ... No such file or directory`: this alsa-lib
inserts a libsamplerate converter even for 48 kHz -> 48 kHz and the module only
resolves on Quasar's `LD_LIBRARY_PATH`. System `aplay` passes the same PCM because
it runs with that path, so probe in the receiver's loader environment. The station
volume therefore scales AirPlay; the sender slider applies on top of it.

The firmware has no `dbus-daemon`/`dbus-send`; the profile runs the packaged
pair. maind publishes `yandexmini-<mv_uuid>` (`mv_uuid` in the Linkplay factory
JSON `/vendor/new_nvram_config`, the value of the firmware `device_id` tool); the
boot/CPU serial is a different id. Do not read `/sys/class/efuse` or
`/sys/class/unifykeys` while looking for it: a plain read froze the speaker until
it was power-cycled.

BetterYAM binds only the LAN address unless told otherwise and passes that address
as `YAM_API_URL`. The supervisor accepts it because it belongs to one of the
speaker's own interfaces; any other non-loopback host is still rejected.
Upstream yamd 0.1.18 does not recognise this Mini (no ring LEDs in sysfs, device
tree only says `Amlogic`). The kernel logs `syscall 403` (`clock_gettime64`) from
the glibc 2.36 runtime; glibc falls back and playback is unaffected.

Owner confirmed audible iPhone AirPlay 2 playback with Alice still running; a
RAOP test tone from a Mac also played. Stop leaves no plugin children; after a
full reboot with the stock BetterYAM init script, the plugin and Alice came back
without intervention.
Long-run stability, multiroom and simultaneous playback with other stations are
unverified.

See [AIRPLAY2.md](AIRPLAY2.md) for protocol checks and
[CONTRIBUTE.md](../CONTRIBUTE.md) for extension/icon rules.
