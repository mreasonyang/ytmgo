"""Independently verify public dual-architecture updater bytes and all Mach-Os."""
import json
from pathlib import Path
import sys
import tarfile

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / 'scripts/release'))
import macos
import macos_compat

OUT = ROOT / 'build/macos-release-0.6.4-public'
AUDIT = Path(__file__).resolve().parent
SOURCE = '23b4118cd459de7f12ebcee734cbd99e704f6457'


def main():
    sums = dict(line.split('  ', 1)[::-1] for line in (OUT / 'SHA256SUMS').read_text().splitlines())
    for name in ('build.json', 'build-amd64.json', 'update-darwin-arm64.json', 'update-darwin-amd64.json'):
        assert macos.digest(OUT / name) == sums[name]
    helper = ROOT / 'build/release/update-package-064'
    macos.run(['go', 'build', '-o', helper, './cmd/update-package'], 'Build public signature verifier')
    macos.ROOT = AUDIT
    records = []
    for arch in ('arm64', 'amd64'):
        metadata = json.loads((OUT / ('build.json' if arch == 'arm64' else 'build-amd64.json')).read_text())
        assert metadata['version'] == '0.6.4' and metadata['commit'] == SOURCE
        assert metadata['architecture'] == arch and metadata['minimum_macos'] == '13.0'
        assert all(metadata[key] is True for key in ('notarized','developer_id_signed','signed_updates'))
        archive = OUT / ('ytmgo-0.6.4-darwin-' + arch + '.tar.gz')
        assert macos.digest(archive) == sums[archive.name]
        macos.run([helper, '--verify', '--arch', arch, '--out', OUT, '--config', AUDIT / 'mygo.json'], 'Verify public Ed25519 signature')
        destination = OUT / arch
        destination.mkdir(exist_ok=True)
        with tarfile.open(archive) as stream:
            for member in stream:
                parts = Path(member.name).parts
                assert parts and parts[0] == 'ytmGO.app' and '..' not in parts
                assert not member.name.startswith('/') and not member.isdev()
        # This source-produced archive has already passed the independent
        # Ed25519 check. Use native tar to preserve the signed bundle's modes.
        macos.run(['tar', '-xzf', archive, '-C', destination], 'Extract verified public updater bundle')
        app = destination / 'ytmGO.app'
        binaries = macos_compat.inspect(app, arch, '13.0')
        macos.run(['codesign', '--verify', '--deep', '--strict', app], 'Verify public app signature')
        macos.run(['xcrun', 'stapler', 'validate', app], 'Verify public app notarization ticket')
        macos.run(['spctl', '--assess', '--type', 'execute', app], 'Assess public app with Gatekeeper')
        records.append({'architecture': arch, 'archive_sha256': sums[archive.name], 'ed25519_signature': True,
                        'developer_id_signature': True, 'notarization_ticket': True, 'gatekeeper': True,
                        'binaries': binaries})
    (AUDIT / 'public-verification.json').write_text(json.dumps({'version': '0.6.4', 'source': SOURCE,
        'minimum_macos': '13.0', 'architectures': records, 'native_execution': 'separate native CI acceptance receipts'}, indent=2)+'\n')
    print('Public updater signatures, deployment targets, architectures, app signatures and tickets passed')


if __name__ == '__main__':
    try:
        main()
    except Exception:
        sys.exit('Public macOS verification failed; raw diagnostics omitted')
