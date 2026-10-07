import json
import unittest

from support import NS, json_format, load, payload_formats, properties_set

S = f"{{{NS['s']}}}"


def debt_to_income_decision(monthly_income, existing_debt, requested_amount, tenure_months):
    """Reference implementation of the mapping documented in docs/api-design.md."""
    a, b = round(requested_amount), tenure_months
    q = a / b
    floor = int(q)
    installment = floor + (1 if q - floor > 0.5 or (q - floor == 0.5 and floor % 2) else 0)  # half-to-even, as the SOAP service
    max_affordable = round((monthly_income * 0.4 - existing_debt) * 100) / 100
    return installment, max_affordable, installment <= max_affordable


class LoanEligibilityApiTest(unittest.TestCase):

    def setUp(self):
        self.api = load("loan-eligibility", "api", "LoanEligibilityAPI")
        self.fault = load("loan-eligibility", "sequences", "loan-fault")
        self.schema = json.loads(load("loan-eligibility", "local-entries", "LoanEligibilityRequestSchema").text)

    def test_schema_requires_core_fields(self):
        self.assertEqual({"customerId", "monthlyIncome", "requestedAmount", "tenureMonths"}, set(self.schema["required"]))
        self.assertFalse(self.schema["additionalProperties"])
        self.assertEqual("integer", self.schema["properties"]["tenureMonths"]["type"])

    def test_validate_mediator_uses_schema_and_returns_400(self):
        validate = self.api.find(".//s:validate", NS)
        self.assertEqual("LoanEligibilityRequestSchema", validate.find("s:schema", NS).get("key"))
        self.assertIn("400", properties_set(validate.find("s:on-fail", NS), "ERROR_STATUS"))

    def test_soap_faults_are_mapped_not_leaked(self):
        fault_branch = self.api.find(".//s:filter[@xpath='boolean($body/soap:Fault)']/s:then", NS)
        self.assertIsNotNone(fault_branch)
        self.assertEqual({"422", "502"}, set(properties_set(fault_branch, "ERROR_STATUS")))
        self.assertEqual([], payload_formats(fault_branch), "fault branch must not build a payload from the SOAP fault")
        keys = [s.get("key") for s in fault_branch.iter(f"{S}sequence")]
        self.assertIn("common-error", keys)

    def test_success_response_shape(self):
        (fmt,) = payload_formats(self.api)
        body = json_format(fmt)
        self.assertEqual({"customerId", "eligible", "decision", "reason", "loan", "requestId", "timestamp"}, set(body))
        self.assertEqual({"requestedAmount", "tenureMonths", "monthlyInstallment", "maxAffordableInstallment", "currency"},
                         set(body["loan"]))

    def test_transport_fault_mapping(self):
        statuses = set(properties_set(self.fault, "ERROR_STATUS"))
        self.assertTrue({"400", "503", "504", "500"} <= statuses)

    def test_decision_rule_examples(self):
        # Values verified against the live service and MI (see tests/integration/loan-eligibility.http).
        self.assertEqual((41667, 33000, False), debt_to_income_decision(120000, 15000, 500000, 12))
        self.assertEqual((25000, 48000, True), debt_to_income_decision(120000, 0, 300000, 12))
        self.assertEqual((2, 0.8, False), debt_to_income_decision(2, 0, 5, 2))


if __name__ == "__main__":
    unittest.main()
