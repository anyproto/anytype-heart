import json
import unittest

from check_query_results import compare_case


def result(tool, args, doc):
    return {"tool": tool, "arguments": args,
            "result": {"content": [{"type": "text", "text": json.dumps(doc)}]}}


def query(archived=False, object_id="q"):
    return result("API-get-object", {"object_id": object_id}, {
        "id": object_id, "type": "query",
        "properties": {"name": "Queue", "is_archived": archived},
        "blocks": [{"views": [{"id": "first", "name": "Main"},
                               {"id": "second", "name": "Alternative"}]}]})


def page(names, view="first", offset=0, total=None, more=False):
    return result("API-get-query-objects", {"query_id": "q", "view": view, "offset": offset}, {
        "data": [{"id": f"object-{offset+i}", "name": name} for i, name in enumerate(names)],
        "total": len(names) if total is None else total, "has_more": more})


class QueryChecks(unittest.TestCase):
    def test_uses_first_stored_view_not_last_response_or_unfiltered_source(self):
        reads = [query(), page(["A"]), page(["Wrong"], view="second"), page(["Wrong"], view=None)]
        self.assertEqual(compare_case("Queue", {"expected_names": ["A"]}, reads)["status"], "pass")

    def test_ignores_archived_query_but_rejects_two_active_names(self):
        reads = [query(), query(True, "old"), page(["A"])]
        self.assertEqual(compare_case("Queue", {"expected_names": ["A"]}, reads)["status"], "pass")
        reads.append(query(False, "duplicate"))
        self.assertEqual(compare_case("Queue", {"expected_names": ["A"]}, reads)["status"], "fail")

    def test_combines_complete_pages_and_does_not_grade_incomplete_pages(self):
        reads = [query(), page(["A"], total=2, more=True), page(["B"], offset=1, total=2)]
        spec = {"expected_names": ["A", "B"], "order_sensitive": True}
        self.assertEqual(compare_case("Queue", spec, reads)["status"], "pass")
        self.assertEqual(compare_case("Queue", spec, reads[:-1])["status"], "unavailable")

    def test_duplicate_names_are_allowed_when_expected_but_ids_must_be_unique(self):
        reads = [query(), page(["Alex", "Alex"])]
        spec = {"expected_names": ["Alex", "Alex"]}
        self.assertEqual(compare_case("Queue", spec, reads)["status"], "pass")
        doc = json.loads(reads[1]["result"]["content"][0]["text"])
        doc["data"][1]["id"] = doc["data"][0]["id"]
        reads[1] = result("API-get-query-objects", reads[1]["arguments"], doc)
        self.assertEqual(compare_case("Queue", spec, reads)["status"], "fail")

    def test_sort_ties_do_not_hide_wrong_order_between_groups(self):
        spec = {"expected_names": ["A", "B", "C"], "ordered_groups": [["A", "B"], ["C"]]}
        self.assertEqual(compare_case("Queue", spec, [query(), page(["B", "A", "C"])])["status"], "pass")
        self.assertEqual(compare_case("Queue", spec, [query(), page(["C", "A", "B"])])["status"], "fail")

    def test_error_response_is_not_an_empty_result_success(self):
        error = result("API-get-query-objects", {"query_id": "q", "view": "first"}, {"status": 404})
        self.assertEqual(compare_case("Queue", {"expected_names": []}, [query(), error])["status"], "unavailable")


if __name__ == "__main__":
    unittest.main()
