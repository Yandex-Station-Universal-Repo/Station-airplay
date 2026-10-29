#!/usr/bin/env python3
"""No device access: exercise the wrapper with a temporary shell supervisor."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

class WrapperTests(unittest.TestCase):
    def test_state_is_separate_and_existing_config_survives(self):
        with tempfile.TemporaryDirectory(prefix='airplay-wrapper-') as d:
            root=Path(d)/'immutable payload';state=Path(d)/'mutable state'
            (root/'bin').mkdir(parents=True);(root/'config').mkdir()
            source=Path(__file__).resolve().parent.parent/'support/launch-airplay-v2.sh'
            wrapper=root/'launch-airplay.sh';shutil.copyfile(source,wrapper)
            example={'native_volume':True,'volume_dependency':'station-volume'}
            (root/'config/airplay-controls-v2.json').write_text(json.dumps(example))
            helper=root/'bin/station-airplay'
            helper.write_text('#!/bin/sh\nset -eu\n[ "$1" = --shared-audio ] && [ "$2" = --controls-config ]\nprintf "%s" "$3" > "$LUNA_PLUGIN_STATE/config-argument"\n')
            helper.chmod(0o755)
            env={'PATH':os.environ['PATH'],'LUNA_PLUGIN_STATE':str(state)}
            subprocess.run(['sh',str(wrapper)],env=env,check=True)
            config=state/'airplay-controls.json'
            self.assertEqual(json.loads(config.read_text()),example)
            self.assertEqual((state/'config-argument').read_text(),str(config))
            config.write_text('{"owner_custom":true}')
            subprocess.run(['sh',str(wrapper)],env=env,check=True)
            self.assertEqual(config.read_text(),'{"owner_custom":true}')
            self.assertFalse((root/'airplay-controls.json').exists())
            self.assertNotEqual(subprocess.run(['sh',str(wrapper)],env={'PATH':os.environ['PATH']},capture_output=True).returncode,0)

if __name__=='__main__':unittest.main()
