"""Synthetic coverage fixtures: no process enumeration or real credentials."""
import unittest

from measure import assemble_measurement
from summarize import summarize


def metadata(*pids):
    return {pid: {"pid": pid, "executable": f"/synthetic/helper-{pid}"} for pid in pids}


def metric(pid, mib):
    return {"pid": pid, "footprint": mib * 1048576, "start_ticks": pid}


def sample(measurement):
    return {"time": "synthetic-time", "elapsed": 1, "phase": "steady", **measurement}


class MeasurementCoverageTests(unittest.TestCase):
    def test_complete_scope_has_expected_and_observed_pids(self):
        row = assemble_measurement({100, 200}, metadata(100, 200), [metric(100, 20), metric(200, 80)])
        self.assertTrue(row["coverage_complete"])
        self.assertEqual(row["expected_pids"], [100, 200])
        self.assertEqual(row["observed_pids"], [100, 200])
        self.assertEqual(row["footprint"], 100 * 1048576)
        self.assertEqual(row["metrics_errors"], [])

    def test_numeric_metric_failure_keeps_missing_helper_visible(self):
        row = assemble_measurement({100, 200}, metadata(100, 200),
                                   [metric(100, 20), {"pid": 200, "error_errno": 3, "ignored": "synthetic"}])
        self.assertFalse(row["coverage_complete"])
        self.assertEqual(row["metrics_missing_pids"], [200])
        self.assertEqual(row["metrics_errors"], [{"pid": 200, "error_errno": 3}])
        self.assertEqual(row["metadata_missing_count"], 0)

    def test_metadata_failure_and_missing_metric_reply_are_not_zero_cost_helpers(self):
        missing_metadata = assemble_measurement({100, 200}, metadata(100), [metric(100, 20)])
        self.assertFalse(missing_metadata["coverage_complete"])
        self.assertEqual(missing_metadata["metadata_missing_pids"], [200])
        self.assertEqual(missing_metadata["metadata_missing_count"], 1)
        missing_reply = assemble_measurement({100, 200}, metadata(100, 200), [metric(100, 20)])
        self.assertFalse(missing_reply["coverage_complete"])
        self.assertEqual(missing_reply["metrics_missing_pids"], [200])

    def test_summary_excludes_incomplete_low_and_high_samples(self):
        full = sample(assemble_measurement({100, 200}, metadata(100, 200), [metric(100, 20), metric(200, 80)]))
        low = sample(assemble_measurement({100, 200}, metadata(100), [metric(100, 20)]))
        high = sample(assemble_measurement({100, 200}, metadata(100), [metric(100, 1000)]))
        result = summarize([low, full, high])
        self.assertEqual(result["complete_samples"], 1)
        self.assertEqual(result["incomplete_samples"], 2)
        self.assertEqual(result["peak_MiB"], 100)
        self.assertEqual(result["phases"]["steady"]["median_MiB"], 100)
        self.assertEqual(result["phases"]["steady"]["process_counts"], [2])

    def test_no_complete_samples_or_legacy_unknown_rows_produce_no_memory_value(self):
        partial = sample(assemble_measurement({100, 200}, metadata(100), [metric(100, 20)]))
        legacy = {key: value for key, value in partial.items() if key != "coverage_complete"}
        result = summarize([partial, legacy])
        self.assertEqual(result["complete_samples"], 0)
        self.assertEqual(result["incomplete_samples"], 1)
        self.assertEqual(result["coverage_unknown_samples"], 1)
        self.assertNotIn("peak_MiB", result)
        self.assertNotIn("median_MiB", result["phases"]["steady"])
        self.assertNotIn("max_MiB", result["phases"]["steady"])
        self.assertNotIn("peak_MiB", summarize([]))


if __name__ == "__main__":
    unittest.main()
