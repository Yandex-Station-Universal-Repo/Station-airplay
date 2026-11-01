# Draft: AirPlay `.yampkg` (YAM v2)

Порядок: AirPlay #12 → #13 → этот packaging Draft. На стороне Luna нужны
#5 и #6; до их принятия и выпуска host capability эта упаковка не является
готовой пользовательской миграцией. Не устанавливалась на колонках.

Python3.11+ staging tool `scripts/build-yampkg.py` создаёт исходный `yam.yaml`
в отдельном новом work-dir, копирует payload и вызывает **официальный
`yam-publish pack`**. Не пишет MANIFEST.yaml или tar.zst вручную, не выполняет
install/publish/хуки и не регистрирует namespace.

Пример offline-кандидата (имя namespace здесь лишь предложено):

```sh
python3 scripts/build-yampkg.py --namespace station --version 1.4.15-alpha.1 \
  --platform yandexmini_2 --supervisor /outside/build/station-airplay-armv7 \
  --publisher /outside/yam/target/debug/yam-publish --profiles /outside/yam/platforms \
  --work-dir /outside/stage-airplay-mini2 --output /outside/airplay-mini2.yampkg
```

Поддержаны пока только platform targets `yandexmini_2`, `yandexplum`
(обе платы Lite2, включая Prune), `yandexstation_2`. Внутри сохраняется
принятый смешанный ABI: ARMv7 supervisor/ARMhf receiver и приватный runtime,
а не неподтверждённая массовая пересборка всех зависимостей в AArch64.
`--supervisor` должен быть заново собран из этой исходной ветки; старый binary
из `bin/` намеренно не используется для новых возможностей.

Runtime ID: `<namespace>.airplay`, provider ID: `<namespace>.volume`.
Payload лежит под `share/luna/plugins/<namespace>.airplay/`; библиотеки не
экспортируются в общий YAM lib. Существующие notices включены; metadata не
выдаёт весь mixed payload за MIT. Namespace нужно согласовать/зарегистрировать
отдельно перед публикацией: пример `station` ничего не резервирует.

`depends` требует `yam/luna>=0.2.7`, virtual capability
`luna-binary-dependencies-v1` и точный native-volume package. Номер0.2.7 сам
по себе не гарантирует новый API. Registry должен содержать реальный host
с соответствующим `provides`; synthetic provider из offline fixture не годится
для устройств.

Runtime wrapper `launch-airplay-v2.sh` получает `LUNA_PLUGIN_STATE`, при первом
запуске сохраняет default config туда и никогда не перезаписывает имеющийся
config. `volume_dependency=station-volume` включает новый resolver; относительное
`../../airplay-controls.json` и запись mutable state в versioned payload не
используются. Wrapper делает exec supervisor, не запускает отдельный daemon.

В этом пакете нет init scripts, system_overrides, CA overlays или попытки
реализовать supervisor YAM. Не запускайте его произвольно через shell:
runtime credentials и lifetime lease выдаёт Luna.

Перед обновлением пакетов остановить все leased YAM binary-плагины в Luna,
дождаться завершения, выполнить транзакцию и включить нужные плагины вновь.
Тестовые .yampkg не заменяют принятый .yap1.4.14 и не повышают версию релиза.
Значение `--version` относится к кандидатному пакету; строка версии supervisor
в этой исходной ветке остаётся release-base1.4.14 до отдельного релиза.
