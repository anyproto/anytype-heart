import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from verify_file_bytes import COMPLETE, check_run, hash_download


def response(doc, **flags):
    return {"content": [{"type": "text", "text": json.dumps(doc)}], **flags}


class FileBytesChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.run = self.root / "run"
        self.run.mkdir()
        self.fixtures = self.root / "fixtures"
        self.fixtures.mkdir()
        self.data = {"logo.png": b"actual image bytes", "usage.txt": b"actual usage bytes"}
        manifest = {}
        for name, data in self.data.items():
            (self.fixtures / name).write_bytes(data)
            manifest[name] = {"bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}
        (self.fixtures / "manifest.json").write_text(json.dumps(manifest))
        (self.run / "run.json").write_text(json.dumps({
            "scenario_id": "MCP-22", "status": COMPLETE,
            "owned_spaces": [{"full_id": "owned.full", "compact_id": "owned"}]}))
        self.events = []
        self.serial = 0
        self.paths = {}
        for i, (name, data) in enumerate(self.data.items()):
            directory = self.root / f"anytype-mcp-download-abcde{i}"
            directory.mkdir()
            self.paths[name] = directory / "download"
            self.paths[name].write_bytes(data)
        temp_roots = patch("verify_file_bytes.allowed_temp_roots", return_value={self.root})
        temp_roots.start()
        self.addCleanup(temp_roots.stop)

    def call(self, tool, args, result, *, error=None):
        self.serial += 1
        self.events.extend([
            {"direction": "to_upstream", "payload": {
                "id": self.serial, "method": "tools/call",
                "params": {"name": tool, "arguments": {"space_id": "owned", **args}}}},
            {"direction": "from_upstream", "payload": {
                "id": self.serial, "result": result, "error": error}},
        ])

    def upload(self, name):
        self.call("API-upload-file", {"file": str(self.fixtures / name)}, response({"id": name}))

    def download(self, name, **args):
        self.call("API-download-file", {"file_id": name, **args}, response({
            "path": str(self.paths[name]), "filename": "download", "size": len(self.data[name])}))

    def check(self):
        (self.run / "mcp-wire.jsonl").write_text("".join(json.dumps(e) + "\n" for e in self.events))
        return check_run(self.run, self.fixtures)

    def test_actual_bytes_match_manifest_for_both_originals(self):
        for name in self.data:
            self.upload(name)
            self.download(name, width=0)
        result = self.check()
        self.assertEqual(result["status"], "pass")
        self.assertTrue(all(d["sha256_matches"] for d in result["downloads"]))
        self.assertIn("mcp-wire.jsonl", result["evidence"])
        self.assertIn("response_ref", result["downloads"][0])

    def test_matching_filename_and_size_cannot_hide_wrong_bytes(self):
        self.upload("logo.png")
        self.download("logo.png")
        self.paths["logo.png"].write_bytes(b"x" * len(self.data["logo.png"]))
        check = self.check()["downloads"][0]
        self.assertEqual(check["status"], "mismatch")
        self.assertTrue(check["bytes_match"])
        self.assertFalse(check["sha256_matches"])

    def test_missing_path_is_separate_from_mismatch(self):
        self.upload("logo.png")
        self.download("logo.png")
        self.paths["logo.png"].unlink()
        self.assertEqual(self.check()["downloads"][0]["status"], "path_missing")

    def test_fixture_path_is_not_a_download_even_if_bytes_match(self):
        self.upload("logo.png")
        self.paths["logo.png"] = self.fixtures / "logo.png"
        self.download("logo.png")
        self.assertEqual(self.check()["downloads"][0]["status"], "invalid")

    def test_symlink_file_and_symlink_directory_are_rejected(self):
        path = self.paths["logo.png"]
        path.unlink()
        path.symlink_to(self.fixtures / "logo.png")
        self.assertEqual(hash_download(str(path))["status"], "invalid")
        path.unlink()
        path.parent.rmdir()
        path.parent.symlink_to(self.fixtures, target_is_directory=True)
        self.assertEqual(hash_download(str(path.parent / "logo.png"))["status"], "invalid")

    def test_hardlinks_and_nonregular_files_are_rejected(self):
        path = self.paths["logo.png"]
        path.unlink()
        path.hardlink_to(self.fixtures / "logo.png")
        self.assertEqual(hash_download(str(path))["status"], "invalid")
        path.unlink()
        path.mkdir()
        self.assertEqual(hash_download(str(path))["status"], "invalid")

    def test_unapproved_root_and_traversal_are_rejected(self):
        with patch("verify_file_bytes.allowed_temp_roots", return_value={Path("/nonexistent")}):
            self.assertEqual(hash_download(str(self.paths["logo.png"]))["status"], "invalid")
        traversal = str(self.paths["logo.png"].parent / ".." / self.paths["logo.png"].parent.name / "download")
        self.assertEqual(hash_download(traversal)["status"], "invalid")

    def test_error_receipt_cannot_authorize_a_read(self):
        for flags in ({"isError": True}, {"isError": False}):
            with self.subTest(flags=flags):
                self.events = []
                self.upload("logo.png")
                doc = {"path": str(self.paths["logo.png"]), "code": "transport_error"}
                self.call("API-download-file", {"file_id": "logo.png"}, response(doc, **flags))
                self.assertEqual(self.check()["downloads"], [])

    def test_rpc_error_cannot_authorize_a_read(self):
        self.upload("logo.png")
        self.call("API-download-file", {"file_id": "logo.png"},
                  response({"path": str(self.paths["logo.png"])}), error={"code": -1})
        self.assertEqual(self.check()["downloads"], [])

    def test_prose_tool_call_arguments_and_to_model_results_are_not_receipts(self):
        self.upload("logo.png")
        self.download("logo.png")
        self.events[-1]["direction"] = "to_model"
        (self.run / "turn-05.final.txt").write_text(str(self.paths["logo.png"]))
        (self.run / "tool-calls.jsonl").write_text(json.dumps({
            "tool": "API-download-file", "status": "completed",
            "result": response({"path": str(self.paths["logo.png"])})}))
        self.assertEqual(self.check()["downloads"], [])

    def test_download_must_link_to_exact_fixture_upload_and_owned_space(self):
        self.download("logo.png")
        self.upload("usage.txt")
        self.download("usage.txt", space_id="another-space")
        self.assertEqual(self.check()["downloads"], [])

    def test_resize_and_dry_run_do_not_verify_original(self):
        self.upload("logo.png")
        self.download("logo.png", width=100)
        self.download("logo.png", dry_run=True)
        self.assertEqual(self.check()["downloads"], [])

    def test_changed_source_fixture_prevents_reading_download(self):
        self.upload("logo.png")
        self.download("logo.png")
        (self.fixtures / "logo.png").write_bytes(b"changed source")
        with patch("verify_file_bytes.hash_download", side_effect=AssertionError("must not read")):
            result = self.check()
        self.assertEqual(result["files"]["logo.png"]["fixture_status"], "invalid")
        self.assertEqual(result["downloads"][0]["status"], "invalid")

    def test_successful_receipt_missing_path_is_invalid(self):
        self.upload("logo.png")
        self.call("API-download-file", {"file_id": "logo.png"}, response({"filename": "logo.png", "size": 18}))
        self.assertEqual(self.check()["downloads"][0]["status"], "invalid")

    def test_reused_rpc_ids_do_not_join_old_upload_to_new_response(self):
        self.upload("logo.png")
        self.download("logo.png")
        self.events.pop()  # No response in this process.
        self.events.append({"direction": "to_upstream", "payload": {
            "id": 0, "method": "initialize", "params": {}}})
        self.events.append({"direction": "from_upstream", "payload": {
            "id": self.serial, "result": response({"path": str(self.paths["logo.png"])})}})
        self.assertEqual(self.check()["downloads"], [])

    def test_missing_wire_evidence_and_running_conversations(self):
        result = check_run(self.run, self.fixtures)
        self.assertEqual(result["status"], "unavailable")
        manifest = json.loads((self.run / "run.json").read_text())
        manifest["status"] = "running"
        (self.run / "run.json").write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "completed MCP-22"):
            check_run(self.run, self.fixtures)


if __name__ == "__main__":
    unittest.main()
