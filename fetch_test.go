package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/automa-saga/logx"
	"github.com/spf13/cobra"
)

type rewriteTransport struct {
	rt     http.RoundTripper
	target *url.URL
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the original
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = t.target.Scheme
	newReq.URL.Host = t.target.Host
	// ensure Host header points to target
	newReq.Host = t.target.Host
	return t.rt.RoundTrip(newReq)
}

func TestFetchPRs_JSON(t *testing.T) {
	// Mock GitHub API server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"full_name":"alice/repo1","name":"repo1","owner":{"login":"alice"}}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/alice/repo1/pulls"):
			w.Header().Set("Content-Type", "application/json")
			prs := []map[string]interface{}{
				{
					"number":     1,
					"title":      "Test PR",
					"html_url":   "https://github.com/alice/repo1/pull/1",
					"user":       map[string]string{"login": "alice"},
					"created_at": "2023-06-01T12:00:00Z",
				},
			}
			_ = json.NewEncoder(w).Encode(prs)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Replace default transport to route requests to the test server
	target, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}
	oldTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{rt: oldTransport, target: target}
	defer func() { http.DefaultTransport = oldTransport }()

	// Set environment variables expected by fetchPRs
	os.Setenv("GITHUB_TOKEN", "fake-token")
	os.Setenv("GITHUB_USERNAME", "alice")
	os.Setenv("REPOS", "alice/repo1")
	// Dates inclusive range covering the PR created_at
	// We'll still pass args to fetchPRs, but set envs in case
	os.Setenv("START_DATE", "2023-01-01")
	os.Setenv("END_DATE", "2023-12-31")

	// Initialize logger (as main does)
	_ = logx.Initialize(logx.LoggingConfig{
		Level:          "debug",
		ConsoleLogging: true,
	})

	// Prepare command and set output flag to json
	cmd := &cobra.Command{}
	cmd.Flags().StringP("output", "o", "plain", "Output format")
	_ = cmd.Flags().Set("output", "json")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function under test
	err = fetchPRs(cmd, []string{"2023-01-01", "2023-12-31"})

	// Restore stdout and read captured output
	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("fetchPRs returned error: %v\noutput: %s", err, buf.String())
	}

	// Parse and assert JSON output
	var got []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal output as JSON: %v\nraw: %s", err, buf.String())
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 PR in output, got %d: %v", len(got), got)
	}
	if got[0]["number"] != "1" {
		t.Fatalf("expected PR number '1', got %q", got[0]["number"])
	}
	if got[0]["repo"] != "alice/repo1" {
		t.Fatalf("expected repo 'alice/repo1', got %q", got[0]["repo"])
	}
}
