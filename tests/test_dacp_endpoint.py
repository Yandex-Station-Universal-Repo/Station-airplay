"""Actual native patch: exact DNS-SD session IDs and IPv4 guards, no network."""
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='station-dacp-endpoint-') as temporary:
    work = Path(temporary)
    subprocess.run(['tar', '-xf', str(root/'third_party/shairport-sync-5.6.2.tar.gz'),
                    '--strip-components=1', '-C', str(work)], check=True)
    for name in ['shared-controls.patch', 'dacp-ipv4-discovery.patch']:
        subprocess.run(['patch', '--batch', '-p1', '-d', str(work),
                        '-i', str(root/'support'/name)], check=True)
    exe = work/'endpoint-test'
    subprocess.run(['cc', '-std=c11', '-Wall', '-Wextra', '-Werror',
                    '-fsanitize=address,undefined', '-I'+str(work),
                    str(root/'support/test-dacp-endpoint.c'), '-o', str(exe)], check=True)
    subprocess.run([str(exe)], check=True)
print('PATCHED_DACP_EXACT_ID_AND_IPV4_GUARDS_SANITIZERS_PASS')
