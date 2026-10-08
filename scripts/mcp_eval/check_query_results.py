#!/usr/bin/env python3
"""Compare independent saved-view reads with reviewed final expectations.

This checks query rows only, not the complete scenario or its model prose.
"""
import argparse
from collections import Counter
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
EXPECTATIONS = ROOT / 'docs/evals/anytype-mcp-v2/final-query-expectations.json'
DEFAULT_RUNS = Path('/private/tmp/anytype-mcp-eval-runs')


def load(path):
    try:
        return json.loads(path.read_text())
    except (OSError, ValueError):
        return None


def content_json(result):
    for part in (result or {}).get('content', []):
        if part.get('type') == 'text':
            try:
                doc = json.loads(part.get('text', ''))
            except (TypeError, ValueError):
                continue
            if isinstance(doc, dict) and 'request_metadata' not in doc:
                yield doc


def query_objects(reads):
    found = {}
    for i, read in enumerate(reads):
        if read.get('tool') != 'API-get-object':
            continue
        for doc in content_json(read.get('result')):
            props = doc.get('properties', {})
            if doc.get('type') == 'query' and not props.get('is_archived'):
                if doc.get('id') and props.get('name'):
                    found.setdefault(props['name'], {})[doc['id']] = {
                        'id': doc['id'], 'ref': f'final-state.json reads[{i}]',
                        'views': [v for b in doc.get('blocks', []) for v in b.get('views', [])]}
    return found


def collect_pages(reads, query_id, view_id):
    pages = []
    for i, read in enumerate(reads):
        args = read.get('arguments', {})
        if read.get('tool') != 'API-get-query-objects' or args.get('query_id') != query_id or args.get('view') != view_id:
            continue
        doc = next(content_json(read.get('result')), {})
        if not isinstance(doc.get('data'), list):
            return None, {'reason': 'query read returned no row array', 'ref': f'final-state.json reads[{i}]'}
        pages.append((args.get('offset', 0), doc, f'final-state.json reads[{i}]'))
    if not pages:
        return None, {'reason': 'no independent read for the selected explicit view'}
    pages.sort(key=lambda page: page[0])
    rows, refs, offset = [], [], 0
    total = pages[0][1].get('total')
    for index, (start, doc, ref) in enumerate(pages):
        if start != offset or doc.get('total') != total:
            return None, {'reason': 'noncontiguous pages or changing total', 'ref': ref}
        if index < len(pages) - 1 and doc.get('has_more') is not True:
            return None, {'reason': 'extra page after a complete response', 'ref': ref}
        rows.extend(doc['data'])
        refs.append(ref)
        offset += len(doc['data'])
    complete = pages[-1][1].get('has_more') is False and isinstance(total, int) and total == len(rows)
    if not complete:
        return None, {'reason': 'pagination is incomplete', 'refs': refs, 'total': total, 'rows_read': len(rows)}
    return rows, {'refs': refs, 'pagination_complete': True, 'total': total}


def compare_case(name, spec, reads):
    candidates = list(query_objects(reads).get(name, {}).values())
    if not candidates:
        return {'status': 'unavailable', 'reason': 'expected active query absent from captured objects'}
    if len(candidates) != 1:
        return {'status': 'fail', 'reason': 'multiple active queries have the expected name', 'ids': [q['id'] for q in candidates]}
    query = candidates[0]
    views = query['views']
    if spec.get('view_name'):
        views = [v for v in views if v.get('name') == spec['view_name']]
        if len(views) != 1:
            return {'status': 'unavailable', 'reason': 'expected named view not uniquely found'}
    if not views:
        return {'status': 'unavailable', 'reason': 'query document has no stored view'}
    view = views[0]
    rows, evidence = collect_pages(reads, query['id'], view['id'])
    base = {'query_id': query['id'], 'query_ref': query['ref'], 'view_id': view['id'],
            'view_name': view.get('name'), 'selection': 'named view' if spec.get('view_name') else 'first stored view', **evidence}
    if rows is None:
        return {'status': 'unavailable', **base}
    names = [row.get('name') for row in rows]
    ids = [row.get('id') for row in rows]
    expected = spec.get('expected_names', [])
    if spec.get('ordered_groups'):
        pos, matches = 0, True
        for group in spec['ordered_groups']:
            matches &= Counter(names[pos:pos + len(group)]) == Counter(group)
            pos += len(group)
        matches &= pos == len(names)
    elif spec.get('order_sensitive'):
        matches = names == expected
    else:
        matches = Counter(names) == Counter(expected)
    unique_ids = None not in ids and len(ids) == len(set(ids))
    return {'status': 'pass' if matches and unique_ids else 'fail', **base,
            'actual_names': names, 'expected_names': expected,
            'order_sensitive': bool(spec.get('order_sensitive') or spec.get('ordered_groups')),
            'ordered_groups': spec.get('ordered_groups'), 'unique_object_ids': unique_ids,
            'limitation': spec.get('reason')}


def check_run(run, catalog):
    manifest = load(run / 'run.json') or {}
    scenario_id = manifest.get('scenario_id')
    result = {'run_id': run.name, 'scenario_id': scenario_id, 'scope': 'query rows only'}
    snapshot = load(run / 'final-state.json')
    if not isinstance(snapshot, dict) or not isinstance(snapshot.get('reads'), list):
        return {**result, 'status': 'unavailable', 'reason': 'missing or invalid final-state.json'}
    case = catalog.get('scenarios', {}).get(scenario_id)
    if not case or not case.get('queries'):
        return {**result, 'status': 'unavailable', 'reason': 'no reviewed expectations'}
    checks = {name: compare_case(name, spec, snapshot['reads']) for name, spec in case['queries'].items()}
    statuses = {check['status'] for check in checks.values()}
    overall = 'fail' if 'fail' in statuses else ('pass' if statuses == {'pass'} else 'incomplete')
    return {**result, 'status': overall, 'queries': checks}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runs', type=Path, default=DEFAULT_RUNS)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('run', nargs='*')
    args = parser.parse_args()
    catalog = load(EXPECTATIONS)
    if not catalog or set(catalog.get('scenarios', {})) != {f'MCP-{i:02d}' for i in range(1, 31)}:
        raise SystemExit('Expectations must contain exactly MCP-01 through MCP-30')
    dirs = [args.runs / name for name in args.run] if args.run else sorted(p for p in args.runs.iterdir() if p.is_dir() and (p / 'run.json').exists())
    args.output.mkdir(parents=True, exist_ok=True)
    results = [check_run(run, catalog) for run in dirs]
    for result in results:
        (args.output / (result['run_id'] + '.json')).write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    (args.output / 'summary.json').write_text(json.dumps({'results': results, 'expectations': str(EXPECTATIONS)}, ensure_ascii=False, indent=2) + '\n')
    print(f'Checked {len(results)} run(s); query checks are separate from full scenario outcomes.')


if __name__ == '__main__':
    main()
