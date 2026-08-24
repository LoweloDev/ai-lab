#!/usr/bin/env python3
"""UX-flaw detection WITHOUT vision: model reviews the raw HTML/CSS source.
Usage: ux-dom-test.py <model-label>
Endpoint configurable: API_BASE (default local), API_KEY, API_MODEL."""
import json, os, sys, time, urllib.request

DIR = os.path.dirname(os.path.abspath(__file__))
API = os.environ.get('API_BASE', 'http://127.0.0.1:8080/v1') + '/chat/completions'
KEY = os.environ.get('API_KEY', 'sk-local')
MODEL = os.environ.get('API_MODEL', 'local')
PAGES = ['index', 'produkte', 'kontakt']

PROMPT = (
    'Du bist ein UX-Reviewer. Unten der komplette Quelltext dreier Seiten einer kleinen '
    'Shop-Website. Nenne die konkreten UX-Probleme als nummerierte Liste — pro Problem: '
    'Seite, was falsch ist, warum es Nutzer behindert. Nur echte Probleme, keine Geschmacksfragen.\n\n')

def run(label):
    src = ''
    for p in PAGES:
        src += f'===== {p}.html =====\n' + open(os.path.join(DIR, 'site', p + '.html')).read() + '\n'
    body = json.dumps({'model': MODEL, 'temperature': 0,
                       'max_tokens': int(os.environ.get('MAX_TOKENS', '6000')),
                       'messages': [{'role': 'user', 'content': PROMPT + src}]}).encode()
    req = urllib.request.Request(API, data=body, headers={'Content-Type': 'application/json', 'Authorization': f'Bearer {KEY}'})
    t0 = time.time()
    r = json.load(urllib.request.urlopen(req, timeout=900))
    out = r['choices'][0]['message'].get('content') or ''
    dt = round(time.time() - t0, 1)
    os.makedirs(os.path.join(DIR, 'runs-ux'), exist_ok=True)
    with open(os.path.join(DIR, 'runs-ux', f'{label}-dom.md'), 'w') as f:
        f.write(f'# UX-Findings (DOM, ohne Vision) {label} ({dt}s)\n\n{out}\n')
    print(f'{label}-dom: {dt}s, {len(out)} Zeichen -> runs-ux/{label}-dom.md')

if __name__ == '__main__':
    run(sys.argv[1])
