#!/usr/bin/env python3
"""Tiny test-site server for the app-control benchmark.
Serves ./site on 127.0.0.1:8090, records form POSTs to submissions.jsonl."""
import http.server, json, os, urllib.parse

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'site')
SUBS = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'submissions.jsonl')

class H(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *a, **kw):
        super().__init__(*a, directory=ROOT, **kw)
    def log_message(self, *a):
        pass
    def do_POST(self):
        if self.path == '/submit':
            n = int(self.headers.get('Content-Length', 0))
            data = urllib.parse.parse_qs(self.rfile.read(n).decode())
            with open(SUBS, 'a') as f:
                f.write(json.dumps({k: v[0] for k, v in data.items()}) + '\n')
            self.send_response(302)
            self.send_header('Location', '/danke.html')
            self.end_headers()
        else:
            self.send_response(404); self.end_headers()

if __name__ == '__main__':
    if os.path.exists(SUBS): os.remove(SUBS)
    http.server.HTTPServer(('127.0.0.1', 8090), H).serve_forever()
