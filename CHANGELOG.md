## 1.4.4 (не выпущено)

- Проверки готовности приватного D-Bus, Avahi и nqptp ждут до 60 с,
  учитывают отмену и завершение дочерних процессов; ошибка называет этап
  и последнюю причину вместо общего таймаута. Зависимости не менялись.

- Точный профиль Mini 2 (`yandexmini_2`): ARMhf, штатный PCM `default`, иконка
  HomePod mini; Алиса продолжает работать.
- Явный `--shared-audio` для Лайт 2 (`plum`, `yandexplum`): общий PCM `speakers`
  без часов и RGB, даже если YAM сообщает сегментные часы. Профиль по умолчанию
  (`plug:dmix_spk` с часами и 3 RGB) не изменён; без сегментных часов выбирается
  общий профиль без UI. Неизвестные модели по-прежнему отклоняются.
- Необязательная общая метка YAM `room` добавляется к имени общего приёмника;
  обрезка по границе UTF-8 в пределах лимита DNS; управляющие символы и
  некорректный UTF-8 отклоняются. Без поля имя прежнее.
- Регрессионные тесты профилей и комнат; обновлены проверки модели в пакете.
- CI собирает оба supervisor, упаковывает статический ARMv7 проверенным UPX
  5.2.1 и проверяет единственный пакет; убраны нерабочий смешанный шаг и
  устаревшие проверки OpenSSL.
- Пройдены lifecycle/discovery на Mini 2, двух Lite 2 и Station 2; живая
  приёмка звука, multiroom, переподключение и холодная загрузка — за владельцем.
- Опциональный слой управления (`--controls-config`): голосовой транспорт через
  мост речи YAM, кнопки отправителя через штатный DACP, двусторонняя нативная
  громкость через station-volume, потоковое приглушение с возобновляемыми lease
  и ограниченная индикация, включая framebuffer Станции 2.
- Переиспользованы существующий supervisor, DACP и station-volume: нет второго
  менеджера, глобального `LD_PRELOAD`, патча Quasar и замены системного ALSA.
- Пакет включает opt-in управления по умолчанию: `permissions` с `ipc.speech` и
  `commands`, `args` с `--shared-audio --controls-config
  ../../airplay-controls.json` и список `[commands].phrases`. Пропавший файл
  конфигурации отключает управление с предупреждением, а не роняет приёмник.
- Готовность нативной громкости на холодной загрузке проверяется с
  ограниченным ретраем (до 60 с, шаг 1 с). yamd стартует плагин раньше
  `mediad` (плагин T+16,2 с, `mediad` T+16,5…24 с), поэтому однократная
  проверка завершала супервизор и оставляла станцию без приёмника до ручного
  перезапуска. Ошибка `station-volume` теперь содержит stderr зависимости.
- Приёмка пилота 26.10.2026 (голос, кнопки, громкость, индикация) относится к
  пилотной сборке; бинарники этого изменения пересобраны и требуют отдельной
  живой приёмки. ARM64 controls, multiroom, холодная загрузка и длительный soak
  не проверены.

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
