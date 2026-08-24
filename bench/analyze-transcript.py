#!/usr/bin/env python3
"""Extract agent metrics from an OpenCode --format json transcript."""
import json, sys, collections

def analyze(path):
    events = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line.startswith('{'):
                try: events.append(json.loads(line))
                except json.JSONDecodeError: pass
    if not events:
        return None
    t0 = events[0].get('timestamp')
    tend = events[-1].get('timestamp')
    first_tool = next((e['timestamp'] for e in events if e['type'] == 'tool_use'), None)
    tools = collections.Counter(e['part'].get('tool') for e in events if e['type'] == 'tool_use')
    tool_errors = sum(1 for e in events if e['type'] == 'tool_use'
                      and e['part'].get('state', {}).get('status') == 'error')
    steps = sum(1 for e in events if e['type'] == 'step_finish')
    tok_in = tok_out = tok_reason = cache_read = 0
    for e in events:
        if e['type'] == 'step_finish':
            t = e['part'].get('tokens', {})
            tok_in += t.get('input', 0); tok_out += t.get('output', 0)
            tok_reason += t.get('reasoning', 0)
            cache_read += t.get('cache', {}).get('read', 0)
    return {
        'events': len(events), 'steps': steps,
        'wall_s': round((tend - t0) / 1000, 1) if t0 and tend else None,
        'first_action_s': round((first_tool - t0) / 1000, 1) if first_tool and t0 else None,
        'tool_calls': sum(tools.values()), 'tools': dict(tools), 'tool_errors': tool_errors,
        'tokens_in': tok_in, 'tokens_out': tok_out, 'tokens_reasoning': tok_reason,
        'cache_read': cache_read,
        'out_tok_per_s': round(tok_out / ((tend - t0) / 1000), 1) if t0 and tend and tend > t0 else None,
    }

if __name__ == '__main__':
    for p in sys.argv[1:]:
        r = analyze(p)
        print(p)
        print(' ', json.dumps(r))
