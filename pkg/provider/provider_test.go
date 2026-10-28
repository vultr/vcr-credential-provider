package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vultr/govultr/v3"
)

func testDriver(t *testing.T, handler http.HandlerFunc) *Driver {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := govultr.NewClient(server.Client())
	if err := client.SetBaseURL(server.URL); err != nil {
		t.Fatal(err)
	}
	return &Driver{Client: client}
}

func requestJSON(image string) string {
	return fmt.Sprintf(`{"apiVersion":%q,"kind":"CredentialProviderRequest","image":%q}`, apiVersion, image)
}

func TestRun(t *testing.T) {
	listCalls, credentialCalls := 0, 0
	driver := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/registries":
			listCalls++
			if r.Method != http.MethodGet || r.URL.Query().Get("per_page") != "300" {
				t.Errorf("unexpected list request: %s %s", r.Method, r.URL)
			}
			switch r.URL.Query().Get("cursor") {
			case "":
				_, _ = fmt.Fprint(w, `{"registries":[{"id":"other","name":"other"}],"meta":{"links":{"next":"page2"}}}`)
			case "page2":
				// Stop on a match even if the API has more pages.
				_, _ = fmt.Fprint(w, `{"registries":[{"id":"wanted","name":"team"}],"meta":{"links":{"next":"page3"}}}`)
			default:
				t.Error("unnecessary page requested")
				http.Error(w, "unexpected page", http.StatusBadRequest)
			}
		case "/v2/registry/wanted/docker-credentials":
			credentialCalls++
			// Exact expiry serialization is a known upstream govultr bug.
			if r.Method != http.MethodOptions || r.URL.Query().Get("expiry_seconds") == "" || r.URL.Query().Get("read_write") != "false" {
				t.Errorf("unexpected credentials request: %s %s", r.Method, r.URL)
			}
			_, _ = fmt.Fprintf(w, `{"auths":{"ewr.vultrcr.com":{"auth":%q}}}`, base64.StdEncoding.EncodeToString([]byte("robot:password:with:colons")))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	var output bytes.Buffer
	if err := driver.Run(context.Background(), strings.NewReader(requestJSON("ewr.vultrcr.com/team/nested/app:v1")), &output); err != nil {
		t.Fatal(err)
	}
	var response CredentialProviderResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.APIVersion != apiVersion || response.Kind != "CredentialProviderResponse" || response.CacheKeyType != "Image" {
		t.Fatalf("unexpected response metadata: %+v", response)
	}
	duration, err := time.ParseDuration(response.CacheDuration)
	if err != nil || duration <= 0 || duration > 12*time.Hour-5*time.Minute {
		t.Fatalf("cache must expire before credentials: %q", response.CacheDuration)
	}
	if len(response.Auth) != 1 || response.Auth["ewr.vultrcr.com/team"] != (AuthConfig{Username: "robot", Password: "password:with:colons"}) {
		t.Fatal("credentials must be scoped to the VCR namespace and preserve password colons")
	}
	if listCalls != 2 || credentialCalls != 1 {
		t.Fatalf("unexpected calls: list=%d credentials=%d", listCalls, credentialCalls)
	}
}

func TestInvalidRequests(t *testing.T) {
	for _, input := range []string{
		"", "{", "null", requestJSON("ewr.vultrcr.com/team/app") + "{}",
		`{"apiVersion":"wrong","kind":"CredentialProviderRequest","image":"ewr.vultrcr.com/team/app"}`,
		`{"apiVersion":"credentialprovider.kubelet.k8s.io/v1","kind":"wrong","image":"ewr.vultrcr.com/team/app"}`,
		requestJSON(""), requestJSON("nginx"), requestJSON("ewr.vultrcr.com/team"),
		requestJSON("https://ewr.vultrcr.com/team/app"), requestJSON("example.com/team/app"),
		requestJSON("ewr.vultrcr.com//app"), requestJSON("ewr.vultrcr.com/team/"),
		requestJSON("user@ewr.vultrcr.com/team/app"), requestJSON("ewr.vultrcr.com/../app"),
	} {
		t.Run(input, func(t *testing.T) {
			var output bytes.Buffer
			// A nil client also ensures validation happens before any API request.
			if err := (&Driver{}).Run(context.Background(), strings.NewReader(input), &output); err == nil {
				t.Fatal("expected invalid request error")
			}
			if output.Len() != 0 {
				t.Fatal("errors must not write to stdout")
			}
		})
	}
}

func TestParseImage(t *testing.T) {
	for _, image := range []string{
		"ewr.vultrcr.com/team/app:latest",
		"ewr.vultrcr.com/team/nested/app@sha256:" + strings.Repeat("a", 64),
	} {
		host, registry, err := parseImage(image)
		if err != nil || host != "ewr.vultrcr.com" || registry != "team" {
			t.Fatalf("parseImage(%q) = %q, %q, %v", image, host, registry, err)
		}
	}
}

func TestAPIFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		list    string
		creds   string
		status  int
		wantErr string
	}{
		{name: "not found", list: `{"registries":[],"meta":{"links":{"next":""}}}`, wantErr: "not found"},
		{name: "missing meta", list: `{"registries":[]}`, wantErr: "not found"},
		{name: "missing links", list: `{"registries":[],"meta":{}}`, wantErr: "not found"},
		{name: "repeated cursor", list: `{"registries":[],"meta":{"links":{"next":"same"}}}`, wantErr: "repeated pagination"},
		{name: "unauthorized", status: http.StatusUnauthorized, wantErr: "list registries"},
		{name: "credentials denied", creds: "denied", status: http.StatusForbidden, wantErr: "create registry credentials"},
		{name: "missing host", creds: `{"auths":{}}`, wantErr: "no auth"},
		{name: "invalid base64", creds: `{"auths":{"ewr.vultrcr.com":{"auth":"!"}}}`, wantErr: "invalid base64"},
		{name: "missing separator", creds: `{"auths":{"ewr.vultrcr.com":{"auth":"c2VjcmV0"}}}`, wantErr: "non-empty username and password"},
		{name: "empty password", creds: `{"auths":{"ewr.vultrcr.com":{"auth":"dXNlcjo="}}}`, wantErr: "non-empty username and password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driver := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v2/registries" {
					if tc.list != "" {
						_, _ = fmt.Fprint(w, tc.list)
					} else if tc.creds != "" {
						_, _ = fmt.Fprint(w, `{"registries":[{"id":"wanted","name":"team"}]}`)
					} else {
						w.WriteHeader(tc.status)
					}
					return
				}
				if tc.creds == "" {
					t.Error("must not request credentials for an unknown registry")
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = fmt.Fprint(w, tc.creds)
			})
			var output bytes.Buffer
			err := driver.Run(context.Background(), strings.NewReader(requestJSON("ewr.vultrcr.com/team/app")), &output)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
			if output.Len() != 0 {
				t.Fatal("failed requests must not produce credentials")
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	driver := testDriver(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	if err := driver.Run(ctx, strings.NewReader(requestJSON("ewr.vultrcr.com/team/app")), &output); err == nil {
		t.Fatal("expected context deadline error")
	}
}
