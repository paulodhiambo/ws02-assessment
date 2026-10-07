package accountbalance

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"jamiisavings/mi/tests/support"
)

func loadAPI(t *testing.T) (api, fault *support.Node) {
	return support.Load(t, "account-balance", "api", "AccountBalanceAPI"),
		support.Load(t, "account-balance", "sequences", "account-balance-fault")
}

func loadDataService(t *testing.T) *support.Node {
	return support.Parse(t, filepath.Join(support.MIRoot(), "account-balance/src/main/dataservice/AccountsDataService.dbs"))
}

func TestRoute(t *testing.T) {
	api, _ := loadAPI(t)
	if api.Attr("context") != "/accounts" {
		t.Errorf("context = %q", api.Attr("context"))
	}
	r := api.Child("resource")
	for attr, want := range map[string]string{
		"methods": "GET", "uri-template": "/{accountNumber}/balance", "faultSequence": "account-balance-fault",
	} {
		if got := r.Attr(attr); got != want {
			t.Errorf("%s = %q, want %q", attr, got, want)
		}
	}
}

func TestSuccessEnvelopeMatchesSpec(t *testing.T) {
	api, _ := loadAPI(t)
	formats := api.PayloadFormats()
	if len(formats) != 1 {
		t.Fatalf("want one payload format, got %d", len(formats))
	}
	body := support.JSONFormat(t, formats[0])
	if got, want := support.Keys(body), []string{"accountNumber", "balance", "requestId", "status", "timestamp"}; !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	balance := body["balance"].(map[string]any)
	if got, want := support.Keys(balance), []string{"amount", "currency"}; !slices.Equal(got, want) {
		t.Errorf("balance keys = %v, want %v", got, want)
	}
	if _, ok := balance["amount"].(float64); !ok {
		t.Error("amount must be a JSON number")
	}
}

func TestAccountNumberValidation(t *testing.T) {
	api, _ := loadAPI(t)
	var pattern *regexp.Regexp
	for _, f := range api.All("filter") {
		if r := f.Attr("regex"); r != "" {
			pattern = regexp.MustCompile("^(?:" + r + ")$") // Synapse uses full-match semantics
			break
		}
	}
	if pattern == nil {
		t.Fatal("no regex filter for the account number")
	}
	if !pattern.MatchString("0100000001") {
		t.Error("valid account number rejected")
	}
	for _, bad := range []string{"123", "01000000011", "01000000a1", ""} {
		if pattern.MatchString(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestErrorCodes(t *testing.T) {
	api, fault := loadAPI(t)
	for _, code := range []string{"INVALID_ACCOUNT_NUMBER", "ACCOUNT_NOT_FOUND"} {
		if !support.Contains(api.PropertiesSet("ERROR_CODE_APP"), code) {
			t.Errorf("API does not set %s", code)
		}
	}
	if !support.Contains(fault.PropertiesSet("ERROR_CODE_APP"), "ACCOUNT_DB_UNAVAILABLE") {
		t.Error("fault sequence does not set ACCOUNT_DB_UNAVAILABLE")
	}
	if !support.Contains(fault.PropertiesSet("ERROR_STATUS"), "503") {
		t.Error("DB failures must be 503")
	}
}

func TestDBFailuresAreIdentifiedByStage(t *testing.T) {
	api, fault := loadAPI(t)
	if !support.Contains(api.PropertiesSet("STAGE"), "DB_CALL") {
		t.Error("API does not mark the DB_CALL stage")
	}
	if f := fault.Child("filter"); f == nil || f.Attr("regex") != "DB_CALL" {
		t.Error("fault sequence must branch on STAGE=DB_CALL")
	}
}

func TestQueryIsParameterised(t *testing.T) {
	ds := loadDataService(t)
	q := ds.Child("query", "id", "getAccountByNumber")
	sql := q.Child("sql").Text
	if !strings.Contains(sql, "account_number = ?") || strings.Contains(sql, "#") || strings.Contains(sql, ":accountNumber") {
		t.Errorf("query must use a bound parameter: %s", sql)
	}
	if q.Child("param").Attr("ordinal") != "1" {
		t.Error("param ordinal must be 1")
	}
}

func TestCredentialsAreInjectedNotHardcoded(t *testing.T) {
	props := map[string]string{}
	for _, p := range loadDataService(t).All("property") {
		props[p.Attr("name")] = strings.TrimSpace(p.Text)
	}
	for _, key := range []string{"url", "username", "password"} {
		if !strings.HasPrefix(props[key], "$SYSTEM:") {
			t.Errorf("%s must come from the environment ($SYSTEM:...), got %q", key, props[key])
		}
	}
}

func TestQueriesAreReadOnly(t *testing.T) {
	for _, q := range loadDataService(t).All("query") {
		if sql := strings.ToUpper(strings.TrimSpace(q.Child("sql").Text)); !strings.HasPrefix(sql, "SELECT") {
			t.Errorf("query %s is not a SELECT", q.Attr("id"))
		}
	}
}
