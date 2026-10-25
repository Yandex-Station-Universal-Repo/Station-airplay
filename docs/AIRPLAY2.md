# Station 2 / Mini 3 Pro: AirPlay 2 backend

Version 1.3.0 uses Shairport Sync **5.6.2** with NQPTP **1.2.9 / smi11** on
both shared-audio models. This replaces the 1.2.0 AirPlay 1 compatibility backend.
The original Plum backend is retained; its hardware was not available for this port.

| Model | Receiver ABI | Shared PCM | Output |
|---|---|---|---|
| `yandexstation_2` | ARMhf | `plug:dmix_spk` | stereo S16_LE / 48000 |
| `yandexmini_3_pro` | AArch64 | `speakers` | stereo S16_LE / 48000 |

Both kernels/supervisors are AArch64. Receiver ABI follows stock ALSA, not the
kernel. A private loader/libc runs only the receiver and timing service; ALSA
and D-Bus libraries resolve from the device. No captured device library is shipped.

The stock dmix runs at 48 kHz. Shairport 5.6 probes ALSA with resampling disabled:
44100 fails that probe on both models, although the old receiver could play it
through ALSA conversion. Pinning 44100 causes the new format search to fail.
Use 48000 and let Shairport transcode 44.1 kHz input. Do not replace dmix or stop Alice.

## Protocol and presentation

Upstream supports PTP-synchronised realtime ALAC and buffered AAC/ALAC, including
24-bit/48 kHz lossless input. These shared hardware profiles output S16_LE; input
support does not imply a 24-bit hardware path. 96/192 kHz lossless and Atmos are
unsupported upstream. See [upstream AirPlay 2 capabilities](https://github.com/mikebrady/shairport-sync/blob/5.6.2/AIRPLAY2.md).

`support/homepod-model.patch` changes only the advertised model to
`AudioAccessory1,1`, consistently in mDNS and `/info`, to retain the HomePod icon.
This is presentation compatibility; it does not turn the speaker into an Apple
HomePod or implement Siri/HomePod firmware features. Other protocol flags remain
the upstream defaults. iOS may retain the previous discovery record briefly after
an upgrade. Do not add competing mDNS publishers to force an icon refresh.

NQPTP needs exclusive UDP 319/320 and a localhost control port 9000. The supervisor
checks these before startup and owns all four children: private D-Bus, private
Avahi, NQPTP and Shairport. Runtime logs stay in bounded RAM files. An occupied
timing port is an error; another service is never killed automatically.

## Build and install

Follow [BUILD.md](../source/BUILD.md). Model-specific archives are standard YAM
packages with the same ID/version/source, narrowed compatibility and only their
required binary payload. The universal archive includes all profiles but needs
more staging space. Source archives remain in the repository rather than speaker flash.

```sh
python3 scripts/package.py --model station2
python3 scripts/package.py --model mini3pro
# Or the larger universal archive:
python3 scripts/package.py
```

Install the matching `dist/station-airplay-1.3.0-MODEL.yap` through YAM. Stop and
disable the previous receiver before installing/enabling. Keep the known-working
1.2.0 `.yap` for rollback. See [shared-model migration](SHARED_MODELS.md).

## Verification boundaries

The port was tested on the firmware versions recorded in SHARED_MODELS.md.
Both models completed transient HAP, encrypted control/event channels and PTP
SETUP. A real iPhone sent buffered AAC/48000/F24/stereo, decoded to the shared
48000/S16_LE output; the owner confirmed simultaneous playback on both.

The owner also confirmed HomePod icons after the model patch. Stop/run,
disable+stop/enable, legacy startup guard and recovery from a killed receiver
or NQPTP passed on both, without changing protected files, boot, Wi-Fi carrier
count or Alice PIDs. Go tests/vet, both architecture builds and migration tests passed.

Continuous simultaneous iPhone playback subsequently passed a 331-second
observation on both models: all 12 samples per device showed active stereo
48000/S16_LE output, with no probe errors, receiver restarts or mDNS hostname
conflicts. Protected files, boot, Wi-Fi carrier counters and Alice PIDs remained
unchanged. Recent receiver timing offsets were approximately 1.6–2 ms against
the common clock; this is software timing telemetry, not a physical acoustic
phase measurement.

**Release gate still pending:** the owner reported intermittent disappearance
and delayed connection during final iPhone testing. Sustained playback has
passed; repeated reconnection, volume and Alice still need final confirmation.
Packet capture
showed Mini answering multicast discovery and exchanging IPv6 control traffic;
its receiver and Wi-Fi carrier count remained unchanged in that observation.
The cause of the earlier intermittent behaviour is not established. Keep the
upstream PR closed until repeated discovery/reconnection passes as well.

The pyatv realtime V2 sender requests NTP, which upstream Shairport does not
implement. Its 501 response is not a valid buffered/PTP playback test. Do not
silently force a legacy fallback and report that as AirPlay 2 verification.

Cold discovery, reconnection, volume, Alice and synchronisation require hardware
checks after the final package. Build success or RTSP 200 alone proves neither
sound nor synchronisation. Lossless and surround input are upstream capabilities;
do not report them as separately hardware-tested without sending those formats.
