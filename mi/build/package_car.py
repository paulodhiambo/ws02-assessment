#!/usr/bin/env python3
"""
Packages the MI integration modules into Carbon Application (.car) archives.

Each module directory (account-balance, customer-proxy, loan-eligibility,
common) becomes one CAR. Artifacts are discovered by folder convention:

    <module>/src/main/synapse-config/api/*.xml          -> synapse/api
    <module>/src/main/synapse-config/sequences/*.xml    -> synapse/sequence
    <module>/src/main/synapse-config/endpoints/*.xml    -> synapse/endpoint
    <module>/src/main/synapse-config/local-entries/*.xml-> synapse/local-entry
    <module>/src/main/synapse-config/templates/*.xml    -> synapse/template
    <module>/src/main/dataservice/*.dbs                 -> service/dataservice
    common/sequences/*.xml                              -> synapse/sequence

The CAR layout is the one produced by WSO2 Integration Studio / the MI VS Code
extension, so the archives hot-deploy into any MI 4.x carbonapps directory:

    <Name>_<version>.car
      artifacts.xml
      <Artifact>_<version>/artifact.xml
      <Artifact>_<version>/<Artifact>.xml|.dbs

Usage:
    package_car.py --version 1.0.0 --out target/cars [--validate-only]

Validation runs before packaging and fails the build on: malformed XML,
an artifact whose name attribute does not match its file name, or two
artifacts with the same name across modules (MI would reject the second).
"""
import argparse
import sys
import zipfile
from pathlib import Path
from xml.etree import ElementTree as ET

MI_ROOT = Path(__file__).resolve().parent.parent
SERVER_ROLE = "EnterpriseIntegrator"

# module directory -> CAR name
MODULES = {
    "common": "JamiiCommon",
    "account-balance": "JamiiAccountBalance",
    "customer-proxy": "JamiiCustomerProxy",
    "loan-eligibility": "JamiiLoanEligibility",
}

# relative folder -> (artifact type, file glob)
SYNAPSE_FOLDERS = {
    "local-entries": "synapse/local-entry",
    "endpoints": "synapse/endpoint",
    "sequences": "synapse/sequence",
    "templates": "synapse/template",
    "api": "synapse/api",
}


class Artifact:
    def __init__(self, name, type_, path):
        self.name, self.type, self.path = name, type_, path

    def __repr__(self):
        return f"{self.type}:{self.name}"


def artifact_name(path, type_):
    root = ET.parse(path).getroot()
    if type_ == "synapse/local-entry":
        return root.get("key")
    return root.get("name")


def discover(module_dir):
    found = []
    synapse_root = module_dir / "src" / "main" / "synapse-config"
    if module_dir.name == "common":
        synapse_root = module_dir
    for folder, type_ in SYNAPSE_FOLDERS.items():
        for path in sorted((synapse_root / folder).glob("*.xml")):
            found.append(Artifact(artifact_name(path, type_), type_, path))
    for path in sorted((module_dir / "src" / "main" / "dataservice").glob("*.dbs")):
        found.append(Artifact(artifact_name(path, "service/dataservice"), "service/dataservice", path))
    return found


def validate(modules):
    errors, seen = [], {}
    for module, artifacts in modules.items():
        if not artifacts:
            errors.append(f"{module}: no artifacts found")
        for a in artifacts:
            if not a.name:
                errors.append(f"{a.path}: missing name/key attribute")
                continue
            if a.name != a.path.stem:
                errors.append(f"{a.path}: name '{a.name}' does not match file name '{a.path.stem}'")
            if a.name in seen:
                errors.append(f"{a.path}: duplicate artifact name '{a.name}' (also in {seen[a.name]})")
            seen[a.name] = a.path
    return errors


def artifact_xml(a, version):
    el = ET.Element("artifact", name=a.name, groupId="ke.co.jamiisavings.mi", version=version,
                    type=a.type, serverRole=SERVER_ROLE)
    ET.SubElement(el, "file").text = a.path.name
    return ET.tostring(el, encoding="unicode")


def package(car_name, artifacts, version, out_dir):
    out_dir.mkdir(parents=True, exist_ok=True)
    car_path = out_dir / f"{car_name}_{version}.car"
    app = ET.Element("artifact", name=car_name, version=version, type="carbon/application")
    with zipfile.ZipFile(car_path, "w", zipfile.ZIP_DEFLATED) as car:
        for a in artifacts:
            folder = f"{a.name}_{version}"
            # MI's extractor does not create parent directories itself, so
            # each artifact folder needs an explicit directory entry.
            car.writestr(f"{folder}/", "")
            car.writestr(f"{folder}/artifact.xml", '<?xml version="1.0" encoding="UTF-8"?>\n' + artifact_xml(a, version))
            car.write(a.path, f"{folder}/{a.path.name}")
            ET.SubElement(app, "dependency", artifact=a.name, version=version,
                          include="true", serverRole=SERVER_ROLE)
        root = ET.Element("artifacts")
        root.append(app)
        car.writestr("artifacts.xml", '<?xml version="1.0" encoding="UTF-8"?>\n' + ET.tostring(root, encoding="unicode"))
    return car_path


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--version", required=True)
    parser.add_argument("--out", default=str(MI_ROOT / "target" / "cars"))
    parser.add_argument("--validate-only", action="store_true")
    args = parser.parse_args()

    try:
        modules = {m: discover(MI_ROOT / m) for m in MODULES}
    except ET.ParseError as e:
        print(f"[package-car] malformed XML: {e}", file=sys.stderr)
        return 1

    errors = validate(modules)
    if errors:
        for e in errors:
            print(f"[package-car] ERROR {e}", file=sys.stderr)
        return 1
    for module, artifacts in modules.items():
        print(f"[package-car] {module}: {len(artifacts)} artifacts {artifacts}")
    if args.validate_only:
        return 0

    for module, car_name in MODULES.items():
        path = package(car_name, modules[module], args.version, Path(args.out))
        print(f"[package-car] built {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
