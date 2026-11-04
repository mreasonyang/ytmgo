"""Return only exact, source-defined compatibility failure categories."""
import ast
from pathlib import Path
import subprocess
import argparse

parser = argparse.ArgumentParser()
parser.add_argument('--run', default='41842662566', type=int)
parser.add_argument('--component', choices=('macos_compat', 'publish'), default='macos_compat')
options = parser.parse_args()

ROOT = Path(__file__).resolve().parents[3]
tree = ast.parse((ROOT / 'scripts/release' / (options.component + '.py')).read_text())
allowed = {'macOS compatibility check failed', 'operation failed'}
for node in ast.walk(tree):
    if isinstance(node, ast.Call) and node.args:
        name = node.func.attr if isinstance(node.func, ast.Attribute) else node.func.id if isinstance(node.func, ast.Name) else ''
        value = node.args[0].value if isinstance(node.args[0], ast.Constant) else None
        if isinstance(value, str) and name in ('PackageError', 'ReleaseError'):
            allowed.add(value)
        if name in ('run', 'gh') and len(node.args) > 1:
            stage = node.args[1]
            if isinstance(stage, ast.Constant) and isinstance(stage.value, str):
                allowed.add(stage.value + ' failed')
result = subprocess.run(['gh', 'run', 'view', str(options.run), '--repo', 'mreasonyang/ytmgo', '--log-failed'],
                        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False, timeout=120)
if result.returncode:
    import re
    statuses = re.findall(rb'HTTP ([1-5][0-9]{2})', result.stderr)
    print({'log_http_status': [int(status) for status in statuses]})
    raise SystemExit('Fixed-category lookup unavailable')
found = sorted(category for category in allowed if category.encode() in result.stdout)
print({'compatibility_failure_categories': found})
