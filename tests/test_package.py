"""One package, shared daemons, one current engine per native audio ABI."""
import pathlib, subprocess, sys, tarfile, tomllib
root = pathlib.Path(__file__).resolve().parent.parent
subprocess.run([sys.executable, str(root/'scripts/package.py')], check=True)
version = tomllib.loads((root/'plugin.toml').read_text())['version']
with tarfile.open(root/'dist'/f'station-airplay-{version}.yap') as archive:
    members = {m.name: m for m in archive.getmembers()}
    assert sum(m.size for m in members.values()) < 10*1024*1024
    manifest = tomllib.loads(archive.extractfile('plugin.toml').read().decode())
    assert manifest['id'] == 'station-airplay'
    assert set(manifest['compatibility']['models']) == {'plum', 'yandexplum', 'yandexmidi', 'yandexstation_2', 'yandexmini_3_pro', 'yandexmicro', 'yandexmini', 'YandexStationMini'}
    engines = {'bin/shairport-sync-ap2': 40, 'bin/shairport-sync-ap2-aarch64': 183}
    assert {n for n in members if '/shairport-sync' in n} == set(engines)
    for name, machine in engines.items():
        elf = archive.extractfile(name).read(20)
        assert elf[:4] == b'\x7fELF' and int.from_bytes(elf[18:20], 'little') == machine
        assert members[name].mode & 0o111
    assert 'bin/nqptp' in members and 'bin/avahi-daemon' in members
    # Mini firmware has no D-Bus: one ARMhf daemon/client pair from dbus-1.16.2.
    for name in ('bin/dbus-daemon', 'bin/dbus-send'):
        elf = archive.extractfile(name).read(20)
        assert elf[:4] == b'\x7fELF' and int.from_bytes(elf[18:20], 'little') == 40
        assert members[name].mode & 0o111
    assert not any(n.startswith(('runtime/', 'source/', 'docs/', 'support/')) for n in members)
    assert not any(n.endswith(('.tar.gz', '.tar.xz')) for n in members)
print('Universal package, native audio ABIs, shared engine paths and size verified')
