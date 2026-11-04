"""Read only playback metadata and credential-safe process identities."""
import datetime
import json
from pathlib import Path
import subprocess
import sys

out = Path(__file__).resolve().parent
path = Path('/Users/easonyang/Library/Application Support/ytmgo/playback-session.json')
data = json.loads(path.read_text())
sample = {k: data.get(k) for k in ('version', 'index', 'positionNs', 'volume', 'unmutedVolume')}
sample['queue'] = [
    {k: item.get(k) for k in ('id', 'title', 'artist', 'album', 'duration', 'kind')}
    for item in data.get('queue', [])
]
sample['time'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
sample['processes'] = json.loads(subprocess.check_output([
    '/Users/easonyang/.codex/bin/safe-processes', '--name', 'ytmgo'
], text=True))
(out / (sys.argv[1] + '.json')).write_text(json.dumps(sample, indent=2) + '\n')
print(json.dumps({k: sample[k] for k in ('index', 'positionNs', 'volume', 'processes')}))
