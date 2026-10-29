"""Offline fixtures for process ownership and safe metadata collection.

Run with: python3 -m unittest discover -s scripts/diagnostics -p 'test_process_scope.py'
All process information comes from the local fixtures below. No OS process
enumeration, process arguments, environments, application launch, or network.
"""

from collections import Counter
from pathlib import Path
import sys
import unittest


sys.path.insert(0, str(Path(__file__).resolve().parent))
from process_scope import collect_owned_metadata, discover_owned_pids


class ProcessGraph:
    """PID-only dependency injection with bounded callbacks to catch cycles."""

    def __init__(self, listed=(), responsible=None, children=None):
        self.listed = tuple(listed)
        self.responsible = responsible or {}
        self.children = children or {}
        self.responsible_calls = []
        self.child_calls = Counter()

    def list_pids(self):
        return iter(self.listed)

    @staticmethod
    def resolve(value):
        if isinstance(value, Exception):
            raise value
        return value

    def responsible_pid(self, pid):
        if type(pid) is not int or pid <= 0:
            raise AssertionError("invalid PID reached responsibility callback")
        self.responsible_calls.append(pid)
        return self.resolve(self.responsible.get(pid, pid))

    def child_pids(self, pid):
        if type(pid) is not int or pid <= 0:
            raise AssertionError("invalid PID reached child callback")
        self.child_calls[pid] += 1
        if self.child_calls[pid] > 20:
            raise AssertionError("cyclic fixture traversal did not terminate")
        return iter(self.resolve(self.children.get(pid, ())))

    def discover(self, root_pid):
        return discover_owned_pids(
            root_pid, self.list_pids, self.responsible_pid, self.child_pids
        )


class DiscoverOwnedPidsTests(unittest.TestCase):
    def test_responsibility_and_descendant_edges_form_one_transitive_scope(self):
        graph = ProcessGraph(
            listed=(100, 200, 300, 400, 900),
            responsible={200: 1, 300: 200, 400: 300, 900: 900},
            children={100: (200,), 400: (401,)},
        )
        self.assertEqual(graph.discover(100), {100, 200, 300, 400, 401})
        self.assertCountEqual(graph.responsible_calls, (200, 300, 400, 900))
        self.assertTrue(all(count == 1 for count in graph.child_calls.values()))

    def test_responsibility_cycles_are_bounded_and_unreachable_cycles_excluded(self):
        graph = ProcessGraph(
            listed=(100, 200, 300, 400, 900, 901),
            responsible={200: 400, 300: 200, 400: 300, 900: 901, 901: 900},
            children={100: (200,), 400: (300,)},
        )
        self.assertEqual(graph.discover(100), {100, 200, 300, 400})
        self.assertTrue(all(count == 1 for count in graph.child_calls.values()))

    def test_responsible_helpers_and_descendants_ignore_names_and_responsibility(self):
        # 200 and 300 represent arbitrarily named system helpers. Discovery has
        # no executable/name input: their responsibility is the inclusion rule.
        # 400 and 401 represent unrelated services (even if similarly named).
        graph = ProcessGraph(
            listed=(100, 101, 200, 201, 300, 400, 401),
            responsible={
                100: 100, 101: 1, 200: 100, 201: 400,
                300: 100, 400: 1, 401: 401,
            },
            children={100: (101,), 200: (201,), 201: (202,), 300: (301,),
                      301: (302,), 400: (402,)},
        )

        self.assertEqual(graph.discover(100), {100, 101, 200, 201, 202, 300, 301, 302})
        self.assertNotIn(400, graph.child_calls)
        self.assertNotIn(401, graph.child_calls)
        self.assertNotIn(402, graph.child_calls)

    def test_descendants_are_not_limited_to_a_small_depth_or_initial_snapshot(self):
        # A chain beyond Python's usual recursion limit requires an unbounded
        # traversal strategy; its descendants are absent from list_pids().
        descendants = range(2000, 3500)
        children = {100: (2000,)}
        children.update({pid: (pid + 1,) for pid in range(2000, 3499)})
        graph = ProcessGraph(listed=(100,), children=children)

        self.assertEqual(graph.discover(100), {100, *descendants})

    def test_root_is_included_when_missing_from_snapshot_and_source_fixture(self):
        graph = ProcessGraph()

        self.assertEqual(graph.discover(987), {987})
        self.assertIn(987, graph.child_calls)

    def test_exited_root_still_has_stable_scope_identity(self):
        graph = ProcessGraph(children={987: ProcessLookupError()})

        self.assertEqual(graph.discover(987), {987})

    def test_exits_during_responsibility_lookup_do_not_abort_other_helpers(self):
        graph = ProcessGraph(
            listed=(100, 200, 201, 300),
            responsible={200: ProcessLookupError(), 201: OSError(), 300: 100},
            children={300: (301,)},
        )

        self.assertEqual(graph.discover(100), {100, 300, 301})
        self.assertNotIn(200, graph.child_calls)
        self.assertNotIn(201, graph.child_calls)

    def test_exits_during_child_lookup_keep_selected_pid_and_continue(self):
        graph = ProcessGraph(
            listed=(100, 200, 300),
            responsible={200: 100, 300: 100},
            children={100: OSError(), 200: ProcessLookupError(), 300: (301,),
                      301: (302,)},
        )

        self.assertEqual(graph.discover(100), {100, 200, 300, 301, 302})

    def test_cycles_self_links_and_duplicate_edges_terminate(self):
        graph = ProcessGraph(
            listed=(100, 100, 200, 200, 900),
            responsible={200: 100, 900: 900},
            children={100: (100, 101, 101), 101: (102,), 102: (100,),
                      200: (201,), 201: (200, 101), 900: (900,)},
        )

        self.assertEqual(graph.discover(100), {100, 101, 102, 200, 201})
        self.assertNotIn(900, graph.child_calls)

    def test_invalid_snapshot_and_child_pids_never_reach_callbacks(self):
        invalid = (True, False, 0, -1, "200", None, 200.0)
        graph = ProcessGraph(
            listed=(*invalid, 100, 200),
            responsible={200: 100},
            children={100: (*invalid, 101), 200: (201, *invalid)},
        )

        self.assertEqual(graph.discover(100), {100, 101, 200, 201})
        self.assertTrue(all(type(pid) is int and pid > 0 for pid in graph.responsible_calls))
        self.assertTrue(all(type(pid) is int and pid > 0 for pid in graph.child_calls))

    def test_invalid_responsibility_values_cannot_alias_root_pid(self):
        graph = ProcessGraph(
            listed=(1, 200, 201, 202, 203, 204, 205, 206),
            responsible={200: True, 201: False, 202: 0, 203: -1,
                         204: "1", 205: None, 206: 1.0},
        )

        self.assertEqual(graph.discover(1), {1})

    def test_invalid_root_is_not_a_scope(self):
        for root in (True, False, 0, -1, "100", None, 100.0):
            with self.subTest(root=root):
                graph = ProcessGraph(listed=(100,), responsible={100: root})
                with self.assertRaises(ValueError):
                    graph.discover(root)
                self.assertEqual(graph.responsible_calls, [])
                self.assertEqual(graph.child_calls, Counter())


class CollectOwnedMetadataTests(unittest.TestCase):
    @staticmethod
    def row(pid, **extra):
        return {"pid": pid, "ppid": 1, "uid": 501, "state": "R",
                "executable": f"/offline-fixture/helper-{pid}", **extra}

    def test_only_selected_pids_are_queried_and_only_allowlisted_fields_returned(self):
        rows = {
            100: self.row(100, unexpected="fixed public fixture", command="do not retain"),
            200: self.row(200, arbitrary={"nested": "do not retain"}),
            900: self.row(900),
        }
        calls = []

        def metadata_for_pid(pid):
            calls.append(pid)
            if pid not in {100, 200}:
                self.fail("metadata queried for an unselected PID")
            return [rows[pid]]

        result = collect_owned_metadata({100, 200}, metadata_for_pid)

        self.assertCountEqual(calls, [100, 200])
        self.assertEqual(set(result), {100, 200})
        allowed = {"pid", "ppid", "uid", "state", "executable"}
        for pid in (100, 200):
            self.assertEqual(result[pid], {key: rows[pid][key] for key in allowed})
            self.assertEqual(set(result[pid]), allowed)

    def test_other_pid_rows_are_rejected_even_when_that_pid_is_also_selected(self):
        calls = []

        def metadata_for_pid(pid):
            calls.append(pid)
            if pid == 100:
                return [self.row(900), self.row(200), self.row(100)]
            return []

        result = collect_owned_metadata({100, 200}, metadata_for_pid)

        self.assertCountEqual(calls, [100, 200])
        self.assertEqual(result, {100: self.row(100)})

    def test_missing_invalid_or_boolean_row_pid_is_not_accepted(self):
        def metadata_for_pid(pid):
            return [{"executable": "/offline-fixture/missing-pid"},
                    self.row(True), self.row(False), self.row(0), self.row(-1),
                    self.row("1"), self.row(1.0)]

        self.assertEqual(collect_owned_metadata({1}, metadata_for_pid), {})

    def test_exited_or_unreadable_pids_are_skipped_independently(self):
        # These are the pure callback contract's skippable failures. The real
        # collector wrapper must instead wrap collection failures in RuntimeError.
        calls = []
        fixtures = {100: [], 200: ProcessLookupError(), 300: OSError(),
                    400: ValueError(), 500: [self.row(500)]}

        def metadata_for_pid(pid):
            calls.append(pid)
            value = fixtures[pid]
            if isinstance(value, Exception):
                raise value
            return value

        self.assertEqual(collect_owned_metadata(set(fixtures), metadata_for_pid),
                         {500: self.row(500)})
        self.assertCountEqual(calls, fixtures)

    def test_metadata_collection_runtime_error_propagates_unchanged(self):
        class MetadataCollectionError(RuntimeError):
            pass

        failure = MetadataCollectionError("metadata collection failed")
        calls = []

        def metadata_for_pid(pid):
            calls.append(pid)
            raise failure

        with self.assertRaises(MetadataCollectionError) as raised:
            collect_owned_metadata({100}, metadata_for_pid)

        self.assertIs(raised.exception, failure)
        self.assertEqual(calls, [100])

    def test_invalid_input_pids_never_reach_metadata_callback(self):
        calls = []

        def metadata_for_pid(pid):
            self.assertIs(type(pid), int)
            self.assertGreater(pid, 0)
            calls.append(pid)
            return [self.row(pid)]

        pids = [True, False, 0, -1, "100", None, 100.0, 100]
        self.assertEqual(collect_owned_metadata(pids, metadata_for_pid),
                         {100: self.row(100)})
        self.assertEqual(calls, [100])

    def test_empty_scope_never_queries_metadata(self):
        def metadata_for_pid(pid):
            self.fail("metadata queried for empty scope")

        self.assertEqual(collect_owned_metadata(set(), metadata_for_pid), {})


if __name__ == "__main__":
    unittest.main()
