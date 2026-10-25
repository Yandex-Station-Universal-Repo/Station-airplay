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

## 1.1.5

- Ускорено целевое обновление визуализатора до 20 FPS (50 мс).
- Сохранена центральная симметричная анимация и статическое имя `Я.Станция`.
- Базовый релиз для подготовки публичного репозитория; поддерживаемый профиль — Plum (Лайт 2).
