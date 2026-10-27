"""Compile the actual patched policy in isolation; no audio or native services."""
from pathlib import Path
import subprocess,tarfile,tempfile
root=Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='station-stream-gain-') as temporary:
    work=Path(temporary);(work/'dbus').mkdir()
    with tarfile.open(root/'third_party/shairport-sync-5.6.2.tar.gz') as archive:
        for name in ['player.c','dbus/dbus-service.c','dbus/org.gnome.ShairportSync.xml']:
            (work/name).write_bytes(archive.extractfile('shairport-sync-5.6.2/'+name).read())
    subprocess.run(['patch','-p1','-d',str(work),'-i',str(root/'support/shared-controls.patch')],check=True)
    exe=work/'gain-test'
    subprocess.run(['cc','-std=c11','-Wall','-Wextra','-Werror','-fsanitize=address,undefined',
                    '-I'+str(work),str(root/'support/test-stream-gain.c'),'-o',str(exe)],check=True)
    subprocess.run([str(exe)],check=True)
print('PATCHED_STREAM_GAIN_LEASE_SANITIZERS_PASS')
