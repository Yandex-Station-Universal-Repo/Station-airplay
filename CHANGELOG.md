## 1.4.4

- Яндекс Станция Мини 3 (`bergamot`): shared-профиль, ARMhf receiver 5.6.2, общий PCM `speakers` 48 кГц/S16_LE, Алиса сохраняется, mDNS-хост `YandexMini3-<serial>`, иконка HomePod mini. Часы и RGB не занимаются. YAM не узнаёт `yandex,bergamot` и схлопывает `yandexmini_3` в модель Мини первого поколения `yandexmini`, поэтому стабильный id задаётся в `device.toml` как `bergamot`.

## 1.4.3

- Station Mini (`yandexmini`; BetterYAM also reports `YandexStationMini`): shared profile with the ARMhf receiver and the firmware ALSA from `/system/vendor/quasar/libs` preloaded. It plays on `softvol` (firmware `Master` -> dmix `dmixer`, 48 kHz S16_LE), so the station volume and the sender slider both apply. Alice stays up; clock and RGB are not used.
- `plug:` PCMs do not work in the receiver on the Mini: its alsa-lib loads a libsamplerate rate converter even for 48 kHz -> 48 kHz, and that module only resolves on Quasar's library path (`Unable to set hw parameters ... No such file or directory`).
- The Mini firmware has no D-Bus. The package now carries ARMhf `dbus-daemon`/`dbus-send` from the pinned dbus-1.16.2, run behind the packaged loader for profiles with `packagedDBus` only. Other models keep firmware D-Bus.
- BetterYAM binds only the LAN address and passes it as `YAM_API_URL`. The supervisor now accepts an address of one of its own interfaces as well as loopback; a remote token target is still rejected.
- The Mini's maind publishes `yandexmini-<mv_uuid>`; the id comes from the Linkplay factory file `/vendor/new_nvram_config`, not the boot serial (`nvramHostID`).

## 1.4.2

- Light 2 (`yandexplum`, `plum`) uses the Station 2 shared mixer `plug:dmix_spk` with the ARMhf receiver and firmware ALSA, so Alice stays up. Avahi reuses `YandexLite-gen2-<device id>`. YAM takes the segment clock and the 3 LEDs for the whole time the service runs and restores the clock frame on stop.

## 1.4.1

- Яндекс Станция Лайт (`yandexmicro`): shared-профиль по образцу Станции 2 — ARMhf receiver 5.6.2, общий `plug:dmix_spk` 48 кГц/S16_LE с прелоадом штатной ALSA, Алиса сохраняется, mDNS-хост `yandexmicro-<serial>`, иконка HomePod mini. Часы/RGB не управляются.

## 1.3.2

- One static ARMv7 supervisor for all supported kernels; remove duplicate AArch64 supervisor and obsolete AirPlay 1 receiver payloads.
- Share AArch64 libc/loader between Avahi and AirPlay; dynamically link namespace helper and stock OpenSSL; small FFmpeg/section-GC builds.
- Deduplicate copyright notices without changing unique texts. Ship runtime + notices; keep source/build/docs in Git. One universal YAM package/ID remains the default.

## 1.3.1

- Select HomePod for Station 2 and HomePod mini for Mini 3 Pro; preserve the original Light 2 HomePod mini identity. Keep Bonjour and `/info` consistent through per-profile receiver configuration.
- Document icon assignments and extension rules in CONTRIBUTE.md. Reconnect delay remains an open issue.

# Changelog

## 1.3.0

- Station 2 / Mini 3 Pro: настоящий AirPlay 2, Shairport Sync 5.6.2 и NQPTP 1.2.9/smi11.
- Штатные shared PCM, 48 кГц/S16_LE, Алиса сохраняется; приватные зависимости вместо замены системных.
- HomePod model advertisement, encrypted control/events, buffered AAC и PTP; модельные варианты стандартного .yap для экономии flash.
- Исходники и воспроизводимые инструкции для обеих ABI, проверка занятых timing-портов и владение четырьмя дочерними процессами.

## 1.2.0

- Единый YAM-плагин: точные профили Station 2 и Mini 3 Pro с проверенным AirPlay1, прямым shared ALSA и сохранением Алисы.
- AArch64 supervisor, ARMhf/AArch64 receivers; Plum AirPlay2 сохранён отдельно.
- Приватные D-Bus/Avahi namespace, штатный serial mDNS hostname без конфликта адресов, ограниченные RAM-логи.
- Обратимая миграция старого standalone автозапуска и общий Python packager.
- CONTRIBUTE.md для владельцев/ИИ-агентов; исходники, лицензии и provenance новых зависимостей.
## 1.2.0

- Профиль Яндекс Станции Миди: кольцо из 24 светодиодов, часы с цифрами 1..4, работа рядом с Алисой (звук через общий dmix, mDNS без конфликта с Quasar). См. `docs/MIDI.md`.
- Громкость станции через пакет `station-volume`: ползунок AirPlay выставляет громкость станции, изменения со станции показываются во время сессии.
- При работающей Алисе кольцо и часы отпускаются вне AirPlay-сессии.
- Профиль выбирается по модели из YAM; кольцо и часы Plum не изменились.

## 1.1.5

- Ускорено целевое обновление визуализатора до 20 FPS (50 мс).
- Сохранена центральная симметричная анимация и статическое имя `Я.Станция`.
- Базовый релиз для подготовки публичного репозитория; поддерживаемый профиль — Plum (Лайт 2).
