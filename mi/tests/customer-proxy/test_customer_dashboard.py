"""Bonus A: GET /customers/{customerId}/dashboard (scatter-gather aggregation)."""
import unittest
from xml.etree import ElementTree as ET

from support import MI_ROOT, NS, load, properties_set

S = f"{{{NS['s']}}}"


class CustomerDashboardTest(unittest.TestCase):

    def setUp(self):
        api = load("customer-proxy", "api", "CustomerAPI")
        self.resource = api.find("s:resource[@uri-template='/{customerId}/dashboard']", NS)
        self.sg = self.resource.find(".//s:scatter-gather", NS)

    def test_parallel_with_deadline(self):
        self.assertEqual("true", self.sg.get("parallel-execution"))
        agg = self.sg.find("s:aggregation", NS)
        branches = self.sg.findall("s:sequence", NS)
        self.assertEqual(str(len(branches)), agg.get("max-messages"))
        self.assertLessEqual(int(agg.get("timeout")), 10000)

    def test_heartbeat_branch_guarantees_completion(self):
        """Without an always-successful branch, a request where every real branch fails never completes."""
        formats = [f.text for f in self.sg.iter(f"{S}format")]
        self.assertTrue(any('"heartbeat"' in f for f in formats))

    def test_branches_cannot_fail_the_whole_request(self):
        for name in ("dashboard-customer-section", "dashboard-accounts-section"):
            seq = load("customer-proxy", "sequences", name)
            self.assertEqual("dashboard-section-error", seq.get("onError"), name)
        handler = load("customer-proxy", "sequences", "dashboard-section-error")
        self.assertIsNotNone(handler.find("s:drop", NS))

    def test_whole_response_errors(self):
        self.assertEqual({"400", "404", "503"}, set(properties_set(self.resource, "ERROR_STATUS")))
        self.assertIn("DASHBOARD_UNAVAILABLE", properties_set(self.resource, "ERROR_CODE_APP"))

    def test_accounts_query_is_parameterised(self):
        ds = ET.parse(MI_ROOT / "account-balance/src/main/dataservice/AccountsDataService.dbs").getroot()
        sql = ds.find("query[@id='getAccountsByCustomer']/sql").text
        self.assertIn("WHERE customer_id = ?", sql)
        self.assertIn("JSON_ARRAYAGG", sql)


if __name__ == "__main__":
    unittest.main()
