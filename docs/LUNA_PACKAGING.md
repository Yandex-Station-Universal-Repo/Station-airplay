# Упаковка подготовленного AirPlay для Luna

Пакет `airplay/airplay-luna@1.4.15+luna.0.2.14` из реестра экспортирует
`share/luna/plugins/station.airplay/`. YAM отклоняет такой архив: для
пространства имён `airplay` каждый путь в `share/` должен содержать компонент,
начинающийся с `airplay.`. Это ошибка упаковки расширения.

Одного переименования каталога недостаточно. Luna для binary-плагина
`namespace.name` разрешает только
`pkgs/namespace/name/current/share/luna/plugins/namespace.name/` и проверяет
принадлежность дескриптора выбранной версии. Поэтому для пакета
`airplay/airplay-luna` **ID в `plugin.toml` и каталог должны совпадать с
`airplay.airplay-luna`**.

Правила upstream: [YAM — namespace в share/](https://github.com/nyakokitsu/yam/blob/main/docs/PACKAGING.md),
[Luna — pinned_root](https://github.com/nyakokitsu/luna/blob/main/src/yam_binary.rs).

## Исправленная сборка

`scripts/package-luna.py` принимает **уже подготовленное дерево Luna-плагина**:
`plugin.toml`, shell launcher, соответствующий ему supervisor, библиотеки,
конфигурацию и лицензии. Это стадия упаковки, общая для Luna-сборок;
она не преобразует legacy `.yap` в рабочий Luna runtime и не выбирает
аудиопрофиль. Подготовьте и проверьте runtime отдельно. Дерево с runtime
dependency aliases требует отдельного рецепта и здесь отклоняется.

Python 3.11+, официальный `yam-publish` и профили платформ нужны на компьютере
сборки. Используйте собственное пространство имён и новую версию; пример
для издателя `airplay` и Миди:

```sh
python3 scripts/package-luna.py \
  --payload /path/to/prepared-luna-airplay \
  --package airplay/airplay-luna --version 1.4.16 \
  --platform cucumber --luna-version 0.2.14 \
  --work-dir /tmp/airplay-luna-staged \
  --publisher /path/to/yam-publish --profiles /path/to/yam/platforms \
  --output /tmp/airplay-luna-cucumber.yampkg
```

Скрипт копирует входные файлы с сохранением executable-битов, задаёт ID и
версию в копии дескриптора и создаёт `yam.yaml` с соответствующим экспортом.
Модели, права, launcher, конфигурация и бинарники остаются из подготовленного
дерева. Архив, `MANIFEST.yaml` и хеши создаёт **официальный `yam-publish pack`**.
Исходное дерево не меняется. Скрипт не устанавливает и не публикует пакет.

Исправление ID создаёт новую идентичность в Luna: прежнее включение плагина
и его runtime state автоматически не переносятся. Старую принятую сборку
не удаляйте до отдельной проверки новой; не запускайте два приёмника вместе.
Для устранения ошибки в каталоге сопровождающему нужно выпустить исправленные
артефакты под новой версией. Повторное нажатие «Установить» для старой версии
не исправляет её содержимое.

## Проверка упаковки

```sh
YAM=/path/to/yam YAM_PUBLISH=/path/to/yam-publish \
  python3 -m unittest discover -s tests -p test_luna_package.py -v
```

Тест воспроизводит отказ для `station.airplay`, затем настоящими YAM tools
упаковывает, устанавливает в отдельный временный root, проверяет `doctor`,
владельца файлов, executable-биты и точное соответствие путей контракту Luna,
после чего удаляет пакет. `yam/luna` в тесте — неисполняемая metadata fixture.
Runtime, звук, postinstall hooks и колонки этот тест не запускает.
