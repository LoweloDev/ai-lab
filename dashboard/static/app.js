/* AI-Lab Control-Dashboard v2 — Vanilla JS, keine Abhängigkeiten. */
(() => {
'use strict';

const view = document.getElementById('view');
const sysbar = document.getElementById('sysbar');
const navEl = document.getElementById('nav');

/* ---------------------------------------------------------------- State */
const S = {
  route: null,
  state: null,               // /api/state (Systemzustand)
  names: {},                 // Modell-ID -> Anzeigename (aus Registry)
  sel: { poly: null, suiteCell: null, robLabel: null, ux: null, wiki: null, job: null, domT: null },
  toggles: { polyHidden: false, suiteOld: false },
  wiz: { step: 1, bench: null, model: null, model_id: '', tasks: [], all_tasks: false,
         dom_task: 'info', backend: 'vulkan', ctx: '', extra_flags: '', timeout: '',
         label: '', start_server: false, force: false },
  compMsg: null, runMsg: null, confirmDel: null, editModel: null,
  fastT: null,
};
try {
  const t = localStorage.getItem('ailab-toggles');
  if (t) Object.assign(S.toggles, JSON.parse(t));
} catch (e) { /* egal */ }

/* ---------------------------------------------------------------- Helfer */
const esc = s => String(s ?? '').replace(/[&<>"']/g,
  c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const de = (n, d = 1) => (n == null || isNaN(n)) ? '–'
  : Number(n).toLocaleString('de-DE', { maximumFractionDigits: d });
const fmtSec = s => s == null ? '–' : (s >= 90 ? de(s / 60, 1) + ' min' : de(s, 1) + ' s');
const fmtDur = s => !s ? '–' : (s >= 5400 ? de(s / 3600, 1) + ' h'
  : s >= 90 ? de(s / 60, 0) + ' min' : de(s, 0) + ' s');
const fmtGB = b => b == null ? '–' : de(b / 1024 ** 3, 1) + ' GiB';
const name = id => S.names[id] || id;
const labelName = lbl => {
  for (const k of Object.keys(S.names)) {
    if (lbl === k) return S.names[k];
    if (lbl.startsWith(k + '-')) return S.names[k] + ' · ' + lbl.slice(k.length + 1);
  }
  return lbl;
};
const runDate = dir => {
  const m = dir.match(/^(\d{4})-(\d{2})-(\d{2})-(\d{2})-(\d{2})/);
  return m ? `${m[3]}.${m[2]}. ${m[4]}:${m[5]} UTC` : dir;
};
const saveToggles = () => {
  try { localStorage.setItem('ailab-toggles', JSON.stringify(S.toggles)); } catch (e) {}
};

async function api(path, opts) {
  const r = await fetch(path, opts);
  let body = null;
  try { body = await r.json(); } catch (e) { /* leer */ }
  if (!r.ok) throw new Error((body && body.error) || r.status + ' ' + r.statusText);
  return body;
}
const post = (path, body) => api(path, { method: 'POST',
  headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });

function copyText(t) {
  (navigator.clipboard ? navigator.clipboard.writeText(t) : Promise.reject())
    .catch(() => {
      const ta = document.createElement('textarea');
      ta.value = t; document.body.appendChild(ta); ta.select();
      document.execCommand('copy'); ta.remove();
    });
}

/* Status-Chip mit Symbol (nie Farbe allein). */
const chip = (kind, text, title) => {
  const map = { ok: ['ok', '✓'], bad: ['bad', '✗'], warn: ['warnc', '⚠'],
                run: ['running', '⏳'], off: ['quiet', '○'], info: ['', 'ℹ'] };
  const [cls, sym] = map[kind] || ['', ''];
  return `<span class="chip ${cls}" ${title ? `title="${esc(title)}"` : ''}>${sym} ${esc(text)}</span>`;
};

/* ---------- Mini-Markdown (aus v1 übernommen) ---------- */
function md2html(md) {
  const inline = t => esc(t)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/(^|[\s(>])\*([^*\s][^*]*?)\*(?=[\s.,;:!?)]|$)/g, '$1<em>$2</em>')
    .replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g,
      '<a href="$2" target="_blank" rel="noopener">$1</a>')
    .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, '$1 <span class="muted">($2)</span>');
  const out = [];
  let para = [], list = null, table = null, code = false, codeBuf = [];
  const flushP = () => { if (para.length) { out.push('<p>' + para.map(inline).join('<br>') + '</p>'); para = []; } };
  const flushL = () => { if (list) { out.push('</' + list + '>'); list = null; } };
  const flushT = () => { if (table) { out.push('<div class="tbl-wrap"><table>' + table.rows.join('') + '</table></div>'); table = null; } };
  const flushAll = () => { flushP(); flushL(); flushT(); };
  for (const line of md.replace(/\r/g, '').split('\n')) {
    if (code) {
      if (/^\s*```/.test(line)) { out.push('<pre><code>' + esc(codeBuf.join('\n')) + '</code></pre>'); code = false; codeBuf = []; }
      else codeBuf.push(line);
      continue;
    }
    if (/^\s*```/.test(line)) { flushAll(); code = true; continue; }
    const h = line.match(/^(#{1,4})\s+(.*)$/);
    if (h) { flushAll(); const lv = Math.min(h[1].length + 1, 5); out.push(`<h${lv}>${inline(h[2])}</h${lv}>`); continue; }
    if (/^\s*\|/.test(line)) {
      flushP(); flushL();
      if (/^[\s|:-]+$/.test(line)) continue;
      const cells = line.trim().replace(/^\||\|$/g, '').split('|').map(c => c.trim());
      if (!table) { table = { rows: [] }; table.rows.push('<tr>' + cells.map(c => '<th>' + inline(c) + '</th>').join('') + '</tr>'); }
      else table.rows.push('<tr>' + cells.map(c => '<td>' + inline(c) + '</td>').join('') + '</tr>');
      continue;
    }
    flushT();
    if (/^\s*(---+|\*\*\*+)\s*$/.test(line)) { flushAll(); out.push('<hr>'); continue; }
    const ul = line.match(/^\s*[-*+]\s+(.*)$/);
    const ol = line.match(/^\s*(\d+)[.)]\s+(.*)$/);
    if (ul || ol) {
      flushP();
      const want = ul ? 'ul' : 'ol';
      if (list !== want) { flushL(); out.push('<' + want + '>'); list = want; }
      out.push(ol ? `<li value="${ol[1]}">` + inline(ol[2]) + '</li>'
                  : '<li>' + inline(ul[1]) + '</li>');
      continue;
    }
    const bq = line.match(/^\s*>\s?(.*)$/);
    if (bq) { flushAll(); out.push('<blockquote><p>' + inline(bq[1]) + '</p></blockquote>'); continue; }
    if (!line.trim()) { flushAll(); continue; }
    flushL();
    para.push(line.trim());
  }
  if (code) out.push('<pre><code>' + esc(codeBuf.join('\n')) + '</code></pre>');
  flushAll();
  return out.join('\n');
}

const barRow = (label, val, max, cls, valText, title) => `
  <div class="crow" title="${esc(title || '')}">
    <div class="clabel">${esc(label)}</div>
    <div class="track"><span class="fill ${cls}" style="width:${max ? Math.max(1.2, val / max * 100) : 0}%"></span></div>
    <div class="cval">${valText}</div>
  </div>`;

const viewHead = (title, sub, extra) => `
  <div class="view-head"><h1>${esc(title)}</h1>
    ${sub ? `<span class="sub">${sub}</span>` : ''}<span class="spacer"></span>${extra || ''}
  </div>`;

/* ============================================================ Navigation */
const NAV = [
  { id: 'uebersicht', label: 'Übersicht', ico: '◈', key: '1' },
  { group: 'Ergebnisse' },
  { id: 'suite', label: 'Suite', ico: '▦', sub: true, key: '2' },
  { id: 'polyglot', label: 'Polyglot', ico: '⬡', sub: true, key: '3' },
  { id: 'apps', label: 'Apps & UX', ico: '▤', sub: true, key: '4' },
  { id: 'perf', label: 'Perf', ico: '↯', sub: true, key: '5' },
  { group: 'Steuern' },
  { id: 'laeufe', label: 'Läufe', ico: '▶', sub: true, key: '6' },
  { group: 'Komponenten' },
  { id: 'modelle', label: 'Modelle', ico: '◆', sub: true, key: '7' },
  { id: 'harnesses', label: 'Harnesses', ico: '⛭', sub: true, key: '8' },
  { id: 'backends', label: 'Backends', ico: '⌁', sub: true, key: '9' },
  { group: 'Wissen' },
  { id: 'wiki', label: 'Wiki', ico: '✎', sub: true, key: '0' },
];
const ROUTES = NAV.filter(n => n.id).map(n => n.id).concat(['einstellungen']);

function renderNav() {
  navEl.innerHTML = NAV.map(n => n.group
    ? `<div class="navgroup">${esc(n.group)}</div>`
    : `<button class="navbtn ${n.sub ? 'sub' : ''} ${S.route === n.id ? 'active' : ''}"
         data-nav="${n.id}"><span class="ico">${n.ico}</span>${esc(n.label)}
         <span class="kbd">${n.key}</span></button>`).join('');
  document.getElementById('nav-settings').classList.toggle('active',
    S.route === 'einstellungen');
}

/* ===================================================== Systemzustand-Leiste */
function renderSysbar() {
  const st = S.state;
  if (!st) { sysbar.innerHTML = '<span class="syschip off">Systemzustand …</span>'; return; }
  const bits = [];
  if (st.lock.locked) {
    bits.push(`<span class="syschip lock" title="${esc(st.lock.reason)}">🔒 <b>GPU gesperrt</b> · ${st.lock.containers.length} Benchmark-Container</span>`);
    for (const c of st.lock.containers) {
      bits.push(`<span class="syschip" title="Image: ${esc(c.image)}">▣ ${esc(c.name)} · ${esc(c.uptime)}</span>`);
    }
  } else {
    bits.push('<span class="syschip free">🟢 <b>GPU frei</b> — Modellserver steuerbar</span>');
  }
  if (st.llama.length) {
    for (const l of st.llama) {
      bits.push(`<span class="syschip run" title="PID ${l.pid} · Port ${l.port} · ctx ${l.ctx ?? '?'} · ${esc(l.device || '')} (aus /proc gelesen, :8080 wird nicht angesprochen)">▲ llama-server: <b>${esc(l.model_name)}</b> · ${esc(l.uptime)}</span>`);
    }
  } else {
    bits.push('<span class="syschip off">△ kein llama-server</span>');
  }
  bits.push(st.port8090
    ? '<span class="syschip">◻ Testseite :8090 läuft</span>'
    : '<span class="syschip off">◻ Testseite :8090 aus</span>');
  const sl = st.slots;
  if (sl && (sl.gpu.used || sl.cloud.used || sl.download.used || (sl.cpu && sl.cpu.used))) {
    const cpu = sl.cpu && sl.cpu.used ? ` · ${sl.cpu.used} CPU` : '';
    bits.push(`<span class="syschip run">⏳ Jobs: ${sl.gpu.used} GPU · ${sl.cloud.used} Cloud · ${sl.download.used} DL${cpu}</span>`);
  }
  sysbar.innerHTML = bits.join('');
}

async function loadState() {
  try {
    S.state = await api('/api/state');
    renderSysbar();
  } catch (e) {
    sysbar.innerHTML = `<span class="syschip lock">⚠ Systemzustand nicht lesbar: ${esc(e.message)}</span>`;
  }
}

async function loadNames() {
  try {
    const d = await api('/api/registry/models');
    S.names = {};
    for (const e of d.models.concat(d.catalog)) S.names[e.id] = e.name;
  } catch (e) { /* Registry noch leer */ }
}

/* ================================================================ Übersicht */
function renderUebersicht(d) {
  const c = d.counts;
  const strip = `
    <div class="stat-strip">
      <span class="pill" data-act="nav" data-nav="suite">▦ <b>${c.suite}</b> Suite-Ergebnisse · ${c.suite_labels} Modelle</span>
      <span class="pill" data-act="nav" data-nav="polyglot">⬡ <b>${c.poly_runs}</b> Polyglot-Läufe${c.poly_running ? ` · <span class="chip running">⏳ ${c.poly_running} läuft</span>` : ''}</span>
      <span class="pill" data-act="nav" data-nav="perf">↯ <b>${c.perf_files}</b> Perf-Dateien</span>
      <span class="pill" data-act="nav" data-nav="apps">▤ <b>${c.dom}</b> DOM · <b>${c.ux}</b> UX</span>
    </div>`;
  const cards = d.cards.map(card => {
    const s = card.suite;
    const suiteVal = s.total
      ? `${s.pass}<span class="dim">/${s.total}</span>`
      : '<span class="dim">–</span>';
    const mini = s.total
      ? `<div class="minibar"><span style="width:${s.pass / s.total * 100}%;background:${s.pass === s.total ? 'var(--good)' : 'var(--warn)'}"></span></div>` : '';
    const p = card.poly;
    let polyVal = '<span class="dim">–</span>', polySub = 'kein Lauf';
    if (p) {
      if (p.running) {
        polyVal = `${p.n}<span class="dim">/${p.expected || '?'}</span>`;
        polySub = `⏳ läuft · bisher p@2 ${p.pct2} %`;
      } else {
        polyVal = `${p.pct2}<span class="dim">%</span>`;
        polySub = `p@2 · pass@1 ${p.pct1} % · ${p.n}${p.expected ? '/' + p.expected : ''} Üb.`;
      }
    }
    const pf = card.perf;
    const tgVal = pf && pf.tg ? de(pf.tg, 1) : '<span class="dim">–</span>';
    const tgSub = pf && pf.tg
      ? `tok/s · ${pf.tg_backend === 'rocm' ? 'ROCm' : 'Vulkan'} · pp ${de(pf.pp, 0)}`
      : 'keine Messung';
    return `
      <article class="hero">
        <header><h2>${esc(card.name)}</h2>
          <div class="note">${esc(card.note)}</div></header>
        <div class="hero-stats">
          <div class="stat"><div class="stat-label">Suite</div>
            <div class="stat-value">${suiteVal}</div>${mini}</div>
          <div class="stat"><div class="stat-label">Polyglot</div>
            <div class="stat-value">${polyVal}</div><div class="stat-sub">${polySub}</div></div>
          <div class="stat"><div class="stat-label">Tempo</div>
            <div class="stat-value">${tgVal}</div><div class="stat-sub">${tgSub}</div></div>
        </div>
        <footer class="verdict">🏁 <span class="win">${esc(card.verdict)}</span></footer>
      </article>`;
  }).join('');
  view.innerHTML = viewHead('Übersicht', 'Stand ' + esc(d.generated))
    + strip + `<div class="cards">${cards}</div>
    <div class="foot-note">Quellen: bench/results.jsonl (dedupliziert) · aider/tmp.benchmarks · logs/perf-*.json · webapp/runs-*</div>`;
}

/* ================================================================ Suite */
const robMini = r => r.buildable === false ? '<span class="dim">n. baubar</span>'
  : `${r.stale ? '⟳ ' : ''}R ${r.real_pass}/${r.real_total} · P ${r.path_pass}/${r.path_total}`;
const robDetail = r => {
  if (r.buildable === false)
    return `<span class="muted">n. baubar</span>${r.error ? ' — <code>' + esc(r.error) + '</code>' : ''}`;
  let s = `R ${r.real_pass}/${r.real_total} · P ${r.path_pass}/${r.path_total} · baubar`;
  if (r.stale) s += ' ⟳ <span class="muted">veraltet</span>';
  if (r.failed && r.failed.length)
    s += '<div class="small muted" style="margin-top:4px">gerissen: ' +
      r.failed.map(f => '<code>' + esc(f) + '</code>').join(' · ') + '</div>';
  return s;
};

function renderSuite(d, rob) {
  rob = rob || { scores: {}, stale: [] };
  const last = new Map(), hist = new Map();
  for (const e of d.entries) {
    const k = e.model + ' ' + e.task;
    if (!e.superseded) last.set(k, e);
    (hist.get(k) || hist.set(k, []).get(k)).push(e);
  }
  const head = d.tasks.map(t => {
    const i = t.indexOf('-');
    return `<th><span class="grp">${esc(i > 0 ? t.slice(0, i) : '')}</span>${esc(i > 0 ? t.slice(i + 1) : t)}</th>`;
  }).join('');
  const robHead = '<th class="robth" title="Robustheit: Zusatz-Metrik — ändert keine Urteile">Robustheit</th>';
  const rows = d.labels.map(lbl => {
    const cells = d.tasks.map(t => {
      const k = lbl + ' ' + t;
      const e = last.get(k);
      if (!e) return '<td class="cell none">–</td>';
      const selCls = S.sel.suiteCell === k ? ' sel' : '';
      const mini = e.robust
        ? `<span class="mini-rob">${robMini(e.robust)}</span>` : '';
      const tip = e.robust && e.robust.failed && e.robust.failed.length
        ? '\nRobustheit: ' + e.robust.failed.join(', ') : '';
      return `<td class="cell ${e.pass ? 'ok' : 'bad'}${selCls}" data-act="cell" data-id="${esc(k)}"
        title="${esc(e.grade)}${esc(tip)}">${e.pass ? '✓' : '✗'} ${fmtSec(e.seconds)}${mini}</td>`;
    }).join('');
    const s = rob.scores[lbl];
    const robCell = s && s.real_total > 0 ? (() => {
      const pct = Math.round(100 * s.real_score);
      const col = pct === 100 ? 'var(--good)' : pct >= 60 ? 'var(--warn)' : 'var(--crit)';
      const stale = rob.stale.some(x => x[1] === lbl) ? ' ⟳' : '';
      /* n_missing > 0: der Score liess Tasks weg (nicht baubar) und ist mit den
         vollstaendigen Labels NICHT vergleichbar — sichtbar markieren, nicht nur
         im n/8 verstecken. */
      const part = s.n_missing ? ' rob-part' : '';
      return `<td class="cell robcol${part}" title="Robustheit: ${pct}% · R ${s.real_pass}/${s.real_total} · P ${s.path_pass}/${s.path_total}${stale}${s.n_missing ? ` · ACHTUNG: ${s.n_missing} von 8 Tasks nicht baubar und aus dem Score herausgerechnet — nicht mit vollstaendigen Labels vergleichbar` : ''}">
        <span class="rob-pct">${pct}<span class="dim">%</span></span>${stale}
        <span class="minibar rob-bar"><span style="width:${pct}%;background:${col}"></span></span>
        <span class="rob-sub">${s.n_missing ? '⚠ ' : ''}${s.n_tasks}/${s.n_tasks + s.n_missing}</span></td>`;
    })() : '<td class="cell none robcol">–</td>';
    return `<tr><td class="rowh">${esc(labelName(lbl))}<span class="sub">${esc(lbl)}</span></td>${cells}${robCell}</tr>`;
  }).join('');
  let detail = '';
  if (S.sel.suiteCell && last.has(S.sel.suiteCell)) {
    const e = last.get(S.sel.suiteCell);
    const h = hist.get(S.sel.suiteCell) || [];
    detail = `
      <div class="card detail">
        <h2>${esc(labelName(e.model))} × ${esc(e.task)} ${e.pass ? chip('ok', 'PASS') : chip('bad', 'FAIL')}</h2>
        <dl>
          <dt>Grade</dt><dd><code>${esc(e.grade)}</code></dd>
          <dt>Dauer</dt><dd>${fmtSec(e.seconds)}</dd>
          <dt>Exit-Code</dt><dd>${esc(e.exit)}${e.exit === 137 ? ' <span class="muted">(Timeout/SIGKILL — Grade zählt trotzdem)</span>' : ''}</dd>
          <dt>Änderungen</dt><dd>${esc((e.changed || '').trim() || 'keine')}</dd>
          ${e.robust ? `<dt>Robustheit</dt><dd>${robDetail(e.robust)}</dd>` : ''}
          ${h.length > 1 ? `<dt>Versuche</dt><dd>${h.length} — ältere: ${h.filter(x => x.superseded).map(x => (x.pass ? '✓' : '✗') + ' ' + fmtSec(x.seconds)).join(' · ')}</dd>` : ''}
        </dl>
      </div>`;
  }
  /* Gesamtzeit-Graph: Summe der jeweils letzten Versuche je Modell-Label.
     Timeouts (exit 124/137) werden als eigenes, schraffiertes Segment gezeigt und aus der
     Arbeitszeit herausgerechnet — sonst dominieren zwei 60-min-Abbrueche jede Bilanz. */
  const isTimeout = e => e.exit === 137 || e.exit === 124;
  const agg = d.labels.map(lbl => {
    let sec = 0, tsec = 0, ok = 0, n = 0, nt = 0;
    for (const t of d.tasks) {
      const e = last.get(lbl + ' ' + t);
      if (!e) continue;
      n++; if (e.pass) ok++;
      if (isTimeout(e)) { tsec += e.seconds || 0; nt++; } else sec += e.seconds || 0;
    }
    return { lbl, sec, tsec, ok, n, nt, tot: sec + tsec };
  }).filter(a => a.n > 0).sort((a, b) => a.tot - b.tot);
  const maxSec = Math.max(1, ...agg.map(a => a.tot));
  const chart = !agg.length ? '' : `
    <h3 class="sec">Gesamtzeit je Modell
      <span class="muted small">Summe der letzten Versuche · <span style="color:var(--accent)">alles PASS</span> / <span style="color:var(--warn)">mit FAILs</span> · <span class="sw-hatch"></span> Timeout-Anteil (Limit erreicht, nicht Arbeitszeit)</span></h3>
    <div class="card bars">${agg.map(a => `
      <div class="bar-row">
        <div class="bar-lbl">${esc(labelName(a.lbl))}
          <span class="sub">${a.ok}/${a.n} PASS · ø ${fmtSec(a.sec / Math.max(1, a.n - a.nt))}/Task ohne Timeouts${a.nt ? ` · ${a.nt} Timeout${a.nt > 1 ? 's' : ''}` : ''}</span></div>
        <div class="bar-track" title="${esc(labelName(a.lbl))}: ${fmtSec(a.sec)} Arbeit${a.nt ? ` + ${fmtSec(a.tsec)} Timeout` : ''} über ${a.n} Tasks">
          <div class="bar-fill${a.ok === a.n ? '' : ' part'}" style="width:${Math.max(a.sec ? 1 : 0, a.sec / maxSec * 100)}%"></div>${a.tsec ? `<div class="bar-fill hatch" style="width:${a.tsec / maxSec * 100}%"></div>` : ''}
          <span class="bar-val">${fmtSec(a.sec)}${a.nt ? ` <span class="muted">+ ${fmtSec(a.tsec)} Timeout</span>` : ''} <span class="muted">(n=${a.n})</span></span>
        </div>
      </div>`).join('')}</div>`;
  const oldTable = S.toggles.suiteOld ? `
    <h3 class="sec">Alle Rohzeilen (results.jsonl) — überholte gedimmt</h3>
    <div class="tbl-wrap"><table class="list">
      <thead><tr><th>#</th><th>Modell-Label</th><th>Task</th><th>Grade</th><th class="num">Dauer</th><th class="num">Exit</th></tr></thead>
      <tbody>${d.entries.map(e => `
        <tr class="${e.superseded ? 'dimrow' : ''}">
          <td class="num">${e.line + 1}</td><td>${esc(e.model)}</td><td>${esc(e.task)}</td>
          <td>${e.pass ? chip('ok', e.grade) : chip('bad', e.grade)}${e.superseded ? ' <span class="muted small">überholt</span>' : ''}</td>
          <td class="num">${fmtSec(e.seconds)}</td><td class="num">${esc(e.exit)}</td>
        </tr>`).join('')}</tbody>
    </table></div>` : '';
  /* Robustheit-Graph: Zusatz-Metrik (battery), ändert keine Urteile.
     Balken = real_score je Label, Pill = Path-Quote, Klick klappt pro-Task auf. */
  const robRows = d.labels.map(lbl => ({ lbl, s: rob.scores[lbl] }))
    .filter(a => a.s && a.s.real_total > 0)
    .sort((a, b) => b.s.real_score - a.s.real_score);
  const robChart = !robRows.length ? '' : `
    <h3 class="sec">Robustheit über alle Suite-Tasks
      <span class="muted small">Zusatz-Metrik, ändert keine Urteile · R realistische Kanten (voller Brief-Vertrag) · P pathologische Eingaben (nur: kein Crash/Hang/Verlust) · <a href="#/wiki" data-act="wikiLink" data-id="wiki:robustheit.md">robustheit ↗</a></span></h3>
    <div class="card bars">${robRows.map(a => {
      const s = a.s, pct = Math.round(100 * s.real_score);
      const ptPct = s.path_total ? Math.round(100 * s.path_pass / s.path_total) : 0;
      const sel = S.sel.robLabel === a.lbl;
      const per = sel ? `<div class="rob-per">${Object.entries(s.per_task).map(([t, r]) => {
        const cell = !r ? '<span class="muted">–</span>'
          : r.buildable === false ? '<span class="muted">n. baubar</span>'
          : `<span class="mono">R ${r.real_pass}/${r.real_total} · P ${r.path_pass}/${r.path_total}</span>${r.stale ? ' ⟳' : ''}`;
        return `<div class="crow rob-per-row"><div class="clabel">${esc(t)}</div><div class="track"></div><div class="cval">${cell}</div></div>`;
      }).join('')}</div>` : '';
      /* Labels mit nicht baubaren Tasks stehen mit weniger Nennern oben — der Score
         ist dann NICHT mit vollstaendigen Labels vergleichbar. Sichtbar warnen,
         statt es im n/8 zu verstecken (Sortierung bleibt real_score, wie spezifiziert). */
      const miss = s.n_missing
        ? chip('warn', `${s.n_missing} Task${s.n_missing > 1 ? 's' : ''} n. baubar`,
               `Score nur über ${s.n_tasks} baubare Tasks — nicht mit Labels über alle 8 vergleichbar.`)
        : '';
      return `
      <div class="bar-row rob-row ${sel ? 'sel' : ''}${s.n_missing ? ' rob-part' : ''}" data-act="robrow" data-id="${esc(a.lbl)}">
        <div class="bar-lbl">${esc(labelName(a.lbl))} ${miss}
          <span class="sub">R ${s.real_pass}/${s.real_total} · P ${s.path_pass}/${s.path_total} · ${s.n_tasks} von ${s.n_tasks + s.n_missing} Tasks</span></div>
        <div class="bar-track" title="${esc(labelName(a.lbl))}: ${pct}% real_score über ${s.n_tasks} Tasks${s.n_missing ? ` (${s.n_missing} nicht baubar, herausgerechnet)` : ''}">
          <div class="bar-fill rob${s.n_missing ? ' part' : ''}" style="width:${Math.max(1.2, pct)}%"></div>
          <span class="bar-val">${pct} % <span class="pill rob-pill">P ${ptPct} %</span></span>
        </div>
      </div>${per}`;
    }).join('')}</div>
    <div class="legend">
      <span><span class="sw" style="background:var(--accent-bg);border-color:var(--accent)"></span>real_score je Label</span>
      <span class="muted">Klick auf die Zeile klappt die Aufgaben auf · ⚠ = Score liess nicht baubare Tasks weg, nicht direkt vergleichbar</span>
    </div>`;
  view.innerHTML = viewHead('Suite', 'Repo-Tasks · OpenCode', `
      <label class="toggle"><input type="checkbox" data-act="suiteOld" ${S.toggles.suiteOld ? 'checked' : ''}>
      ältere Versuche zeigen (${d.superseded_count})</label>`)
    + `<div class="legend">
      <span><span class="sw" style="background:var(--good-bg);border-color:var(--good)"></span>✓ PASS</span>
      <span><span class="sw" style="background:var(--crit-bg);border-color:var(--crit)"></span>✗ FAIL</span>
      <span class="muted">Es zählt der jeweils letzte Versuch · Zelle anklicken für Details</span>
    </div>
    <div class="tbl-wrap"><table class="matrix"><tr><th></th>${head}${robHead}</tr>${rows}</table></div>
    ${detail}${chart}${robChart}${oldTable}`;
}

/* ================================================================ Polyglot */
function renderPolyglot(d) {
  const toggles = `
    <label class="toggle"><input type="checkbox" data-act="polyHidden" ${S.toggles.polyHidden ? 'checked' : ''}>
    verworfene zeigen (${d.hidden_count})</label>`;
  if (!d.runs.length) {
    view.innerHTML = viewHead('Polyglot', 'Aider · python+go', toggles)
      + '<div class="empty">Keine sichtbaren Polyglot-Läufe.<br><span class="small">Verworfene über den Schalter oben einblenden.</span></div>';
    return;
  }
  if (!S.sel.poly || !d.runs.some(r => r.dir === S.sel.poly)) S.sel.poly = d.runs[0].dir;
  const run = d.runs.find(r => r.dir === S.sel.poly);
  const stChip = r => r.status === 'läuft' ? chip('run', 'läuft')
    : r.status === 'fertig' ? chip('ok', 'fertig')
    : r.status === 'fehler' ? chip('bad', 'Fehler')
    : chip('off', 'verworfen');
  const picker = d.runs.map(r => `
    <button class="runbtn ${r.dir === S.sel.poly ? 'sel' : ''}" data-act="poly" data-id="${esc(r.dir)}">
      <div class="t">${esc(r.label ? (name(r.label) || r.label) : (r.dir.split('--')[1] || r.dir))} ${stChip(r)}</div>
      <div class="s">${runDate(r.dir)} · ${r.n}${r.expected ? '/' + r.expected : ''} Übungen${r.n ? ' · p@2 ' + Math.round(100 * r.pass2 / r.n) + ' %' : ''}</div>
    </button>`).join('');
  const head = viewHead('Polyglot', 'Aider-Benchmark · python+go', toggles);
  if (!run || !run.n) {
    view.innerHTML = head + `<div class="runlist">${picker}</div>
      <div class="empty">Dieser Lauf hat noch keine Ergebnisse.</div>`;
    return;
  }
  const pct = x => Math.round(100 * x / run.n);
  const progress = run.status === 'läuft' && run.expected ? `
    <div class="notebox">⏳ Lauf aktiv: <b>${run.n}/${run.expected}</b> Übungen fertig — Werte wachsen live mit.
      <div class="minibar" style="max-width:340px"><span style="width:${100 * run.n / run.expected}%;background:var(--accent)"></span></div>
    </div>` : '';
  const noteBox = run.note ? `<div class="notebox">✎ ${esc(run.note)}</div>` : '';
  const tiles = `
    <div class="tiles">
      <div class="tile"><div class="v">${pct(run.pass2)}<span class="dim"> %</span></div><div class="l">pass@2</div></div>
      <div class="tile"><div class="v">${pct(run.pass1)}<span class="dim"> %</span></div><div class="l">pass@1</div></div>
      <div class="tile"><div class="v">${run.n}<span class="dim">${run.expected ? ' / ' + run.expected : ''}</span></div><div class="l">Übungen</div></div>
      <div class="tile"><div class="v">${fmtDur(run.duration)}</div><div class="l">Σ Modellzeit</div></div>
    </div>`;
  const langNames = { python: 'Python', go: 'Go', rust: 'Rust', javascript: 'JavaScript', java: 'Java', cpp: 'C++' };
  const langBars = Object.entries(run.langs).map(([lang, L]) => {
    const w = x => (100 * x / L.n).toFixed(1) + '%';
    const extra2 = L.p2 - L.p1, fail = L.n - L.p2;
    return `
      <div class="crow" title="${esc(langNames[lang] || lang)}: pass@1 ${L.p1} · pass@2 ${L.p2} · von ${L.n}">
        <div class="clabel">${esc(langNames[lang] || lang)}</div>
        <div class="stack">
          ${L.p1 ? `<span class="st-p1" style="width:${w(L.p1)}"></span>` : ''}
          ${extra2 ? `<span class="st-p2" style="width:${w(extra2)}"></span>` : ''}
          ${fail ? `<span class="st-fail" style="width:${w(fail)}"></span>` : ''}
        </div>
        <div class="cval">${L.p2}/${L.n} p@2</div>
      </div>`;
  }).join('');
  const sym = { p1: '✓', p2: '2', fail: '✗', err: '!' };
  const groups = {};
  for (const e of run.exercises) (groups[e.lang] = groups[e.lang] || []).push(e);
  const grid = Object.entries(groups).map(([lang, exs]) => `
    <h3 class="sec">${esc(langNames[lang] || lang)} · ${exs.length}</h3>
    <div class="exgrid">${exs.map(e => `
      <span class="ex st-${e.status}" title="${esc(e.name)} · ${e.tries} Versuch(e) · ${fmtSec(e.duration)}">
        <b>${sym[e.status]}</b>${esc(e.name)}</span>`).join('')}
    </div>`).join('');
  const annotBtn = run.status !== 'läuft' ? `
    <button class="btn ghost sm" data-act="polyHide" data-id="${esc(run.dir)}" data-hide="${run.hidden ? '0' : '1'}">
      ${run.hidden ? 'Lauf wieder einblenden' : 'Lauf verwerfen (ausblenden)'}</button>` : '';
  view.innerHTML = head + `
    <div class="runlist">${picker}</div>
    ${progress}${noteBox}${tiles}
    <h3 class="sec">Sprachen</h3>
    <div class="chart" style="max-width:640px">${langBars}</div>
    <div class="legend" style="margin-top:14px">
      <span><span class="sw" style="background:var(--good-bg);border-color:var(--good)"></span>✓ pass@1</span>
      <span><span class="sw" style="background:var(--warn-bg);border-color:var(--warn)"></span>2 pass@2</span>
      <span><span class="sw" style="background:var(--crit-bg);border-color:var(--crit)"></span>✗ fail</span>
    </div>
    ${grid}
    <div class="foot-note">Ordner: ${esc(run.dir)} · Overlay: dashboard/registry/run-annotations.json (Rohdaten unangetastet) ${annotBtn}</div>`;
}

/* ================================================================ Perf */
function renderPerf(d) {
  const head = viewHead('Perf', 'llama-bench · tok/s');
  if (!d.series.length) {
    view.innerHTML = head + '<div class="empty">Keine perf-*.json unter logs/ gefunden.</div>';
    return;
  }
  const bname = b => b === 'rocm' ? 'ROCm' : 'Vulkan';
  const bcls = b => b === 'rocm' ? 'r' : 'v';
  const pure = (e, kind) => kind === 'tg'
    ? (e.tg || 0) > 0 && !(e.pp || 0)
    : (e.pp || 0) > 0 && !(e.tg || 0);
  const at0 = (s, kind) => s.entries.find(e => !e.depth && pure(e, kind));
  const mkChart = kind => {
    const rows = d.series.map(s => ({ s, e: at0(s, kind) })).filter(x => x.e);
    const max = Math.max(...rows.map(x => x.e.ts));
    return rows.map(x => barRow(
      labelName(x.s.label) + ' · ' + bname(x.s.backend),
      x.e.ts, max, bcls(x.s.backend), de(x.e.ts, 1),
      `${x.s.model_type} · ±${x.e.stddev}`)).join('');
  };
  const depthSeries = d.series.filter(s => s.entries.some(e => e.depth > 0));
  const maxTgAll = Math.max(...d.series.flatMap(s => s.entries.filter(e => pure(e, 'tg')).map(e => e.ts)), 1);
  const depthCards = depthSeries.map(s => {
    const depths = [...new Set(s.entries.filter(e => pure(e, 'tg') || pure(e, 'pp')).map(e => e.depth || 0))].sort((a, b) => a - b);
    const mixed = s.entries.filter(e => (e.tg || 0) > 0 && (e.pp || 0) > 0);
    const rows = depths.map(dep => {
      const tg = s.entries.find(e => (e.depth || 0) === dep && pure(e, 'tg'));
      const pp = s.entries.find(e => (e.depth || 0) === dep && pure(e, 'pp'));
      if (!tg && !pp) return '';
      return `
        <div class="crow" title="Kontexttiefe ${de(dep, 0)}">
          <div class="clabel">d ${dep ? de(dep / 1000, 0) + 'k' : '0'}</div>
          <div class="track">${tg ? `<span class="fill ${bcls(s.backend)}" style="width:${Math.max(1.2, tg.ts / maxTgAll * 100)}%"></span>` : ''}</div>
          <div class="cval">${tg ? 'tg ' + de(tg.ts, 1) : ''}<span class="muted">${pp ? ' · pp ' + de(pp.ts, 0) : ''}</span></div>
        </div>`;
    }).join('');
    return `
      <div class="card">
        <header style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:10px">
          <span class="dot ${bcls(s.backend)}"></span><b>${esc(labelName(s.label))}</b>
          <span class="chip">${bname(s.backend)}</span>
          ${s.partial ? chip('warn', 'unvollständig', 'Quelldatei abgebrochen — lesbare Einträge werden gezeigt') : ''}
        </header>
        <div class="chart">${rows}</div>
        ${mixed.length ? `<div class="muted small" style="margin-top:8px">kombiniert: ${mixed.map(e => `pp${e.pp}+tg${e.tg} @ d${de(e.depth || 0, 0)} → ${de(e.ts, 1)} tok/s`).join(' · ')}</div>` : ''}
      </div>`;
  }).join('');
  view.innerHTML = head + `
    <div class="legend">
      <span><span class="dot v"></span>Vulkan</span>
      <span><span class="dot r"></span>ROCm</span>
      <span class="muted">Quelle: logs/perf-*.json</span>
    </div>
    <h3 class="sec">Prompt-Verarbeitung · pp2048 · Tiefe 0</h3>
    <div class="chart" style="max-width:760px">${mkChart('pp')}</div>
    <h3 class="sec">Generierung · tg128 · Tiefe 0</h3>
    <div class="chart" style="max-width:760px">${mkChart('tg')}</div>
    <h3 class="sec">Einbruch über Kontexttiefe</h3>
    <div class="depth-cards">${depthCards}</div>`;
}

/* ================================================================ Apps & UX */
function renderApps(dom, ux, uxDoc, transcript) {
  const stat = r => r.success === true ? chip('ok', 'ok')
    : r.success === false ? chip('bad', 'fehl') : chip('off', 'unklar');
  const rows = dom.runs.map(r => `
    <tr>
      <td><b>${esc(labelName(r.label))}</b> <span class="muted small">${esc(r.label)}</span></td>
      <td>${esc(r.task)}</td>
      <td title="${r.source === 'ergebnis' ? 'Ergebniszeile eines Dashboard-Laufs' : 'aus dem Transkript abgeleitet'}">${stat(r)}</td>
      <td class="num">${r.steps}</td>
      <td class="num">${r.seconds != null ? fmtSec(r.seconds) : '–'}</td>
      <td>${r.answer != null ? '<code>' + esc(r.answer) + '</code>' : '<span class="muted">–</span>'}</td>
      <td><button class="linkbtn" data-act="domt" data-id="${esc(r.file)}">${S.sel.domT === r.file ? 'zuklappen' : 'Transkript'}</button></td>
    </tr>
    ${S.sel.domT === r.file && transcript ? `<tr><td colspan="7">${renderTranscript(transcript)}</td></tr>` : ''}`).join('');
  const domTable = dom.runs.length ? `
    <div class="tbl-wrap"><table class="list">
      <thead><tr><th>Modell</th><th>Aufgabe</th><th>Ergebnis</th><th class="num">Schritte</th><th class="num">Dauer</th><th>Antwort</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table></div>` : '<div class="empty">Keine DOM-Läufe unter bench/webapp/runs-dom/.</div>';
  const uxBtns = ux.docs.map(u => `
    <button class="runbtn ${S.sel.ux === u.file ? 'sel' : ''}" data-act="ux" data-id="${esc(u.file)}">
      <div class="t">${esc(u.file.replace(/\.md$/, ''))}</div>
      <div class="s">${esc(u.mtime)} · ${de(u.size / 1024, 1)} KB</div>
    </button>`).join('');
  view.innerHTML = viewHead('Apps & UX', 'DOM-Navigation + UX-Reviews') + `
    <h3 class="sec">DOM-Navigation (Testseite auf :8090)</h3>
    ${domTable}
    <h3 class="sec">UX-Findings (webapp/runs-ux)</h3>
    ${ux.docs.length ? `<div class="runlist">${uxBtns}</div>
      <div class="mdbody">${uxDoc ? md2html(uxDoc.md) : '<span class="muted">Review auswählen …</span>'}</div>`
      : '<div class="empty">Keine UX-Reviews vorhanden.</div>'}`;
}

function renderTranscript(t) {
  const pretty = st => {
    if (st.raw) return esc(st.raw);
    let act = null;
    const m = (st.model || '').match(/\{[\s\S]*\}/);
    if (m) { try { act = JSON.parse(m[0]); } catch (e) { /* roh */ } }
    if (!act) return '<code>' + esc(st.model || '') + '</code>';
    switch (act.action) {
      case 'click': return `Klick auf Element [${esc(act.ref)}]`;
      case 'fill': return `Feld [${esc(act.ref)}] = »${esc(act.value)}«`;
      case 'submit': return 'Formular absenden';
      case 'answer': return `Endantwort: »${esc(act.text)}«`;
      default: return '<code>' + esc(st.model || '') + '</code>';
    }
  };
  return `<div class="transcript">${t.lines.map(st =>
    `<div class="step"><span class="muted">${st.step != null ? 'S' + st.step : ''} ${esc(st.page || '')}</span> → ${pretty(st)}</div>`).join('')}
  </div>`;
}

/* ================================================================ Läufe */
function wizSummary(meta) {
  const w = S.wiz;
  const b = meta.benchmarks.find(x => x.id === w.bench);
  const parts = [];
  if (b) parts.push(b.name);
  if (w.bench === 'suite-api') { if (w.model_id) parts.push(w.model_id); }
  else if (w.model) parts.push(name(w.model));
  if (['suite', 'suite-api'].includes(w.bench)) {
    parts.push(w.all_tasks ? 'alle Tasks' : (w.tasks.length ? w.tasks.join(', ') : '…'));
  }
  if (w.bench === 'dom') parts.push(w.dom_task);
  return parts.join(' · ');
}

function renderLaeufe(meta, jobs, det) {
  const w = S.wiz;
  const bench = meta.benchmarks.find(b => b.id === w.bench);
  const needsModel = w.bench && w.bench !== 'suite-api' && w.bench !== 'robustheit';
  const needsTasks = ['suite', 'suite-api'].includes(w.bench);
  const lock = meta.lock;

  /* ---- Schritt 1: Benchmark ---- */
  const s1body = `
    <div class="optlist">${meta.benchmarks.map(b => {
      const gpuBlocked = b.cls === 'gpu' && lock.locked;
      const bchip = b.cls === 'gpu' ? '⚛ GPU' : b.cls === 'cpu' ? '☰ CPU' : '☁ Cloud';
      return `
      <button class="opt ${w.bench === b.id ? 'sel' : ''}" data-act="wbench" data-id="${b.id}">
        <div><div class="ot">${esc(b.name)}
          <span class="chip ${b.cls === 'gpu' ? '' : 'quiet'}">${bchip}</span>
          ${gpuBlocked ? chip('warn', 'GPU gesperrt', lock.reason) : ''}
          ${b.needs_8090 && !meta.port8090 ? chip('warn', ':8090 aus') : ''}</div>
        <div class="os">${esc(b.desc)}</div></div>
      </button>`; }).join('')}
    </div>`;

  /* ---- Schritt 2: Modell ---- */
  let s2body = '<p class="muted small">Zuerst Benchmark wählen.</p>';
  if (w.bench === 'suite-api') {
    s2body = `
      <div class="optlist">${meta.api_models.map(m => `
        <button class="opt ${w.model_id === m.id ? 'sel' : ''}" data-act="wapimodel" data-id="${esc(m.id)}">
          <div><div class="ot">${esc(m.name)}
            ${m.key_present ? chip('ok', 'Key da', m.key_env) : chip('bad', m.key_env + ' fehlt', 'In ~/ai-lab/.env eintragen')}</div>
          <div class="os">${esc(m.id)} · ${esc(m.provider)}</div></div>
        </button>`).join('')}
      </div>
      <div class="form-grid" style="margin-top:12px">
        <div><label>oder eigene Modell-ID (anbieter/modell)</label>
          <input type="text" data-f="model_id" value="${esc(w.model_id)}" placeholder="deepseek/deepseek-v4-flash"></div>
      </div>`;
  } else if (w.bench === 'robustheit') {
    s2body = `
      <p class="muted small">Kein Modell nötig — die Batterie läuft gegen alle vorhandenen Abgaben in
        <code>bench/runs</code>. CPU, ohne GPU-Lock; die GPU bleibt frei.</p>`;
  } else if (w.bench) {
    const installed = (S.regModels || []).filter(e => e.status === 'installiert');
    const llama = meta.llama || [];
    s2body = `
      <div class="optlist">${installed.map(m => {
        const serving = llama.some(l => l.model_file === m.file
          && (m.kind !== 'vision' || l.mmproj === (m.mmproj || '')));
        const other = llama.length && !serving;
        return `
        <button class="opt ${w.model === m.id ? 'sel' : ''}" data-act="wmodel" data-id="${esc(m.id)}">
          <div><div class="ot">${esc(m.name)}
            ${serving ? chip('ok', 'Server läuft', 'llama-server bedient genau diese Datei')
              : other ? chip('warn', 'anderer Server aktiv', 'llama-server bedient ein anderes Modell — Dashboard stoppt keine Server')
              : chip('off', 'kein Server')}</div>
          <div class="os">${esc(m.file)} · ${fmtGB(m.size)}${m.kind === 'vision' ? ' · Vision' : ''}</div></div>
        </button>`; }).join('')
        || '<p class="muted">Keine installierten Modelle in der Registry.</p>'}
      </div>
      ${w.model && !llama.length ? (lock.locked
        ? `<div class="warnbox">🔒 Kein llama-server aktiv, und die GPU ist gesperrt — ${esc(lock.reason)}</div>`
        : `<label class="toggle" style="margin-top:10px"><input type="checkbox" data-f="start_server" ${w.start_server ? 'checked' : ''}>
           Server zuerst starten (serve.sh ${esc(w.model)} ${esc(w.backend)}) — Teil des Jobs</label>`) : ''}
      ${w.model && llama.length && !llama.some(l => (installed.find(m => m.id === w.model) || {}).file === l.model_file)
        ? '<div class="warnbox">⚠ Der laufende llama-server bedient ein anderes Modell. Das Dashboard stoppt niemals Server — bitte passendes Modell wählen oder von Hand wechseln.</div>' : ''}`;
  }

  /* ---- Schritt 3: Tasks + Erweitert + Start ---- */
  let s3body = '<p class="muted small">Zuerst Benchmark und Modell wählen.</p>';
  const ready = w.bench && (w.bench === 'suite-api' ? !!w.model_id
    : ['robustheit', 'uxdom', 'dom'].includes(w.bench) ? true : !!w.model);
  if (ready) {
    let taskSel = '';
    if (needsTasks) {
      taskSel = `
        <div><label>Tasks</label>
        <div class="taskchips">
          <button class="tchip ${w.all_tasks ? 'sel' : ''}" data-act="wtask" data-id="__all">alle (${meta.tasks.length})</button>
          ${meta.tasks.map(t => `<button class="tchip ${!w.all_tasks && w.tasks.includes(t) ? 'sel' : ''}" data-act="wtask" data-id="${esc(t)}">${esc(t)}</button>`).join('')}
        </div></div>`;
    } else if (w.bench === 'dom') {
      taskSel = `
        <div><label>Aufgabe</label><select data-f="dom_task">
          <option value="info" ${w.dom_task === 'info' ? 'selected' : ''}>info — Preis finden</option>
          <option value="form" ${w.dom_task === 'form' ? 'selected' : ''}>form — Formular senden</option>
        </select></div>`;
    } else if (w.bench === 'polyglot') {
      taskSel = '<p class="muted small">Fester Umfang: 73 Übungen (python+go) — Laufzeit mehrere Stunden.</p>';
    } else if (w.bench === 'robustheit') {
      taskSel = `
        <p class="muted small">Fester Umfang: alle Batterie-Tasks × alle Abgabe-Labels. Nur Paare mit
          geändertem ws-Fingerprint werden neu gerechnet; die Ergebnisse landen in
          <code>bench/robustness-battery/results.json</code> (Schema 2).</p>
        <label class="toggle"><input type="checkbox" data-f="force" ${w.force ? 'checked' : ''}>
          <code>--force</code>: alle Paare neu berechnen, auch unveränderte</label>`;
    }
    s3body = `
      <div class="form-grid">
        ${taskSel}
        <details class="adv"><summary>Erweitert</summary>
          <div class="form-grid">
            ${needsModel ? `
            <div><label>Backend (für „Server starten“)</label>
              <select data-f="backend">${meta.backends.map(b => `<option ${w.backend === b ? 'selected' : ''}>${b}</option>`).join('')}</select></div>
            <div><label>Kontextgröße (leer = serve.sh-Default)</label>
              <input type="number" data-f="ctx" value="${esc(w.ctx)}" placeholder="32768" min="2048" max="131072"></div>
            <div><label>Zusätzliche llama-server-Flags (max. 8)</label>
              <input type="text" data-f="extra_flags" value="${esc(w.extra_flags)}" placeholder="-b 2048"></div>` : ''}
            <div><label>Timeout ${needsTasks ? 'je Task' : ''} in Sekunden</label>
              <input type="number" data-f="timeout" value="${esc(w.timeout)}" placeholder="${w.bench === 'polyglot' ? '21600' : meta.timeout_default}"></div>
            <div><label>Label (optional, für results.jsonl)</label>
              <input type="text" data-f="label" value="${esc(w.label)}" placeholder="${w.bench === 'suite-api' ? 'oc-v4-flash' : (w.model || '') + '-' + w.backend}"></div>
          </div>
        </details>
        <button class="btn" data-act="wstart">▶ Lauf starten</button>
        ${S.runMsg ? `<div class="${S.runMsg.ok ? 'okline' : 'errline'}">${S.runMsg.ok ? '✓' : '✗'} ${esc(S.runMsg.text)}</div>` : ''}
      </div>`;
  }

  const step = (n, title, val, body, open, done) => `
    <section class="wstep ${open ? 'open' : ''} ${done ? 'done' : ''}">
      <header data-act="wgoto" data-id="${n}">
        <span class="n">${done && !open ? '✓' : n}</span>
        <span class="t">${title}</span>
        <span class="v">${esc(val || '')}</span>
      </header>
      ${open ? `<div class="wbody">${body}</div>` : ''}
    </section>`;

  const benchName = bench ? bench.name : '';
  const modelName = w.bench === 'suite-api' ? w.model_id : (w.model ? name(w.model) : '');
  const wizard = `
    <div class="wizard">
      ${step(1, 'Benchmark', benchName, s1body, w.step === 1, !!w.bench)}
      ${step(2, 'Modell', modelName, s2body, w.step === 2, !!(w.bench === 'suite-api' ? w.model_id : w.bench === 'robustheit' ? true : w.model))}
      ${step(3, 'Umfang & Start', ready && w.step !== 3 ? wizSummary(meta) : '', s3body, w.step === 3, false)}
    </div>`;

  /* ---- Job-Karten ---- */
  const jobChip = j => j.status === 'läuft' ? chip('run', 'läuft')
    : j.status === 'fertig' ? chip('ok', 'fertig')
    : j.status === 'fehler' ? chip('bad', 'Fehler')
    : j.status === 'timeout' ? chip('bad', 'Timeout')
    : j.status === 'abgebrochen' ? chip('off', 'abgebrochen')
    : chip('off', j.status);
  const jobCards = jobs.jobs.map(j => {
    const sel = det && det.id === j.id;
    const steps = (j.steps || []).map(s => `
      <span class="stepchip ${s.status === 'fertig' ? 'ok' : s.status === 'läuft' ? 'run' : ['fehler', 'timeout'].includes(s.status) ? 'bad' : ''}">${esc(s.name)}</span>`).join('');
    return `
      <div class="card runcard ${sel ? 'sel' : ''}" data-act="job" data-id="${esc(j.id)}">
        <header><b>${esc(j.desc)}</b>${jobChip(j)}
          <span class="chip quiet">${esc(j.class_name || j.class)}</span>
          <span class="grow"></span>
          <span class="muted small num">${esc(j.started)}${j.seconds ? ' · ' + fmtDur(j.seconds) : ''}</span>
          ${j.status === 'läuft' && !j.detached ? `<button class="btn danger sm" data-act="jobcancel" data-id="${esc(j.id)}">■ abbrechen</button>` : ''}
        </header>
        <div class="steps">${steps}</div>
        ${sel ? `<div class="logtail">${esc((det.tail || []).join('\n')) || 'noch keine Ausgabe'}</div>` : ''}
      </div>`;
  }).join('');
  const slots = jobs.slots;
  const cpuSlots = slots.cpu ? ` · CPU ${slots.cpu.used}/${slots.cpu.max}` : '';
  view.innerHTML = viewHead('Läufe', `Slots: GPU ${slots.gpu.used}/${slots.gpu.max} · Cloud ${slots.cloud.used}/${slots.cloud.max} · Download ${slots.download.used}/${slots.download.max}${cpuSlots}`)
    + `<div class="laeufe-grid">
      <div>${wizard}
        <p class="muted small" style="margin-top:12px">Kein automatischer Neustart: Ein fehlgeschlagener Lauf bleibt stehen, bis du hier neu startest.</p></div>
      <div>
        <h3 class="sec" style="margin-top:2px">Historie (${jobs.jobs.length})</h3>
        ${jobCards || '<div class="empty">Noch keine Jobs über das Dashboard gestartet.</div>'}
      </div>
    </div>`;
  if (jobs.active) {
    clearTimeout(S.fastT);
    S.fastT = setTimeout(() => { if (S.route === 'laeufe') load({ silent: true }); }, 3000);
  }
}

async function startJob() {
  const w = S.wiz;
  const body = { action: w.bench };
  if (w.bench === 'suite-api') body.model_id = w.model_id.trim();
  else body.model = w.model;
  if (['suite', 'suite-api'].includes(w.bench)) {
    if (w.all_tasks) body.all_tasks = true; else body.tasks = w.tasks;
  }
  if (w.bench === 'dom') body.task = w.dom_task;
  if (w.bench === 'robustheit') body.force = w.force;
  if (w.bench !== 'suite-api' && w.bench !== 'robustheit') {
    body.backend = w.backend;
    if (w.start_server) body.start_server = true;
    if (w.ctx) body.ctx = w.ctx;
    if (w.extra_flags.trim()) body.extra_flags = w.extra_flags.trim();
  }
  if (w.timeout) body.timeout = w.timeout;
  if (w.label.trim()) body.label = w.label.trim();
  try {
    const r = await post('/api/jobs', body);
    S.runMsg = { ok: true, text: 'Gestartet: ' + r.id };
    S.sel.job = r.id;
  } catch (e) {
    S.runMsg = { ok: false, text: e.message };
  }
  load({ silent: true });
}

/* ================================================================ Modelle */
function modelCard(m, isCatalog) {
  const st = m.status === 'installiert' ? chip('ok', 'installiert')
    : m.status === 'teilweise' ? chip('warn', 'mmproj fehlt')
    : chip('off', 'nicht installiert');
  const served = m.served ? chip('run', 'Server läuft', m.use_reason) : '';
  const del = S.confirmDel === m.id ? `
    <div class="confirmbox">
      <div>⚠ <b>${esc(m.file)}</b> wirklich löschen? Zum Bestätigen die Modell-ID
        <code>${esc(m.id)}</code> eintippen:</div>
      <input type="text" id="delconfirm" placeholder="${esc(m.id)}" autocomplete="off">
      ${m.mmproj ? `<label class="toggle"><input type="checkbox" id="delmmproj"> mmproj (${esc(m.mmproj)}) mitlöschen</label>` : ''}
      <div style="display:flex;gap:8px">
        <button class="btn danger sm" data-act="delGo" data-id="${esc(m.id)}">endgültig löschen</button>
        <button class="btn ghost sm" data-act="delOff">abbrechen</button>
      </div>
    </div>` : '';
  const args = m.args || {};
  const argStr = ['ctx' in args ? 'ctx ' + de(args.ctx, 0) : '',
                  args.kv ? 'KV ' + args.kv : '',
                  'ncmoe' in args ? 'ncmoe ' + args.ncmoe : '',
                  args.extra || ''].filter(Boolean).join(' · ');
  return `
    <article class="card comp">
      <header><h2>${esc(m.name)}</h2><span class="chip quiet">${esc(m.id)}</span>
        <span class="grow"></span>${st}${served}</header>
      <dl class="kv">
        <dt>Datei</dt><dd>${esc(m.file)}${m.size != null ? ' · ' + fmtGB(m.size) : ''}</dd>
        ${m.mmproj ? `<dt>mmproj</dt><dd>${esc(m.mmproj)}${m.mmproj_exists === false ? ' · <span class="chip bad">✗ fehlt</span>' : m.mmproj_size ? ' · ' + fmtGB(m.mmproj_size) : ''}</dd>` : ''}
        ${m.hf_repo ? `<dt>HF-Repo</dt><dd>${esc(m.hf_repo)}</dd>` : ''}
        ${argStr ? `<dt>Server-Args</dt><dd>${esc(argStr)}</dd>` : ''}
        ${m.vram ? `<dt>VRAM</dt><dd>${esc(m.vram)}</dd>` : ''}
      </dl>
      ${m.notes ? `<p class="small muted" style="margin:2px 0">${esc(m.notes)}</p>` : ''}
      <div class="foot">
        ${m.status !== 'installiert' ? `<button class="btn sm" data-act="dl" data-id="${esc(m.id)}">⬇ Herunterladen</button>` : ''}
        ${m.status !== 'fehlt' ? `<button class="btn ghost sm" data-act="delOn" data-id="${esc(m.id)}" ${m.in_use ? `disabled title="${esc(m.use_reason)}"` : ''}>Löschen …</button>` : ''}
        <button class="btn ghost sm" data-act="editModel" data-id="${esc(m.id)}">Bearbeiten</button>
      </div>
      ${del}
    </article>`;
}

function renderModelle(d) {
  S.regModels = d.models.concat(d.catalog);
  const lockNote = d.lock.locked
    ? `<div class="warnbox">🔒 ${esc(d.lock.reason)}<br><span class="small">Downloads und Registry-Pflege bleiben verfügbar.</span></div>` : '';
  const inst = d.models.filter(m => m.status !== 'fehlt');
  const missing = d.models.filter(m => m.status === 'fehlt');
  const e = S.editModel;
  const form = `
    <details class="adv" ${e ? 'open' : ''}><summary>${e && e.id ? 'Modell bearbeiten: ' + esc(e.id) : 'Modell hinzufügen (HF-Repo + Datei oder direkte URL)'}</summary>
      <div class="form-grid" style="max-width:560px">
        <div><label>ID *</label><input type="text" data-mf="id" value="${esc(e?.id || '')}" placeholder="mistral-small" ${e?.id ? 'readonly' : ''}></div>
        <div><label>Name</label><input type="text" data-mf="name" value="${esc(e?.name || '')}" placeholder="Mistral Small 24B"></div>
        <div><label>HF-Repo (owner/repo)</label><input type="text" data-mf="hf_repo" value="${esc(e?.hf_repo || '')}" placeholder="unsloth/Modell-GGUF"></div>
        <div><label>HF-Datei (*.gguf)</label><input type="text" data-mf="hf_file" value="${esc(e?.hf_file || '')}" placeholder="Modell-UD-Q4_K_XL.gguf"></div>
        <div><label>oder direkte URL (https)</label><input type="text" data-mf="url" value="${esc(e?.url || '')}" placeholder="https://huggingface.co/…/resolve/main/….gguf"></div>
        <div><label>mmproj-Datei (optional)</label><input type="text" data-mf="mmproj" value="${esc(e?.mmproj || '')}" placeholder="mmproj-….gguf"></div>
        <div><label>Kontext (Default-Arg)</label><input type="number" data-mf="ctx" value="${esc(e?.args?.ctx ?? '')}" placeholder="32768"></div>
        <div><label>Notizen / VRAM-Einschätzung</label><input type="text" data-mf="notes" value="${esc(e?.notes || '')}" placeholder="passt komplett in 20 GB VRAM …"></div>
        <div style="display:flex;gap:8px">
          <button class="btn sm" data-act="modelSave">Speichern</button>
          ${e ? '<button class="btn ghost sm" data-act="modelEditOff">abbrechen</button>' : ''}
        </div>
        ${S.compMsg ? `<div class="${S.compMsg.ok ? 'okline' : 'errline'}">${S.compMsg.ok ? '✓' : '✗'} ${esc(S.compMsg.text)}</div>` : ''}
      </div>
    </details>`;
  view.innerHTML = viewHead('Modelle', 'Registry: dashboard/registry/models.json')
    + lockNote + form
    + '<h3 class="sec">Installiert</h3>'
    + (inst.length ? `<div class="comp-grid">${inst.map(m => modelCard(m)).join('')}</div>`
       : '<div class="empty">Nichts installiert.</div>')
    + (missing.length ? '<h3 class="sec">Registriert, aber Datei fehlt</h3><div class="comp-grid">'
       + missing.map(m => modelCard(m)).join('') + '</div>' : '')
    + '<h3 class="sec">Verfügbar (Katalog — noch nicht installiert)</h3>'
    + (d.catalog.length ? `<div class="comp-grid">${d.catalog.map(m => modelCard(m, true)).join('')}</div>`
       : '<div class="empty">Katalog leer — über das Formular oben ergänzen.</div>')
    + (d.unregistered.length ? `
      <h3 class="sec">Dateien in models/ ohne Registry-Eintrag</h3>
      <div class="tbl-wrap"><table class="list"><thead><tr><th>Datei</th><th class="num">Größe</th></tr></thead>
      <tbody>${d.unregistered.map(u => `<tr><td>${esc(u.file)}</td><td class="num">${fmtGB(u.size)}</td></tr>`).join('')}</tbody></table></div>
      <p class="muted small">Werden vom Dashboard nie gelöscht (kein Registry-Eintrag).</p>` : '');
}

/* =========================================================== Harness/Backend */
function renderHarnesses(d) {
  view.innerHTML = viewHead('Harnesses', 'Agenten-Werkzeuge für Benchmarks')
    + `<div class="comp-grid">${d.harnesses.map(h => `
      <article class="card comp">
        <header><h2>${esc(h.name)}</h2><span class="grow"></span>
          ${h.installed ? chip('ok', 'installiert') : chip('bad', 'fehlt')}</header>
        <dl class="kv">
          <dt>Typ</dt><dd>${h.type === 'pacman' ? 'Arch-Paket' : 'Podman-Image'}</dd>
          <dt>Erkannt</dt><dd>${esc(h.detail || '')}</dd>
          ${h.version_pinned ? `<dt>Version</dt><dd>gepinnt: ${esc(h.version_pinned)}</dd>` : ''}
        </dl>
        ${h.notes ? `<p class="small muted" style="margin:2px 0">${esc(h.notes)}</p>` : ''}
        <div class="cmdbox"><code>${esc(h.install)}</code>
          <button class="copybtn" data-act="copy" data-copy="${esc(h.install)}">kopieren</button></div>
      </article>`).join('')}</div>
    <p class="foot-note">(Neu-)Installation ist bewusst manuell: Kommando kopieren und selbst ausführen.</p>`;
}

function renderBackends(d) {
  view.innerHTML = viewHead('Backends', 'GGML-Rechen-Backends (llama.cpp)')
    + `<div class="comp-grid">${d.backends.map(b => `
      <article class="card comp">
        <header><h2>${esc(b.name)}</h2><span class="grow"></span>
          ${b.installed ? chip('ok', b.version || 'installiert') : chip('bad', 'fehlt')}</header>
        <dl class="kv">
          <dt>Paket</dt><dd>${esc(b.package)}</dd>
          <dt>Backend</dt><dd>${esc(b.ggml_backend || '')}</dd>
        </dl>
        ${b.notes ? `<p class="small muted" style="margin:2px 0">${esc(b.notes)}</p>` : ''}
        <div class="cmdbox"><code>${esc(b.install_cmd)}</code>
          <button class="copybtn" data-act="copy" data-copy="${esc(b.install_cmd)}">kopieren</button></div>
      </article>`).join('')}</div>
    <p class="foot-note">${esc(d.hinweis)}</p>`;
}

/* ================================================================ Wiki */
function renderWiki(w, doc) {
  const btn = d => `
    <button class="${S.sel.wiki === d.id ? 'sel' : ''}" data-act="wiki" data-id="${esc(d.id)}">
      ${esc(d.title)}${d.exists === false ? ' <span class="chip bad">✗ fehlt</span>' : ''}</button>`;
  const nav = w.sections.map(sec => {
    let items = sec.docs.map(btn).join('');
    if (sec.legacy) items += sec.legacy.map(btn).join('');
    return `<div class="navgroup">${esc(sec.title)}</div>`
      + (items || '<span class="muted small" style="padding:0 12px">noch leer</span>');
  }).join('');
  const emptyNote = w.empty ? `
    <div class="notebox">✎ Die Wiki-Inhalte (Konzepte/Anleitungen) entstehen gerade —
      sie landen als Markdown unter <code>dashboard/wiki/</code> und erscheinen hier
      automatisch. Die Referenz-Dokumente unten sind schon da.</div>` : '';
  view.innerHTML = viewHead('Wiki', 'Konzepte · Anleitungen · Referenz') + emptyNote + `
    <div class="wiki-grid">
      <nav class="wnav">${nav}</nav>
      <div class="mdbody">${doc && doc.md ? md2html(doc.md) : '<span class="muted">' + esc((doc && doc.error) || 'Dokument wählen …') + '</span>'}</div>
    </div>`;
}

/* ============================================================ Einstellungen */
function renderEinstellungen() {
  let theme = 'system';
  try { theme = localStorage.getItem('ailab-theme') || 'system'; } catch (e) {}
  view.innerHTML = viewHead('Einstellungen', '') + `
    <div class="settings">
      <div class="card">
        <h2>Darstellung</h2>
        <div class="radio-row" style="margin-top:8px">
          ${['system', 'light', 'dark'].map(t => `
            <label><input type="radio" name="theme" value="${t}" ${theme === t ? 'checked' : ''} data-act="theme">
            ${t === 'system' ? 'System' : t === 'light' ? 'Hell' : 'Dunkel'}</label>`).join('')}
        </div>
      </div>
      <div class="card">
        <h2>Standard-Schalter</h2>
        <label class="toggle" style="margin-top:8px"><input type="checkbox" data-act="defPolyHidden" ${S.toggles.polyHidden ? 'checked' : ''}>
          Polyglot: verworfene Läufe standardmäßig zeigen</label><br>
        <label class="toggle" style="margin-top:8px"><input type="checkbox" data-act="defSuiteOld" ${S.toggles.suiteOld ? 'checked' : ''}>
          Suite: ältere Versuche standardmäßig zeigen</label>
      </div>
      <div class="card">
        <h2>Fakten</h2>
        <dl class="kv" style="margin-top:8px">
          <dt>Server</dt><dd>127.0.0.1:8100 (nur lokal)</dd>
          <dt>Schreibt in</dt><dd>dashboard/ (registry/, runs/) + Downloads nach models/</dd>
          <dt>Liest</dt><dd>bench/, logs/, models/ — strikt read-only</dd>
          <dt>Tastatur</dt><dd>1–9, 0 wechseln die Ansicht</dd>
          <dt>Auto-Refresh</dt><dd>alle 10 s (aktive Ansicht + Systemleiste)</dd>
        </dl>
      </div>
    </div>`;
}

/* ================================================================ Router */
async function load(opts = {}) {
  const r = S.route;
  if (!opts.silent) view.innerHTML = '<div class="loading">Lade …</div>';
  try {
    if (r === 'uebersicht') renderUebersicht(await api('/api/overview'));
    else if (r === 'suite') {
      const [suite, rob] = await Promise.all([
        api('/api/suite'), api('/api/robustness').catch(() => null)]);
      renderSuite(suite, rob);
    }
    else if (r === 'polyglot') renderPolyglot(await api('/api/polyglot' + (S.toggles.polyHidden ? '?hidden=1' : '')));
    else if (r === 'perf') renderPerf(await api('/api/perf'));
    else if (r === 'apps') {
      const [dom, ux] = await Promise.all([api('/api/dom'), api('/api/ux')]);
      if (!S.sel.ux && ux.docs.length) S.sel.ux = ux.docs[0].file;
      const [uxDoc, tr] = await Promise.all([
        S.sel.ux ? api('/api/ux/doc?file=' + encodeURIComponent(S.sel.ux)).catch(() => null) : null,
        S.sel.domT ? api('/api/dom/transcript?file=' + encodeURIComponent(S.sel.domT)).catch(() => null) : null,
      ]);
      renderApps(dom, ux, uxDoc, tr);
    } else if (r === 'laeufe') {
      const [meta, jobs, reg] = await Promise.all([
        api('/api/meta'), api('/api/jobs'), api('/api/registry/models')]);
      S.regModels = reg.models.concat(reg.catalog);
      let det = null;
      const active = jobs.jobs.find(x => x.status === 'läuft');
      const selId = S.sel.job && jobs.jobs.some(x => x.id === S.sel.job) ? S.sel.job
        : (active || jobs.jobs[0] || {}).id;
      if (selId) det = await api('/api/jobs/' + selId).catch(() => null);
      renderLaeufe(meta, jobs, det);
    } else if (r === 'modelle') renderModelle(await api('/api/registry/models'));
    else if (r === 'harnesses') renderHarnesses(await api('/api/registry/harnesses'));
    else if (r === 'backends') renderBackends(await api('/api/registry/backends'));
    else if (r === 'wiki') {
      const w = await api('/api/wiki');
      if (!S.sel.wiki) {
        const first = w.sections.flatMap(s => s.docs)[0]
          || (w.sections[2] && w.sections[2].legacy || [])[0];
        S.sel.wiki = first ? first.id : null;
      }
      const doc = S.sel.wiki
        ? await api('/api/wiki/doc?id=' + encodeURIComponent(S.sel.wiki)).catch(e => ({ error: e.message }))
        : null;
      renderWiki(w, doc);
    } else if (r === 'einstellungen') renderEinstellungen();
  } catch (e) {
    view.innerHTML = `<div class="error-box">⚠ Fehler beim Laden: ${esc(e.message)}</div>`;
  }
}

function setRoute(r, push = true) {
  if (!ROUTES.includes(r)) r = 'uebersicht';
  S.route = r;
  S.runMsg = S.compMsg = null;
  renderNav();
  if (push && location.hash !== '#/' + r) location.hash = '/' + r;
  load();
}

/* ================================================================ Events */
navEl.addEventListener('click', e => {
  const b = e.target.closest('[data-nav]');
  if (b) setRoute(b.dataset.nav);
});
document.getElementById('nav-settings').addEventListener('click',
  () => setRoute('einstellungen'));
window.addEventListener('hashchange', () => {
  const r = location.hash.replace(/^#\/?/, '');
  if (r && r !== S.route) setRoute(r, false);
});

view.addEventListener('click', async e => {
  const t = e.target.closest('[data-act]');
  if (!t) return;
  const a = t.dataset;
  const silent = () => load({ silent: true });
  switch (a.act) {
    case 'nav': setRoute(a.nav); break;
    case 'poly': S.sel.poly = a.id; silent(); break;
    case 'cell': S.sel.suiteCell = S.sel.suiteCell === a.id ? null : a.id; silent(); break;
    case 'robrow': S.sel.robLabel = S.sel.robLabel === a.id ? null : a.id; silent(); break;
    case 'wikiLink': S.sel.wiki = a.id; setRoute('wiki'); break;
    case 'ux': S.sel.ux = a.id; silent(); break;
    case 'wiki': S.sel.wiki = a.id; silent(); break;
    case 'domt': e.stopPropagation(); S.sel.domT = S.sel.domT === a.id ? null : a.id; silent(); break;
    case 'job': if (!e.target.closest('.logtail') && !e.target.closest('button')) { S.sel.job = a.id; silent(); } break;
    case 'jobcancel': e.stopPropagation();
      try { await post('/api/jobs/' + a.id + '/cancel', {}); } catch (err) { alert(err.message); }
      silent(); break;
    case 'wgoto': S.wiz.step = +a.id; silent(); break;
    case 'wbench': S.wiz.bench = a.id; S.wiz.step = 2; S.runMsg = null;
      S.wiz.tasks = []; S.wiz.all_tasks = false; S.wiz.start_server = false; S.wiz.force = false; silent(); break;
    case 'wmodel': S.wiz.model = a.id; S.wiz.step = 3; silent(); break;
    case 'wapimodel': S.wiz.model_id = a.id; S.wiz.step = 3; silent(); break;
    case 'wtask':
      if (a.id === '__all') { S.wiz.all_tasks = !S.wiz.all_tasks; }
      else {
        S.wiz.all_tasks = false;
        const i = S.wiz.tasks.indexOf(a.id);
        if (i >= 0) S.wiz.tasks.splice(i, 1); else S.wiz.tasks.push(a.id);
      }
      silent(); break;
    case 'wstart': startJob(); break;
    case 'polyHide':
      try { await post('/api/polyglot/annotate', { dir: a.id, hidden: a.hide === '1' }); }
      catch (err) { alert(err.message); }
      silent(); break;
    case 'suiteOld': S.toggles.suiteOld = t.checked; saveToggles(); silent(); break;
    case 'polyHidden': S.toggles.polyHidden = t.checked; saveToggles(); silent(); break;
    case 'defPolyHidden': S.toggles.polyHidden = t.checked; saveToggles(); break;
    case 'defSuiteOld': S.toggles.suiteOld = t.checked; saveToggles(); break;
    case 'copy': copyText(a.copy); t.textContent = '✓ kopiert';
      setTimeout(() => { t.textContent = 'kopieren'; }, 1500); break;
    case 'dl':
      try {
        const r = await post('/api/registry/models/download', { id: a.id });
        S.compMsg = { ok: true, text: 'Download gestartet: ' + r.id + ' (siehe Läufe)' };
      } catch (err) { S.compMsg = { ok: false, text: err.message }; }
      silent(); break;
    case 'delOn': S.confirmDel = a.id; silent(); break;
    case 'delOff': S.confirmDel = null; silent(); break;
    case 'delGo': {
      const inp = document.getElementById('delconfirm');
      const mm = document.getElementById('delmmproj');
      try {
        const r = await post('/api/registry/models/remove',
          { id: a.id, confirm: inp ? inp.value.trim() : '', with_mmproj: !!(mm && mm.checked) });
        S.compMsg = { ok: true, text: r.message };
        S.confirmDel = null;
      } catch (err) { S.compMsg = { ok: false, text: err.message }; }
      silent(); break;
    }
    case 'editModel': S.editModel = (S.regModels || []).find(m => m.id === a.id) || null; silent(); break;
    case 'modelEditOff': S.editModel = null; silent(); break;
    case 'modelSave': {
      const g = f => { const el = view.querySelector(`[data-mf="${f}"]`); return el ? el.value.trim() : ''; };
      const body = { id: g('id'), name: g('name'), hf_repo: g('hf_repo'),
        hf_file: g('hf_file'), url: g('url'), mmproj: g('mmproj'),
        notes: g('notes'), args: { ctx: g('ctx') } };
      if (S.editModel) body.kind = S.editModel.kind;
      try {
        const r = await post('/api/registry/models', body);
        S.compMsg = { ok: true, text: (r.updated ? 'Aktualisiert: ' : 'Angelegt: ') + r.id };
        S.editModel = null;
      } catch (err) { S.compMsg = { ok: false, text: err.message }; }
      silent(); break;
    }
    case 'theme': {
      const v = t.value;
      try {
        if (v === 'system') localStorage.removeItem('ailab-theme');
        else localStorage.setItem('ailab-theme', v);
      } catch (err) {}
      if (v === 'system') delete document.documentElement.dataset.theme;
      else document.documentElement.dataset.theme = v;
      break;
    }
  }
});

view.addEventListener('change', e => {
  const t = e.target.closest('[data-f]');
  if (!t) return;
  const f = t.dataset.f;
  S.wiz[f] = t.type === 'checkbox' ? t.checked : t.value;
  S.runMsg = null;
  if (['start_server', 'backend', 'dom_task'].includes(f)) load({ silent: true });
});
view.addEventListener('input', e => {
  const t = e.target.closest('[data-f]');
  if (t && t.type !== 'checkbox') S.wiz[t.dataset.f] = t.value;
});

document.addEventListener('keydown', e => {
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  const tag = (document.activeElement || {}).tagName;
  if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') return;
  const keys = { 1: 'uebersicht', 2: 'suite', 3: 'polyglot', 4: 'apps', 5: 'perf',
                 6: 'laeufe', 7: 'modelle', 8: 'harnesses', 9: 'backends', 0: 'wiki' };
  if (keys[e.key]) setRoute(keys[e.key]);
});

/* Auto-Refresh: alle 10 s aktive Ansicht + Systemleiste. */
setInterval(() => {
  if (document.visibilityState !== 'visible') return;
  loadState();
  const el = document.activeElement;
  if (el && el.matches('input, select, textarea')) return;
  if (S.confirmDel || S.editModel) return;
  load({ silent: true });
}, 10000);

/* ---------------------------------------------------------------- Start */
(async () => {
  const qp = new URLSearchParams(location.search);
  const qv = qp.get('view');
  await loadNames();
  loadState();
  const initial = qv || location.hash.replace(/^#\/?/, '') || 'uebersicht';
  setRoute(initial, !qv);
})();
})();
