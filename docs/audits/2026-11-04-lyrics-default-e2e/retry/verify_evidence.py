"""Verify retry evidence against the unchanged installed app, not a moving worktree."""
import hashlib
import json
import re
import subprocess
from pathlib import Path
from PIL import Image, ImageChops, ImageStat

OUT = Path(__file__).resolve().parent
BASE = OUT.parent
ROOT = BASE.parents[2]
APP = Path('/Applications/ytmgo.app')

def ax(name):
    return (OUT / (name + '.ax.txt')).read_text()

def position(text):
    return float(re.search(r'Description: Playback position, Value: ([0-9.eE+-]+)', text)[1])

def follow(text):
    return bool(re.search(r'\bbutton (?:Follow playback|Following playback)$', text, re.M))

expected = json.loads((BASE / 'installed-manifest.json').read_text())
actual = {}
for p in APP.rglob('*'):
    name = str(p.relative_to(APP))
    if p.is_symlink():
        actual[name] = {'link': str(p.readlink())}
    elif p.is_file():
        actual[name] = {'sha256': hashlib.sha256(p.read_bytes()).hexdigest()}
assert actual == expected
assert Image.open(OUT / '09-compact-before-restart.jpg').size == (1514, 1282)
first = ax('11-compact-first-play')
assert 'Lyrics · Now Playing' in first and 'Lyrics · Content' in first
assert re.search(r'\bbutton Pause$', first, re.M) and not follow(first)
assert Image.open(OUT / '11-compact-first-play.jpg').size == (1514, 1282)
assert position(ax('12-compact-seek')) > 170
assert not follow(ax('12-compact-seek'))
assert follow(ax('15-compact-manual'))
assert not follow(ax('16-compact-resumed'))
before = ax('13-compact-cover-reading')
after = ax('14-compact-cover-later')
assert position(after) - position(before) > 20
assert not follow(before) and not follow(after)
# Compare the observed stationary album-cover region, without altering evidence images.
region = (647, 294, 867, 514)
one = Image.open(OUT / '13-compact-cover-reading.jpg').convert('RGB').crop(region)
two = Image.open(OUT / '14-compact-cover-later.jpg').convert('RGB').crop(region)
cover_mae = sum(ImageStat.Stat(ImageChops.difference(one, two)).mean) / 3
assert cover_mae < 2, cover_mae
assert 'Exit immersive mode' in ax('17-compact-immersive')
assert 'Lyrics · Content' in ax('17-compact-immersive')
assert 'Unable to play this track.' in ax('04-playback-error')
assert json.loads((OUT / '05-audio-status.json').read_text())['Category'] == 'audio_stream_failed'
assert position(ax('06-after-retry')) > position(ax('04-playback-error'))
assert 'People Help the People' in ax('07-short-revealed')
assert position(ax('18-global-hotkey-foreground')) == position(ax('17-compact-immersive'))
assert re.search(r'\bbutton Play$', ax('18-global-hotkey-foreground'), re.M)
original = json.loads((ROOT / 'build/lyrics-default-e2e-20261104/original-playback.json').read_text())
saved = json.loads(Path('/Users/easonyang/Library/Application Support/ytmgo/playback-session.json').read_text())
assert original == saved
final = ax('21-final-restored')
assert position(final) == original['positionNs'] / 1e9
assert 'Account · YouTube Music Premium' in final
assert re.search(r'\bbutton Play$', final, re.M)
assert Image.open(OUT / '21-final-restored.jpg').size == (3840, 1992)
processes = json.loads(subprocess.check_output([
    '/Users/easonyang/.codex/bin/safe-processes', '--name', 'ytmgo'
], text=True))
assert [p['executable'] for p in processes] == ['/Applications/ytmgo.app/Contents/MacOS/ytmgo']
hashes = json.loads((BASE / 'source-hashes.json').read_text())
source_drift = [f for f, h in hashes.items() if hashlib.sha256((ROOT / f).read_bytes()).hexdigest() != h]
result = {
    'compact_lyrics_native_e2e': 'PASS',
    'complete_app_acceptance': False,
    'installed_manifest_unchanged': True,
    'binary_sha256': actual['Contents/MacOS/ytmgo']['sha256'],
    'native_compact_size': [757, 641],
    'native_short_size': [961, 641],
    'outer_cover_mean_pixel_difference': cover_mae,
    'outer_stability_playback_seconds': [position(before), position(after)],
    'audio_incident': {'category': 'audio_stream_failed', 'recovered_by_retry': True, 'root_cause': 'unresolved'},
    'global_shortcut': 'configured, synthetic foreground retry did not toggle; physical input unverified',
    'current_source_drift_from_initial_build': source_drift,
    'original_playback_restored': True,
    'original_queue_count': len(original['queue']),
    'final_processes': processes,
}
(OUT / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result))
