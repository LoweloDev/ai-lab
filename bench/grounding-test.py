#!/usr/bin/env python3
"""GUI-grounding feasibility test against the local vision model.
Asks for click coordinates (0-1000 normalized) of known UI elements and
checks them against hand-verified ground-truth boxes in original pixels."""
import base64, json, re, time, urllib.request

IMG = '/home/lowelodev/ai-lab/bench/screenshot-1920.png'
W, H = 7680, 2160  # original screen
TARGETS = [
    ("Start-Button in der Taskleiste unten links", (0, 140, 2090, 2160)),
    ("Steam-Desktop-Icon in der linken Icon-Spalte", (60, 210, 1400, 1640)),
    ("Uhrzeit-Anzeige in der Taskleiste ganz unten rechts", (7350, 7680, 2080, 2160)),
    ("Schliessen-Knopf (X) des Terminal-Fensters oben rechts am Fenster", (1850, 1960, 10, 70)),
]

b64 = base64.b64encode(open(IMG, 'rb').read()).decode()
results = []
for desc, (x0, x1, y0, y1) in TARGETS:
    prompt = (f"Das ist ein Screenshot eines Linux-Desktops. Wo würdest du klicken für: {desc}? "
              "Antworte NUR mit JSON im Format {\"x\": <int>, \"y\": <int>} mit Koordinaten "
              "normalisiert auf 0-1000 in beiden Achsen (x=0 links, x=1000 rechts, y=0 oben, y=1000 unten).")
    body = json.dumps({
        "model": "local", "max_tokens": 3000, "temperature": 0,
        "messages": [{"role": "user", "content": [
            {"type": "image_url", "image_url": {"url": f"data:image/png;base64,{b64}"}},
            {"type": "text", "text": prompt},
        ]}]}).encode()
    req = urllib.request.Request("http://127.0.0.1:8080/v1/chat/completions", data=body,
        headers={"Content-Type": "application/json", "Authorization": "Bearer sk-local"})
    t0 = time.time()
    try:
        resp = json.load(urllib.request.urlopen(req, timeout=300))
        dt = time.time() - t0
        txt = resp["choices"][0]["message"]["content"] or ""
        m = re.search(r'\{[^{}]*"x"[^{}]*\}', txt)
        if not m:
            results.append((desc, None, None, dt, "no-json:" + txt[:80])); continue
        c = json.loads(m.group(0))
        px, py = c["x"] / 1000 * W, c["y"] / 1000 * H
        hit = x0 <= px <= x1 and y0 <= py <= y1
        results.append((desc, (int(px), int(py)), hit, dt, ""))
    except Exception as e:
        results.append((desc, None, None, time.time() - t0, str(e)[:80]))

hits = sum(1 for r in results if r[2])
for desc, pt, hit, dt, err in results:
    status = "TREFFER" if hit else ("FEHL" if hit is False else "ERROR")
    print(f"{status:8} {dt:5.1f}s  {str(pt):>16}  {desc}  {err}")
print(f"\nGrounding: {hits}/{len(TARGETS)}")
