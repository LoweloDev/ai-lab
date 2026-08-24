#!/usr/bin/env python3
"""UX-flaw detection test: feed page screenshots to the vision server,
collect findings. Ground truth: UX-FLAWS.md (5 planted flaws).
Usage: ux-vision-test.py <model-label>"""
import base64, json, os, sys, time, urllib.request

DIR = os.path.dirname(os.path.abspath(__file__))
API = 'http://127.0.0.1:8080/v1/chat/completions'
PAGES = ['index', 'index-b', 'produkte', 'kontakt', 'danke']

PROMPT = (
    'Du bist ein UX-Reviewer. Das sind Screenshots einer kleinen Shop-Website: '
    'Bild 1 und 2 sind ZWEI MOMENTAUFNAHMEN derselben Startseite (kurz nacheinander), '
    'dann Produktliste, Kontaktformular und die Seite nach dem Absenden des Formulars. '
    'Nenne die konkreten UX-Probleme, die du siehst — als nummerierte Liste, pro Problem: '
    'Seite, was falsch ist, warum es Nutzer behindert. Achte auch auf Probleme, die sich erst '
    'über mehrere Seiten hinweg oder im Ablauf zeigen. Nur echte Probleme, keine Geschmacksfragen.')

def b64(p):
    return base64.b64encode(open(p, 'rb').read()).decode()

def run(label):
    content = [{'type': 'image_url', 'image_url': {'url': f'data:image/png;base64,{b64(os.path.join(DIR, "shots", p + ".png"))}'}}
               for p in PAGES]
    content.append({'type': 'text', 'text': PROMPT})
    body = json.dumps({'model': 'local', 'temperature': 0, 'max_tokens': 6000,
                       'messages': [{'role': 'user', 'content': content}]}).encode()
    req = urllib.request.Request(API, data=body, headers={'Content-Type': 'application/json', 'Authorization': 'Bearer sk-local'})
    t0 = time.time()
    r = json.load(urllib.request.urlopen(req, timeout=900))
    out = r['choices'][0]['message'].get('content') or ''
    dt = round(time.time() - t0, 1)
    os.makedirs(os.path.join(DIR, 'runs-ux'), exist_ok=True)
    with open(os.path.join(DIR, 'runs-ux', f'{label}.md'), 'w') as f:
        f.write(f'# UX-Findings {label} ({dt}s)\n\n{out}\n')
    print(f'{label}: {dt}s, {len(out)} Zeichen -> runs-ux/{label}.md')

if __name__ == '__main__':
    run(sys.argv[1])
