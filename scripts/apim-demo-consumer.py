#!/usr/bin/env python3
"""
Onboards a demo consuming application through the APIM Developer Portal
REST API, doing what a developer would do in the Developer Portal UI:

  1. create (or reuse) the application "JamiiDemoApp"
  2. subscribe it to the three APIs on their tiers
     (Accounts/Customers: Gold, Loan Eligibility: Bronze) and to the
     JamiiCoreBankingProduct API Product (Gold)
  3. generate production OAuth2 keys (client_credentials) and an API key
  4. print ready-to-run curl commands and write the credentials to
     tests/postman/apim.env.json for the Postman collection (git-ignored)

Usage:
  scripts/apim-demo-consumer.py [--apim https://localhost:9443] [--gateway https://localhost:8243]

Credentials: APIM_ADMIN_USER / APIM_ADMIN_PASSWORD (default admin/admin, the
stock dev image). TLS verification is off: the dev image uses a self-signed cert.
"""
import argparse
import base64
import json
import os
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

APP_NAME = "JamiiDemoApp"
SUBSCRIPTIONS = {
    "JamiiAccountsAPI": "Gold",
    "JamiiCustomersAPI": "Gold",
    "JamiiLoanEligibilityAPI": "Bronze",
    "JamiiCoreBankingProduct": "Gold",  # the API Product (listed alongside APIs in the Dev Portal)
}
CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE


def http(method, url, body=None, headers=None, form=False):
    headers = dict(headers or {})
    data = None
    if body is not None:
        if form:
            data = urllib.parse.urlencode(body).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        else:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, context=CTX, timeout=30) as resp:
            raw = resp.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raise SystemExit(f"{method} {url} -> {e.code}: {e.read().decode(errors='replace')[:500]}")


def basic(user, password):
    return {"Authorization": "Basic " + base64.b64encode(f"{user}:{password}".encode()).decode()}


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--apim", default=os.environ.get("APIM_URL", "https://localhost:9443"))
    p.add_argument("--gateway", default=os.environ.get("APIM_GATEWAY_URL", "https://localhost:8243"))
    args = p.parse_args()
    user = os.environ.get("APIM_ADMIN_USER", "admin")
    password = os.environ.get("APIM_ADMIN_PASSWORD", "admin")
    dev = f"{args.apim}/api/am/devportal/v3"

    # Developer Portal REST access token (dynamic client registration + password grant).
    client = http("POST", f"{args.apim}/client-registration/v0.17/register", {
        "callbackUrl": "https://localhost", "clientName": "jamii_devportal_cli", "owner": user,
        "grantType": "password refresh_token", "saasApp": True,
    }, basic(user, password))
    token = http("POST", f"{args.apim}/oauth2/token", {
        "grant_type": "password", "username": user, "password": password,
        "scope": "apim:subscribe apim:app_manage apim:sub_manage apim:api_key",
    }, basic(client["clientId"], client["clientSecret"]), form=True)["access_token"]
    auth = {"Authorization": f"Bearer {token}"}

    # 1. application
    apps = http("GET", f"{dev}/applications?query={APP_NAME}", headers=auth)["list"]
    app = next((a for a in apps if a["name"] == APP_NAME), None) or http("POST", f"{dev}/applications", {
        "name": APP_NAME, "throttlingPolicy": "Unlimited", "tokenType": "JWT",
        "description": "Demo consumer for the Jamii Savings assignment",
    }, auth)
    app_id = app["applicationId"]
    print(f"application: {APP_NAME} ({app_id})", file=sys.stderr)

    # 2. subscriptions
    existing = {s["apiInfo"]["name"] for s in http("GET", f"{dev}/subscriptions?applicationId={app_id}&limit=100", headers=auth)["list"]}
    for api_name, tier in SUBSCRIPTIONS.items():
        apis = http("GET", f"{dev}/apis?query=name:{api_name}", headers=auth)["list"]
        api = next((a for a in apis if a["name"] == api_name), None)
        if not api:
            raise SystemExit(f"{api_name} is not published in the Developer Portal - run scripts/deploy-apim.sh first")
        if api_name in existing:
            print(f"subscribed:  {api_name} (already)", file=sys.stderr)
            continue
        http("POST", f"{dev}/subscriptions", {"applicationId": app_id, "apiId": api["id"], "throttlingPolicy": tier}, auth)
        print(f"subscribed:  {api_name} on {tier}", file=sys.stderr)

    # 3. OAuth2 keys (reuse if already generated) and an API key
    keys = http("GET", f"{dev}/applications/{app_id}/oauth-keys", headers=auth)["list"]
    prod = next((k for k in keys if k["keyType"] == "PRODUCTION"), None) or http(
        "POST", f"{dev}/applications/{app_id}/generate-keys", {
            "keyType": "PRODUCTION", "keyManager": "Resident Key Manager",
            "grantTypesToBeSupported": ["client_credentials"], "validityTime": 3600,
        }, auth)
    access_token = http("POST", f"{args.apim}/oauth2/token", {"grant_type": "client_credentials"},
                        basic(prod["consumerKey"], prod["consumerSecret"]), form=True)["access_token"]
    api_key = http("POST", f"{dev}/applications/{app_id}/api-keys/PRODUCTION/generate",
                   {"validityPeriod": 3600}, auth)["apikey"]

    env = {"gatewayUrl": args.gateway, "accessToken": access_token, "apiKey": api_key}
    out = Path(__file__).resolve().parent.parent / "tests" / "postman" / "apim.env.json"
    out.write_text(json.dumps({
        "name": "jamii-apim", "values": [{"key": k, "value": v, "enabled": True} for k, v in env.items()],
    }, indent=2))
    print(f"wrote {out.relative_to(out.parents[2])}", file=sys.stderr)

    g = args.gateway
    print(f"""export GW={g}/jamii TOKEN={access_token} APIKEY={api_key}

# OAuth2 (Accounts, Customers) - token valid 1h
curl -sk {g}/jamii/accounts/v1/0100000001/balance -H 'Authorization: Bearer {access_token}'
curl -sk {g}/jamii/customers/v1/1 -H 'Authorization: Bearer {access_token}'

# Same operations through the API Product
curl -sk {g}/jamii/core/0100000001/balance -H 'Authorization: Bearer {access_token}'

# API key (Loan Eligibility)
curl -sk {g}/jamii/loans/v1/eligibility -H 'apikey: {api_key}' -H 'Content-Type: application/json' \\
  -d '{{"customerId":"1","monthlyIncome":120000,"existingMonthlyDebt":15000,"requestedAmount":500000,"tenureMonths":12}}'
""")


if __name__ == "__main__":
    main()
