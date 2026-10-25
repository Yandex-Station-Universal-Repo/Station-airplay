# Яндекс Станция Миди

Профиль `midi` выбирается по модели `yandexmidi` из YAM (`led.ring.24`, сегментные часы Midi). Алису можно оставить работать рядом с AirPlay или выключать на время работы приёмника.

## Алиса: включена или выключена

Режим задаётся файлом `/data/shairport/quasar` (или переменной `STATION_AIRPLAY_QUASAR`) и читается при запуске плагина:

| Значение | Что происходит |
| --- | --- |
| `on` (по умолчанию, файла нет) | Алиса работает. Звук AirPlay смешивается с ней через общий dmix, громкость станции общая (см. ниже). |
| `off` | При запуске плагин останавливает Quasar (`TERM` для `quasar_launcher2`, все `maind` завершаются вместе с ним). Отдельный процесс-наблюдатель запускает Quasar штатным `run.sh`, как только приёмник завершится: при `yam stop`, `yam disable`, падении или убийстве `SIGKILL`. Громкость применяет сам Shairport Sync, `station-volume` не используется: без Quasar ему не к кому обращаться. |

```sh
echo off > /data/shairport/quasar && yam stop station-airplay && yam run -d station-airplay
rm /data/shairport/quasar && yam stop station-airplay && yam run -d station-airplay   # обратно
```

## Что отличается от Plum

- **Звук.** Общий dmix на Midi держит Quasar, а встроенная в пакет (более старая) alsa-lib к нему подключиться не может (`unable to create IPC shm instance`). Поэтому Shairport Sync выводит звук в пустое ALSA-устройство с отводом в FIFO (`/tmp/sps.fifo`, 44,1 кГц S16), а супервизор проигрывает FIFO системным `aplay -D speakers` — через штатный `/etc/asound.conf`, общий dmix и регулятор Алисы. Музыка AirPlay смешивается с Алисой и другими плеерами и подчиняется громкости станции.
- **Громкость.** Громкостью станции владеет Quasar. Если установлен пакет [station-volume](https://github.com/Yandex-Station-Universal-Repo/station-volume), Shairport Sync играет без своего ослабления (`ignore_volume_control = "yes"`), а ползунок AirPlay на телефоне выставляет громкость станции командой `station-volume set`, как кнопки. Изменения с кнопок, от Алисы и из приложения показываются на кольце и часах во время AirPlay-сессии. Ползунок на телефоне при этом не двигается: в этой сборке Shairport Sync нет удалённого управления AirPlay. Без пакета `station-volume` громкость, как раньше, применяет сам Shairport Sync поверх громкости станции.
- **Индикация.** Кольцо из 24 светодиодов: полоса громкости и спектр из трёх полос по кругу. Часы: цифры на позициях 1..4. Вне AirPlay-сессии плагин отпускает кольцо и часы, и Алиса показывает свою индикацию как обычно.
- **mDNS.** Quasar сам отвечает за адрес `yandex-station-midi-<serial>.local`. Avahi берёт то же имя хоста без собственных адресных записей и публикует только сервисы AirPlay, иначе два mDNS-ответчика бесконечно переименовываются.

## Установка

```sh
yam install station-volume          # по желанию: громкость с телефона = громкость станции
yam enable station-volume
yam install station-airplay-1.2.0.yap
yam enable station-airplay
yam logs station-airplay            # "running on midi"
```

Другой AirPlay-приёмник (например, собственный Shairport Sync) должен быть остановлен: плагин проверяет занятость порта 7000 и процессы `shairport-sync`, `nqptp`.

Часовой пояс для часов: `/data/shairport/tz` в формате POSIX, например `UZT-5` или `MSK-3`.

Проверено на Midi с прошивкой `0.369.1.46.4778262955.20261013.160`: воспроизведение с Mac (AirPlay 2) без искажений, ползунок громкости на Mac меняет громкость станции, режимы `on` и `off` с восстановлением Алисы после `yam stop`.
