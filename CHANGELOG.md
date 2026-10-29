## 1.4.13

- Handle receiver-confirmed AirPlay 2 attachment before audio starts: inherit
  native volume and show connected indication without waiting for playback.
  Always query the existing GetControlStatus API; its live principal event
  client is authoritative over stale/empty classic Client metadata. Classic
  Available alone does not create a connected session.
- Station 2 framebuffer output is cooperative: retain one descriptor, read
  before painting, do not periodically rewrite an already displayed frame,
  yield to foreign frames/clears, debounce return after native animation, and
  clear only our own exact frame on stop. No suspension of platformd/Quasar.
- Station 2's connected/not-playing state uses the existing dim paused frame;
  sysfs models keep their existing ring colour. Alice speech/volume gates stay.
- Owner live-tested all four on 1.4.12; remaining reported issues were delayed
  handoff/indication until playback and Station 2 native/AirPlay LED contention.
  Do not count that as acceptance of this new build or of Station 2 LED output.

## 1.4.12

- Native volume is the initial source of truth on AirPlay activation: publish
  the current station level to the sender instead of only saving a baseline.
  Wait for its control channel and volume acknowledgement; stale phone
  metadata during handover cannot raise or overwrite native Alice volume.
- Clear pending sender-volume acknowledgements when playback becomes inactive.
  No saved-volume restoration on disconnect/stop: Alice keeps the final native
  level selected during AirPlay. Existing two-way synchronization is retained.
- Regression tests cover initial native/mute/max levels, delayed DACP, sender
  failures/timeouts, genuine phone changes, sender change, pause/resume,
  disconnect/shutdown persistence and feature opt-out.

## 1.4.11

- One embedded exact transport dictionary generates the YAM manifest allowlist.
  All synonyms, including skip/скип/скипни, are declared, not silently removed
  to fit 32 entries. Requires YAM with bounded expanded policy (8192 maximum).
- Russian/English pause, resume, next, previous and restart variants plus
  finite polite/transport wrappers. Punctuation uses YAM-compatible normalization.
- Tests check every declared phrase against its handler, no undeclared entries,
  all dictionary variants with Alice prefix, and negative/unrelated/compound
  requests. No global Quasar rules or keyword matching.
- Owner accepted 1.4.10 previous/restart semantics on Lite 2 .196.

## 1.4.10

- Voice previous-track phrases send Previous twice, 350 ms apart. Repeat/start
  again phrases send Previous once, not repeat mode. The second command is
  cancelled if the sender changes/disconnects or the event is cancelled.
- YAM permits only 32 declared phrases: prioritize Russian previous/restart
  variants alongside core pause/resume/next commands. Wider service synonyms
  remain subject to this exact manifest allowlist.
- Owner accepted 1.4.9 on Lite 2 .196: button, voice and indication work.
- Sender behaviour near the start of an item remains sender-dependent: two
  Previous commands can skip two items. No position-aware guarantee is made.

## 1.4.9

- Короткое нажатие, поставившее AirPlay на паузу, теперь оставляет сессию
  активной: индикация продолжает гореть, а голосовая команда «продолжить»
  перехватывается и возобновляет отправителя, а не запускает музыку Алисы.
  Раньше после нажатия `active` сразу становился false, кольцо гасло, а
  голосовой транспорт не срабатывал.
- В таблицу команд добавлены «продолжить», «продолжить музыку», «продолжаем»,
  «продолжим», «возобновить», «поставь на паузу», «давай дальше» и другие
  естественные формулировки.
- Список `[commands].phrases` в манифесте синхронизирован с таблицей команд.
  YAM выдаёт разрешение на перехват только для объявленных фраз, поэтому
  недекларированные варианты (в том числе «продолжить») раньше не
  перехватывались и доставались Алисе.
- При голосовом возобновлении/переключении штатный плеер ставится на паузу
  сразу, не дожидаясь следующего тика; окно рукопожатия остаётся 12 с.

## 1.4.8

- Исправлено подавление штатной музыки после короткого нажатия: окно
  подавления запускалось с контекстом одного события управляющего цикла и
  завершалось сразу после возврата обработчика (в журнале `suppression: ended`
  через 20 мс). Теперь окно живёт свои 8 с и не зависит от таймаута события;
  останавливают его только истечение окна и `releaseNative`.
- Диагностический журнал `/tmp/station-airplay-button.log` дополнен фазами
  речи, результатами голосового транспорта, подавлением по голосу и кадрами
  индикации, чтобы разбирать поведение без пересборки.

## 1.4.7

- Кнопка Алисы действительно подавляет штатную музыку Quasar. Сервис
  наследует каталог библиотек пакета (`LD_LIBRARY_PATH` с более новым glibc для
  shairport/avahi), а штатный CLI — это shell-скрипт
  `/system/vendor/quasar/q`: загрузчик `/bin/sh` падал с
  `GLIBC_2.35 not found` ещё до того, как скрипт выбирал библиотеки Quasar.
  Поэтому `Player Pause` не доходил до проигрывателя: AirPlay вставал на паузу,
  а короткое нажатие запускало музыку Алисы.
- CLI теперь запускается с системным окружением без `LD_LIBRARY_PATH` и
  `LD_PRELOAD` (как `aplay` в pipe-бэкенде); скрипт сам задаёт путь библиотек
  для `maind`.
- Окно подавления после нажатия увеличено до 8 с, чтобы перекрыть передачу PCM
  от shairport к штатному проигрывателю.
- Добавлен ограниченный диагностический журнал кнопки
  `/tmp/station-airplay-button.log` (несколько строк на нажатие).
- Регрессионный тест: окружение CLI не содержит библиотек пакета.

## 1.4.6

- Необязательная кнопка Алисы как пауза/продолжение AirPlay теперь доступна не
  только на Mini 2: квалифицированы `yandexmini_2`, `yandexplum` (Lite 2) и
  `yandexstation_2` (Station 2).
- Устройство ввода больше не ищется по имени `sar0_pad0`. Все четыре модели
  сообщают кнопку Алисы одним и тем же кодом `KEY_ALICE` (582), поэтому узел
  `/dev/input/event*` выбирается по битовой карте `EVIOCGBIT(EV_KEY)`. Это
  снимает зависимость от имён `aw933xx`, `aw9610x`, `midi_touch` и от
  нестабильных номеров event-узлов; отсутствие или неоднозначность узла —
  ошибка запуска, а не чтение чужого устройства.
- Предполётная проверка тоже читает `/dev/input`, а не `/sys/class/input`, и
  разрешает флаг только для перечисленных моделей.
- Поведение прежнее: чтение только на чтение, короткое нажатие 30–600 мс при
  живом AirPlay-клиенте, длинное нажатие и диалог Алисы не затрагиваются.

## 1.4.5

- Необязательный флаг `alice_button`: короткое нажатие кнопки Алисы ставит на
  паузу и возобновляет текущего AirPlay-отправителя. Штатная музыка Quasar
  подавляется ограниченным окном через CLI `o`, транспорт — DACP
  `Pause`/`Play`. Без активной сессии кнопка работает штатно.
- Флаг требует `native_player` и проверяет наличие клавиатуры на старте.

## 1.4.4 (расширение поверх main)

- Яндекс Станция Мини 3 (`bergamot`): shared-профиль, ARMhf receiver 5.6.2, общий PCM `speakers` 48 кГц/S16_LE, Алиса сохраняется, mDNS-хост `YandexMini3-<serial>`, иконка HomePod mini. Часы и RGB не занимаются. YAM не узнаёт `yandex,bergamot` и схлопывает `yandexmini_3` в модель Мини первого поколения `yandexmini`, поэтому стабильный id задаётся в `device.toml` как `bergamot`.

- После отключения питания Mini 2 не прошла 10-секундную проверку API
  приёмника, хотя Avahi и сам приёмник уже были запущены. Проверка управления
  теперь использует общее ожидание до 60 с и сообщает последнюю ошибку API;
  устранение сбоя требует повторной холодной загрузки точного пакета.

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
