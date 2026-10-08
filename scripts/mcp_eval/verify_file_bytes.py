#!/usr/bin/env python3
"""Verify MCP-22 downloaded bytes locally, without making any API/model calls.

Usage: python3 scripts/mcp_eval/verify_file_bytes.py --runs PATH_TO_CAMPAIGN
   or: python3 scripts/mcp_eval/verify_file_bytes.py PATH_TO_COMPLETED_RUN [...]

Only paired successful upstream upload/download receipts authorize file reads.
The sole output is each completed run's file-byte-verification.json. Original
traces and snapshots are never changed. A pass covers bytes, not the scenario.
"""
import argparse
from datetime import datetime, timezone
import errno
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import tempfile

from tool_names import capability


ROOT = Path(__file__).resolve().parents[2]
FIXTURES = ROOT / "docs/evals/anytype-mcp-v2/fixtures"
NAMES = ("logo.png", "usage.txt")
COMPLETE = "conversation_completed_pending_review"
MAX_DOWNLOAD_BYTES = 16 * 1024 * 1024
TOOLS = {"upload_file": "upload", "download_file": "download"}


def digest_file(path):
    digest = hashlib.sha256()
    size = 0
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(65536), b""):
            digest.update(block)
            size += len(block)
    return {"bytes": size, "sha256": digest.hexdigest()}


def documents(result):
    if not isinstance(result, dict) or result.get("isError"):
        return []
    docs = []
    for part in result.get("content", []):
        if not isinstance(part, dict) or part.get("type") != "text":
            continue
        try:
            doc = json.loads(part.get("text", ""))
        except (TypeError, ValueError):
            continue
        if isinstance(doc, dict):
            docs.append(doc)
    return docs


def successful_documents(payload):
    if not isinstance(payload, dict) or payload.get("error"):
        return []
    docs = documents(payload.get("result"))
    if any("code" in doc or "error" in doc or
           isinstance(doc.get("status"), int) and doc["status"] >= 400 for doc in docs):
        return []
    return [doc for doc in docs if "request_metadata" not in doc]


def wire_receipts(path):
    """Do not accept model prose, tool arguments, or unpaired responses as paths."""
    pending, receipts, issues = {}, [], []
    if not path.exists():
        return [], [{"status": "unavailable", "reason": "mcp-wire.jsonl is absent"}]
    with path.open() as stream:
        for line_number, line in enumerate(stream, 1):
            ref = f"mcp-wire.jsonl:{line_number}"
            try:
                event = json.loads(line)
                payload = event["payload"]
                direction = event["direction"]
                if not isinstance(payload, dict):
                    raise ValueError("payload is not an object")
            except (TypeError, ValueError, KeyError):
                # Pairing across damaged evidence is unsafe.
                pending.clear()
                issues.append({"status": "invalid", "ref": ref, "reason": "invalid wire event"})
                continue
            call_id = payload.get("id")
            if not isinstance(call_id, (str, int)) or isinstance(call_id, bool):
                continue
            if direction == "to_upstream":
                # Each turn restarts the MCP process and reuses numeric IDs.
                pending.pop(call_id, None)
                if payload.get("method") == "initialize":
                    pending.clear()
                params = payload.get("params", {})
                if payload.get("method") == "tools/call" and isinstance(params, dict):
                    kind = TOOLS.get(capability(params.get("name")))
                    if kind:
                        pending[call_id] = (kind, params.get("arguments", {}), ref)
            elif direction == "from_upstream" and call_id in pending:
                kind, arguments, request_ref = pending.pop(call_id)
                docs = successful_documents(payload)
                receipts.append({"kind": kind, "arguments": arguments,
                                 "documents": docs, "successful": bool(docs),
                                 "request_ref": request_ref, "response_ref": ref})
    for kind, _, ref in pending.values():
        issues.append({"status": "unavailable", "ref": ref,
                       "reason": f"{kind} request has no upstream response"})
    return receipts, issues


def allowed_temp_roots():
    roots = {Path("/tmp").resolve(), Path("/private/tmp").resolve()}
    native = Path(tempfile.gettempdir()).resolve()
    if re.fullmatch(r"/private/var/folders/[A-Za-z0-9_]{2}/[A-Za-z0-9_]+/T", str(native)):
        roots.add(native)
    return roots


def hash_download(raw_path):
    """Open only direct regular files in an MCP-created system-temp directory."""
    if not isinstance(raw_path, str) or not raw_path or "\x00" in raw_path:
        return {"status": "invalid", "reason": "download receipt lacks a valid path"}
    path = Path(raw_path)
    if (not path.is_absolute() or ".." in path.parts or
            not re.fullmatch(r"anytype-mcp-download-[A-Za-z0-9]{6}", path.parent.name) or
            not re.fullmatch(r"[A-Za-z0-9 ._-]{1,180}", path.name) or
            path.name in {".", ".."}):
        return {"status": "invalid", "reason": "path is outside the MCP download layout"}
    root = path.parent.parent.resolve()
    if root not in allowed_temp_roots():
        return {"status": "invalid", "reason": "path is outside permitted system temp roots"}
    descriptors = []
    try:
        # dir_fd plus O_NOFOLLOW protects both the generated directory and file
        # against symlink substitution between validation and open.
        root_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        descriptors.append(root_fd)
        directory_fd = os.open(path.parent.name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                               dir_fd=root_fd)
        descriptors.append(directory_fd)
        file_fd = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK,
                          dir_fd=directory_fd)
        descriptors.append(file_fd)
        before = os.fstat(file_fd)
        if not stat.S_ISREG(before.st_mode) or before.st_nlink != 1:
            return {"status": "invalid", "reason": "download is not a single-link regular file"}
        if before.st_size > MAX_DOWNLOAD_BYTES:
            return {"status": "invalid", "reason": "download exceeds the local inspection limit"}
        digest, size = hashlib.sha256(), 0
        while block := os.read(file_fd, 65536):
            size += len(block)
            if size > MAX_DOWNLOAD_BYTES:
                return {"status": "invalid", "reason": "download grew past the local inspection limit"}
            digest.update(block)
        after = os.fstat(file_fd)
        if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (
                after.st_size, after.st_mtime_ns, after.st_ctime_ns) or size != after.st_size:
            return {"status": "invalid", "reason": "download changed during inspection"}
        return {"status": "hashed", "bytes": size, "sha256": digest.hexdigest(),
                "resolved_path": str(root / path.parent.name / path.name)}
    except FileNotFoundError:
        return {"status": "path_missing", "reason": "returned download path no longer exists"}
    except OSError as exc:
        invalid = exc.errno in {errno.ELOOP, errno.ENOTDIR}
        return {"status": "invalid" if invalid else "unavailable",
                "reason": "unsafe download path" if invalid else "download could not be read",
                "errno": exc.errno}
    finally:
        for descriptor in reversed(descriptors):
            os.close(descriptor)


def aggregate(statuses):
    for status in ("mismatch", "invalid", "path_missing", "unavailable"):
        if status in statuses:
            return status
    return "pass" if statuses and set(statuses) == {"pass"} else "unavailable"


def check_run(run_dir, fixture_dir=FIXTURES):
    manifest = json.loads((run_dir / "run.json").read_text())
    if manifest.get("scenario_id") != "MCP-22" or manifest.get("status") != COMPLETE:
        raise ValueError("Requires a completed MCP-22 conversation")
    spaces = manifest.get("owned_spaces", [])
    if len(spaces) != 1 or not spaces[0].get("full_id"):
        raise ValueError("Requires exactly one recorded owned space")
    scope = {spaces[0][key] for key in ("full_id", "compact_id") if spaces[0].get(key)}
    source_manifest = json.loads((fixture_dir / "manifest.json").read_text())
    output = {"version": 1, "scenario_id": "MCP-22", "run_id": run_dir.name,
              "recorded_at": datetime.now(timezone.utc).isoformat(),
              "purpose": "Independent local SHA-256 checks of recorded original downloads",
              "scope": "File bytes only; does not grade attachments, icons, or the complete scenario",
              "receipt_source": "Paired successful from_upstream responses in mcp-wire.jsonl",
              "files": {}, "downloads": [], "ignored_receipts": [], "evidence": {}}
    for name in ("run.json", "mcp-wire.jsonl", "final-state.json"):
        path = run_dir / name
        if path.exists():
            output["evidence"][name] = digest_file(path)
    output["evidence"]["fixture_manifest"] = {
        "path": str(fixture_dir / "manifest.json"), **digest_file(fixture_dir / "manifest.json")}
    for name in NAMES:
        expected = source_manifest.get(name, {})
        entry = {"source_path": str(fixture_dir / name), "expected": expected}
        output["files"][name] = entry
        try:
            actual = digest_file(fixture_dir / name)
            entry["source_actual"] = actual
            entry["fixture_status"] = "pass" if actual == expected else "invalid"
        except OSError:
            entry["fixture_status"] = "unavailable"
        entry["status"] = "unavailable"
    receipts, issues = wire_receipts(run_dir / "mcp-wire.jsonl")
    output["receipt_issues"] = issues
    uploaded = {}
    for receipt in receipts:
        args = receipt["arguments"]
        if (not isinstance(args, dict) or args.get("space_id") not in scope or
                args.get("dry_run") or not receipt["successful"]):
            output["ignored_receipts"].append({"ref": receipt["response_ref"],
                                               "reason": "unsuccessful, dry-run, or out-of-scope receipt"})
            continue
        if receipt["kind"] == "upload":
            # Exact supplied fixture path links a file ID to an expectation.
            # Neither the receipt's filename nor its size establishes identity.
            names = [name for name in NAMES if args.get("file") == str(fixture_dir / name)]
            ids = [doc["id"] for doc in receipt["documents"] if isinstance(doc.get("id"), str)]
            if len(names) == len(ids) == 1:
                uploaded.setdefault(ids[0], set()).add(names[0])
                output["files"][names[0]].setdefault("upload_receipts", []).append({
                    "file_id": ids[0], "ref": receipt["response_ref"]})
            continue
        names = uploaded.get(args.get("file_id"), set())
        if len(names) != 1 or args.get("width") not in (None, 0):
            output["ignored_receipts"].append({"ref": receipt["response_ref"],
                                               "reason": "not an original download of a proven fixture upload"})
            continue
        name = next(iter(names))
        entry = output["files"][name]
        docs = [doc for doc in receipt["documents"] if "path" in doc]
        check = {"fixture": name, "file_id": args["file_id"],
                 "request_ref": receipt["request_ref"], "response_ref": receipt["response_ref"]}
        if len(docs) != 1:
            check.update(status="invalid", reason="successful download has no unique path document")
        elif entry["fixture_status"] != "pass":
            check.update(status=entry["fixture_status"], reason="source fixture does not verify against manifest")
        else:
            check["returned_path"] = docs[0]["path"]
            check["receipt_metadata"] = {key: docs[0].get(key) for key in ("filename", "size", "media_type")}
            check.update(hash_download(docs[0]["path"]))
            if check["status"] == "hashed":
                check["sha256_matches"] = check["sha256"] == entry["expected"]["sha256"]
                check["bytes_match"] = check["bytes"] == entry["expected"]["bytes"]
                check["status"] = "pass" if check["sha256_matches"] and check["bytes_match"] else "mismatch"
        output["downloads"].append(check)
    for name, entry in output["files"].items():
        statuses = [check["status"] for check in output["downloads"] if check["fixture"] == name]
        entry["status"] = aggregate(statuses) if entry["fixture_status"] == "pass" else entry["fixture_status"]
        if not statuses:
            entry["reason"] = "No successful original download linked to this fixture upload"
    output["status"] = aggregate([entry["status"] for entry in output["files"].values()] +
                                  [issue["status"] for issue in issues])
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_dir", nargs="*", type=Path)
    parser.add_argument("--runs", type=Path, help="Recursively discover completed MCP-22 runs")
    args = parser.parse_args()
    if bool(args.run_dir) == bool(args.runs):
        parser.error("Supply explicit run directories or --runs, but not both")
    runs = args.run_dir
    if args.runs:
        runs = []
        for path in sorted(args.runs.rglob("run.json")):
            manifest = json.loads(path.read_text())
            if manifest.get("scenario_id") == "MCP-22" and manifest.get("status") == COMPLETE:
                runs.append(path.parent)
    for run_dir in runs:
        result = check_run(run_dir)
        output = run_dir / "file-byte-verification.json"
        temporary = output.with_suffix(".tmp")
        temporary.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
        temporary.replace(output)
        print(f"{run_dir.name}: {result['status']} — {output}")
    print(f"Verified {len(runs)} completed MCP-22 run(s); no API/model calls made.")


if __name__ == "__main__":
    main()
