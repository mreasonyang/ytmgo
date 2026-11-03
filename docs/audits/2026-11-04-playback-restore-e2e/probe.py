"""Capture only playback metadata and credential-safe process identities."""
import datetime
import json
from pathlib import Path
import subprocess
import sys

directory = Path(__file__).resolve().parent
session_path = Path('/Users/easonyang/Library/Application Support/ytmgo/playback-session.json')
session = json.loads(session_path.read_text())
sample = {key: session.get(key) for key in ('version', 'index', 'positionNs', 'volume', 'unmutedVolume')}
sample['queue'] = [
    {key: item.get(key) for key in ('id', 'title', 'artist', 'album', 'duration', 'kind')}
    for item in session.get('queue', [])
]
sample['time'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
sample['mode'] = oct(session_path.stat().st_mode & 0o777)
sample['processes'] = json.loads(subprocess.check_output([
    '/Users/easonyang/.codex/bin/safe-processes', '--name', 'ytmgo'
], text=True))
(directory / (sys.argv[1] + '.json')).write_text(json.dumps(sample, indent=2) + '\n')
print(json.dumps({key: sample[key] for key in ('index', 'positionNs', 'volume', 'unmutedVolume', 'mode', 'processes')}))
