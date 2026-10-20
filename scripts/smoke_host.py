#!/usr/bin/env python3
"""Real compiled headless-backend check, NOT a macOS window test."""
import json, os, platform, selectors, signal, subprocess, tempfile, urllib.request
from pathlib import Path
ROOT = Path(__file__).resolve().parents[1]
binary = ROOT/'build/iconcraft-host'
subprocess.run(['go','build','-o',str(binary),'./cmd/iconcraft'],cwd=ROOT,check=True)
assert subprocess.check_output([str(binary),'--version'],text=True).strip()=='0.12.0'
with tempfile.TemporaryDirectory(prefix='iconcraft-smoke-') as home:
    env=dict(os.environ,HOME=home)
    proc=subprocess.Popen([str(binary),'--headless'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    try:
        with selectors.DefaultSelector() as sel:
            sel.register(proc.stdout,selectors.EVENT_READ)
            if not sel.select(15):raise RuntimeError('headless startup timed out')
        url=proc.stdout.readline().strip()
        assert url.startswith('http://127.0.0.1:')
        with urllib.request.urlopen(url+'api/session',timeout=5) as response:
            session=json.load(response)
            assert session['version']=='0.12.0'
            assert session['setupRequired'] is True
        with urllib.request.urlopen(url,timeout=5) as response:
            assert b'IconCraft' in response.read()
        other=subprocess.run([str(binary),'--headless'],env=env,capture_output=True,text=True,timeout=5)
        assert other.returncode==0 and 'already running' in other.stderr
        proc.send_signal(signal.SIGTERM)
        proc.wait(timeout=10)
        assert proc.returncode==0
    finally:
        if proc.poll() is None:
            proc.kill();proc.wait()
result={'host':platform.system(),'nativeMacWindow':'NOT RUN','compiledBackend':'passed','version':'passed','httpSession':'passed','servesInterface':'passed','duplicateInstance':'passed','gracefulShutdown':'passed','userData':'isolated temporary HOME only'}
(ROOT/'build/host-smoke.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
