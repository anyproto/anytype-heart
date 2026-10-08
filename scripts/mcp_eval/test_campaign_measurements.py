import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import campaign_measurements as measurements


class MeasurementTests(unittest.TestCase):
    def test_runtime_type_schema_is_counted_without_becoming_object_patch_op(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            campaign = base / "artifacts/luna-three-runs"
            (campaign / "spec").mkdir(parents=True)
            (campaign / "spec/surface-snapshot.json").write_text(json.dumps({
                "tools": [{"name": "API-get-schema"}, {"name": "API-get-op-schema"}],
                "schemas": {"object": {}, "ops/set_properties": {}}}))
            calls = [
                {"tool": "API-get-schema", "arguments": {"kind": "object"}},
                {"tool": "API-get-op-schema", "arguments": {"op": "add_property"}},
                {"tool": "API-get-op-schema", "arguments": {"op": "add_property"}},
                {"tool": "API-get-op-schema", "arguments": {"op": "remove_property"}},
            ]
            slots = [{"status": "ready_for_astra_review", "scenario_id": "MCP-08", "replicate": 2,
                      "run": {"run_dir": str(base / "run")}}]
            with patch.object(measurements, "BASE", base), \
                    patch.object(measurements, "build", return_value={"slots": slots}), \
                    patch.object(measurements, "call_records", return_value=calls), \
                    patch.object(measurements, "is_failed", return_value=False), \
                    contextlib.redirect_stdout(io.StringIO()):
                measurements.main()
            result = json.loads((campaign / "measurements.json").read_text())
            self.assertEqual(result["schema_discovery_calls"], {
                "object": 1, "ops/set_properties": 0, "ops/add_property": 2, "ops/remove_property": 1})
            self.assertEqual(sum(result["schema_discovery_calls"].values()), 4)
            self.assertEqual(set(result["patch_operations"]), {"set_properties"})
            self.assertEqual(result["summary"]["actor_calls"], 4)


if __name__ == "__main__":
    unittest.main()
