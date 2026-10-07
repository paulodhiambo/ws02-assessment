package customerproxy

// Bonus A: GET /customers/{customerId}/dashboard (scatter-gather aggregation).

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"jamiisavings/mi/tests/support"
)

func dashboard(t *testing.T) (resource, scatterGather *support.Node) {
	api := support.Load(t, "customer-proxy", "api", "CustomerAPI")
	resource = api.Child("resource", "uri-template", "/{customerId}/dashboard")
	if resource == nil {
		t.Fatal("dashboard resource not found")
	}
	return resource, resource.First("scatter-gather")
}

func TestDashboardIsParallelWithDeadline(t *testing.T) {
	_, sg := dashboard(t)
	if sg.Attr("parallel-execution") != "true" {
		t.Error("branches must run in parallel")
	}
	agg := sg.Child("aggregation")
	branches := 0
	for _, c := range sg.Children {
		if c.Name() == "sequence" {
			branches++
		}
	}
	if agg.Attr("max-messages") != strconv.Itoa(branches) {
		t.Errorf("max-messages = %s, branches = %d", agg.Attr("max-messages"), branches)
	}
	if ms, _ := strconv.Atoi(agg.Attr("timeout")); ms == 0 || ms > 10000 {
		t.Errorf("aggregation timeout must be set and <= 10s, got %q", agg.Attr("timeout"))
	}
}

// Without an always-successful branch, a request where every real branch
// fails never completes (the aggregation timer starts on the first message).
func TestDashboardHeartbeatGuaranteesCompletion(t *testing.T) {
	_, sg := dashboard(t)
	if !slices.ContainsFunc(sg.PayloadFormats(), func(f string) bool { return strings.Contains(f, `"heartbeat"`) }) {
		t.Error("missing heartbeat branch")
	}
}

func TestDashboardBranchesCannotFailTheWholeRequest(t *testing.T) {
	for _, name := range []string{"dashboard-customer-section", "dashboard-accounts-section"} {
		if seq := support.Load(t, "customer-proxy", "sequences", name); seq.Attr("onError") != "dashboard-section-error" {
			t.Errorf("%s must use dashboard-section-error as onError", name)
		}
	}
	if support.Load(t, "customer-proxy", "sequences", "dashboard-section-error").Child("drop") == nil {
		t.Error("dashboard-section-error must drop the failed branch")
	}
}

func TestDashboardWholeResponseErrors(t *testing.T) {
	resource, _ := dashboard(t)
	if got := support.Set(resource.PropertiesSet("ERROR_STATUS")...); !slices.Equal(got, []string{"400", "404", "503"}) {
		t.Errorf("whole-response statuses = %v, want [400 404 503]", got)
	}
	if !support.Contains(resource.PropertiesSet("ERROR_CODE_APP"), "DASHBOARD_UNAVAILABLE") {
		t.Error("missing DASHBOARD_UNAVAILABLE")
	}
}

func TestDashboardAccountsQueryIsParameterised(t *testing.T) {
	ds := support.Parse(t, filepath.Join(support.MIRoot(), "account-balance/src/main/dataservice/AccountsDataService.dbs"))
	sql := ds.Child("query", "id", "getAccountsByCustomer").Child("sql").Text
	if !strings.Contains(sql, "WHERE customer_id = ?") || !strings.Contains(sql, "JSON_ARRAYAGG") {
		t.Errorf("unexpected query: %s", sql)
	}
}
