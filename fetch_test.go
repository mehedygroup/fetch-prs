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
	"sync"
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

var testLoggerOnce sync.Once

func TestFetchPRs_JSONIncludesPRsAndCommitClosedIssues(t *testing.T) {
	ts := newMockGitHubServer(t)
	defer ts.Close()
	configureGitHubTransport(t, ts.URL)
	configureTestEnv(t)
	initializeTestLogger()

	cmd := newTestFetchCommand(t)
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("failed to set json flag: %v", err)
	}

	output, err := executeFetchPRs(t, cmd, []string{"2023-01-01", "2023-12-31"})
	if err != nil {
		t.Fatalf("fetchPRs returned error: %v\noutput: %s", err, output)
	}

	var got []WorkItem
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("failed to unmarshal output as JSON: %v\nraw: %s", err, output)
	}

	want := []WorkItem{
		{
			Type:   "pr",
			Status: "done",
			Number: "1",
			Title:  "Test PR",
			Repo:   "alice/repo1",
			URL:    "https://github.com/alice/repo1/pull/1",
		},
		{
			Type:   "pr",
			Status: "wip",
			Number: "3",
			Title:  "In progress PR",
			Repo:   "alice/repo1",
			URL:    "https://github.com/alice/repo1/pull/3",
		},
		{
			Type:      "issue_commit",
			Status:    "done",
			Number:    "42",
			Title:     "Direct fix issue",
			Repo:      "alice/repo1",
			URL:       "https://github.com/alice/repo1/issues/42",
			CommitSHA: "abc1234567890",
		},
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d work items, got %d: %v", len(want), len(got), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected work item at index %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestFetchPRs_PlainFlagFormatsInvoiceLines(t *testing.T) {
	ts := newMockGitHubServer(t)
	defer ts.Close()
	configureGitHubTransport(t, ts.URL)
	configureTestEnv(t)
	initializeTestLogger()

	cmd := newTestFetchCommand(t)
	if err := cmd.Flags().Set("plain", "true"); err != nil {
		t.Fatalf("failed to set plain flag: %v", err)
	}

	output, err := executeFetchPRs(t, cmd, []string{"2023-01-01", "2023-12-31"})
	if err != nil {
		t.Fatalf("fetchPRs returned error: %v\noutput: %s", err, output)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	want := []string{
		"[done] alice/repo1: Test PR, #1",
		"[wip] alice/repo1: In progress PR, #3",
		"[done] alice/repo1: Direct fix issue, #42 (commit abc1234)",
	}

	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %v", len(want), len(lines), lines)
	}

	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("unexpected plain output at index %d: got %q want %q", i, lines[i], want[i])
		}
	}
}

func TestFetchPRs_RejectsConflictingOutputFlags(t *testing.T) {
	cmd := newTestFetchCommand(t)
	if err := cmd.Flags().Set("plain", "true"); err != nil {
		t.Fatalf("failed to set plain flag: %v", err)
	}
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("failed to set json flag: %v", err)
	}

	err := fetchPRs(cmd, []string{"2023-01-01", "2023-12-31"})
	if err == nil {
		t.Fatal("expected conflicting output flags to return an error")
	}

	if !strings.Contains(err.Error(), "cannot use --plain and --json together") {
		t.Fatalf("unexpected error for conflicting output flags: %v", err)
	}
}

func newMockGitHubServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1":
			fmt.Fprint(w, `{"full_name":"alice/repo1","name":"repo1","owner":{"login":"alice"}}`)
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1/pulls":
			fmt.Fprint(w, `[
				{"number":1,"title":"Test PR","html_url":"https://github.com/alice/repo1/pull/1","state":"closed","user":{"login":"alice"},"created_at":"2023-01-01T00:00:00Z","updated_at":"2023-07-01T00:00:00Z","merged_at":"2023-07-01T00:00:00Z"},
				{"number":3,"title":"In progress PR","html_url":"https://github.com/alice/repo1/pull/3","state":"open","user":{"login":"alice"},"created_at":"2022-12-20T00:00:00Z","updated_at":"2023-06-15T09:00:00Z"},
				{"number":2,"title":"Someone else's PR","html_url":"https://github.com/alice/repo1/pull/2","user":{"login":"bob"},"created_at":"2023-06-01T12:00:00Z"}
			]`)
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1/issues":
			fmt.Fprint(w, `[
				{"number":42,"title":"Direct fix issue","html_url":"https://github.com/alice/repo1/issues/42","closed_at":"2023-06-02T10:00:00Z"},
				{"number":43,"title":"Closed by someone else","html_url":"https://github.com/alice/repo1/issues/43","closed_at":"2023-06-03T10:00:00Z"},
				{"number":44,"title":"Actually a PR","html_url":"https://github.com/alice/repo1/pull/44","closed_at":"2023-06-04T10:00:00Z","pull_request":{"url":"https://api.github.com/repos/alice/repo1/pulls/44"}}
			]`)
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1/issues/42/timeline":
			fmt.Fprint(w, `[
				{"event":"referenced","commit_id":"abc1234567890"},
				{"event":"closed","commit_id":"abc1234567890"}
			]`)
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1/issues/43/timeline":
			fmt.Fprint(w, `[
				{"event":"closed","commit_id":"def9876543210"}
			]`)
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1/commits/abc1234567890":
			fmt.Fprint(w, `{"sha":"abc1234567890","author":{"login":"alice"},"commit":{"message":"Fixes #42"}}`)
		case r.Method == "GET" && r.URL.Path == "/repos/alice/repo1/commits/def9876543210":
			fmt.Fprint(w, `{"sha":"def9876543210","author":{"login":"bob"},"commit":{"message":"Fixes #43"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func configureGitHubTransport(t *testing.T, serverURL string) {
	t.Helper()

	target, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	oldTransport := http.DefaultTransport
	http.DefaultTransport = &rewriteTransport{rt: oldTransport, target: target}
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
}

func configureTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "fake-token")
	t.Setenv("GITHUB_USERNAME", "alice")
	t.Setenv("REPOS", "alice/repo1")
	t.Setenv("START_DATE", "2023-01-01")
	t.Setenv("END_DATE", "2023-12-31")
}

func initializeTestLogger() {
	testLoggerOnce.Do(func() {
		_ = logx.Initialize(logx.LoggingConfig{
			Level:          "debug",
			ConsoleLogging: true,
		})
	})
}

func newTestFetchCommand(t *testing.T) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().StringP("output", "o", "plain", "Legacy output format")
	cmd.Flags().Bool("plain", false, "Output invoice-friendly plain text")
	cmd.Flags().Bool("json", false, "Output work items as JSON")
	return cmd
}

func executeFetchPRs(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = w

	fetchErr := fetchPRs(cmd, args)
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return strings.TrimSpace(buf.String()), fetchErr
}
