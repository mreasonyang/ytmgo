"""Validate captured E2E observations without launching or changing the app."""
import json
import math
import re
from pathlib import Path

directory = Path(__file__).resolve().parent
read = lambda name: json.loads((directory / name).read_text())
samples = read('ui-samples.json')
by_name = {sample['name']: sample for sample in samples}
original = read('00-original-session.json')
playing = read('06-playing-quit-session.json')
muted = read('11-muted-quit-session.json')
paused = read('18-paused-quit-session.json')

def close(a, b):
    return math.isclose(a, b, abs_tol=1e-9)

def same_queue(name):
    ax = (directory / (name + '.ax.txt')).read_text()
    panel = ax.split('button Queue · Now Playing', 1)[1].split('container Player controls', 1)[0]
    labels = re.findall(r'^\t\t\d+ button (.+)$', panel, re.MULTILINE)
    expected = [item['title'] + ' · ' + item['artist'] for item in original['queue']]
    assert labels == expected, name

for name in ('09-restored-quiet-after-account', '13-unmuted-restored-volume', '19-paused-reopened', '22-rapid-previous-settled', '24-original-state-restored'):
    same_queue(name)

assert len(original['queue']) == 13
assert playing['queue'] == original['queue']
assert playing['index'] == 12
for name in ('07-playing-quit-reopened', '09-restored-quiet-after-account'):
    sample = by_name[name]
    assert not sample['playing']
    assert close(sample['position'], playing['positionNs'] / 1e9)
    assert close(sample['volume'], .35)
assert by_name['09-restored-quiet-after-account']['premium']

start, end = by_name['04-resumed-playing'], by_name['05-before-playing-quit']
assert start['playing'] and end['playing']
assert 84.077 <= start['position'] < 86
assert end['position'] > start['position'] + 5

assert muted['index'] == 12 and muted['positionNs'] == playing['positionNs']
assert muted['volume'] == 0 and close(muted['unmutedVolume'], .35)
assert not by_name['12-muted-reopened']['playing']
assert by_name['12-muted-reopened']['volume'] == 0
assert close(by_name['12-muted-reopened']['position'], playing['positionNs'] / 1e9)
assert close(by_name['13-unmuted-restored-volume']['volume'], .35)

assert by_name['14-switch-interior-track']['position'] == 0
assert by_name['15-interior-track-playing']['playing']
assert by_name['15-interior-track-playing']['current'] == 'Within Daft Punk'
assert paused['index'] == 3 and paused['volume'] == .7
for name in ('17-paused-seek-ready', '19-paused-reopened'):
    sample = by_name[name]
    assert sample['current'] == 'Within Daft Punk' and not sample['playing'] and not sample['loading']
    assert close(sample['position'], paused['positionNs'] / 1e9)

assert by_name['20-restored-next-track']['current'].startswith('Instant Crush')
assert by_name['20-restored-next-track']['position'] == 0
assert by_name['21-rapid-previous-track']['current'] == 'Within Daft Punk'
assert by_name['22-rapid-previous-settled']['current'] == 'Within Daft Punk'
assert by_name['22-rapid-previous-settled']['playing']

artifact = read('artifact.json')
app_executable = artifact['appPath'] + '/Contents/MacOS/ytmgo'
original_pids = {row['pid'] for row in artifact['processes'] if row['executable'] == app_executable}
restarted_pids = {row['pid'] for row in read('08-playing-restart-process.json')['processes'] if row['executable'] == app_executable}
assert original_pids and restarted_pids and original_pids.isdisjoint(restarted_pids)
for name in ('06-playing-quit-session.json', '11-muted-quit-session.json', '18-paused-quit-session.json', '23-final-test-quit.json'):
    data = read(name)
    assert data['mode'] == '0o600'
    assert not any(row['executable'] == app_executable for row in data['processes'])

cleanup = by_name['24-original-state-restored']
assert cleanup['current'] == 'Contact Daft Punk' and not cleanup['playing']
assert cleanup['volume'] == .7 and close(cleanup['position'], original['positionNs'] / 1e9)

result = {
    'status': 'PASS',
    'platform': 'macOS arm64',
    'restartCyclesUnderTest': 3,
    'cleanupLaunches': 1,
    'cases': [
        'Restored paused seek and manual resume from the edited bookmark',
        'Playing quit: exact bookmark, volume, selected track and ordered queue restored',
        'Asynchronous Premium account restore leaves playback paused',
        'Muted quit and a second restart without playback retain the bookmark and unmute volume',
        'Interior queue selection starts at zero; paused seek survives quit and restart',
        'Restored queue index drives Next; rapid Previous wins over the pending load',
    ],
    'productSourceEdited': False,
    'limitations': ['Normal graceful quits only; no crash or forced-kill durability test', 'No Windows/Linux, account switch, sign-out, offline recovery, sleep/wake, or release-package acceptance', 'Playback verified by native UI/decoder position advancement; no acoustic loopback recording'],
}
(directory / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result, indent=2))
