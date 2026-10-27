"""Container-local ELF closure. Reuse existing package libraries; never firmware."""
from pathlib import Path
import json,re,shutil,subprocess
repo=Path('/repo');work=Path('/work');dest=work/'runtime-additions';dest.mkdir(exist_ok=True)
binary=work/'armhf/shairport-sync';queue=[binary];seen=set();added=[]
search=[Path('/usr/lib/arm-linux-gnueabihf'),Path('/lib/arm-linux-gnueabihf'),Path('/usr/arm-linux-gnueabihf/lib')]
while queue:
    source=queue.pop()
    output=subprocess.check_output(['arm-linux-gnueabihf-readelf','-d',str(source)],text=True)
    for name in re.findall(r'Shared library: \[(.*?)\]',output):
        if name in seen:continue
        seen.add(name)
        if (repo/'lib'/name).is_file():queue.append(repo/'lib'/name);continue
        src=next((p/name for p in search if (p/name).is_file()),None)
        assert src is not None,('unresolved ELF dependency',name)
        assert name not in ['libasound.so.2','libc.so.6','ld-linux-armhf.so.3'],'do not replace platform ABI'
        shutil.copyfile(src.resolve(),dest/name);queue.append(dest/name)
        added.append({'name':name,'container_source':str(src),'bytes':src.stat().st_size})
        # Preserve upstream package notices for the newly shipped public libraries.
        package=subprocess.check_output(['dpkg-query','-S',str(src)],text=True).split(': ')[0].split(',')[0]
        package_name=package.split(':')[0]
        copyright_file=Path('/usr/share/doc')/package_name/'copyright'
        if not copyright_file.exists():copyright_file=Path('/usr/share/doc')/package/'copyright'
        assert copyright_file.is_file(),('missing copyright',package)
        shutil.copyfile(copyright_file, dest/(package_name+'.copyright'))
(work/'runtime-additions.json').write_text(json.dumps({'libraries':added,'all_needed':sorted(seen)},indent=2)+'\n')
print(json.dumps(added,indent=2))
