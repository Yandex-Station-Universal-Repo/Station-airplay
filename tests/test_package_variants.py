"""Catch installing the wrong userspace ABI or oversized all-model payload."""
import pathlib, subprocess, sys, tarfile, tomllib

root = pathlib.Path(__file__).resolve().parent.parent
for model, yam_model, abi, machine in [
    ('station2', 'yandexstation_2', 'armhf', 40),
    ('mini3pro', 'yandexmini_3_pro', 'aarch64', 183),
]:
    subprocess.run([sys.executable, str(root/'scripts/package.py'), '--model', model], check=True)
    version = tomllib.loads((root/'plugin.toml').read_text())['version']
    with tarfile.open(root/'dist'/f'station-airplay-{version}-{model}.yap') as archive:
        members = {m.name: m for m in archive.getmembers()}
        manifest = tomllib.loads(archive.extractfile('plugin.toml').read().decode())
        assert manifest['id'] == 'station-airplay'
        assert manifest['compatibility']['models'] == [yam_model]
        assert sum(m.size for m in members.values()) < 24*1024*1024
        assert not any(n.startswith('lib/') or n.startswith('receivers/') for n in members)
        for program in ['shairport-sync', 'nqptp']:
            name = f'runtime/ap2-{abi}/bin/{program}'
            elf = archive.extractfile(name).read(20)
            assert elf[:4] == b'\x7fELF' and int.from_bytes(elf[18:20], 'little') == machine
            assert members[name].mode & 0o111
        other = 'aarch64' if abi == 'armhf' else 'armhf'
        assert not any(n.startswith(f'runtime/ap2-{other}/') for n in members)
        assert 'bin/station-airplay-aarch64' in members
        assert 'runtime/aarch64/bin/avahi-daemon' in members
        assert 'support/homepod-model.patch' in members
print('Model package ABI, compatibility, size and executable contracts passed')
