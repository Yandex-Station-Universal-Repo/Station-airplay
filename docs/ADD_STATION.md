# Add a station model

Agent runbook. One profile inside this plugin. Do not add a second supervisor, a per-model package id, or a private launcher script.

Policy, secrets, and the PR rules are in [CONTRIBUTE.md](../CONTRIBUTE.md). This file is the procedure. Copy the closest row in the table. Change a field only when a command on the target speaker proves the new value.

## Stop

- Do not put a model in `plugin.toml` until `yam device` prints that exact `model` string.
- Do not write `device.toml` or pass `--model` so a different speaker looks like a supported one.
- Do not stop Quasar, `maind`, or `mediad` to make the package look successful.
- Do not open `hw:0,0` or any exclusive PCM when a shared mixer already accepts audio.
- Do not invent `mdnsHostPrefix`. A wrong prefix leaves the process running and the phone empty.
- Do not commit serials, tokens, Wi-Fi data, IPs, or full device logs.
- Do not add a receiver binary. `bin/shairport-sync-ap2` is ARMhf. `bin/shairport-sync-ap2-aarch64` is AArch64. Both are Shairport Sync 5.6.2. One ARMhf `bin/nqptp` serves both.

## Paths already in the tree

`run` in `source/profiles.go` selects the path:

| Condition | Function | What it does |
|---|---|---|
| `pcm == ""` | `runPlum` | FIFO renderer. Midi sets `pipeOutput` and plays with system `aplay`. |
| `pcm != ""` and `ui == false` | `runShared` | Direct ALSA to `pcm`. No clock, no LED. |
| `pcm != ""` and `ui == true` | `runShared` | Same mixer, plus a PCM tee and the YAM clock/LED loop. |

Shared profiles require `yam device` `architecture` `aarch64`. That is the kernel (and the arch `yamd` was built for). Userspace ABI is a separate field, `receiverABI`.

| YAM model | `pcm` | `receiverABI` | `mdnsHostPrefix` | `ui` | AirPlay `model` |
|---|---|---|---|---|---|
| `yandexstation_2` | `plug:dmix_spk` | `armhf` | `yandex-station-gen2` | false | `AudioAccessory1,1` |
| `yandexmicro` | `plug:dmix_spk` | `armhf` | `yandexmicro` | false | `AudioAccessory5,1` |
| `yandexmini_3_pro` | `speakers` | `aarch64` | `Yandex-Station-Mini-3-Pro` | false | `AudioAccessory5,1` |
| `yandexplum`, `plum` | `plug:dmix_spk` | `armhf` | `YandexLite-gen2` | true | `AudioAccessory5,1` |
| `yandexmidi` | empty | | `yandex-station-midi` | `pipeOutput` | `AudioAccessory1,1` |

Compact speakers use `AudioAccessory5,1` (HomePod mini). Full-size stations use `AudioAccessory1,1` (HomePod). The string is `airplayModel` on the profile. It is copied into `general.model`.

## 1. Read the speaker

Run these. Record the outputs in the PR. Strip serials before committing.

```sh
yam device
yam ps
id
uname -m
cat /system/vendor/quasar/version_full
cat /proc/asound/cards
cat /etc/asound.conf
tr ' ' '\n' < /proc/cmdline | sed -n 's/^androidboot.serialno=//p'
od -An -tx1 -N 20 /usr/lib/libasound.so.2
od -An -tx1 -N 20 /usr/bin/dbus-daemon
ps | grep -E 'maind|quasar_launcher' | grep -v grep
```

From a computer on the same LAN:

```sh
dns-sd -B _yandexio._tcp local.
dns-sd -L '<instance from the browse>' _yandexio._tcp local.
```

ELF byte 4 is the class. Bytes 18–19 little-endian are the machine.

| class | machine | `receiverABI` | Receiver |
|---|---|---|---|
| `01` | `28 00` | `armhf` | `shairport-sync-ap2`, preload `/usr/lib/libasound.so.2` |
| `02` | `b7 00` | `aarch64` | `shairport-sync-ap2-aarch64`, library path `lib/aarch64:/usr/lib:/lib` |

`uname -m` `aarch64` with class `01` is the Station 2 / Light / Light 2 case: AArch64 kernel, ARMhf userspace. Use `armhf`. Do not build a new AArch64 receiver for that.

## 2. Prove the mixer

Leave `maind --mediad` running. One second of silence:

```sh
dd if=/dev/zero bs=192000 count=1 | aplay -D '<candidate>' -f S16_LE -r 48000 -c 2 -t raw
echo $?
```

Candidates, in order: the `pcm.` name in `/etc/asound.conf` that is `type plug` onto a `type dmix` (`plug:dmix_spk`, `dmix_spk_plug`), then `speakers` if that PCM is a softvol or plug onto the same dmix. Use the first name that prints `APLAY:0` while `mediad` is still in `ps`.

| Result | Profile |
|---|---|
| Exit 0 | `pcm` is that name, `quasarCoexist: true`, `ui: false` unless step 3 applies. |
| Exit not 0, and `aplay -D speakers` works only with system `aplay` | `pcm` empty, `pipeOutput: true`, same pattern as Midi. Document why firmware `libasound` cannot join the dmix. |
| No shared PCM exists | Stop. Exclusive `hw:` playback drops Alice. Do it only after the owner asks for audio-only mode, and set `quasarCoexist: false`. |

44.1 kHz is not a substitute. These mixers are 48 kHz. `receiverConfig` already sets `output_rate = 48000` and `S16_LE`.

## 3. Clock and LEDs

Set `ui: true` only when all of these are true:

- `yam device` features include `display.clock.segment.v1`, `led.ring.rgb`, and `led.ring.<N>`.
- The digit layout is known. Plum digits are `0..3` with colon `DH`,`DL` (`digitBase` 0). Midi digits are `1..4` (`digitBase` 1) and stay on the pipe path.
- The owner wants the plugin to hold the display while the service runs.

`ui: true` writes `~/.asoundrc` (`uiAsound`) that tees `spk_tee` to the FIFO and to `pcm`. Shairport's device is `spk_tee`. The shared mixer name stays in the asound file, not in `output_device`. YAM takes the clock on the first `/clock` write and detaches Alice's ring trigger on the first `/led` write. `clockOff` on process exit restores Alice's saved clock frame. Do not open `/sys/class/leds` from this process.

If the speaker has no confirmed clock, leave `ui: false`. Missing LEDs must not block audio.

## 4. Hostname

`dns-sd -L` prints `can be reached at <host>.local`. `<host>` must equal `mdnsHostPrefix + "-" + androidboot.serialno`.

`stockHostname` in `source/profiles.go` does that join. The serial is the alphanumeric `androidboot.serialno` value, not the Unix hostname and not the last four characters. If the browsed host does not match that join, do not guess another prefix.

`avahiConfig` then sets `host-name` to that string, `publish-addresses=no`, and `allow-interfaces=wlan0`. Do not publish a second A record. Do not change `allow-interfaces` unless the speaker's Wi-Fi interface is not `wlan0` and discovery failed because of it.

## 5. Edit

1. Add one `stationProfile` to the `profiles` slice in `source/service.go`. `models` contains the exact `yam device` model and any alias `yam` already emits. `name` is the string the phone shows.
2. Add that model to `plugin.toml` `[compatibility].models`. Bump `version` there and the `version` constant in `source/service.go` to the same value.
3. Extend tests:
   - `TestSharedProfilesKeepProvenAudioContract` for `ui: false` (must contain `pcm`, icon, `port = 5000`, 48 kHz, stereo; must not contain `hw:0,0` or `spk_tee`).
   - `TestLight2SharesMixerAndHoldsDisplay` is the pattern for `ui: true`: receiver device is `spk_tee`, `uiAsound` slaves to the shared PCM.
   - `TestStockMDNSHostname` for the prefix.
   - `tests/test_package.py` model set.
4. Update the README model table, `CHANGELOG.md`, `docs/SHARED_MODELS.md`, and the icon table in `CONTRIBUTE.md`.

Template, `ui: false`:

```go
{name: "Shown On Phone", models: []string{"exact-yam-model"}, pcm: "plug:dmix_spk", quasarCoexist: true, mdnsHostPrefix: "prefix-from-dns-sd", receiverABI: "armhf", airplayModel: "AudioAccessory5,1"}
```

`ui: true` also sets `leds`, `digitBase`, `colon`, and `ui: true`.

Do not route a new model through `runPlum` unless `pcm` is empty. `runPlum` refuses to start while Quasar is running when `quasarCoexist` is false, and its default ALSA slave is `hw:0,0`.

## 6. Build and install

```sh
cd source
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go test -c -o /tmp/station-airplay.test .
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o ../bin/station-airplay .
cd ..
upx --best --lzma bin/station-airplay && upx -t bin/station-airplay
python3 scripts/package.py
python3 tests/test_package.py
```

`go test` on Darwin fails: `Pdeathsig` exists only for `GOOS=linux`. Compile the test binary as above and run it on the speaker, or run `go test ./...` on Linux. Pack the supervisor. `tests/test_package.py` rejects an unpacked archive over 10 MiB.

Install the `.yap` from the computer. `dist/` is gitignored; do not commit it.

```sh
yam service stop station-airplay
yam install dist/station-airplay-<version>.yap
yam service start station-airplay
yam ps
yam logs station-airplay
```

`yam service restart` while the old process is still exiting can delete the new runtime token. The log then says `invalid runtime token` and the package stops. Stop, confirm `ps` has no `station-airplay`, then start.

## 7. Done only when all of these are true

- `yam ps` shows one `station-airplay` row, `running`, and the new version.
- Log contains `model=<id> protocol=AirPlay2 pcm=<pcm> Alice=shared`.
- `ps` on the speaker shows one supervisor, one `dbus-daemon` on `/run/station-airplay/private-bus`, one `avahi-daemon`, one `nqptp`, one `shairport-sync-ap2`, and `maind --mediad` still present.
- Avahi log says `Host name is <prefix>-<serial>.local` with no `-2` suffix.
- A computer on the LAN sees `_airplay._tcp` under `name`, on port 5000, at that same host.
- The owner hears audio from a phone. `aplay` exit 0 is not that check.
- With `ui: true`, clock segments for the current time are non-zero and the ring trigger is `[none]` while the service runs. After `yam service stop`, the clock returns to Alice.
- `yam service stop` leaves no `shairport`, `nqptp`, or plugin `avahi` process.

## Failures

| Symptom | Look at |
|---|---|
| Package running, phone sees nothing | Avahi host differs from the `_yandexio` host, or the log says `Host name conflict`. |
| `invalid runtime token` right after restart | Stop, wait until the process is gone, start again. |
| `Quasar/Alice is running and may own ALSA` | Profile took `runPlum` with `quasarCoexist` false. Shared models must set `pcm`. |
| `snd_interval_empty` or `Exec format error` | Receiver opened `hw:0,0` or loaded the wrong-ABI `libasound`. Armhf shared launch must preload `/usr/lib/libasound.so.2`. |
| Supervisor exits after five restarts | A child is crashing. Read `/run/station-airplay/shairport-sync.log`. YAM does not restart a dead supervisor. |
| Clock or ring never moves | `ui` is false, or `yam device` lacks `display.clock.segment.v1` / `led.ring.<N>`. |
