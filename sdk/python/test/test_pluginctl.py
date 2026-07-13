import tempfile
import unittest
from pathlib import Path

from ptpatronus_plugin_sdk.pluginctl import write_json_output, write_market_index


class PluginctlLineEndingTests(unittest.TestCase):
    def test_generated_json_uses_lf_on_all_platforms(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            package_path = write_json_output(str(root / "package.json"), {"name": "demo"})
            market_path = root / "market.json"
            write_market_index(market_path, {"plugins": []})
            self.assertNotIn(b"\r\n", package_path.read_bytes())
            self.assertNotIn(b"\r\n", market_path.read_bytes())


if __name__ == "__main__":
    unittest.main()
