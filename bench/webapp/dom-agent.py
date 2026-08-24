#!/usr/bin/env python3
"""DOM navigation harness: a local model steers a website via text snapshots.
Usage: dom-agent.py <model-label> <task:info|form> [--max-steps N]
Grades objectively; transcript to runs-dom/<label>-<task>.jsonl"""
import json, os, re, sys, time, urllib.request, urllib.parse
from html.parser import HTMLParser

BASE = 'http://127.0.0.1:8090'
API = os.environ.get('API_BASE', 'http://127.0.0.1:8080/v1') + '/chat/completions'
API_KEY = os.environ.get('API_KEY', 'sk-local')
API_MODEL = os.environ.get('API_MODEL', 'local')
DIR = os.path.dirname(os.path.abspath(__file__))
SUBS = os.path.join(DIR, 'submissions.jsonl')

class Snap(HTMLParser):
    def __init__(self):
        super().__init__(); self.els = []; self.stack = []; self.text = []
        self._cur = None
    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if tag == 'a' and 'href' in a:
            self._cur = {'tag': 'link', 'href': a['href'], 'text': ''}
        elif tag == 'button':
            self._cur = {'tag': 'button', 'text': '', 'type': a.get('type', 'submit')}
        elif tag == 'input':
            self.els.append({'tag': 'input', 'name': a.get('name', ''), 'itype': a.get('type', 'text'),
                             'placeholder': a.get('placeholder', ''), 'id': a.get('id', '')})
        elif tag == 'textarea':
            self._cur = {'tag': 'textarea', 'name': a.get('name', ''), 'text': ''}
        elif tag in ('h1', 'h2', 'th', 'td', 'label', 'title', 'p'):
            self.stack.append(tag)
    def handle_endtag(self, tag):
        if self._cur and tag in ('a', 'button', 'textarea'):
            self.els.append(self._cur); self._cur = None
        elif self.stack and self.stack[-1] == tag:
            self.stack.pop()
    def handle_data(self, d):
        d = d.strip()
        if not d: return
        if self._cur is not None: self._cur['text'] += d
        elif self.stack: self.text.append(d)

def snapshot(path):
    html = urllib.request.urlopen(BASE + path, timeout=10).read().decode()
    s = Snap(); s.feed(html)
    lines = [f'AKTUELLE SEITE: {path}', 'SICHTBARER TEXT: ' + ' | '.join(s.text[:60]), 'ELEMENTE:']
    refs = []
    for e in s.els:
        i = len(refs)
        if e['tag'] == 'link':
            lines.append(f'  [{i}] Link "{e["text"]}" -> {e["href"]}')
        elif e['tag'] == 'button':
            lines.append(f'  [{i}] Button "{e["text"]}" (type={e["type"]})')
        elif e['tag'] == 'input':
            lines.append(f'  [{i}] Eingabefeld name={e["name"] or e["id"]} placeholder="{e["placeholder"]}"')
        else:
            lines.append(f'  [{i}] Textarea name={e["name"]}')
        refs.append(e)
    return '\n'.join(lines), refs

SYSTEM = """Du steuerst eine Website über Text-Snapshots. Antworte in JEDEM Zug NUR mit einem JSON-Objekt, kein anderer Text:
{"action":"click","ref":N}                  – Link/Button N anklicken
{"action":"fill","ref":N,"value":"..."}     – Eingabefeld/Textarea N ausfüllen
{"action":"submit"}                          – das Formular der Seite absenden
{"action":"answer","text":"..."}            – Endantwort geben, wenn die Aufgabe gelöst ist
Fülle Formulare vollständig aus, bevor du absendest."""

TASKS = {
    'info': {'prompt': 'Aufgabe: Finde heraus, was der Artikel "Gizmo Pro" kostet, und gib den Preis als Endantwort.',
             'grade': lambda st: '49,90' in st.get('answer', '')},
    'form': {'prompt': 'Aufgabe: Sende über das Kontaktformular eine Nachricht. Name: "Tobias Test", E-Mail: "tobias@example.com", Nachricht: "Benchmark Gruss 42". Gib danach als Endantwort "gesendet".',
             'grade': lambda st: st.get('submitted', False)},
}

def call_model(messages):
    body = json.dumps({'model': API_MODEL, 'messages': messages, 'temperature': 0, 'max_tokens': 4000}).encode()
    req = urllib.request.Request(API, data=body, headers={'Content-Type': 'application/json', 'Authorization': f'Bearer {API_KEY}'})
    r = json.load(urllib.request.urlopen(req, timeout=600))
    return r['choices'][0]['message'].get('content') or ''

def run(label, task_key, max_steps=12):
    task = TASKS[task_key]
    if os.path.exists(SUBS): os.remove(SUBS)
    path, state = '/index.html', {'form': {}}
    log, t0 = [], time.time()
    messages = [{'role': 'system', 'content': SYSTEM}]
    for step in range(max_steps):
        snap, refs = snapshot(path)
        messages.append({'role': 'user', 'content': f'{task["prompt"]}\n\n{snap}'})
        out = call_model(messages)
        messages.append({'role': 'assistant', 'content': out})
        m = re.search(r'\{.*\}', out, re.S)
        log.append({'step': step, 'page': path, 'model': out[:400]})
        if not m: continue
        try: act = json.loads(m.group(0))
        except json.JSONDecodeError:
            messages.append({'role': 'user', 'content': 'Ungültiges JSON. Antworte nur mit einem JSON-Objekt.'}); continue
        a = act.get('action')
        if a == 'answer':
            state['answer'] = act.get('text', ''); break
        elif a == 'click':
            try: e = refs[int(act['ref'])]
            except Exception: continue
            if e['tag'] == 'link':
                path = '/' + e['href'].lstrip('/')
            elif e['tag'] == 'button' and e.get('type') == 'submit':
                a = 'submit'
        if a == 'fill':
            try: e = refs[int(act['ref'])]
            except Exception: continue
            state['form'][e.get('name', '')] = act.get('value', '')
        if a == 'submit':
            data = urllib.parse.urlencode(state['form']).encode()
            urllib.request.urlopen(urllib.request.Request(BASE + '/submit', data=data), timeout=10)
            state['submitted_raw'] = True
            path = '/index.html'
            messages.append({'role': 'user', 'content': 'Formular wurde gesendet.'})
    # grade
    if task_key == 'form':
        ok = False
        if os.path.exists(SUBS):
            s = open(SUBS).read()
            ok = 'Tobias Test' in s and 'tobias@example.com' in s and 'Gruss 42' in s
        state['submitted'] = ok
    result = {'label': label, 'task': task_key, 'success': bool(task['grade'](state)),
              'steps': step + 1, 'seconds': round(time.time() - t0, 1), 'state': {k: v for k, v in state.items() if k != 'form'}}
    os.makedirs(os.path.join(DIR, 'runs-dom'), exist_ok=True)
    with open(os.path.join(DIR, 'runs-dom', f'{label}-{task_key}.jsonl'), 'w') as f:
        for l in log: f.write(json.dumps(l) + '\n')
    print(json.dumps(result))
    return result

if __name__ == '__main__':
    run(sys.argv[1], sys.argv[2], int(sys.argv[4]) if len(sys.argv) > 4 else 12)
