import json
from pathlib import Path
import re
import tempfile
import unittest

from space_guard import Guard, prepare_call
from fault_hooks import FaultHooks
from tool_names import Vocabulary, body, caller_retry_key, capability, is_bridge_name

HERE = Path(__file__).resolve().parent
GOLDEN = HERE.parents[1] / "core/api/wrapper/full/testdata/tools_list.golden.json"

# A bridge tool name as a string literal: API-x / API_x, host-prefixed or not,
# or the host prefix itself, or the old hand-written normalisers.
BRIDGE_LITERAL = re.compile(r"""["'](?:mcp__anytype__)?API[-_][a-z]|["']mcp__anytype__|removeprefix\(\s*["']API""")


class VocabularyTableTests(unittest.TestCase):
    def test_no_bridge_shaped_literal_outside_the_table(self):
        offenders = []
        for path in sorted(HERE.glob("*.py")):
            if path.name == "tool_names.py" or path.name.startswith("test_"):
                continue
            for number, line in enumerate(path.read_text().splitlines(), 1):
                if BRIDGE_LITERAL.search(line):
                    offenders.append(f"{path.name}:{number}: {line.strip()}")
        self.assertEqual([], offenders, "tool names belong in tool_names.py")

    def test_the_guard_pattern_catches_what_it_must(self):
        for line in ['x = "API-patch-object"', "y = 'API_create_space'", 'z = "mcp__anytype__API_get_object"',
                     'n.removeprefix("mcp__anytype__")', 'n.removeprefix("API_")']:
            self.assertRegex(line, BRIDGE_LITERAL)
        self.assertNotRegex('"patch_object"', BRIDGE_LITERAL)

    def test_every_spelling_has_one_capability(self):
        for name in ["API-patch-object", "API_patch_object", "mcp__anytype__API_patch_object",
                     "mcp__anytype__API-patch-object", "patch_object", "mcp__anytype__patch_object"]:
            self.assertEqual("patch_object", capability(name), name)
        self.assertTrue(is_bridge_name("mcp__anytype__API_patch_object"))
        self.assertFalse(is_bridge_name("mcp__anytype__patch_object"))

    def test_surfaces_name_and_shape_calls(self):
        bridge, full = Vocabulary("bridge"), Vocabulary("full")
        self.assertEqual("API-create-space", bridge.tool("create_space"))
        self.assertEqual("create_space", full.tool("create_space"))
        ops = {"ops": [{"op": "delete_block", "id": "b"}]}
        self.assertEqual({"space_id": "s", "body": ops}, bridge.arguments({"space_id": "s"}, ops))
        self.assertEqual({"space_id": "s", **ops}, full.arguments({"space_id": "s"}, ops))
        with self.assertRaises(ValueError):
            Vocabulary("npm")

    def test_body_is_read_the_same_way_on_both_surfaces(self):
        ops = [{"op": "delete_block", "id": "b"}]
        self.assertEqual(ops, body("API-patch-object", {"space_id": "s", "body": {"ops": ops}})["ops"])
        self.assertEqual(ops, body("patch_object", {"space_id": "s", "ops": ops})["ops"])
        # a capability-only caller still finds a bridge body by its shape
        self.assertEqual(ops, body("patch_object", {"space_id": "s", "body": {"ops": ops}})["ops"])

    def test_the_callers_retry_key_is_read_on_both_surfaces(self):
        self.assertEqual("k1", caller_retry_key({"emoji": "x", "request_key": "k1"}))
        self.assertEqual("k2", caller_retry_key({"emoji": "x", "idempotency_key": "k2"}))
        self.assertIsNone(caller_retry_key({"emoji": "x"}))

    def test_on_full_only_the_reaction_toggle_exposes_a_retry_key(self):
        tools = json.loads(GOLDEN.read_text())["tools"]
        self.assertEqual(["toggle_chat_reaction"],
                         [t["name"] for t in tools if "idempotency_key" in t["inputSchema"].get("properties", {})])

    def test_no_full_tier_tool_takes_a_member_named_body(self):
        # the premise of body(): a nested body dict can only be the bridge's
        tools = json.loads(GOLDEN.read_text())["tools"]
        self.assertGreater(len(tools), 40)
        takers = [t["name"] for t in tools if "body" in t["inputSchema"].get("properties", {})]
        self.assertEqual([], takers)


class FullTierGuardTests(unittest.TestCase):
    def test_space_pinning_with_flat_full_tier_arguments(self):
        with tempfile.TemporaryDirectory() as tmp:
            guard = Guard(Path(tmp) / "scope.json", "eval-run")
            guard.register("bafyreiowned123456.k")
            hooks = FaultHooks(None)
            call = lambda args: {"jsonrpc": "2.0", "id": 1, "method": "tools/call",
                                 "params": {"name": "patch_object", "arguments": args}}
            forwarded, refusal, _ = prepare_call(guard, call({"space_id": "123456", "object_id": "o",
                                                             "ops": [{"op": "delete_block", "id": "b"}]}), hooks)
            self.assertIsNone(refusal)
            self.assertEqual("bafyreiowned123456.k", forwarded["params"]["arguments"]["space_id"])
            self.assertEqual([{"op": "delete_block", "id": "b"}], forwarded["params"]["arguments"]["ops"])
            _, refusal, _ = prepare_call(guard, call({"space_id": "bafyother.k", "object_id": "o", "ops": []}), hooks)
            self.assertIsNotNone(refusal)
            _, refusal, _ = prepare_call(guard, {"jsonrpc": "2.0", "id": 2, "method": "tools/call",
                                                 "params": {"name": "create_space", "arguments": {"name": "eval-run — x"}}}, hooks)
            self.assertIsNotNone(refusal, "a second space is refused on the full tier too")


if __name__ == "__main__":
    unittest.main()
