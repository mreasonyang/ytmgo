"""Keep fixed test identifiers, allowlisted categories and numeric metrics only."""
import json
from pathlib import Path
import re
import sys

events = []
for line in sys.stdin:
    try:
        events.append(json.loads(line))
    except ValueError:
        pass
output = ''.join(x.get('Output', '') for x in events)
categories = [value for value in (
    'context deadline exceeded', 'bootstrap_network_timeout',
    'bootstrap_network_failed', 'bootstrap_tls_failed', 'script_process_failed',
    'connection refused', 'no such host', 'proxy_configuration_unavailable',
    'catalog_request_failed', 'service_unavailable', 'catalog_fetch_failed',
    'audio_stream_failed', 'proof_failed', 'resolver_http_error',
) if value in output]
metrics = []
for pattern in (
    r'real search: first_requests=\d+ repeated_requests=\d+ items=\d+',
    r'real lyrics: first_requests=\d+ repeated_requests=\d+ timed_lines=\d+',
    r'public player cache: cold=[0-9.a-zµ]+ restart=[0-9.a-zµ]+ code_bytes=\d+ restart_network_requests=0 restart_extractions=0',
):
    metrics.extend(re.findall(pattern, output))
row = {
    'check': sys.argv[1],
    'packages': [{k: x[k] for k in ('Package', 'Action', 'Elapsed') if k in x}
                 for x in events if 'Test' not in x and x.get('Action') in ('pass', 'fail')],
    'counts': {action: sum(x.get('Action') == action and bool(x.get('Test')) for x in events)
               for action in ('pass', 'fail', 'skip')},
    'categories': categories,
    'locations': sorted(set(re.findall(r'\b\w+_test\.go:\d+', output))),
    'metrics': metrics,
}
Path(__file__).with_name(sys.argv[1] + '.json').write_text(json.dumps(row, indent=2) + '\n')
print(json.dumps(row))
