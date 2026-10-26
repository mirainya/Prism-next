import importlib.util
import json
import pathlib
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).with_name("build_clean_baseline.py")
spec = importlib.util.spec_from_file_location("clean_baseline", SCRIPT)
baseline = importlib.util.module_from_spec(spec)
spec.loader.exec_module(baseline)


class CleanBaselineTest(unittest.TestCase):
    def test_quoted_values_are_not_normalized_as_sql(self):
        for left, right in [("DEFAULT ' AUTO_INCREMENT=42 '", "DEFAULT ' '"),
                            ("DEFAULT 'two  spaces'", "DEFAULT 'two spaces'")]:
            self.assertNotEqual(baseline.canonical_ddl(left), baseline.canonical_ddl(right))
        self.assertEqual(baseline.canonical_ddl("CREATE TABLE `t` ( `id` int ) ENGINE=InnoDB AUTO_INCREMENT=42;"),
                         "CREATE TABLE `t` ( `id` int ) ENGINE=InnoDB")

    def test_only_builtin_seeds_enter_public_output(self):
        root = SCRIPT.parent.parent / ".temp"
        root.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(prefix="temp_baseline_test_", dir=root) as folder:
            folder = pathlib.Path(folder)
            structure = folder / "structure.sql"
            structure.write_text("CREATE TABLE `gw_catalog_runtime_state` (\n `id` int NOT NULL\n) ENGINE=InnoDB AUTO_INCREMENT=42 DEFAULT CHARSET=utf8mb4;\n", encoding="utf-8")
            data = folder / "data.sql"
            data.write_text("INSERT INTO `gw_catalog_runtime_state` (`id`) VALUES (1);\n"
                            "INSERT INTO `video_channels` (`name`) VALUES ('private-provider');\n", encoding="utf-8")
            output = folder / "baseline.sql"
            report = baseline.build(structure, data, output)
            content = output.read_text(encoding="utf-8")
            self.assertNotIn("private-provider", content)
            self.assertNotIn("AUTO_INCREMENT=42", content)
            self.assertIn("CREATE TABLE IF NOT EXISTS", content)
            self.assertIn("ON DUPLICATE KEY UPDATE `id` = `id`", content)
            self.assertEqual(report["excluded_data_tables"], ["video_channels"])
            self.assertFalse(report["verified_in_mysql"])
            schema = json.loads(output.with_suffix(".schema.json").read_text(encoding="utf-8"))
            self.assertEqual(list(schema["tables"]), ["gw_catalog_runtime_state"])
            with self.assertRaises(ValueError):
                baseline.build(structure, data, output)
            structure.write_text("INSERT INTO `video_channels` VALUES (1);", encoding="utf-8")
            with self.assertRaises(ValueError):
                baseline.build(structure, data, folder / "unsafe.sql")


if __name__ == "__main__":
    unittest.main()
