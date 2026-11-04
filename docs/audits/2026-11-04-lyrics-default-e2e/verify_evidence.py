"""Assert source/install binding and persisted native-UI acceptance evidence."""
import hashlib
import json
import math
from pathlib import Path
import re

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[2]
WORK = ROOT / 'build/lyrics-default-e2e-20261104'
APP = Path('/Applications/ytmgo.app')

def read(name):
    return json.loads((OUT / name).read_text())

def ax(name):
    return (OUT / (name + '.ax.txt')).read_text()

def slider(state, description):
    match = re.search(r'Description: ' + re.escape(description) + r', Value: ([0-9.eE+-]+)', state)
    assert match, description
    return float(match.group(1))

def follow(state):
    return bool(re.search(r'\bbutton (?:Follow playback|Following playback)$', state, re.M))

checks = read('checks.json')
assert len(checks) == 10 and all(row['passed'] for row in checks)
assert read('ui-tester.json')['passed']
assert read('ui-tester.json')['snapshot_count'] == 10
assert all(hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest
           for name, digest in read('source-hashes.json').items())
manifest = read('installed-manifest.json')
actual = {}
for path in sorted(APP.rglob('*')):
    name = str(path.relative_to(APP))
    if path.is_symlink():
        actual[name] = {'link': str(path.readlink())}
    elif path.is_file():
        actual[name] = {'sha256': hashlib.sha256(path.read_bytes()).hexdigest()}
assert actual == manifest
assert read('installation.json')['signature_valid']
assert 'Now Playing' in ax('03-first-play-loaded')
assert 'Lyrics · Content' in ax('03-first-play-loaded')
assert not follow(ax('05-wide-paused'))
assert follow(ax('07-manual-scroll'))
assert not follow(ax('08-keyboard-resume'))
assert 'focused UI element is 147 scroll area Lyrics · Content' in ax('08-keyboard-resume')
assert slider(ax('09-follow-playing'), 'Playback position') > 10
assert follow(ax('18-try-manual'))
assert not follow(ax('20-mouse-resume'))
assert slider(ax('20-mouse-resume'), 'Playback position') > 170
assert 'Lyrics are not available for this track.' in ax('28-voyager-lyrics')
assert not follow(ax('28-voyager-lyrics'))
saved = read('31-restart-saved.json')
assert saved['processes'] == []
restored = ax('32-restart-paused')
assert re.search(r'\bbutton Play$', restored, re.M)
assert 'Account · YouTube Music Premium' in restored
assert math.isclose(slider(restored, 'Playback position'), saved['positionNs'] / 1e9, abs_tol=1e-7)
assert slider(restored, 'Volume') == saved['volume']
queue_state = ax('33-restored-queue').split('button Queue · Now Playing', 1)[1].split('container Player controls', 1)[0]
labels = re.findall(r'^\s*\d+ button (.+)$', queue_state, re.M)
expected = [item['title'] + ' · ' + item['artist'] for item in saved['queue']]
assert labels == expected
closed = ax('36-preplay-dismiss-loaded')
assert re.search(r'\bbutton Pause$', closed, re.M)
assert 'Lyrics · Now Playing' not in closed
assert 'button Close panel' not in closed
assert slider(closed, 'Playback position') > saved['positionNs'] / 1e9
assert read('40-final-test-exited.json')['processes'] == []
original = json.loads((WORK / 'original-playback.json').read_text())
final = read('42-final-process-and-session.json')
for key in ('version', 'index', 'positionNs', 'volume', 'unmutedVolume'):
    assert final[key] == original[key]
assert final['queue'] == [{k: item.get(k) for k in ('id', 'title', 'artist', 'album', 'duration', 'kind')}
                          for item in original['queue']]
assert [p['executable'] for p in final['processes']] == ['/Applications/ytmgo.app/Contents/MacOS/ytmgo']
final_ax = ax('41-original-state-restored')
assert re.search(r'\bbutton Play$', final_ax, re.M)
assert 'Account · YouTube Music Premium' in final_ax
assert slider(final_ax, 'Playback position') == original['positionNs'] / 1e9
assert slider(final_ax, 'Volume') == original['volume']
result = {
    'installed_main_flows_passed': True,
    'new_product_failures': [],
    'source_unchanged_during_acceptance': True,
    'installed_manifest_matches_build': True,
    'original_playback_restored': True,
    'restart_queue_count': len(saved['queue']),
    'original_queue_count': len(original['queue']),
    'native_window_size_coverage': '1920x995/996 logical pixels; compact layouts tested with native UI Tester',
    'ui_tester_sizes': ['420x640', '960x740', '1440x900'],
    'pending_physical_input_checks': [
        'Installed app in a genuinely narrow/short OS window: window drags did not obtain the requested size',
        'Global Command+Shift+Space: configuration retained; synthetic keys sent with Calculator in front did not toggle playback',
    ],
    'not_covered': ['physical diagonal trackpad gestures', 'acoustic loopback', 'network fault injection',
                    'sleep/wake', 'crash/force-quit recovery', 'account sign-out/switching',
                    'Windows/Linux', 'long-duration soak', 'public notarized release package'],
}
(OUT / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result))
