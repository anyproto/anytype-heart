"""Offline MCP-30 fixture checks over evaluator-captured, unmodified API reads.

This module makes no API calls, writes no receipts, and changes no runner state.
It validates server-reported identities, not signatures or receipt authenticity.
The caller must establish capture provenance, capture setup before peer writes,
freeze peer writes during each page walk, and capture arrival after turn 5.

Call records have the existing {tool, arguments, result} evidence shape. A
history capture contains space_id, chat_id, peer_id, current_member (one
get-member-me call), setup_pages (pre-seed chat reads), and pages (post-seed
chat reads). peer_member is an optional independently captured peer /me call.
Page walks start unbounded at the newest end, use reactions=full, and follow
next_before through has_more=false. One limit=100 page is complete evidence;
29 live messages still exercise more than one default (25-message) page.

validate_peer_arrival takes an arrival capture with the same identity fields
and pages, plus the original history capture. Prior history is revalidated;
changes to previously observed content/read state are actor-state discrepancies,
not failures to supply Update 28. These observations alone do not establish who
caused a discrepancy: attribute it using the actor trace before grading.

All supplied raw evidence is deep-copied into the result without normalization.
No missing setup content is inferred, repaired, or silently supplied.
"""
from copy import deepcopy
import html
import json
import re

from tool_names import capability


def _name(name):
    return capability(name)


def _issue(out, category, code, **details):
    out[category].append({"code": code, **details})


def _result(kind, capture):
    return {"kind": kind, "evidence": deepcopy(capture), "evidence_errors": [],
            "fixture_errors": [], "actor_state_discrepancies": [], "facts": {}}


def _finish(out):
    out["fixture_valid"] = not out["evidence_errors"] and not out["fixture_errors"]
    # Setup failure blocks a history fixture; later actor mistakes must not
    # prevent delivery of the mark-read turn or become a fixture failure.
    out["ready"] = out["fixture_valid"] and (
        out["kind"] == "peer_arrival" or not out["actor_state_discrepancies"])
    out["status"] = ("unverified" if out["evidence_errors"] else
                     "fixture_invalid" if out["fixture_errors"] else
                     "actor_state_discrepancy" if out["actor_state_discrepancies"] else "validated")
    return out


def _document(call, tool, space, out, label, field):
    if not isinstance(call, dict) or not isinstance(call.get("arguments"), dict):
        _issue(out, "evidence_errors", "invalid_call", at=label)
        return None
    if (_name(str(call.get("tool", ""))) != tool or
            call["arguments"].get("space_id") != space):
        _issue(out, "evidence_errors", "wrong_call_scope", at=label)
        return None
    raw = call.get("result")
    if (not isinstance(raw, dict) or raw.get("isError") or raw.get("error") or
            call.get("status") in {"failed", "error"} or call.get("error")):
        _issue(out, "evidence_errors", "failed_read", at=label)
        return None
    docs = []
    content = raw.get("content", [])
    if not isinstance(content, list):
        _issue(out, "evidence_errors", "invalid_content_blocks", at=label)
        return None
    for part in content:
        if isinstance(part, dict) and part.get("type") == "text":
            try:
                doc = json.loads(part.get("text", ""))
            except (TypeError, ValueError):
                continue
            if isinstance(doc, dict):
                docs.append(doc)
    if any(d.get("error") or (isinstance(d.get("status"), int) and d["status"] >= 400)
           for d in docs):
        _issue(out, "evidence_errors", "failed_read_payload", at=label)
        return None
    found = [d for d in docs if field in d]
    if len(found) != 1:
        _issue(out, "evidence_errors", "missing_or_ambiguous_payload", at=label)
        return None
    return found[0]


def _identities(capture, out):
    space, chat, peer = (capture.get(k) for k in ("space_id", "chat_id", "peer_id"))
    if not all(isinstance(v, str) and v for v in (space, chat, peer)):
        _issue(out, "evidence_errors", "missing_scope_or_peer_id")
        return None
    current = _document(capture.get("current_member"), "get_member_me", space, out,
                        "current_member", "id")
    if not current or not isinstance(current.get("id"), str) or not current["id"]:
        _issue(out, "evidence_errors", "missing_current_member")
        return None
    if current["id"] == peer:
        _issue(out, "fixture_errors", "peer_is_current_member")
    if "peer_member" in capture:
        other = _document(capture["peer_member"], "get_member_me", space, out, "peer_member", "id")
        if other and other.get("id") != peer:
            _issue(out, "fixture_errors", "peer_member_does_not_match")
        if (other and current.get("identity") and other.get("identity") and
                current["identity"] == other["identity"]):
            _issue(out, "fixture_errors", "peer_has_same_account_identity")
    out["facts"].update(space_id=space, chat_id=chat, peer_id=peer, current_member_id=current["id"])
    return current["id"]


def _walk(calls, capture, out, label):
    """Validate a complete backward cursor walk without sorting away defects."""
    if not isinstance(calls, list) or not calls:
        _issue(out, "evidence_errors", "missing_pages", at=label)
        return [], {}
    groups, states, counts = [], [], []
    expected_before = None
    ids, orders = set(), set()
    for index, call in enumerate(calls):
        loc = f"{label}[{index}]"
        doc = _document(call, "get_chat_messages", capture.get("space_id"), out, loc, "messages")
        if doc is None:
            continue
        args = call["arguments"]
        if (args.get("chat_id") != capture.get("chat_id") or args.get("after") or args.get("offset") or
                args.get("before") != expected_before or args.get("reactions") != "full"):
            _issue(out, "evidence_errors", "wrong_page_scope_or_cursor", at=loc)
        rows, more, state = doc.get("messages"), doc.get("has_more"), doc.get("state")
        if (not isinstance(rows, list) or type(more) is not bool or
                not isinstance(state, dict) or not isinstance(state.get("last_state_id"), str) or
                not state["last_state_id"]):
            _issue(out, "evidence_errors", "invalid_page_shape", at=loc)
            continue
        states.append(state)
        count = doc.get("message_count")
        if type(count) is not int or count < 0:
            _issue(out, "evidence_errors", "invalid_lifetime_count", at=loc)
        else:
            counts.append(count)
        page = []
        for row in rows:
            if not isinstance(row, dict) or not all(isinstance(row.get(k), str) and row[k]
                                                    for k in ("id", "order", "author_id")):
                _issue(out, "evidence_errors", "invalid_message_identity", at=loc)
                continue
            if not isinstance(row.get("text"), str):
                _issue(out, "evidence_errors", "invalid_message_text", at=loc)
                continue
            if row["id"] in ids or row["order"] in orders:
                _issue(out, "evidence_errors", "duplicate_message_or_order", at=loc, message_id=row["id"])
            ids.add(row["id"])
            orders.add(row["order"])
            if page and page[-1]["order"] >= row["order"]:
                _issue(out, "evidence_errors", "page_not_chronological", at=loc)
            if expected_before is not None and row["order"] >= expected_before:
                _issue(out, "evidence_errors", "page_crosses_exclusive_cursor", at=loc)
            page.append(row)
        groups.append(page)
        if more:
            cursor = doc.get("next_before")
            if not page or cursor != page[0]["order"] or index == len(calls) - 1:
                _issue(out, "evidence_errors", "incomplete_or_invalid_pagination", at=loc)
            expected_before = cursor
        elif index != len(calls) - 1:
            _issue(out, "evidence_errors", "pages_after_end", at=loc)
        if doc.get("next_after"):
            _issue(out, "evidence_errors", "unexpected_forward_cursor", at=loc)
    messages = [row for page in reversed(groups) for row in page]
    if states and any(state != states[0] for state in states[1:]):
        _issue(out, "evidence_errors", "state_changed_during_page_walk", at=label)
    if counts and (len(set(counts)) != 1 or counts[0] < len(messages)):
        _issue(out, "evidence_errors", "inconsistent_lifetime_count", at=label)
    return messages, states[0] if states else {}


_MENTION = re.compile(r'<mention object_id="([^"]+)">([^<>]+)</mention>')
_UPDATE = re.compile(r"^Update (\d{2})(?:\s|$)")


def mention_targets(text):
    """Recognize canonical served mention tags; ignore escaped/code literals.

    The server renderer emits object_id (not objectId): chat_test.go's
    'mention marks render as §8 mention tags'. This is deliberately a narrow
    fixture reader, not a replacement for the complete AnyBlock codec.
    """
    targets, pos = [], 0
    while pos < len(text):
        if text[pos] == "\\":
            pos += 2
            continue
        if text[pos] == "`":
            run = re.match(r"`+", text[pos:]).group()
            closing = re.search(r"(?<!`)" + re.escape(run) + r"(?!`)", text[pos + len(run):])
            if closing:
                pos += len(run) + closing.end()
                continue
            pos += len(run)
            continue
        match = _MENTION.match(text, pos)
        if match and match[2].strip() and not match[2].endswith("\\"):
            targets.append(html.unescape(match[1]))
            pos = match.end()
        else:
            pos += 1
    return targets


def _setup(messages, current, out):
    announcements = [m for m in messages if m["author_id"] == current and m["text"] == "Morning handover starts"]
    replies = [m for m in messages if m["author_id"] == current and m["text"] == "Checking the logs"]
    if len(announcements) != 1 or len(replies) != 1 or len(messages) != 2:
        _issue(out, "actor_state_discrepancies", "actor_setup_messages_missing_or_ambiguous")
        return None
    announcement, reply = announcements[0], replies[0]
    if (messages[0]["id"] != announcement["id"] or reply.get("reply_to") != announcement["id"] or
            not announcement.get("attachments")):
        _issue(out, "actor_state_discrepancies", "actor_setup_attachment_reply_or_order_wrong")
    return announcement


def _compare_before(before, after, out, *, reactions=False):
    by_id = {m["id"]: m for m in after}
    for old in before:
        new = by_id.get(old["id"])
        if new is None:
            _issue(out, "actor_state_discrepancies", "previous_message_missing", message_id=old["id"])
            continue
        fields = ("order", "author_id", "text", "blocks_text", "reply_to", "attachments")
        if reactions:
            fields += ("reactions", "reacted_by")
        changed = [k for k in fields
                   if old.get(k) != new.get(k)]
        if changed:
            _issue(out, "actor_state_discrepancies", "previous_message_changed", message_id=old["id"], fields=changed)


def validate_peer_history(capture):
    out = _result("peer_history", capture)
    current = _identities(capture, out)
    if current is None:
        return _finish(out)
    setup, _ = _walk(capture.get("setup_pages"), capture, out, "setup_pages")
    messages, state = _walk(capture.get("pages"), capture, out, "pages")
    announcement = _setup(setup, current, out)
    _compare_before(setup, messages, out)
    setup_ids = {m["id"] for m in setup}
    if any(m["author_id"] == current and m["id"] not in setup_ids for m in messages):
        _issue(out, "actor_state_discrepancies", "additional_actor_messages_after_setup")
    peers = [m for m in messages if m["author_id"] == capture["peer_id"]]
    numbers = [int(match[1]) if (match := _UPDATE.match(m["text"])) else None for m in peers]
    if numbers != list(range(1, 28)):
        _issue(out, "fixture_errors", "peer_updates_not_exactly_01_through_27", observed=numbers)
    if len(messages) <= 25:
        _issue(out, "fixture_errors", "history_does_not_span_default_pages")
    if setup and peers and peers[0]["order"] <= setup[-1]["order"]:
        _issue(out, "fixture_errors", "peer_history_precedes_actor_setup")
    if any(m["author_id"] not in {current, capture["peer_id"]} for m in messages):
        _issue(out, "fixture_errors", "unexpected_third_author")
    mentions = [m for m in peers if current in mention_targets(m["text"])]
    unread_mentions = [m for m in peers[-5:] if current in mention_targets(m["text"])]
    if not mentions:
        _issue(out, "fixture_errors", "current_member_not_mentioned")
    if not unread_mentions:
        _issue(out, "fixture_errors", "no_unread_peer_mention")
    if len(peers) >= 5:
        expected = {"unread_messages": 5, "oldest_unread_order": peers[-5]["order"],
                    "unread_mentions": len(unread_mentions)}
        if unread_mentions:
            expected["oldest_unread_mention_order"] = unread_mentions[0]["order"]
        for key, value in expected.items():
            if type(state.get(key)) is not type(value) or state[key] != value:
                _issue(out, "fixture_errors", "initial_unread_state_wrong", field=key, expected=value,
                       observed=state.get(key))
    # Do not label missing original actor setup as a fixture implementation error.
    if announcement:
        live = next((m for m in messages if m["id"] == announcement["id"]), None)
        if live:
            reacted_by, reactions = live.get("reacted_by", {}), live.get("reactions", {})
            if not isinstance(reacted_by, dict) or not isinstance(reactions, dict):
                _issue(out, "evidence_errors", "invalid_reaction_maps")
                reacted_by, reactions = {}, {}
            found = [emoji for emoji, members in reacted_by.items()
                     if isinstance(members, list) and all(isinstance(m, str) for m in members) and
                     capture["peer_id"] in members and type(reactions.get(emoji)) is int and
                     reactions[emoji] == len(set(members)) and len(members) == len(set(members))]
            if not found:
                _issue(out, "fixture_errors", "peer_reaction_on_original_announcement_missing")
            # This is the reaction CHANGE order (chathandler.onReactionAdded),
            # not the announcement message's order or message ID.
            if not isinstance(state.get("unread_reaction_order"), str) or not state["unread_reaction_order"]:
                _issue(out, "fixture_errors", "initial_unread_reaction_missing")
    out["facts"].update(messages=deepcopy(messages), state=deepcopy(state),
                        peer_message_ids=[m["id"] for m in peers],
                        summary_up_to=messages[-1]["order"] if messages else None,
                        summary_last_state_id=state.get("last_state_id"))
    return _finish(out)


def validate_peer_arrival(capture, history_capture):
    out = _result("peer_arrival", capture)
    out["history_evidence"] = deepcopy(history_capture)
    history = validate_peer_history(history_capture)
    out["history_validation"] = {k: v for k, v in history.items() if k != "evidence"}
    if not history["ready"]:
        _issue(out, "evidence_errors", "prior_history_not_validated")
        return _finish(out)
    current = _identities(capture, out)
    if current is None:
        return _finish(out)
    before = history["facts"]
    if any(capture.get(k) != before[k] for k in ("space_id", "chat_id", "peer_id")) or current != before["current_member_id"]:
        _issue(out, "fixture_errors", "arrival_identity_or_chat_changed")
    messages, state = _walk(capture.get("pages"), capture, out, "pages")
    arrivals = [m for m in messages if m["author_id"] == capture["peer_id"] and
                (match := _UPDATE.match(m["text"])) and int(match[1]) == 28]
    if len(arrivals) != 1:
        _issue(out, "fixture_errors", "unique_peer_update_28_missing")
    else:
        arrival = arrivals[0]
        if (arrival["id"] in {m["id"] for m in before["messages"]} or
                arrival["order"] <= before["summary_up_to"]):
            _issue(out, "fixture_errors", "update_28_not_after_history")
        out["facts"]["arrival"] = deepcopy(arrival)
    if state.get("last_state_id", "") <= before["summary_last_state_id"]:
        _issue(out, "evidence_errors", "arrival_state_did_not_advance")
    _compare_before(before["messages"], messages, out, reactions=True)
    old_ids = {m["id"] for m in before["messages"]}
    if any(m["author_id"] == current and m["id"] not in old_ids for m in messages):
        _issue(out, "actor_state_discrepancies", "additional_actor_messages_during_summary")
    if any(m["author_id"] == capture["peer_id"] and m["id"] not in old_ids and m not in arrivals for m in messages):
        _issue(out, "fixture_errors", "unexpected_additional_peer_arrival")
    expected_state = {k: v for k, v in before["state"].items() if k != "last_state_id"}
    expected_state["unread_messages"] = 6
    for key, value in expected_state.items():
        if type(state.get(key)) is not type(value) or state[key] != value:
            _issue(out, "actor_state_discrepancies", "read_state_changed_during_summary", field=key,
                   expected=value, observed=state.get(key))
    out["facts"].update(state=deepcopy(state), messages=deepcopy(messages),
                        summary_up_to=before["summary_up_to"],
                        summary_last_state_id=before["summary_last_state_id"])
    return _finish(out)
