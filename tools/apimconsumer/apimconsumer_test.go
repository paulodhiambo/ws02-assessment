package apimconsumer

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeAPIM implements just enough of the APIM REST APIs for one onboarding run.
type fakeAPIM struct {
	mu            sync.Mutex
	subscriptions map[string]string // apiId -> tier
	appCreated    bool
}

func (f *fakeAPIM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	body, _ := io.ReadAll(r.Body)
	p := r.URL.Path
	switch {
	case p == "/client-registration/v0.17/register":
		reply(map[string]string{"clientId": "cid", "clientSecret": "csecret"})
	case p == "/oauth2/token":
		if strings.Contains(string(body), "client_credentials") {
			reply(map[string]string{"access_token": "app-token"})
		} else {
			reply(map[string]string{"access_token": "portal-token"})
		}
	case p == "/api/am/devportal/v3/applications" && r.Method == http.MethodGet:
		reply(map[string]any{"list": []any{}})
	case p == "/api/am/devportal/v3/applications" && r.Method == http.MethodPost:
		f.appCreated = true
		reply(map[string]string{"applicationId": "app-1", "name": AppName})
	case p == "/api/am/devportal/v3/subscriptions" && r.Method == http.MethodGet:
		reply(map[string]any{"list": []any{}})
	case p == "/api/am/devportal/v3/subscriptions" && r.Method == http.MethodPost:
		var s struct{ APIID, ThrottlingPolicy string }
		_ = json.Unmarshal(body, &s)
		f.subscriptions[s.APIID] = s.ThrottlingPolicy
		reply(map[string]string{})
	case p == "/api/am/devportal/v3/apis":
		name := strings.TrimPrefix(r.URL.Query().Get("query"), "name:")
		reply(map[string]any{"list": []map[string]string{{"id": "id-" + name, "name": name}}})
	case strings.HasSuffix(p, "/oauth-keys"):
		reply(map[string]any{"list": []any{}})
	case strings.HasSuffix(p, "/generate-keys"):
		reply(map[string]string{"keyType": "PRODUCTION", "consumerKey": "ck", "consumerSecret": "cs"})
	case strings.HasSuffix(p, "/api-keys/PRODUCTION/generate"):
		reply(map[string]string{"apikey": "the-api-key"})
	default:
		http.Error(w, "unexpected "+r.Method+" "+p, http.StatusNotFound)
	}
}

func TestRunOnboardsAppSubscriptionsAndKeys(t *testing.T) {
	fake := &fakeAPIM{subscriptions: map[string]string{}}
	srv := httptest.NewTLSServer(fake)
	defer srv.Close()

	envFile := filepath.Join(t.TempDir(), "apim.env.json")
	res, err := Run(Config{APIM: srv.URL, Gateway: "https://gw", User: "admin", Password: "admin", EnvFile: envFile, Log: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if res.AccessToken != "app-token" || res.APIKey != "the-api-key" {
		t.Fatalf("unexpected result %+v", res)
	}
	if !fake.appCreated {
		t.Error("application was not created")
	}
	for _, s := range Subscriptions {
		if fake.subscriptions["id-"+s.API] != s.Tier {
			t.Errorf("%s: subscribed on %q, want %q", s.API, fake.subscriptions["id-"+s.API], s.Tier)
		}
	}
	env, _ := os.ReadFile(envFile)
	if !strings.Contains(string(env), `"value": "app-token"`) || !strings.Contains(string(env), `"value": "the-api-key"`) {
		t.Errorf("env file missing credentials:\n%s", env)
	}
	if cmds := Commands("https://gw", res); !strings.HasPrefix(cmds, "export GW=https://gw/jamii TOKEN=app-token APIKEY=the-api-key") {
		t.Errorf("unexpected commands:\n%s", cmds)
	}
}

func TestRunFailsWhenAPIIsNotPublished(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/am/devportal/v3/apis" {
			_, _ = io.WriteString(w, `{"list":[]}`)
			return
		}
		(&fakeAPIM{subscriptions: map[string]string{}}).ServeHTTP(w, r)
	}))
	defer srv.Close()
	_, err := Run(Config{APIM: srv.URL, User: "admin", Password: "admin", Log: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "not published") {
		t.Fatalf("want 'not published' error, got %v", err)
	}
}
