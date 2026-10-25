#!/usr/bin/env python3
"""Build the same multi-model .yap on Windows and Linux (Python 3.8+)."""
import hashlib
import pathlib
import re
import tarfile
import argparse
import io

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--model', choices=['all', 'station2', 'mini3pro', 'plum'], default='all')
model = parser.parse_args().model

root = pathlib.Path(__file__).resolve().parent.parent
version = re.search(r'^version = "([^"]+)"', (root / 'plugin.toml').read_text(), re.M)[1]
required = ['bin/launch.sh', 'bin/station-airplay', 'bin/station-airplay-aarch64',
            'bin/avahi-namespace-aarch64', 'runtime/ap2-armhf/bin/shairport-sync',
            'runtime/ap2-aarch64/bin/shairport-sync', 'runtime/aarch64/bin/avahi-daemon']
for name in required:
    if not (root / name).is_file():
        raise SystemExit('Missing build input: ' + name)
suffix = '' if model == 'all' else '-' + model
out = root / 'dist' / ('station-airplay-' + version + suffix + '.yap')
out.parent.mkdir(exist_ok=True)
with tarfile.open(out, 'w:gz') as archive:
    for name in ['plugin.toml', 'bin', 'lib', 'config', 'LICENSES', 'source',
                 'support', 'docs', 'receivers', 'runtime', 'third_party', 'CONTRIBUTE.md']:
        path = root / name
        for file in ([path] if path.is_file() else sorted(path.rglob('*'))):
            if not file.is_file() or '__pycache__' in file.parts:
                continue
            rel = file.relative_to(root).as_posix()
            # Same plugin/source, smaller model-specific payloads for flash-limited devices.
            if rel.startswith('receivers/'):
                continue
            if model != 'all':
                if model == 'plum':
                    if rel.startswith('runtime/') or rel in ['bin/station-airplay-aarch64', 'bin/avahi-namespace-aarch64']:
                        continue
                else:
                    abi = 'armhf' if model == 'station2' else 'aarch64'
                    if rel.startswith('lib/') or rel in ['bin/station-airplay', 'bin/shairport-sync-ap2', 'bin/nqptp', 'bin/avahi-daemon']:
                        continue
                    if rel.startswith('runtime/ap2-') and not rel.startswith('runtime/ap2-' + abi + '/'):
                        continue
            # Corresponding upstream source is kept in the repository, off speaker flash.
            if rel.startswith('third_party/') and file.name in ['ffmpeg-7.1.3.tar.xz', 'shairport-sync-5.6.2.tar.gz', 'nqptp-1.2.9.tar.gz']:
                continue
            data = file.read_bytes()
            if rel == 'plugin.toml' and model != 'all':
                models = {'station2': '["yandexstation_2"]', 'mini3pro': '["yandexmini_3_pro"]', 'plum': '["yandexplum", "plum"]'}
                data = re.sub(rb'^models = .*$', ('models = ' + models[model]).encode(), data, flags=re.M)
            info = archive.gettarinfo(str(file), rel)
            info.size = len(data)
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            info.mode = 0o755 if data[:4] == b'\x7fELF' or file.suffix == '.sh' else 0o644
            archive.addfile(info, io.BytesIO(data))
print(str(out))
print('SHA256 ' + hashlib.sha256(out.read_bytes()).hexdigest())
