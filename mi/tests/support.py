"""Helpers shared by the MI artifact tests."""
import json
from pathlib import Path
from xml.etree import ElementTree as ET

MI_ROOT = Path(__file__).resolve().parent.parent
NS = {"s": "http://ws.apache.org/ns/synapse"}
ERROR_ENVELOPE_KEYS = ("error", "code", "message", "status", "requestId", "correlationId", "timestamp")


def synapse_dir(module):
    if module == "common":
        return MI_ROOT / "common"
    return MI_ROOT / module / "src" / "main" / "synapse-config"


def load(module, folder, name):
    return ET.parse(synapse_dir(module) / folder / f"{name}.xml").getroot()


def all_artifacts():
    """Every synapse XML file across all modules, as (path, root element)."""
    for module in ("common", "account-balance", "customer-proxy", "loan-eligibility"):
        for path in sorted(synapse_dir(module).rglob("*.xml")):
            yield path, ET.parse(path).getroot()


def properties_set(root, name):
    """Values assigned to a property anywhere under root."""
    return [p.get("value") for p in root.iter(f"{{{NS['s']}}}property")
            if p.get("name") == name and p.get("value") is not None]


def payload_formats(root):
    return [f.text for f in root.iter(f"{{{NS['s']}}}format") if f.text]


def json_format(text):
    """Render a payloadFactory JSON format as parseable JSON by filling placeholders."""
    filled = text
    for i in range(20, 0, -1):
        filled = filled.replace(f'"${i}"', '"x"').replace(f"${i}", "0")
    return json.loads(filled)
