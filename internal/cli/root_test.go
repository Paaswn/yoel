package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"yoel/internal/graderapi"
)

func TestNewRootCommandShowsHelpWithoutSubcommands(t *testing.T) {
	command := NewRootCommand()
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte("Usage:")) {
		t.Fatalf("help output = %q, want Usage", output.String())
	}
}

func TestNewRootCommandIncludesLoginCommand(t *testing.T) {
	root := NewRootCommand()
	login, _, err := root.Find([]string{"login"})
	if err != nil {
		t.Fatalf("Find(login) error = %v", err)
	}
	if login == root {
		t.Fatal("Find(login) returned the root command")
	}
	if login.Use != "login" {
		t.Fatalf("login.Use = %q, want %q", login.Use, "login")
	}
	for _, name := range []string{"login", "username", "password"} {
		if login.Flags().Lookup(name) != nil {
			t.Fatalf("login command exposes credential flag --%s", name)
		}
	}
}

func TestLoginCommandCallsGraderAPI(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request["login"] != "fake-username" || request["password"] != "fake-password" {
			t.Errorf("request body = %#v", request)
		}
		_, _ = w.Write([]byte(`{"token":"fake-token","expires_at":"2030-01-02T03:04:05Z","user":{"id":7,"login":"fake-login","full_name":"Fake Student"}}`))
	}))
	defer server.Close()

	credentials := []string{"fake-username", "fake-password"}
	var labels []string
	promptIndex := 0
	command := newRootCommand(func(_ *cobra.Command, label string) (string, error) {
		labels = append(labels, label)
		credential := credentials[promptIndex]
		promptIndex++
		return credential, nil
	})
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{
		"login",
		"--base-url", server.URL,
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := output.String(); got != "login successful\n" {
		t.Fatalf("output = %q", got)
	}
	if bytes.Contains(output.Bytes(), []byte("fake-token")) {
		t.Fatal("output contains the bearer token")
	}
	if bytes.Contains(output.Bytes(), []byte("fake-username")) || bytes.Contains(output.Bytes(), []byte("fake-password")) {
		t.Fatal("output contains credentials")
	}
	if len(labels) != 2 || labels[0] != "Username: " || labels[1] != "Password: " {
		t.Fatalf("prompt labels = %#v", labels)
	}
	session, err := loadSession(time.Now())
	if err != nil {
		t.Fatalf("load saved session: %v", err)
	}
	if session.Token != "fake-token" || session.BaseURL != server.URL || session.User.ID != 7 {
		t.Fatalf("saved session = %#v", session)
	}
	info, err := os.Stat(filepath.Join(configDir, applicationDir, sessionFilename))
	if err != nil {
		t.Fatalf("stat saved session: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("session permissions = %o, want 600", got)
	}
}

func TestQuestionListCachesAndRefreshesProblems(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var requests atomic.Int32
	questionName := "arrays"
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/problems" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = fmt.Fprintf(w, `[{"id":42,"name":%q,"full_name":"Array Problem","difficulty":3},{"id":43,"name":"unknown","full_name":"Unknown Difficulty","difficulty":null}]`, questionName)
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-token",
		ExpiresAt: time.Now().Add(time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	firstOutput := executeQuestionList(t)
	if firstOutput != "Question_Name Id Difficulty\narrays 42 3\nunknown 43 -\n" {
		t.Fatalf("first output = %q", firstOutput)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests after first list = %d, want 1", got)
	}

	secondOutput := executeQuestionList(t)
	if secondOutput != firstOutput {
		t.Fatalf("cached output = %q, want %q", secondOutput, firstOutput)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("cached list made a server request; count = %d", got)
	}

	cache, err := loadQuestionCache()
	if err != nil {
		t.Fatal(err)
	}
	cache.FetchedAt = time.Now().Add(-questionCacheTTL - time.Minute)
	if err := saveQuestionCache(cache); err != nil {
		t.Fatal(err)
	}
	questionName = "graphs"

	refreshedOutput := executeQuestionList(t)
	if refreshedOutput != "Question_Name Id Difficulty\ngraphs 42 3\nunknown 43 -\n" {
		t.Fatalf("refreshed output = %q", refreshedOutput)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after stale cache = %d, want 2", got)
	}
}

func executeQuestionList(t *testing.T) string {
	t.Helper()
	command := newRootCommand(func(_ *cobra.Command, _ string) (string, error) {
		t.Fatal("question list unexpectedly prompted for credentials")
		return "", nil
	})
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"question", "list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question list: %v", err)
	}
	return output.String()
}

func newCLITestServer(handler http.Handler) *httptest.Server {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	return server
}
