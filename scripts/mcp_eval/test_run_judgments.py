import contextlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import run_judgments


class DispatchBoundaryTest(unittest.TestCase):
    def test_pending_authorization_cannot_launch_even_with_prepared_jobs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            prepared = {"authorization": {"status": "pending"}, "jobs": [(root / "run", root / "review")],
                        "errors": [], "matrix": root / "matrix.json"}
            with patch("sys.argv", ["run_judgments.py", "--campaign", directory]), \
                 patch.object(run_judgments, "plan", return_value=prepared), \
                 patch.object(run_judgments, "judge") as judge:
                with self.assertRaisesRegex(SystemExit, "explicit user approval"):
                    run_judgments.main()
                judge.assert_not_called()

    def test_check_only_never_launches_even_if_approval_is_present(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            prepared = {"authorization": {"status": "approved"}, "jobs": [(root / "run", root / "review")],
                        "errors": [], "matrix": root / "matrix.json"}
            with patch("sys.argv", ["run_judgments.py", "--campaign", directory, "--check-only"]), \
                 patch.object(run_judgments, "plan", return_value=prepared), \
                 patch.object(run_judgments, "judge") as judge, contextlib.redirect_stdout(io.StringIO()):
                run_judgments.main()
                judge.assert_not_called()


if __name__ == "__main__":
    unittest.main()
