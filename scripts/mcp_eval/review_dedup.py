"""Represent identical snapshot reads by explicit source/line aliases.

Original text, hashes, line numbers, and every observation header stay intact.
Only complete, equal JSON values (tool, arguments, result and metadata) alias.
"""
import hashlib
import json
import re


def read_ranges(text):
    document = json.loads(text)
    if not isinstance(document, dict) or not isinstance(document.get("reads"), list):
        return []
    marker = re.search(r'^  "reads": \[\s*\n', text, re.MULTILINE)
    if marker is None:
        return []
    position = marker.end()
    decoder = json.JSONDecoder()
    result = []
    for expected in document["reads"]:
        while position < len(text) and text[position].isspace():
            position += 1
        value, end = decoder.raw_decode(text, position)
        if value != expected or not isinstance(value, dict) or not {"tool", "arguments", "result"} <= set(value):
            return []
        start_line = text.count("\n", 0, position) + 1
        end_line = text.count("\n", 0, end) + 1
        if start_line == end_line:
            return []  # Do not replace lines that might contain other values.
        result.append((start_line, end_line, value))
        position = end
        while position < len(text) and text[position].isspace():
            position += 1
        if text[position:position + 1] == ",":
            position += 1
    while position < len(text) and text[position].isspace():
        position += 1
    if text[position:position + 1] != "]":
        return []
    return result


def alias_snapshot_reads(sources):
    seen = {}
    for name, source in sources.items():
        if name != "run/final-state.json" and not name.startswith("run/snapshots/"):
            continue
        selected = set(source["included_lines"])
        aliases = []
        for start, end, value in read_ranges(source["text"]):
            if not set(range(start, end + 1)) <= selected:
                continue
            signature = json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
            if signature in seen:
                aliases.append({"line_start": start, "line_end": end,
                                "identical_to": seen[signature],
                                "original_json_sha256": hashlib.sha256(signature.encode()).hexdigest()})
            else:
                seen[signature] = {"source": name, "line_start": start, "line_end": end}
        if aliases:
            source["identical_read_aliases"] = aliases
