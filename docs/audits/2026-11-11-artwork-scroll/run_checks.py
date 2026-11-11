#!/usr/bin/env python3
"""Keep structured test events only; never store or print raw diagnostics."""
import argparse
import json
import os
import re
from pathlib import Path
import subprocess
import time

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
CHECKS = {
    'go-tests': ['go', 'test', '-json', '-count=1', './...'],
    'artwork-race': ['go', 'test', '-race', '-json', '-run', '^TestArtwork', '-count=1', '-timeout=120s', '.'],
    'go-race': ['go', 'test', '-race', '-json', '-count=1', '-timeout=600s', './...'],
    'go-vet': ['go', 'vet', './...'],
    'linux-build': ['go', 'build', './...'],
    'windows-build': ['go', 'build', './...'],
    'i18n': ['go', 'tool', 'mygo', 'i18n', 'check'],
}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', choices=CHECKS, required=True)
    args = parser.parse_args()
    command = CHECKS[args.check]
    # Disable opt-in account/live fixtures. Do not inspect or return values.
    for name in tuple(os.environ.keys()):
        if name.startswith(('YTMGO_LIVE_', 'YTMGO_TEST_', 'YTMGO_ARTWORK_', 'MYGO_FRAME_STATS')):
            del os.environ[name]
    if args.check in ('linux-build','windows-build'):
        os.environ['GOOS'] = args.check.split('-')[0]
        os.environ['GOARCH'] = 'amd64'
        os.environ['CGO_ENABLED'] = '0'
    started = time.monotonic()
    child = subprocess.Popen(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    events = []
    race_locations = set()
    diagnostics = []
    for line in child.stdout:
        if '-json' not in command:
            continue
        try:
            item = json.loads(line)
        except (ValueError, TypeError):
            continue
        output = item.get('Output','')
        if 'WARNING: DATA RACE' in output:
            diagnostics.append('data_race_reported')
        if 'test timed out' in output:
            diagnostics.append('test_timeout')
        match = re.match(r'^\s+(/.*?\.go):([0-9]+)',output)
        if match:
            race_locations.add(match.group(1)+':'+match.group(2))
        if item.get('Action') in ('pass', 'skip', 'fail'):
            events.append({k:item[k] for k in ('Action','Package','Test','Elapsed') if k in item})
    code = child.wait()
    result = {'command':command, 'package_count':len({e.get('Package') for e in events if 'Package' in e}), 'exit_code':code, 'elapsed_seconds':round(time.monotonic()-started,3),
              'passed_tests':sum(e['Action']=='pass' and 'Test' in e for e in events),
              'skipped_tests':sum(e['Action']=='skip' and 'Test' in e for e in events),
              'diagnostic_categories':sorted(set(diagnostics)), 'race_source_locations':sorted(race_locations), 'failed_tests':[e for e in events if e['Action']=='fail'], 'package_results':[e for e in events if 'Test' not in e]}
    (OUT/(args.check+'.json')).write_text(json.dumps(result,indent=2)+'\n')
    print(json.dumps({k:v for k,v in result.items() if k!='package_results'}))
    return code

if __name__ == '__main__':
    raise SystemExit(main())
