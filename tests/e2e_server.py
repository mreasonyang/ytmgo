"""Loopback-only browser E2E server with an isolated real preview backend.

Run: python3 tests/e2e_server.py --upstream http://127.0.0.1:PORT
Edit .mygo/e2e/scenario.json to inject HTTP failures/delays or empty searches.
An empty scenario uses real anonymous catalog requests and real local persistence.
This helper is never embedded in the desktop application.
"""
import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import Request, build_opener, ProxyHandler

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / 'web'
SCENARIO = ROOT / '.mygo/e2e/scenario.json'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--upstream', required=True)
    args = parser.parse_args()
    target = urlsplit(args.upstream)
    if target.scheme != 'http' or target.hostname != '127.0.0.1' or not target.port:
        parser.error('upstream must be a loopback preview URL')
    opener = build_opener(ProxyHandler({}))

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass  # Never emit arbitrary request bodies, headers, or upstream logs.

        def respond(self, status, body, content_type='application/json; charset=utf-8'):
            self.send_response(status)
            self.send_header('Content-Type', content_type)
            self.send_header('Content-Length', str(len(body)))
            self.send_header('Cache-Control', 'no-store')
            self.send_header('Referrer-Policy', 'strict-origin-when-cross-origin')
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            self.handle_request()

        def do_PUT(self):
            self.handle_request()

        def handle_request(self):
            path = urlsplit(self.path).path
            if not path.startswith('/api/'):
                file = (WEB / ('index.html' if path == '/' else path.lstrip('/'))).resolve()
                if WEB not in file.parents or not file.is_file():
                    self.respond(404, b'{}')
                    return
                mime = {'.html': 'text/html', '.css': 'text/css', '.js': 'application/javascript'}.get(file.suffix, 'application/octet-stream')
                self.respond(200, file.read_bytes(), mime + '; charset=utf-8')
                return
            try:
                scenario = json.loads(SCENARIO.read_text()) if SCENARIO.exists() else {}
                key = self.command + ' ' + path
                time.sleep(min(5, max(0, scenario.get('delayMs', {}).get(key, 0) / 1000)))
                if key in scenario.get('fail', []):
                    self.respond(503, b'{"error":"E2E injected failure"}')
                    return
                if path == '/api/search' and scenario.get('emptySearch'):
                    self.respond(200, b'{"tracks":[]}')
                    return
                body = self.rfile.read(min(int(self.headers.get('Content-Length', 0)), 4 << 20)) if self.command == 'PUT' else None
                request = Request(args.upstream + self.path, data=body, method=self.command, headers={
                    'Content-Type': 'application/json', 'Origin': args.upstream, 'X-Ytmgo-Client': '1',
                })
                with opener.open(request, timeout=35) as response:
                    self.respond(response.status, response.read(4 << 20))
            except HTTPError as error:
                self.respond(error.code, b'{"error":"upstream failure"}')
            except (URLError, OSError, ValueError):
                self.respond(502, b'{"error":"E2E backend unavailable"}')

    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    print(f'E2E browser server: http://127.0.0.1:{server.server_port}', flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == '__main__':
    main()
