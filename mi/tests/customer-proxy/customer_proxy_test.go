package customerproxy

import (
	"regexp"
	"strings"
	"testing"

	"jamiisavings/mi/tests/support"
)

type fixture struct {
	api, proxy, fault, endpoint *support.Node
}

func load(t *testing.T) fixture {
	api := support.Load(t, "customer-proxy", "api", "CustomerAPI")
	return fixture{
		api:      api,
		proxy:    api.Child("resource", "uri-template", "/{customerId}"),
		fault:    support.Load(t, "customer-proxy", "sequences", "customer-fault"),
		endpoint: support.Load(t, "customer-proxy", "endpoints", "CustomerBackendEP"),
	}
}

func TestCredentialsAndInternalHeadersAreStripped(t *testing.T) {
	f := load(t)
	removed := map[string]bool{}
	for _, h := range f.proxy.All("header") {
		if h.Attr("action") == "remove" {
			removed[strings.ToLower(h.Attr("name"))] = true
		}
	}
	for _, h := range []string{"authorization", "cookie", "apikey", "x-jwt-assertion", "x-internal-user-id", "x-internal-debug", "accept-encoding"} {
		if !removed[h] {
			t.Errorf("%s is not stripped before the backend call", h)
		}
	}
}

func TestBackendResponseHeadersAreDroppedOnSuccess(t *testing.T) {
	success := load(t).proxy.First("case", "regex", "200")
	if success.First("property", "name", "TRANSPORT_HEADERS", "action", "remove") == nil {
		t.Error("backend headers are not dropped")
	}
	if h := success.First("header", "name", "Cache-Control"); h == nil || h.Attr("value") != "no-store" {
		t.Error("success response must be Cache-Control: no-store")
	}
}

// Overriding messageType would force MI to parse and re-serialise the body.
func TestSuccessBodyIsNotRebuilt(t *testing.T) {
	success := load(t).proxy.First("case", "regex", "200")
	if len(success.PropertiesSet("messageType")) != 0 {
		t.Error("success path must not set messageType")
	}
}

func TestBackendURLComesFromEnvironment(t *testing.T) {
	f := load(t)
	prop := f.proxy.First("property", "name", "uri.var.customerBackendUrl")
	if prop == nil || prop.Attr("expression") != "get-property('env', 'CUSTOMER_BACKEND_URL')" {
		t.Error("backend base URL must come from CUSTOMER_BACKEND_URL")
	}
	if tpl := f.endpoint.Child("http").Attr("uri-template"); !strings.HasPrefix(tpl, "{+uri.var.customerBackendUrl}") {
		t.Errorf("base URL must use reserved expansion, got %s", tpl)
	}
}

func TestTimeoutIsFault(t *testing.T) {
	timeout := load(t).endpoint.First("timeout")
	if d := strings.TrimSpace(timeout.Child("duration").Text); d != "5000" {
		t.Errorf("timeout = %s, want 5000", d)
	}
	if a := strings.TrimSpace(timeout.Child("responseAction").Text); a != "fault" {
		t.Errorf("responseAction = %s, want fault", a)
	}
}

func TestFaultMapping(t *testing.T) {
	f := load(t)
	mapping := f.fault.Child("filter", "regex", "MAPPING")
	if mapping == nil || strings.Join(mapping.PropertiesSet("ERROR_STATUS"), ",") != "502" {
		t.Error("response read failures (STAGE=MAPPING) must be 502")
	}
	statuses := map[string]string{}
	for _, c := range f.fault.All("case") {
		statuses[c.Attr("regex")] = c.PropertiesSet("ERROR_STATUS")[0]
	}
	if statuses["101504"] != "504" {
		t.Error("timeout (101504) must be 504")
	}
	for pattern, status := range statuses {
		if pattern == "101504" {
			continue
		}
		re := regexp.MustCompile("^(?:" + pattern + ")$")
		for _, code := range []string{"101503", "101505", "303001"} {
			if !re.MatchString(code) {
				t.Errorf("%s not covered by %s", code, pattern)
			}
		}
		if status != "503" {
			t.Errorf("connection failures must be 503, got %s", status)
		}
	}
}

func TestUpstreamStatuses(t *testing.T) {
	codes := load(t).proxy.PropertiesSet("ERROR_CODE_APP")
	for _, c := range []string{"CUSTOMER_NOT_FOUND", "CUSTOMER_BACKEND_ERROR", "INVALID_CUSTOMER_ID"} {
		if !support.Contains(codes, c) {
			t.Errorf("missing %s", c)
		}
	}
}
