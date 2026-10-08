from copy import deepcopy
import json
import unittest

from peer_fixture_validation import mention_targets, validate_peer_arrival, validate_peer_history


SPACE, CHAT, ACTOR, PEER = "owned-space", "chat", "member-actor", "member-peer"


def call(tool, args, doc):
    return {"tool": "API-" + tool, "arguments": {"space_id": SPACE, **args},
            "result": {"content": [{"type": "text", "text": json.dumps(doc)},
                                   {"type": "text", "text": json.dumps({"request_metadata": {"trace": "retained"}})}]}}


def rows():
    messages = [{"id": "announcement", "order": "o000", "author_id": ACTOR,
                 "text": "Morning handover starts", "attachments": [{"id": "case", "type": "link"}]},
                {"id": "reply", "order": "o001", "author_id": ACTOR,
                 "text": "Checking the logs", "reply_to": "announcement"}]
    for n in range(1, 28):
        messages.append({"id": f"peer-{n}", "order": f"o{n + 1:03}", "author_id": PEER,
                         "text": f"Update {n:02}"})
    messages[-2]["text"] += f' <mention object_id="{ACTOR}">Current member</mention>'
    messages[0].update(reacted_by={"👍": [PEER]}, reactions={"👍": 1})
    return messages


def state(unread=5, last="state027"):
    return {"unread_messages": unread, "oldest_unread_order": "o024", "unread_mentions": 1,
            "oldest_unread_mention_order": "o027", "unread_reaction_order": "reaction-change-order", "last_state_id": last}


def walk(messages, chat_state, limit=25, lifetime=None):
    calls, end, before = [], len(messages), None
    while True:
        start = max(0, end - limit)
        page = deepcopy(messages[start:end])
        args = {"chat_id": CHAT, "reactions": "full", "limit": limit}
        if before is not None:
            args["before"] = before
        doc = {"messages": page, "state": deepcopy(chat_state), "message_count": lifetime or len(messages),
               "has_more": start > 0}
        if start:
            doc["next_before"] = page[0]["order"]
        calls.append(call("get-chat-messages", args, doc))
        if not start:
            return calls
        end, before = start, page[0]["order"]


def history():
    messages = rows()
    setup = deepcopy(messages[:2])
    setup[0].pop("reacted_by")
    setup[0].pop("reactions")
    return {"space_id": SPACE, "chat_id": CHAT, "peer_id": PEER,
            "current_member": call("get-member-me", {}, {"id": ACTOR, "identity": "account-actor"}),
            "peer_member": call("get-member-me", {}, {"id": PEER, "identity": "account-peer"}),
            "setup_pages": walk(setup, {"unread_messages": 0, "unread_mentions": 0, "last_state_id": "state000"}),
            "pages": walk(messages, state())}


def arrival():
    capture = history()
    capture.pop("setup_pages")
    messages = rows() + [{"id": "peer-28", "order": "o029", "author_id": PEER, "text": "Update 28"}]
    capture["pages"] = walk(messages, state(6, "state028"))
    return capture


def document(call):
    return json.loads(call["result"]["content"][0]["text"])


def change(call, mutate):
    doc = document(call)
    mutate(doc)
    call["result"]["content"][0]["text"] = json.dumps(doc)


def codes(result, category):
    return {x["code"] for x in result[category]}


class PeerFixtureTests(unittest.TestCase):
    def test_complete_backward_walk_and_raw_evidence_are_preserved(self):
        capture = history()
        original = deepcopy(capture)
        result = validate_peer_history(capture)
        self.assertEqual(result["status"], "validated", result)
        self.assertTrue(result["ready"])
        self.assertEqual(result["facts"]["peer_message_ids"], [f"peer-{n}" for n in range(1, 28)])
        self.assertEqual(result["facts"]["summary_up_to"], "o028")
        self.assertEqual(capture, original)
        self.assertEqual(result["evidence"], original)
        capture["pages"].clear()
        self.assertEqual(result["evidence"], original)

    def test_single_complete_large_page_is_valid_and_lifetime_count_is_not_live_count(self):
        capture = history()
        capture["pages"] = walk(rows(), state(), limit=100, lifetime=500)
        self.assertTrue(validate_peer_history(capture)["ready"])

    def test_requires_all_updates_in_order_not_only_update27(self):
        for messages in (rows()[:2] + rows()[-1:], rows()[:3] + rows()[4:], rows()[:2] + rows()[3:5] + rows()[2:3] + rows()[5:]):
            with self.subTest(size=len(messages)):
                capture = history()
                # Keep order IDs chronological so a mislabeled sequence is the defect.
                for index, msg in enumerate(messages):
                    msg["order"] = f"o{index:03}"
                capture["pages"] = walk(messages, state())
                self.assertIn("peer_updates_not_exactly_01_through_27",
                              codes(validate_peer_history(capture), "fixture_errors"))

    def test_rejects_current_account_as_peer_even_with_different_display_name(self):
        capture = history()
        capture["peer_id"] = ACTOR
        result = validate_peer_history(capture)
        self.assertIn("peer_is_current_member", codes(result, "fixture_errors"))
        capture = history()
        change(capture["peer_member"], lambda d: d.update(identity="account-actor"))
        self.assertIn("peer_has_same_account_identity", codes(validate_peer_history(capture), "fixture_errors"))

    def test_wrong_space_chat_cursor_and_counts_only_are_unverified(self):
        for field, value, page_index in (("space_id", "other", 0), ("chat_id", "other", 0),
                                         ("before", "made-up-cursor", 1), ("after", "o000", 0),
                                         ("reactions", "counts", 0)):
            capture = history()
            capture["pages"][page_index]["arguments"][field] = value
            self.assertFalse(validate_peer_history(capture)["fixture_valid"], field)

    def test_rejects_incomplete_pagination_and_extra_pages_after_end(self):
        capture = history()
        capture["pages"].pop()
        self.assertIn("incomplete_or_invalid_pagination", codes(validate_peer_history(capture), "evidence_errors"))
        capture = history()
        capture["pages"].append(deepcopy(capture["pages"][-1]))
        self.assertIn("pages_after_end", codes(validate_peer_history(capture), "evidence_errors"))

    def test_rejects_duplicate_message_and_order_ids(self):
        for field in ("id", "order"):
            capture = history()
            change(capture["pages"][0], lambda d: d["messages"][1].update({field: d["messages"][0][field]}))
            self.assertIn("duplicate_message_or_order", codes(validate_peer_history(capture), "evidence_errors"))

    def test_rejects_nonchronological_page_and_unfrozen_walk(self):
        capture = history()
        change(capture["pages"][0], lambda d: d["messages"].reverse())
        self.assertIn("page_not_chronological", codes(validate_peer_history(capture), "evidence_errors"))
        capture = history()
        change(capture["pages"][1], lambda d: d["state"].update(unread_messages=4))
        self.assertIn("state_changed_during_page_walk", codes(validate_peer_history(capture), "evidence_errors"))

    def test_mentions_require_real_canonical_tag_not_prose_escape_code_or_wrong_target(self):
        for text in ("Update 26 @Current member", 'Update 26 <mention objectId="member-actor">A</mention>',
                     'Update 26 \\<mention object_id="member-actor">A\\</mention>',
                     'Update 26 `<mention object_id="member-actor">A</mention>`',
                     'Update 26 <mention object_id="somebody-else">A</mention>'):
            capture, messages = history(), rows()
            messages[-2]["text"] = text
            capture["pages"] = walk(messages, state())
            self.assertIn("current_member_not_mentioned", codes(validate_peer_history(capture), "fixture_errors"), text)

    def test_mention_can_follow_code_and_entity_encoded_ids_are_decoded(self):
        text = '``literal ` text`` <mention object_id="member&amp;id">A</mention>'
        self.assertEqual(mention_targets(text), ["member&id"])

    def test_read_old_mention_does_not_supply_unread_mention(self):
        capture, messages = history(), rows()
        messages[2]["text"] += f' <mention object_id="{ACTOR}">A</mention>'
        messages[-2]["text"] = "Update 26"
        capture["pages"] = walk(messages, state())
        self.assertIn("no_unread_peer_mention", codes(validate_peer_history(capture), "fixture_errors"))

    def test_reaction_must_be_peer_membership_on_original_announcement(self):
        for mutate in (lambda m: m[0].update(reacted_by={"👍": [ACTOR]}),
                       lambda m: m[0].pop("reacted_by"),
                       lambda m: m[0].update(reactions={"👍": 0}),
                       lambda m: m[1].update(reacted_by=m[0].pop("reacted_by"))):
            capture, messages = history(), rows()
            mutate(messages)
            capture["pages"] = walk(messages, state())
            self.assertIn("peer_reaction_on_original_announcement_missing",
                          codes(validate_peer_history(capture), "fixture_errors"))

    def test_last_five_unread_needs_counter_and_exact_order_boundary(self):
        for key, value in (("unread_messages", 4), ("oldest_unread_order", "peer-23"),
                           ("unread_mentions", 0), ("oldest_unread_mention_order", "o020")):
            capture = history()
            changed = state()
            changed[key] = value
            capture["pages"] = walk(rows(), changed)
            self.assertIn("initial_unread_state_wrong", codes(validate_peer_history(capture), "fixture_errors"))

    def test_reaction_unread_order_is_a_change_order_not_message_order(self):
        capture = history()
        self.assertTrue(validate_peer_history(capture)["ready"])
        changed = state()
        changed.pop("unread_reaction_order")
        capture["pages"] = walk(rows(), changed)
        self.assertIn("initial_unread_reaction_missing", codes(validate_peer_history(capture), "fixture_errors"))

    def test_actor_setup_failure_is_separate_and_never_repaired(self):
        capture = history()
        capture["setup_pages"] = walk([], {"last_state_id": "state000", "unread_messages": 0, "unread_mentions": 0})
        result = validate_peer_history(capture)
        self.assertEqual(result["fixture_errors"], [])
        self.assertFalse(result["ready"])
        self.assertIn("actor_setup_messages_missing_or_ambiguous", codes(result, "actor_state_discrepancies"))

    def test_read_error_in_later_content_block_is_not_silently_ignored(self):
        capture = history()
        capture["pages"][0]["result"]["content"].append(
            {"type": "text", "text": json.dumps({"status": 403, "code": "forbidden"})})
        self.assertEqual(validate_peer_history(capture)["status"], "unverified")

    def test_malformed_reaction_maps_and_content_blocks_are_reported(self):
        capture, messages = history(), rows()
        messages[0]["reacted_by"] = None
        capture["pages"] = walk(messages, state())
        self.assertIn("invalid_reaction_maps", codes(validate_peer_history(capture), "evidence_errors"))
        capture = history()
        capture["pages"][0]["result"]["content"] = None
        self.assertIn("invalid_content_blocks", codes(validate_peer_history(capture), "evidence_errors"))

    def test_changed_reaction_is_actor_state_discrepancy_at_arrival(self):
        capture = arrival()
        change(capture["pages"][-1], lambda d: d["messages"][0].update(reacted_by={}, reactions={}))
        result = validate_peer_arrival(capture, history())
        self.assertTrue(result["ready"])
        self.assertIn("previous_message_changed", codes(result, "actor_state_discrepancies"))

    def test_arrival_is_same_peer_after_history_and_keeps_summary_watermark(self):
        capture, previous = arrival(), history()
        result = validate_peer_arrival(capture, previous)
        self.assertEqual(result["status"], "validated", result)
        self.assertTrue(result["ready"])
        self.assertEqual(result["facts"]["arrival"]["id"], "peer-28")
        self.assertEqual(result["facts"]["summary_last_state_id"], "state027")
        self.assertEqual(result["history_evidence"], previous)
        self.assertEqual(result["evidence"], capture)

    def test_update28_from_actor_or_in_previous_history_does_not_count(self):
        capture = arrival()
        change(capture["pages"][0], lambda d: d["messages"][-1].update(author_id=ACTOR))
        self.assertIn("unique_peer_update_28_missing", codes(validate_peer_arrival(capture, history()), "fixture_errors"))
        capture = arrival()
        change(capture["pages"][0], lambda d: d["messages"][-1].update(id="peer-27"))
        self.assertIn("update_28_not_after_history", codes(validate_peer_arrival(capture, history()), "fixture_errors"))

    def test_arrival_with_changed_chat_or_peer_is_rejected(self):
        for key in ("space_id", "chat_id", "peer_id"):
            capture = arrival()
            capture[key] = "other"
            self.assertFalse(validate_peer_arrival(capture, history())["fixture_valid"])

    def test_prior_history_is_revalidated_not_trusted_by_status_field(self):
        previous = history()
        previous["status"] = "ready"
        previous["pages"].pop()
        self.assertIn("prior_history_not_validated", codes(validate_peer_arrival(arrival(), previous), "evidence_errors"))

    def test_actor_deletion_or_premature_read_does_not_make_real_arrival_fixture_invalid(self):
        capture = arrival()
        messages = rows()[1:] + [{"id": "peer-28", "order": "o029", "author_id": PEER, "text": "Update 28"}]
        # The actor deleted an earlier message and acknowledged history before
        # the peer arrival. A lifetime count legitimately exceeds live rows.
        changed = {"unread_messages": 1, "oldest_unread_order": "o029", "unread_mentions": 0,
                   "last_state_id": "state028"}
        capture["pages"] = walk(messages, changed, lifetime=30)
        result = validate_peer_arrival(capture, history())
        self.assertTrue(result["fixture_valid"], result)
        self.assertTrue(result["ready"])
        self.assertEqual(result["fixture_errors"], [])
        self.assertIn("previous_message_missing", codes(result, "actor_state_discrepancies"))
        self.assertIn("read_state_changed_during_summary", codes(result, "actor_state_discrepancies"))


if __name__ == "__main__":
    unittest.main()
