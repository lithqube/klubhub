#!/usr/bin/env python3
"""Pure harness acceptance tests; never creates containers."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

DRILL = Path(__file__).with_name('restore_drill.py')


class DrillExit(unittest.TestCase):
    def test_any_equality_mismatch_exits_one(self):
        with tempfile.TemporaryDirectory(dir=os.environ['TMPDIR']) as root:
            path = Path(root) / 'equality.json'
            for failed in ('equality_db', 'equality_keys', 'equality_bodies'):
                with self.subTest(failed=failed):
                    result = dict.fromkeys(
                        ('equality_db', 'equality_keys', 'equality_bodies'), True)
                    result[failed] = False
                    path.write_text(json.dumps(result))
                    p = subprocess.run(['python3', str(DRILL), '--check-equality',
                                        str(path)], capture_output=True, text=True)
                    self.assertEqual(p.returncode, 1, p.stdout + p.stderr)

    def test_all_equal_exits_zero(self):
        with tempfile.TemporaryDirectory(dir=os.environ['TMPDIR']) as root:
            path = Path(root) / 'equality.json'
            path.write_text(json.dumps(dict.fromkeys(
                ('equality_db', 'equality_keys', 'equality_bodies'), True)))
            p = subprocess.run(['python3', str(DRILL), '--check-equality', str(path)],
                               capture_output=True, text=True)
            self.assertEqual(p.returncode, 0, p.stdout + p.stderr)

    def test_no_garage_is_not_advertised(self):
        p = subprocess.run(['python3', str(DRILL), '--help'],
                           capture_output=True, text=True)
        self.assertEqual(p.returncode, 0)
        self.assertNotIn('--no-garage', p.stdout)


if __name__ == '__main__':
    unittest.main(verbosity=2)
