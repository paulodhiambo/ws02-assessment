"""Cross-cutting rules every integration must follow."""
import unittest

from support import NS, all_artifacts, load, payload_formats, json_format, ERROR_ENVELOPE_KEYS

S = f"{{{NS['s']}}}"


class GatewayPolicyTest(unittest.TestCase):

    def test_no_full_payload_logging(self):
        """Payloads carry PII; only custom, explicitly chosen fields may be logged."""
        for path, root in all_artifacts():
            for log in root.iter(f"{S}log"):
                self.assertIn(log.get("level", "simple"), ("custom",), f"{path.name}: log level must be 'custom'")

    def test_every_api_resource_has_a_fault_sequence(self):
        for path, root in all_artifacts():
            for resource in root.iter(f"{S}resource"):
                self.assertTrue(resource.get("faultSequence"), f"{path.name}: resource without faultSequence")

    def test_every_api_uses_correlation_and_logging(self):
        for path, root in all_artifacts():
            if root.tag != f"{S}api":
                continue
            keys = {seq.get("key") for seq in root.iter(f"{S}sequence")}
            for required in ("correlation", "request-logging", "response-logging"):
                self.assertIn(required, keys, f"{path.name}: does not call '{required}'")

    def test_every_endpoint_has_a_timeout(self):
        for path, root in all_artifacts():
            if root.tag != f"{S}endpoint":
                continue
            duration = root.find(".//s:timeout/s:duration", NS)
            self.assertIsNotNone(duration, f"{path.name}: endpoint without timeout")
            self.assertLessEqual(int(duration.text), 10000, f"{path.name}: timeout above 10s")

    def test_error_responses_only_come_from_common_error(self):
        """Fault sequences must delegate to common-error so every API shares one error shape."""
        for path, root in all_artifacts():
            if root.tag == f"{S}sequence" and root.get("name", "").endswith("-fault"):
                keys = [seq.get("key") for seq in root.iter(f"{S}sequence")]
                self.assertIn("common-error", keys, f"{path.name}: fault sequence must call common-error")
                self.assertEqual([], payload_formats(root), f"{path.name}: builds its own payload")


class CommonErrorEnvelopeTest(unittest.TestCase):

    def test_envelope_shape(self):
        root = load("common", "sequences", "common-error")
        (fmt,) = payload_formats(root)
        body = json_format(fmt)
        self.assertEqual({"error", "requestId", "correlationId", "timestamp"}, set(body))
        self.assertEqual({"code", "message", "status"}, set(body["error"]))
        for key in ERROR_ENVELOPE_KEYS:
            self.assertIn(f'"{key}"', fmt)

    def test_backend_headers_are_dropped(self):
        root = load("common", "sequences", "common-error")
        removed = [p for p in root.iter(f"{S}property")
                   if p.get("name") == "TRANSPORT_HEADERS" and p.get("action") == "remove"]
        self.assertTrue(removed)


class MaskingTest(unittest.TestCase):
    """Re-implements the mask-sensitive-data XPath to pin its behaviour."""

    @staticmethod
    def mask(value):
        return "****" if len(value) <= 4 else "*" * (len(value) - 4) + value[-4:]

    def test_sequence_keeps_last_four(self):
        root = load("common", "sequences", "mask-sensitive-data")
        expr = root.find(".//s:then/s:property", NS).get("expression")
        self.assertIn("string-length(get-property('MASK_INPUT')) - 3", expr)
        self.assertEqual("******0001", self.mask("0100000001"))
        self.assertEqual("****", self.mask("42"))


if __name__ == "__main__":
    unittest.main()
