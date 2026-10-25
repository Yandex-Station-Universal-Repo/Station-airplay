# AirPlay 2 runtime

All models use upstream Shairport Sync **5.6.2**, FFmpeg **7.1.3** and one NQPTP
**1.2.9/smi11**. There is one source engine, compiled for the two native audio
ABIs: ARMhf for Light2/Midi/Station2 and AArch64 for Mini3Pro.

One packed static ARMv7 supervisor, one original Avahi0.7 daemon, one private
ARMhf libc and one small namespace helper replace the parallel runtimes.
The AArch64 receiver uses firmware libc/ALSA/D-Bus/OpenSSL and packaged libplist.
ARMhf uses minimal static Mbed TLS crypto; no OpenSSL library is shipped.
D-Bus is rebuilt without systemd, removing its unrelated dependency chain.
No firmware library is redistributed and no receiver is downloaded at runtime.

Station2/Mini3Pro retain direct shared ALSA **48kHz/S16_LE stereo** and Alice.
A single ARMhf receiver with external aplay was rejected for these profiles:
it would remove native ALSA latency feedback and weaken multiroom synchronization.
Midi retains the upstream FIFO/aplay path and renderer; Light2 retains its PCM/UI.

The supervisor owns its children, checks ports, bounds RAM logs, retries crashes
and keeps YAM tokens private. Private D-Bus/mount namespace preserve global system
accounts and configuration. Build and package: [BUILD.md](../source/BUILD.md).
One deterministic `.yap`, one YAM ID; no model-specific archive generator.

Station2/Midi advertise HomePod; Mini3Pro/Light2 advertise HomePod mini through
`general.model`, consistently in Bonjour and `/info`. No HomePod/Siri feature
capabilities are added. [CONTRIBUTE.md](../CONTRIBUTE.md) documents profile/icon rules.

## Verification boundaries

1.3.2 owner confirmed both playing and distinct icons. Initial1.4.0 unified-runtime
build passed lifecycle/crash/HAP/PTP on both; owner confirmed Mini audio/Alice.
Fresh results for the final dependency rebuild are reported in PR#2. Baseline
331-second continuous playback is evidence for1.3.1, not every subsequent build.

**Known issue remains:** this owner's iPhone may take20–25s to reconnect; earlier
connection errors/disappearance have no established root cause. Jailbreak/tweak
involvement is unproven. No reconnection fix is claimed by size optimization.
Lossless/surround,24-bit DAC output, Light2/Midi hardware and full reboot after
this upgrade are unverified. Pyatv HAP/PTP setup alone is not an audio test.
