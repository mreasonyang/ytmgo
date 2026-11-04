"""Keep only bounded test metadata; never retain raw command diagnostics."""
import collections
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
env = os.environ.copy()
env['YTMGO_RUNTIME_DIR'] = str(ROOT / 'build/runtime')
results = []
stages = [
    ('focused_scroll', ['go', 'test', '-json', '-count=1', '-run', 'TestNative(ShelfScroll|Lyrics)', '.']),
    ('go_race', ['go', 'test', '-json', '-race', '-count=1', './...']),
    ('go_vet', ['go', 'vet', './...']),
    ('translations', ['go', 'tool', 'mygo', 'i18n', 'check']),
    ('runtime_manifest', ['python3', 'scripts/runtime/test_runtime_manifest.py']),
    ('runtime_protocol', ['python3', 'scripts/runtime/test_protocol_version.py']),
    ('login_verify', ['python3', 'scripts/login/prepare.py', '--verify']),
]
for name, command in stages:
    start = time.monotonic()
    print(json.dumps({'stage': name, 'status': 'running'}), flush=True)
    run = subprocess.run(command, cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    row = dict(stage=name, exit_code=run.returncode, passed=run.returncode == 0,
               elapsed_seconds=round(time.monotonic()-start, 2))
    if name in ('focused_scroll', 'go_race'):
        counts = collections.Counter()
        failures = []
        for line in run.stdout.splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            action = event.get('Action')
            if event.get('Test') and action in ('pass', 'fail', 'skip'):
                counts[action] += 1
            if action == 'fail':
                failures.append({k: event.get(k) for k in ('Package', 'Test')})
        row.update(counts=dict(counts), failures=failures)
    results.append(row)
    (OUT/'checks.json').write_text(json.dumps(results, indent=2)+'\n')
    print(json.dumps(row), flush=True)
    if run.returncode:
        raise SystemExit(1)
