import re
import unittest
from xml.etree import ElementTree as ET

from support import MI_ROOT, NS, json_format, load, payload_formats, properties_set


class AccountBalanceApiTest(unittest.TestCase):

    def setUp(self):
        self.api = load("account-balance", "api", "AccountBalanceAPI")
        self.fault = load("account-balance", "sequences", "account-balance-fault")

    def test_route(self):
        self.assertEqual("/accounts", self.api.get("context"))
        resource = self.api.find("s:resource", NS)
        self.assertEqual("GET", resource.get("methods"))
        self.assertEqual("/{accountNumber}/balance", resource.get("uri-template"))
        self.assertEqual("account-balance-fault", resource.get("faultSequence"))

    def test_success_envelope_matches_spec(self):
        (fmt,) = payload_formats(self.api)
        body = json_format(fmt)
        self.assertEqual({"accountNumber", "status", "balance", "requestId", "timestamp"}, set(body))
        self.assertEqual({"amount", "currency"}, set(body["balance"]))
        self.assertIsInstance(body["balance"]["amount"], (int, float), "amount must be a JSON number")

    def test_account_number_validation(self):
        regexes = [f.get("regex") for f in self.api.iter(f"{{{NS['s']}}}filter") if f.get("regex")]
        pattern = re.compile(regexes[0])
        self.assertTrue(pattern.fullmatch("0100000001"))
        for bad in ("123", "01000000011", "01000000a1", ""):
            self.assertIsNone(pattern.fullmatch(bad), bad)

    def test_error_codes(self):
        self.assertIn("INVALID_ACCOUNT_NUMBER", properties_set(self.api, "ERROR_CODE_APP"))
        self.assertIn("ACCOUNT_NOT_FOUND", properties_set(self.api, "ERROR_CODE_APP"))
        self.assertIn("ACCOUNT_DB_UNAVAILABLE", properties_set(self.fault, "ERROR_CODE_APP"))
        self.assertIn("503", properties_set(self.fault, "ERROR_STATUS"))

    def test_db_failures_are_identified_by_stage(self):
        stages = properties_set(self.api, "STAGE")
        self.assertIn("DB_CALL", stages)
        db_filter = self.fault.find("s:filter", NS)
        self.assertEqual("DB_CALL", db_filter.get("regex"))


class AccountsDataServiceTest(unittest.TestCase):

    def setUp(self):
        self.ds = ET.parse(MI_ROOT / "account-balance/src/main/dataservice/AccountsDataService.dbs").getroot()

    def test_query_is_parameterised(self):
        sql = self.ds.find("query/sql").text
        self.assertIn("account_number = ?", sql)
        self.assertNotIn("#", sql)
        self.assertNotIn(":accountNumber", sql)
        self.assertEqual("1", self.ds.find("query/param").get("ordinal"))

    def test_credentials_are_injected_not_hardcoded(self):
        props = {p.get("name"): p.text for p in self.ds.iter("property")}
        for key in ("url", "username", "password"):
            self.assertTrue(props[key].startswith("$SYSTEM:"), key)

    def test_read_only_columns(self):
        sql = self.ds.find("query/sql").text.upper()
        self.assertTrue(sql.strip().startswith("SELECT"))


if __name__ == "__main__":
    unittest.main()
