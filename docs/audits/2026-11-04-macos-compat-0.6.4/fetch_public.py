"""Fetch official public release byte ranges and verify whole-file SHA-256."""
from concurrent.futures import ThreadPoolExecutor, as_completed
import hashlib
import json
from pathlib import Path
import shutil
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / "build/macos-release-0.6.4-public"
BASE = 'https://github.com/mreasonyang/ytmgo/releases/download/v0.6.4/'
CHUNK = 1024 * 1024


def get(url, headers=None):
    h = {'User-Agent': 'ytmgo-public-verifier', 'Accept-Encoding': 'identity'}
    h.update(headers or {})
    return urllib.request.urlopen(urllib.request.Request(url, headers=h), timeout=90)


def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        while data := f.read(CHUNK):
            h.update(data)
    return h.hexdigest()


def main():
    with get('https://api.github.com/repos/mreasonyang/ytmgo/releases/tags/v0.6.4') as response:
        release = json.load(response)
    names = ['build.json', 'build-amd64.json', 'SHA256SUMS']
    for arch in ('arm64', 'amd64'):
        names += ['ytmgo-0.6.4-darwin-' + arch + '.tar.gz', 'update-darwin-' + arch + '.json']
    assets = {a['name']: a for a in release['assets'] if a['name'] in names}
    assert len(assets) == 7
    OUT.mkdir(parents=True, exist_ok=True)
    parts = OUT / 'range-parts'
    parts.mkdir(exist_ok=True)
    tasks, remaining, ranges = [], {}, {}
    for name in names:
        asset = assets[name]
        assert asset['browser_download_url'] == BASE + name
        expected = asset['digest'].removeprefix('sha256:')
        dest = OUT / name
        if dest.exists() and sha(dest) == expected:
            print(json.dumps({'file': name, 'verified': True, 'cached': True}), flush=True)
            continue
        folder = parts / name
        folder.mkdir(exist_ok=True)
        prefix = OUT / (name + '.partial')
        spans = [(start, min(start + CHUNK, asset['size']) - 1) for start in range(0, asset['size'], CHUNK)]
        ranges[name] = spans
        remaining[name] = 0
        for i, (start, end) in enumerate(spans):
            target = folder / str(i)
            if not target.exists() and prefix.exists() and prefix.stat().st_size > end:
                with prefix.open('rb') as source:
                    source.seek(start)
                    target.write_bytes(source.read(end - start + 1))
            if target.exists() and target.stat().st_size == end - start + 1:
                continue
            tasks.append((name, i, start, end, target))
            remaining[name] += 1

    def fetch(task):
        name, i, start, end, target = task
        size = end - start + 1
        for attempt in range(3):
            try:
                with get(BASE + name, {'Range': 'bytes=' + str(start) + '-' + str(end)}) as response:
                    assert response.status == 206 or (start == 0 and size == assets[name]['size'] and response.status == 200)
                    data = response.read(size + 1)
                assert len(data) == size
                target.write_bytes(data)
                return name
            except Exception:
                if attempt == 2:
                    raise RuntimeError('public_range_transfer_failed')
                time.sleep(attempt + 1)

    def assemble(name):
        temp = OUT / (name + '.assembled')
        with temp.open('wb') as dest:
            for i in range(len(ranges[name])):
                with (parts / name / str(i)).open('rb') as source:
                    shutil.copyfileobj(source, dest)
        assert temp.stat().st_size == assets[name]['size']
        assert assets[name]['digest'] == 'sha256:' + sha(temp)
        temp.replace(OUT / name)
        print(json.dumps({'file': name, 'verified': True, 'bytes': assets[name]['size']}), flush=True)

    for name, count in remaining.items():
        if count == 0:
            assemble(name)
    with ThreadPoolExecutor(max_workers=16) as pool:
        for future in as_completed([pool.submit(fetch, task) for task in tasks]):
            name = future.result()
            remaining[name] -= 1
            if remaining[name] == 0:
                assemble(name)
    print(json.dumps({'verified_official_downloads': 7}), flush=True)


if __name__ == '__main__':
    try:
        main()
    except Exception:
        raise SystemExit('Public parallel download did not pass; raw diagnostics omitted')
