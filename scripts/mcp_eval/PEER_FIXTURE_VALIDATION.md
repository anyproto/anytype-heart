# Offline MCP-30 fixture validation

`peer_fixture_validation.py` is independent of the running evaluator. It imports
no runner, calls no API, writes no receipts, and never alters a campaign. Existing
live processes continue using their already imported fixture implementation.

Supply actual evaluator-captured call records, including every original MCP
content block, in this shape:

```python
history_capture = {
    "space_id": full_owned_space_id,
    "chat_id": actual_actor_created_chat_id,
    "peer_id": actual_peer_participant_id,
    "current_member": current_account_get_member_me_call,
    "peer_member": peer_account_get_member_me_call,  # optional corroboration
    "setup_pages": actor_chat_reads_before_peer_seeding,
    "pages": chat_reads_after_updates_01_through_27,
}
```

Each call is `{"tool": "API-get-chat-messages", "arguments": {...},
"result": ORIGINAL_MCP_RESULT}` (or the member tool). Use the full owned space
ID in arguments. Page walks request `reactions="full"`, begin without cursors,
then follow `next_before` exactly until `has_more=false`. Every page must be
chronological and must carry the same state and lifetime count. A complete
`limit=100` page is valid; the 29-message history exceeds the default page size.
Lifetime `message_count` may exceed the live row count after deletion.

The setup capture must show the actor's two original messages, with announcement
attachment and reply target. The validator does not resolve the attachment to
the Login problem object: retain the actor setup evidence for semantic review.
It never creates or repairs missing actor setup. Put the current-member mention
in Update 23–27 so it remains unread with the last five messages. Canonical
served mention syntax is `<mention object_id="PARTICIPANT_ID">Name</mention>`.
Literal escaped or code-span tags do not qualify. The narrow mention reader
accepts canonical tags with simple labels; unusual nested label tags require
manual review instead of silently accepting them.
`unread_reaction_order` is a reaction-change order, not the announcement's
message order. Its nonempty value proves unread reaction state; peer membership
on the announcement is independently checked through `reacted_by`.

```python
from peer_fixture_validation import validate_peer_history, validate_peer_arrival

history_check = validate_peer_history(history_capture)
# Check history_check["ready"] before authorizing the history receipt.

arrival_capture = {
    "space_id": full_owned_space_id,
    "chat_id": actual_actor_created_chat_id,
    "peer_id": actual_peer_participant_id,
    "current_member": current_account_get_member_me_call,
    "pages": complete_chat_reads_after_update_28,
}
arrival_check = validate_peer_arrival(arrival_capture, history_capture)
# Check arrival_check["ready"] before authorizing the arrival receipt.
```

Results retain deep copies of exact input evidence and distinguish
`evidence_errors`, `fixture_errors`, and `actor_state_discrepancies`. The arrival
check revalidates prior history, retains the original summary watermark, and
requires a new peer-authored Update 28 after that history. Premature read changes,
deleted/edited prior messages, and changed reactions are reported separately;
they do not turn an otherwise valid Update 28 into a fixture failure. Attribute
those discrepancies using the actor trace before grading responsibility.

This validates **server-reported author IDs**, not cryptographic signatures or
receipt authenticity. The caller must establish that reads came from the actual
clients and owned space, keep peer writes frozen during history inspection,
and post Update 28 only after the actor completes its summary. Returned order and
state IDs establish content order, not wall-clock timing against the actor turn.
No second account means the fixture remains unavailable/unverified. This module
does not mint a receipt or promote an unavailable fixture to a triggered hook.
