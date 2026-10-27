# Global YAM rooms and AirPlay names

A room belongs to the device in YAM, not to the AirPlay package. With the companion
YAM room-metadata change, users can assign any room after installing YAM:

```sh
yam room "Kitchen"
yam room
yam room --clear
```

The YAM UI has a room input. External management uses `GET/PUT /api/room`, for
example `{"room":"Kitchen"}`; `GET /api/device` and the authenticated binary
runtime `/device` expose the same optional `room` field. Lua sees `device.room`.
The companion change stores it atomically in YAM's data directory, outside plugin
files. It does not modify the hardware model, LED capabilities or audio routing.
This API is a proposed YAM extension, **not an existing feature of stock0.1.17**.

Shared AirPlay profiles read that field on service startup. For example,
`Yandex Mini 2 AirPlay` becomes `Yandex Mini 2 AirPlay (Kitchen)`.
Old YAM without the field or with null/blank room keeps the original name.
Long labels are shortened at a UTF-8 boundary only for DNS display (50-byte
receiver name leaves room for RAOP's MAC prefix); YAM retains the full label.
Control characters/invalid UTF-8 are rejected. Room does not select a model.

Set room before enabling AirPlay, or normally restart the AirPlay service after
changing it. A restart interrupts the current stream; metadata changes do not
silently restart plugins. The legacy Plum/Midi UI path is otherwise unchanged.
Do not copy private hardware overrides or add per-home room constants to a build.
No dedicated room database, second AirPlay daemon or cloud service is needed.
