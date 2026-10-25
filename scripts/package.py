#!/usr/bin/env python3
"""Build the same multi-model .yap on Windows and Linux (Python 3.8+)."""
import hashlib
import pathlib
import re
import tarfile

root = pathlib.Path(__file__).resolve().parent.parent
version = re.search(r'^version = "([^"]+)"', (root / 'plugin.toml').read_text(), re.M)[1]
required = ['bin/launch.sh', 'bin/station-airplay', 'bin/station-airplay-aarch64',
            'bin/avahi-namespace-aarch64', 'receivers/armhf/shairport-sync',
            'receivers/aarch64/shairport-sync', 'runtime/aarch64/bin/avahi-daemon']
for name in required:
    if not (root / name).is_file():
        raise SystemExit('Missing build input: ' + name)
out = root / 'dist' / ('station-airplay-' + version + '.yap')
out.parent.mkdir(exist_ok=True)
with tarfile.open(out, 'w:gz') as archive:
    for name in ['plugin.toml', 'bin', 'lib', 'config', 'LICENSES', 'source',
                 'support', 'docs', 'receivers', 'runtime', 'third_party', 'CONTRIBUTE.md']:
        path = root / name
        for file in ([path] if path.is_file() else sorted(path.rglob('*'))):
            if not file.is_file() or '__pycache__' in file.parts:
                continue
            rel = file.relative_to(root).as_posix()
            info = archive.gettarinfo(str(file), rel)
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            info.mode = 0o755 if file.read_bytes()[:4] == b'\x7fELF' or file.suffix == '.sh' else 0o644
            with file.open('rb') as stream:
                archive.addfile(info, stream)
print(str(out))
print('SHA256 ' + hashlib.sha256(out.read_bytes()).hexdigest())
