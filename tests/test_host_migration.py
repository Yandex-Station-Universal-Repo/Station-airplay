#!/usr/bin/env python3
import tempfile, os, subprocess, pathlib, shutil
project=pathlib.Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='yam-host-test-') as x:
    root=pathlib.Path(x)
    (root/'data/shairport').mkdir(parents=True)
    (root/'etc/init.d').mkdir(parents=True)
    (root/'etc/dbus-1/system.d').mkdir(parents=True)
    (root/'fakebin').mkdir()
    (root/'project/config').mkdir(parents=True)
    (root/'project/support').mkdir(parents=True)
    shutil.copyfile(project/'config/avahi-dbus.conf',root/'project/config/avahi-dbus.conf')
    old=root/'data/shairport/airplay.sh'
    old.write_text('#!/bin/sh\necho "$1" >> "'+str(root/'calls')+'"\n')
    old.chmod(0o755)
    init=root/'etc/init.d/S99zairplay'
    init.write_text(f'''#!/bin/sh
case "$1" in
  start) {old} killquasar; {old} start ;;
  stop)  {old} stop ;;
esac
''')
    init.chmod(0o755)
    script_text=[]
    for f in ('prepare-host.sh','restore-host.sh'):
        text=(project/'support'/f).read_text()
        # Only substitute the real path constants, leaving shell commands intact.
        text=text.replace('/data/',str(root/'data')+'/').replace('/etc/',str(root/'etc')+'/')
        dest=root/'project/support'/f
        dest.write_text(text)
        dest.chmod(0o755)
        script_text.append(dest)
    mnt=root/'fakebin/mount'
    mnt.write_text('#!/bin/sh\nexit 0\n');mnt.chmod(0o755)
    p=root/'fakebin/pidof'
    p.write_text('#!/bin/sh\nexit 1\n');p.chmod(0o755)
    env=dict(os.environ);env['PATH']=str(root/'fakebin')+':'+env.get('PATH','')
    def call(f):
        p=subprocess.run(['sh',str(f)],env=env,capture_output=True,text=True)
        if p.returncode: raise AssertionError(f'{f.name} rc={p.returncode}: {p.stdout}\n{p.stderr}')
        return p.stdout
    if os.getuid()==0:
        print(call(script_text[0]))
        assert 'YAM migration' in init.read_text()
        assert (root/'data/yam-airplay-host-backup/S99zairplay.original').exists()
        assert (root/'etc/dbus-1/system.d/avahi-dbus.conf').exists()
        assert 'stop\nkillquasar\n' in (root/'calls').read_text()
        print(call(script_text[0]))
        print(call(script_text[1]))
        assert 'YAM migration' not in init.read_text()
        assert f'{old} start' in init.read_text()
        print('HOST_MIGRATION_TEST_OK')
    else: print('SKIPPED test: requires root fake id')
