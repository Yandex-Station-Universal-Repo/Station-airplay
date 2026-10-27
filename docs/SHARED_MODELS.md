# Shared audio profiles

| YAM model | Native audio ABI | Shared PCM | Icon |
|---|---|---|---|
| `yandexstation_2` | ARMhf | `plug:dmix_spk` | HomePod |
| `yandexmini_3_pro` | AArch64 | `speakers` | HomePod mini |
| `bergamot` (Мини 3) | ARMhf | `speakers` | HomePod mini |
| `yandexplum`, `plum` (Лайт 2) | ARMhf | `plug:dmix_spk` | HomePod mini |
| `yandexmicro` (Лайт) | ARMhf | `soft_volume` | HomePod mini |
| `yandexmini`, `YandexStationMini` (Мини) | ARMhf | `softvol` | HomePod mini |

Test firmware: Station 2 `0.369.1.50.4794649087.20261021.295`, Mini 3 Pro
`0.369.1.50.4795647264.20261021.107`, Mini 3 `0.369.1.50.4794652521.20261021.117`.
Station 2 and Mini 3 Pro were checked with YAM 0.1.13. Those kernels execute the single
static ARMv7 supervisor. Audio uses 48 kHz/S16 stereo and retains Alice.
Both mixers reject 44.1 kHz when resampling is disabled in the new receiver.
Station 2, Mini 3, Mini 3 Pro and Lite do not touch clock or RGB. Light 2 plays through the same shared mixer and, while the service is running, YAM holds its segment clock and 3 LEDs. The hostname maind already publishes is `YandexLite-gen2-<device id>`.

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

A second Lite (`Yandex Station Micro v1.2` device tree, same `.46` firmware), fresh
YAM 0.1.18: YAM detects `yandexmicro`. The profile runs on firmware D-Bus and on
`/usr/lib/libasound.so.2` (the library `maind --mediad` maps). Avahi takes the stock
`yandexmicro-<serial>` host without a `-2` suffix. iPhone AirPlay 2 playback works
with Alice running, and `yam service stop` leaves no children or ports. YAM 0.1.18's
installer creates `/etc/init.d/S99yam`; after a reboot yamd, the plugin and Alice came
back on their own. Binary packages run only after `allow_plugin_binaries` is enabled
(a web UI switch). After the service was restarted while the iPhone was streaming to
it, the iPhone stopped listing the Lite. The Mac still saw the same advert, with
`pk`/`pi`/`deviceid` unchanged. The Lite came back on the iPhone after the speaker
rebooted. The cause was not isolated.

Since 1.4.5 the Lite plays on the firmware `soft_volume` PCM, the same chain Alice
uses: softvol `Master Playback Volume` -> `dmix_spk_plug` -> `dmix_spk`. Alice sets
the station volume only on that softvol control (`set_volume.sh` runs
`amixer set 'Master' N%`). With
`plug:dmix_spk` AirPlay bypassed it: the sender slider ran 0–100 % of full scale
and Alice became nearly inaudible next to the music. Now the station volume is
the ceiling and the sender slider works inside it, as on the Mini. The owner confirmed
this by ear.

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

Mini 3 (`bergamot`, firmware `0.369.1.50.4794652521.20261021.117`): the ARMhf
receiver plays on the shared `speakers` mixer with firmware ALSA from
`/usr/lib/libasound.so.2`. maind publishes `YandexMini3-<serial>`. Clock and RGB
stay with Alice. Current yamd reports `unknown` for `yandex,bergamot` and rewrites
an id containing `yandexmini`, including `yandexmini_3`, to the first-generation
Mini `yandexmini`. The stable id is `bergamot` in `device.toml`. Owner confirmed
iPhone AirPlay 2 playback, the phone volume slider, and Alice answering while the
stream plays.

See [AIRPLAY2.md](AIRPLAY2.md) for protocol checks and
[CONTRIBUTE.md](../CONTRIBUTE.md) for extension/icon rules.
