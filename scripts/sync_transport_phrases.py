#!/usr/bin/env python3
"""Generate the YAM allowlist from the exact embedded AirPlay dictionary."""
import json
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parent.parent
phrases = json.loads((root / 'source/transport_dictionary.json').read_text())
assert 0 < len(phrases) <= 8192
assert set(phrases.values()) == {'Pause', 'Play', 'Next', 'Previous', 'Restart'}
assert all(0 < len(p.encode()) <= 128 for p in phrases)
line = 'phrases = ' + json.dumps(sorted(phrases), ensure_ascii=False)
manifests = [p for p in (root / 'plugin.toml', root / 'source/plugin.toml') if p.is_file()]
assert manifests, 'plugin.toml not found'
for p in manifests:
    text = p.read_text()
    new, count = re.subn(r'^phrases = \[.*?\]$', lambda _: line, text, count=1, flags=re.M)
    assert count == 1
    if '--check' in sys.argv:
        assert text == new, f'{p}: run scripts/sync_transport_phrases.py'
    else:
        p.write_text(new)
print(f'YAM allowlist: {len(phrases)} exact phrases; no missing handler entries')
