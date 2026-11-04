"""Verify recorded native UI, source, decoder and restoration evidence."""
import hashlib
import json
from pathlib import Path
import re
from PIL import Image

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[2]
def read(name):
    return (OUT / (name+".ax.txt")).read_text()
def position(name):
    return float(re.search(r"Description: Playback position, Value: ([0-9.]+)", read(name))[1])
def paused(name):
    assert re.search(r"button Play\n", read(name)), name
    assert not re.search(r"button Pause\n", read(name)), name
def following(name):
    assert "button Follow playback" not in read(name), name

provider=json.loads((OUT/'provider-cues.json').read_text())
assert provider['passed'] and provider['anonymous']
cue_starts=[c['start'] for c in provider['cues']]
expected={
    '08-paused-drag-settled':55.03, '10-paused-backward-settled':51.43,
    '11-click-no-seek':51.43, '12-horizontal-no-seek':51.43,
    '13-wheel-manual-no-seek':51.43, '14-drag-from-manual-reading':89.89,
    '15-manual-drag-settled':89.89, '17-playing-drag-release':103,
    '19-outside-release-last-cue':155.48, '20-manual-top':155.48,
    '21-first-cue-boundary':0, '24-compact-drag':51.43,
    '28-narrow-standard-drag':55.03, '30-narrow-expanded-drag':59.45,
    '33-immersive-drag':64.2, '43-pre-restart-settled':64.2,
    '45-restart-paused':64.2,
}
for name, want in expected.items():
    assert position(name)==want, (name, position(name), want)
    assert want in cue_starts, name
for name in ['08-paused-drag-settled','10-paused-backward-settled','11-click-no-seek',
             '12-horizontal-no-seek','13-wheel-manual-no-seek','15-manual-drag-settled',
             '19-outside-release-last-cue','43-pre-restart-settled','45-restart-paused']:
    paused(name)
for name in ['08-paused-drag-settled','10-paused-backward-settled','11-click-no-seek',
             '12-horizontal-no-seek','14-drag-from-manual-reading','18-playing-drag-advanced',
             '19-outside-release-last-cue','21-first-cue-boundary','28-narrow-standard-drag',
             '30-narrow-expanded-drag','33-immersive-drag']:
    following(name)
assert 'button Follow playback' in read('13-wheel-manual-no-seek')
assert position('18-playing-drag-advanced') > position('17-playing-drag-release')+2
assert 'button Pause\n' in read('18-playing-drag-advanced')
assert position('37-unavailable-visible')==position('38-unavailable-drag-no-seek')
paused('38-unavailable-drag-no-seek')
assert 'Lyrics are not available for this track.' in read('38-unavailable-drag-no-seek')
assert 'Drag lyrics to seek on release' not in read('38-unavailable-drag-no-seek')
assert 'Writing your name' not in read('38-unavailable-drag-no-seek')
saved=json.loads((OUT/'44-restart-saved.json').read_text())
assert saved['recording_id']==provider['recording_id'] and saved['position_seconds']==64.2
assert saved['processes']==[]
checks=json.loads((OUT/'checks.json').read_text())['checks']
assert len(checks)==10 and all(c['passed'] for c in checks)
decoder=json.loads((OUT/'decoder-checks.json').read_text())
assert decoder['passed'] and len(decoder['cases'])==26
base=json.loads((OUT/'baseline.json').read_text())
assert all(hashlib.sha256((ROOT/p).read_bytes()).hexdigest()==h for p,h in base['source_sha256'].items())
assert hashlib.sha256(Path('/Applications/ytmGO.app/Contents/MacOS/ytmGO').read_bytes()).hexdigest()==base['binary_sha256']
dimensions={}
for name in ['04-retry','27-system-compact']:
    with Image.open(OUT/(name+'.jpg')) as screenshot:
        dimensions[name]=screenshot.size
assert dimensions['27-system-compact']==(1514,1282)
assert position('46-restart-account-restored')==64.2
paused('46-restart-account-restored')
assert 'Account · YouTube Music Premium' in read('46-restart-account-restored')
restoration=json.loads((OUT/'49-original-restoration.json').read_text())
assert restoration['queue_equal'] and restoration['position_equal'] and restoration['volume_equal']
assert restoration['processes_before_restore']==[]
assert position('50-final-original-state')==29.765
paused('50-final-original-state')
assert 'Account · YouTube Music Premium' in read('50-final-original-state')
assert restoration['final_process_executable']=='/Applications/ytmGO.app/Contents/MacOS/ytmGO'
result={'feature_status':'PASS','live_seek_targets_seconds':sorted(set(expected.values())),
        'go_test_counts':checks[0]['counts'],'decoder_cases':len(decoder['cases']),
        'native_screenshot_pixels':dimensions,'source_and_installed_feature_match':True,
        'original_playback_restored':True,'network_incident':'bootstrap_network_timeout; Retry recovered; root cause not established',
        'scope':'macOS installed feature acceptance; read README for automated-only and untested boundaries'}
(OUT/'result.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result))
