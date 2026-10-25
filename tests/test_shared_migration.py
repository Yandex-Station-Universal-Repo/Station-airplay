import os
from pathlib import Path
import subprocess
import tempfile
import unittest

class SharedMigration(unittest.TestCase):
    def test_apply_idempotence_start_guard_and_exact_rollback(self):
        for model, folder in [('yandexstation_2','airplay-station2'), ('yandexmini_3_pro','airplay-mini3pro')]:
            with self.subTest(model=model), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                base = root/'data/local'/folder
                base.mkdir(parents=True)
                log = root/'calls'
                original = '#!/bin/sh\necho "$1" >> "'+str(log)+'"\n'
                old = base/'service.sh'
                old.write_text(original)
                old.chmod(0o755)
                text = (Path(__file__).resolve().parent.parent/'support/migrate-shared.sh').read_text()
                text = text.replace('/data/local/', str(root/'data/local')+'/')
                script = root/'migrate.sh'
                script.write_text(text)
                fake = root/'bin'
                fake.mkdir()
                (fake/'pidof').write_text('#!/bin/sh\nexit 1\n')
                (fake/'pidof').chmod(0o755)
                env = dict(os.environ, PATH=str(fake)+':'+os.environ['PATH'])
                def run(*args):
                    subprocess.run(['sh',str(script),*args],env=env,check=True,capture_output=True)
                run('apply',model)
                self.assertEqual(log.read_text(),'stop\n')
                run('apply',model)
                subprocess.run(['sh',str(old),'start'],check=True)
                subprocess.run(['sh',str(old),'supervise'],check=True)
                self.assertEqual(log.read_text(),'stop\n')
                run('rollback',model)
                self.assertEqual(old.read_text(),original)
                self.assertEqual(log.read_text(),'stop\nstart\n')
                self.assertFalse((base/'disabled-by-yam').exists())

if __name__ == '__main__':
    unittest.main()
