# AirPlay 2: Station 2 and Mini 3 Pro

Shairport Sync **5.6.2**, NQPTP **1.2.9 / smi11**. The original Plum backend is
retained. [SHARED_MODELS.md](SHARED_MODELS.md) records firmware, PCM and rollback.

## Runtime

One static ARMv7 Go supervisor runs on all supported kernels. Station 2 uses an
ARMhf receiver, Mini 3 Pro AArch64, following stock ALSA rather than kernel ABI.
One common AArch64 loader/libc serves Avahi, namespace helper and Mini's receiver;
Station 2 retains its ARMhf libc/loader. Stock ALSA/OpenSSL are loaded from firmware,
and a private D-Bus/Avahi instance advertises without modifying system accounts,
D-Bus or the read-only root. No captured firmware library is redistributed.

Output is shared stereo **48 kHz/S16_LE**, retaining Alice. The new receiver probes
ALSA with resampling disabled; both mixers reject 44.1 kHz in that probe.
Upstream supports buffered AAC/ALAC and PTP realtime ALAC, including lossless
24-bit/48 kHz input; the output profile is still S16_LE. 96/192 kHz/Atmos are not
claimed. See [upstream capabilities](https://github.com/mikebrady/shairport-sync/blob/5.6.2/AIRPLAY2.md).

NQPTP owns UDP319/320 and localhost9000. The supervisor checks occupied ports,
stops only its four children, retries their unexpected exits and bounds RAM logs.
The namespace helper dynamically uses the common loader; it never changes global
passwd/group. The source-only migration helper guards previous standalone installs.

## Build / package

Follow [BUILD.md](../source/BUILD.md). `python3 scripts/package.py` creates one
universal `.yap`/plugin ID. Optional local `--model` filters conserve flash; they
are not separate YAM registry entries. Runtime + license notices are packaged;
source/build/docs remain in Git. Keep the previous tested package for rollback.

1.3.2 removes the duplicate supervisor, obsolete AP1 receivers and duplicate
AArch64 libc. Receivers use firmware OpenSSL, small FFmpeg and linker section GC.
Each distinct third-party copyright notice is preserved byte-for-byte once.
The user's reference ap2-min kit uses older 4.3.7/FFmpeg4; its dependency-pruning
idea was compared, while this port retains 5.6.2/FFmpeg7 and both tested ABIs.

## Presentation / checks

`general.model` consistently controls Bonjour and `/info`: Station2 HomePod,
Mini3Pro HomePod mini. Light2's original HomePod mini identity is unchanged.
This adds no Siri/HomePod firmware capability bits. [CONTRIBUTE.md](../CONTRIBUTE.md)
contains the full icon mapping and extension rules. iOS may cache the old icon.

1.3.1 baseline: real iPhone AAC/48k playback on both/simultaneously,331s continuous
observation without restarts, owner heard no interruptions/desync. 1.3.2: startup,
stop/run, disable+stop/enable, legacy guard and receiver/NQPTP crash recovery pass
on both; protected files/boot/Wi-Fi carrier/Alice PIDs remain unchanged. Actual
playback after the new rebuild requires a fresh owner check.

Known issue: iPhone reconnection can take20–25s; earlier errors/disappearance are
unexplained. Captures showed a later Station2 control connection and an event
response400 from iPhone. Cause is unproven, including any jailbreak/tweak effect.
PR is open at the owner's request with this limitation disclosed.
The pyatv realtime NTP/501 path is not a buffered/PTP audio test. Lossless/surround,
24-bit DAC output, Plum hardware and full reboot are not claimed tested.
