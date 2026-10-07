// Package apimconsumer onboards a demo consuming application through the
// APIM Developer Portal REST API, doing what a developer would do in the
// Developer Portal UI:
//
//  1. create (or reuse) the application "JamiiDemoApp"
//  2. subscribe it to the three APIs on their tiers (Accounts/Customers:
//     Gold, Loan Eligibility: Bronze) and to the JamiiCoreBankingProduct API
//     Product (Gold)
//  3. generate production OAuth2 keys (client_credentials) and an API key
//  4. write the credentials as a Postman environment and return ready-to-run
//     shell exports and curl commands
package apimconsumer

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// AppName is the demo application created in the Developer Portal.
const AppName = "JamiiDemoApp"

// Subscriptions in the order they are made, with their tiers.
var Subscriptions = []struct{ API, Tier string }{
	{"JamiiAccountsAPI", "Gold"},
	{"JamiiCustomersAPI", "Gold"},
	{"JamiiLoanEligibilityAPI", "Bronze"},
	{"JamiiCoreBankingProduct", "Gold"}, // the API Product (listed alongside APIs in the Dev Portal)
}

// Config for one onboarding run.
type Config struct {
	APIM, Gateway  string // e.g. https://localhost:9443, https://localhost:8243
	User, Password string // APIM admin (stock dev image: admin/admin)
	EnvFile        string // Postman environment to write
	Log            io.Writer
}

// Result is what a consumer needs to call the APIs.
type Result struct {
	AccessToken, APIKey string
}

type client struct {
	http *http.Client
	cfg  Config
}

// The dev image uses a self-signed certificate, so TLS verification is off.
func newClient(cfg Config) *client {
	return &client{
		cfg: cfg,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // dev only
		},
	}
}

func basic(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

// call sends a JSON (or form, if form is set) request and decodes the JSON response into out.
func (c *client) call(method, u, auth string, body any, form url.Values, out any) error {
	var reader io.Reader
	contentType := ""
	switch {
	case form != nil:
		reader, contentType = strings.NewReader(form.Encode()), "application/x-www-form-urlencoded"
	case body != nil:
		b, _ := json.Marshal(body)
		reader, contentType = bytes.NewReader(b), "application/json"
	}
	req, err := http.NewRequest(method, u, reader)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, u, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		if len(raw) > 500 {
			raw = raw[:500]
		}
		return fmt.Errorf("%s %s -> %d: %s", method, u, resp.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type listResponse[T any] struct {
	List []T `json:"list"`
}

// Run performs the onboarding and returns the credentials.
func Run(cfg Config) (Result, error) {
	c := newClient(cfg)
	dev := cfg.APIM + "/api/am/devportal/v3"
	logf := func(format string, a ...any) { fmt.Fprintf(cfg.Log, format+"\n", a...) }

	// Developer Portal REST access token (dynamic client registration + password grant).
	var reg struct{ ClientID, ClientSecret string }
	if err := c.call("POST", cfg.APIM+"/client-registration/v0.17/register", basic(cfg.User, cfg.Password), map[string]any{
		"callbackUrl": "https://localhost", "clientName": "jamii_devportal_cli", "owner": cfg.User,
		"grantType": "password refresh_token", "saasApp": true,
	}, nil, &reg); err != nil {
		return Result{}, err
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.call("POST", cfg.APIM+"/oauth2/token", basic(reg.ClientID, reg.ClientSecret), nil, url.Values{
		"grant_type": {"password"}, "username": {cfg.User}, "password": {cfg.Password},
		"scope": {"apim:subscribe apim:app_manage apim:sub_manage apim:api_key"},
	}, &tok); err != nil {
		return Result{}, err
	}
	auth := "Bearer " + tok.AccessToken

	// 1. application
	type application struct{ ApplicationID, Name string }
	var apps listResponse[application]
	if err := c.call("GET", dev+"/applications?query="+url.QueryEscape(AppName), auth, nil, nil, &apps); err != nil {
		return Result{}, err
	}
	var app application
	for _, a := range apps.List {
		if a.Name == AppName {
			app = a
		}
	}
	if app.ApplicationID == "" {
		if err := c.call("POST", dev+"/applications", auth, map[string]any{
			"name": AppName, "throttlingPolicy": "Unlimited", "tokenType": "JWT",
			"description": "Demo consumer for the Jamii Savings assignment",
		}, nil, &app); err != nil {
			return Result{}, err
		}
	}
	logf("application: %s (%s)", AppName, app.ApplicationID)

	// 2. subscriptions
	var subs listResponse[struct {
		APIInfo struct{ Name string } `json:"apiInfo"`
	}]
	if err := c.call("GET", dev+"/subscriptions?limit=100&applicationId="+app.ApplicationID, auth, nil, nil, &subs); err != nil {
		return Result{}, err
	}
	existing := map[string]bool{}
	for _, s := range subs.List {
		existing[s.APIInfo.Name] = true
	}
	for _, s := range Subscriptions {
		var apis listResponse[struct{ ID, Name string }]
		if err := c.call("GET", dev+"/apis?query="+url.QueryEscape("name:"+s.API), auth, nil, nil, &apis); err != nil {
			return Result{}, err
		}
		apiID := ""
		for _, a := range apis.List {
			if a.Name == s.API {
				apiID = a.ID
			}
		}
		if apiID == "" {
			return Result{}, fmt.Errorf("%s is not published in the Developer Portal - run scripts/deploy-apim.sh first", s.API)
		}
		if existing[s.API] {
			logf("subscribed:  %s (already)", s.API)
			continue
		}
		if err := c.call("POST", dev+"/subscriptions", auth, map[string]any{
			"applicationId": app.ApplicationID, "apiId": apiID, "throttlingPolicy": s.Tier,
		}, nil, nil); err != nil {
			return Result{}, err
		}
		logf("subscribed:  %s on %s", s.API, s.Tier)
	}

	// 3. OAuth2 keys (reuse if already generated) and an API key
	type keys struct{ KeyType, ConsumerKey, ConsumerSecret string }
	var existingKeys listResponse[keys]
	if err := c.call("GET", dev+"/applications/"+app.ApplicationID+"/oauth-keys", auth, nil, nil, &existingKeys); err != nil {
		return Result{}, err
	}
	var prod keys
	for _, k := range existingKeys.List {
		if k.KeyType == "PRODUCTION" {
			prod = k
		}
	}
	if prod.ConsumerKey == "" {
		if err := c.call("POST", dev+"/applications/"+app.ApplicationID+"/generate-keys", auth, map[string]any{
			"keyType": "PRODUCTION", "keyManager": "Resident Key Manager",
			"grantTypesToBeSupported": []string{"client_credentials"}, "validityTime": 3600,
		}, nil, &prod); err != nil {
			return Result{}, err
		}
	}
	var res Result
	if err := c.call("POST", cfg.APIM+"/oauth2/token", basic(prod.ConsumerKey, prod.ConsumerSecret), nil,
		url.Values{"grant_type": {"client_credentials"}}, &tok); err != nil {
		return Result{}, err
	}
	res.AccessToken = tok.AccessToken
	var key struct {
		APIKey string `json:"apikey"`
	}
	if err := c.call("POST", dev+"/applications/"+app.ApplicationID+"/api-keys/PRODUCTION/generate", auth,
		map[string]any{"validityPeriod": 3600}, nil, &key); err != nil {
		return Result{}, err
	}
	res.APIKey = key.APIKey

	// 4. Postman environment for scripts/test.sh --integration --apim
	if cfg.EnvFile != "" {
		env := map[string]any{"name": "jamii-apim", "values": []map[string]any{
			{"key": "gatewayUrl", "value": cfg.Gateway, "enabled": true},
			{"key": "accessToken", "value": res.AccessToken, "enabled": true},
			{"key": "apiKey", "value": res.APIKey, "enabled": true},
		}}
		b, _ := json.MarshalIndent(env, "", "  ")
		if err := os.WriteFile(cfg.EnvFile, b, 0o600); err != nil {
			return Result{}, err
		}
		logf("wrote %s", cfg.EnvFile)
	}
	return res, nil
}

// Commands returns shell exports and example curl commands for the credentials.
func Commands(gateway string, r Result) string {
	g := gateway
	return fmt.Sprintf(`export GW=%[1]s/jamii TOKEN=%[2]s APIKEY=%[3]s

# OAuth2 (Accounts, Customers) - token valid 1h
curl -sk %[1]s/jamii/accounts/v1/0100000001/balance -H 'Authorization: Bearer %[2]s'
curl -sk %[1]s/jamii/customers/v1/1 -H 'Authorization: Bearer %[2]s'

# Same operations through the API Product
curl -sk %[1]s/jamii/core/0100000001/balance -H 'Authorization: Bearer %[2]s'

# API key (Loan Eligibility)
curl -sk %[1]s/jamii/loans/v1/eligibility -H 'apikey: %[3]s' -H 'Content-Type: application/json' \
  -d '{"customerId":"1","monthlyIncome":120000,"existingMonthlyDebt":15000,"requestedAmount":500000,"tenureMonths":12}'
`, g, r.AccessToken, r.APIKey)
}
