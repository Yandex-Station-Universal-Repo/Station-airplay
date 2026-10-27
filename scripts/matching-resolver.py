"""Use the SAME Debian libc family as the package, never a mixed private ABI."""
from pathlib import Path
import hashlib,json,subprocess,urllib.request,shutil
root=Path('/repo');work=Path('/work')
manifest=json.loads((root/'third_party/airplay2-build.json').read_text())
entry=next(p for p in manifest['build_packages']['armhf'] if p['Package']=='libc6')
data=urllib.request.urlopen('https://deb.debian.org/debian/'+entry['Filename'],timeout=60).read()
assert hashlib.sha256(data).hexdigest()==entry['SHA256']
deb=work/'matching-libc6.deb';deb.write_bytes(data)
target=work/'matching-glibc';subprocess.run(['dpkg-deb','-x',str(deb),str(target)],check=True)
lib=next(target.rglob('libc.so.6'))
assert hashlib.sha256(lib.read_bytes()).digest()==hashlib.sha256((root/'lib/libc.so.6').read_bytes()).digest(), 'package libc differs: review ABI before adding resolver'
resolver=next(target.rglob('libresolv.so.2'))
shutil.copyfile(resolver,work/'runtime-additions/libresolv.so.2')
copyright_file=next(target.glob('usr/share/doc/libc6/copyright'))
shutil.copyfile(copyright_file,work/'runtime-additions/libc6.copyright')
print('MATCHING_LIBC_RESOLVER_SHA256_PASS')
