#!/usr/bin/env python3
"""Stage and pack an offline YAM v2 candidate; never install or publish it."""
import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess
import tomllib

PLATFORMS = ('yandexmini_2', 'yandexplum', 'yandexstation_2')

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--namespace', required=True, help='Proposed/owned YAM namespace; never claimed by this tool')
    p.add_argument('--version', required=True, help='Explicit candidate SemVer, e.g. 1.4.15-alpha.1')
    p.add_argument('--platform', required=True, choices=PLATFORMS)
    p.add_argument('--supervisor', required=True, type=Path, help='ARMv7 Linux supervisor built from this source revision')
    p.add_argument('--publisher', required=True, type=Path)
    p.add_argument('--profiles', required=True, type=Path)
    p.add_argument('--work-dir', required=True, type=Path, help='Fresh staging directory outside this checkout')
    p.add_argument('--output', required=True, type=Path)
    a = p.parse_args()
    root = Path(__file__).resolve().parent.parent
    if not re.fullmatch(r'[a-z0-9-]{1,64}', a.namespace): p.error('invalid namespace')
    if not re.fullmatch(r'\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?', a.version): p.error('invalid candidate version')
    if a.work_dir.resolve().is_relative_to(root): p.error('staging must be outside the source checkout')
    if a.output.exists(): p.error('output already exists; choose another path')
    binary = a.supervisor.read_bytes()
    if binary[:5] != b'\x7fELF\x01' or binary[18:20] != b'\x28\x00': p.error('expected Linux ARMv7/ARMhf supervisor input')
    a.work_dir.mkdir(parents=True, exist_ok=False)
    plugin_id = a.namespace + '.airplay'
    payload = a.work_dir/'payload'
    payload.mkdir()
    for name in ['bin', 'lib', 'config', 'LICENSES', 'third_party/licenses']:
        source = root/name
        if not source.is_dir(): p.error('missing runtime input: '+name)
        for f in sorted(source.rglob('*')):
            if not f.is_file() or '__pycache__' in f.parts: continue
            target = payload/f.relative_to(root)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(f, target)
            data = target.read_bytes()
            target.chmod(0o755 if data[:4] == b'\x7fELF' or target.suffix == '.sh' else 0o644)
    shutil.copyfile(a.supervisor, payload/'bin/station-airplay')
    (payload/'bin/station-airplay').chmod(0o755)
    text = (root/'plugin.toml').read_text()
    for key, value in [('id', plugin_id), ('version', a.version), ('entry', 'launch-airplay.sh'), ('path', 'launch-airplay.sh')]:
        text, count = re.subn(r'^'+key+r'\s*=.*$', key+' = '+json.dumps(value), text, flags=re.M)
        if count != 1: raise RuntimeError('unexpected descriptor field: '+key)
    text, count = re.subn(r'^args\s*=.*\n', '', text, flags=re.M)
    if count != 1: raise RuntimeError('unexpected launch args')
    text = text.replace('[launch]\n', '[launch]\ninterpreter = "sh"\n', 1)
    text += '\n[dependencies]\nstation-volume = '+json.dumps(a.namespace+'.volume')+'\n'
    descriptor = tomllib.loads(text)
    assert len(descriptor['commands']['phrases']) == 3732
    (payload/'plugin.toml').write_text(text)
    wrapper = (root/'support/launch-airplay-v2.sh').read_bytes()
    (payload/'launch-airplay.sh').write_bytes(wrapper)
    (payload/'launch-airplay.sh').chmod(0o755)
    shutil.copyfile(root/'support/airplay-controls-v2.example.json', payload/'config/airplay-controls-v2.json')
    manifest = {'name': a.namespace+'/airplay', 'version': a.version, 'license': 'Mixed; see bundled LICENSES/ and third_party/licenses/',
        'description': 'AirPlay candidate hosted by Luna; no YAM service supervisor assumed',
        'targets': [a.platform],
        'depends': ['yam/luna >=0.2.7', 'luna-binary-dependencies-v1', a.namespace+'/volume =1.0.2+all4.20261026'],
        'files': {'share/luna/plugins/'+plugin_id+'/': 'payload/'}}
    (a.work_dir/'yam.yaml').write_text(json.dumps(manifest, indent=2)+'\n')
    a.output.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run([str(a.publisher.resolve()), 'pack', '--source', str(a.work_dir.resolve()), '--profiles', str(a.profiles.resolve()), '--platform', a.platform, '--output', str(a.output.resolve())], check=True)
    print('CANDIDATE_ONLY', a.output)

if __name__ == '__main__': main()
