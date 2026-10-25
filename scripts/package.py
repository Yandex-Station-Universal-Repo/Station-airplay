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
required = ['bin/station-airplay', 'bin/avahi-namespace-aarch64',
            'runtime/ap2-armhf/bin/shairport-sync', 'runtime/ap2-aarch64/bin/shairport-sync',
            'runtime/aarch64/bin/avahi-daemon']
for name in required:
    if not (root / name).is_file():
        raise SystemExit('Missing build input: ' + name)
suffix = '' if model == 'all' else '-' + model
out = root / 'dist' / ('station-airplay-' + version + suffix + '.yap')
out.parent.mkdir(exist_ok=True)
with tarfile.open(out, 'w:gz') as archive:
    # ponytail: ship runtime and notices; sources/build/docs stay in the repository.
    for name in ['plugin.toml', 'bin', 'lib', 'config', 'LICENSES', 'runtime', 'third_party/licenses']:
        path = root / name
        for file in ([path] if path.is_file() else sorted(path.rglob('*'))):
            if not file.is_file() or '__pycache__' in file.parts:
                continue
            rel = file.relative_to(root).as_posix()
            # Same plugin/source, smaller model-specific payloads for flash-limited devices.
            if model != 'all':
                if model == 'plum':
                    if rel.startswith('runtime/') or rel == 'bin/avahi-namespace-aarch64':
                        continue
                else:
                    abi = 'armhf' if model == 'station2' else 'aarch64'
                    if rel.startswith('lib/') or rel in ['bin/shairport-sync-ap2', 'bin/nqptp', 'bin/avahi-daemon']:
                        continue
                    if rel.startswith('runtime/ap2-') and not rel.startswith('runtime/ap2-' + abi + '/'):
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
