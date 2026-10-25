# Shared audio profiles

| YAM model | Receiver ABI / stock OpenSSL | Shared PCM | Icon |
|---|---|---|---|
| `yandexstation_2` | ARMhf / 1.1 | `plug:dmix_spk` | HomePod |
| `yandexmini_3_pro` | AArch64 / 3 | `speakers` | HomePod mini |

Test firmware: Station 2 `0.369.1.50.4794649087.20261021.295`, Mini 3 Pro
`0.369.1.50.4795647264.20261021.107`; YAM 0.1.13. Both kernels execute the single
static ARMv7 supervisor. Audio uses 48 kHz/S16 stereo and retains Alice.
Both mixers reject 44.1 kHz when resampling is disabled in the new receiver.
Clock/RGB remain with the original Plum backend; these profiles do not touch them.

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

See [AIRPLAY2.md](AIRPLAY2.md) for protocol checks and
[CONTRIBUTE.md](../CONTRIBUTE.md) for extension/icon rules.
