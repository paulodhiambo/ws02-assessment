import re
import unittest

from support import NS, load, properties_set

S = f"{{{NS['s']}}}"


class CustomerApiTest(unittest.TestCase):

    def setUp(self):
        self.api = load("customer-proxy", "api", "CustomerAPI")
        self.fault = load("customer-proxy", "sequences", "customer-fault")
        self.endpoint = load("customer-proxy", "endpoints", "CustomerBackendEP")

    def removed_headers(self):
        return {h.get("name").lower() for h in self.api.iter(f"{S}header") if h.get("action") == "remove"}

    def test_credentials_and_internal_headers_are_stripped(self):
        for header in ("authorization", "cookie", "apikey", "x-jwt-assertion", "x-internal-user-id", "x-internal-debug"):
            self.assertIn(header, self.removed_headers())

    def test_backend_response_headers_are_dropped_on_success(self):
        success = self.api.find(".//s:switch/s:case[@regex='200']", NS)
        removed = [p for p in success.iter(f"{S}property")
                   if p.get("name") == "TRANSPORT_HEADERS" and p.get("action") == "remove"]
        self.assertTrue(removed)
        cache = [h for h in success.iter(f"{S}header") if h.get("name") == "Cache-Control"]
        self.assertEqual("no-store", cache[0].get("value"))

    def test_backend_url_comes_from_environment(self):
        prop = [p for p in self.api.iter(f"{S}property") if p.get("name") == "uri.var.customerBackendUrl"][0]
        self.assertEqual("get-property('env', 'CUSTOMER_BACKEND_URL')", prop.get("expression"))
        template = self.endpoint.find("s:http", NS).get("uri-template")
        self.assertTrue(template.startswith("{+uri.var.customerBackendUrl}"), "base URL must use reserved expansion")

    def test_timeout_is_fault(self):
        timeout = self.endpoint.find(".//s:timeout", NS)
        self.assertEqual("5000", timeout.find("s:duration", NS).text)
        self.assertEqual("fault", timeout.find("s:responseAction", NS).text)

    def test_fault_mapping(self):
        cases = {c.get("regex"): properties_set(c, "ERROR_STATUS")[0] for c in self.fault.iter(f"{S}case")}
        self.assertEqual("504", cases["101504"])
        unavailable = re.compile(next(r for r in cases if r != "101504"))
        for code in ("101503", "101505", "303001"):
            self.assertTrue(unavailable.fullmatch(code), code)
        self.assertEqual("503", cases[unavailable.pattern])

    def test_upstream_statuses(self):
        codes = properties_set(self.api, "ERROR_CODE_APP")
        self.assertIn("CUSTOMER_NOT_FOUND", codes)
        self.assertIn("CUSTOMER_BACKEND_ERROR", codes)
        self.assertIn("INVALID_CUSTOMER_ID", codes)


if __name__ == "__main__":
    unittest.main()
