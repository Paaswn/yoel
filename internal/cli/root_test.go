package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yoel/internal/graderapi"

	"github.com/spf13/cobra"
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

func TestNewRootCommandIncludesUpdateCommand(t *testing.T) {
	root := NewRootCommandWithVersion("v0.2.0")
	update, _, err := root.Find([]string{"update"})
	if err != nil {
		t.Fatalf("Find(update) error = %v", err)
	}
	if update == root || update.Use != "update" {
		t.Fatalf("update command = %#v", update)
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

	command := newRootCommand(func(_ *cobra.Command) (string, string, error) {
		return "fake-username", "fake-password", nil
	})
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetArgs([]string{
		"login",
		"--base-url", server.URL,
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stdout.String(); got != "✓ Login successful as fake-login\n" {
		t.Fatalf("output = %q", got)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("fake-token")) {
		t.Fatal("output contains the bearer token")
	}
	if bytes.Contains(stdout.Bytes(), []byte("fake-username")) || bytes.Contains(stdout.Bytes(), []byte("fake-password")) {
		t.Fatal("output contains credentials")
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

func TestLoginCommandReportsAuthenticationFailureWithoutSecrets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, "fake-private-response")
	}))
	defer server.Close()

	command := newRootCommand(func(_ *cobra.Command) (string, string, error) {
		return "fake-username", "fake-password", nil
	})
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"login", "--base-url", server.URL})

	err := command.Execute()
	if !errors.Is(err, graderapi.ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	for _, secret := range []string{"fake-username", "fake-password", "fake-private-response"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error exposes %q: %v", secret, err)
		}
	}
}

func TestLoginCommandStopsWhenFormFails(t *testing.T) {
	formErr := errors.New("fake form canceled")
	command := newRootCommand(func(_ *cobra.Command) (string, string, error) {
		return "", "", formErr
	})
	command.SetArgs([]string{"login"})

	err := command.Execute()
	if !errors.Is(err, formErr) {
		t.Fatalf("error = %v, want form cancellation", err)
	}
}

func TestQuestionListReloginsAndContinuesWithRefreshedToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TERM", "dumb")
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	var loginRequests atomic.Int32
	var problemRequests atomic.Int32
	var pdfRequests atomic.Int32
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginRequests.Add(1)
			if r.Method != http.MethodPost {
				t.Errorf("login method = %s", r.Method)
			}
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode login request: %v", err)
			}
			if request["login"] != "fake-username" || request["password"] != "fake-password" {
				t.Errorf("login request = %#v", request)
			}
			_, _ = fmt.Fprintln(w, `{"token":"fake-refreshed-token","expires_at":"2030-01-02T03:04:05Z","user":{"id":7,"login":"fake-login","full_name":"Fake Student"}}`)
		case "/api/v1/problems":
			problemRequests.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer fake-refreshed-token" {
				t.Errorf("Authorization = %q, want refreshed token", got)
			}
			_, _ = fmt.Fprintln(w, `[{"id":42,"name":"arrays","full_name":"Array Problem","difficulty":3}]`)
		case "/api/v1/problems/42/files/pdf":
			pdfRequests.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer fake-refreshed-token" {
				t.Errorf("PDF Authorization = %q, want refreshed token", got)
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = fmt.Fprintln(w, "%PDF-1.7")
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-expired-token",
		ExpiresAt: time.Now().Add(-time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	var confirmations atomic.Int32
	command := newRootCommandWithDependencies(
		func(_ *cobra.Command) (string, string, error) {
			return "fake-username", "fake-password", nil
		},
		func(_ *cobra.Command) (bool, error) {
			confirmations.Add(1)
			return true, nil
		},
		func(string) error { return nil },
	)
	output := new(bytes.Buffer)
	command.SetIn(strings.NewReader("1\n"))
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"question", "list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question list after re-login: %v", err)
	}

	if confirmations.Load() != 1 || loginRequests.Load() != 1 || problemRequests.Load() != 1 || pdfRequests.Load() != 1 {
		t.Fatalf("confirmations=%d login requests=%d problem requests=%d PDF requests=%d, want 1 each", confirmations.Load(), loginRequests.Load(), problemRequests.Load(), pdfRequests.Load())
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, "arrays", "42.cpp")); err != nil {
		t.Fatalf("stat interactively created source file: %v", err)
	}
	if !strings.Contains(output.String(), "✓ Login successful as fake-login") || !strings.Contains(output.String(), "arrays") {
		t.Fatalf("output = %q, want login success and question list", output.String())
	}
	for _, secret := range []string{"fake-password", "fake-refreshed-token", "fake-expired-token"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("output exposes %q", secret)
		}
	}
	session, err := loadStoredSession()
	if err != nil {
		t.Fatal(err)
	}
	if session.Token != "fake-refreshed-token" || session.BaseURL != server.URL {
		t.Fatalf("saved refreshed session = %#v", session)
	}
}

func TestQuestionListDeclinesReloginWithoutOpeningForm(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	if err := saveSession(storedSession{
		BaseURL:   "https://grader.example",
		Token:     "fake-expired-token",
		ExpiresAt: time.Now().Add(-time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	command := newRootCommandWithDependencies(
		func(_ *cobra.Command) (string, string, error) {
			t.Fatal("declined re-login unexpectedly opened credential form")
			return "", "", nil
		},
		func(_ *cobra.Command) (bool, error) { return false, nil },
		func(string) error { return nil },
	)
	command.SetArgs([]string{"question", "list"})
	if err := command.Execute(); !errors.Is(err, errReloginDeclined) {
		t.Fatalf("error = %v, want errReloginDeclined", err)
	}
}

func TestQuestionListReportsFailedReloginWithoutSecrets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, "fake-private-response")
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-expired-token",
		ExpiresAt: time.Now().Add(-time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	command := newRootCommandWithDependencies(
		func(_ *cobra.Command) (string, string, error) {
			return "fake-username", "fake-password", nil
		},
		func(_ *cobra.Command) (bool, error) { return true, nil },
		func(string) error { return nil },
	)
	command.SetArgs([]string{"question", "list"})
	err := command.Execute()
	if !errors.Is(err, graderapi.ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	for _, secret := range []string{"fake-username", "fake-password", "fake-expired-token", "fake-private-response"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error exposes %q: %v", secret, err)
		}
	}
}

func TestQuestionListCachesAndRefreshesProblems(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TERM", "dumb")
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	var requests atomic.Int32
	questionName := "arrays"
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/v1/problems":
			requests.Add(1)
			if r.Method != http.MethodGet {
				t.Errorf("problem request method = %s", r.Method)
			}
			_, _ = fmt.Fprintf(w, `[{"id":42,"name":%q,"full_name":"Array Problem","difficulty":3},{"id":43,"name":"unknown","full_name":"Unknown Difficulty","difficulty":null}]`, questionName)
		case "/api/v1/problems/42/files/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = fmt.Fprintln(w, "%PDF-1.7")
		default:
			t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
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
	for _, expected := range []string{"Questions List", "arrays", "unknown"} {
		if !strings.Contains(firstOutput, expected) {
			t.Fatalf("first output %q does not contain %q", firstOutput, expected)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests after first list = %d, want 1", got)
	}
	if err := os.RemoveAll(filepath.Join(workingDirectory, "arrays")); err != nil {
		t.Fatal(err)
	}

	secondOutput := executeQuestionList(t)
	if !strings.Contains(secondOutput, "arrays") {
		t.Fatalf("cached output = %q, want arrays option", secondOutput)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("cached list made a server request; count = %d", got)
	}
	if err := os.RemoveAll(filepath.Join(workingDirectory, "arrays")); err != nil {
		t.Fatal(err)
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
	if !strings.Contains(refreshedOutput, "graphs") || strings.Contains(refreshedOutput, "arrays") {
		t.Fatalf("refreshed output = %q, want updated graphs option", refreshedOutput)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after stale cache = %d, want 2", got)
	}
}

func TestQuestionListCreatesSelectedQuestion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TERM", "dumb")
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	pdf := []byte("%PDF-1.7\nselected question")
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/problems":
			_, _ = fmt.Fprintln(w, `[{"id":42,"name":"arrays","full_name":"Array Problem"},{"id":43,"name":"graphs","full_name":"Graph Problem"}]`)
		case "/api/v1/problems/43/files/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(pdf)
		default:
			t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
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

	var openedPath string
	command := newRootCommandWithOpener(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("question list unexpectedly prompted for credentials")
		return "", "", nil
	}, func(path string) error {
		openedPath = path
		return nil
	})
	command.SetIn(strings.NewReader("2\n"))
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"question", "list", "--language", "go"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question list: %v", err)
	}

	source, err := os.ReadFile(filepath.Join(workingDirectory, "graphs", "43.go"))
	if err != nil {
		t.Fatalf("read interactively created source file: %v", err)
	}
	if got, want := string(source), "// --- Automatically Created by yoel ---\n#include <iostream>\nusing namespace std;\n\nint main(){\n\n}"; got != want {
		t.Fatalf("created source = %q, want %q", got, want)
	}
	if filepath.Ext(openedPath) != ".pdf" {
		t.Fatalf("opened path = %q, want cached PDF path", openedPath)
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, "arrays", "42.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unselected question source exists or returned unexpected error: %v", err)
	}
}

func TestShowQuestionsRejectsEmptyList(t *testing.T) {
	command := &cobra.Command{}
	if _, err := showQuestions(command, nil); err == nil || err.Error() != "no accessible questions" {
		t.Fatalf("showQuestions() error = %v, want no accessible questions", err)
	}
}

func TestQuestionShowResolvesDownloadsCachesAndRefreshesPDF(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var downloads atomic.Int32
	pdf := []byte("%PDF-1.7\nfirst version")
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/problems/673/files/pdf" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/pdf" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="server-name.pdf"`)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	defer server.Close()

	session := storedSession{
		BaseURL:   server.URL,
		Token:     "fake-token",
		ExpiresAt: time.Now().Add(time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}
	if err := saveSession(session); err != nil {
		t.Fatal(err)
	}
	if err := saveQuestionCache(questionCache{
		BaseURL:   server.URL,
		UserID:    7,
		FetchedAt: time.Now(),
		Problems:  []graderapi.Problem{{ID: 673, Name: "arrays"}},
	}); err != nil {
		t.Fatal(err)
	}

	var openedPaths []string
	opener := func(path string) error {
		openedPaths = append(openedPaths, path)
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(contents, pdf) {
			return fmt.Errorf("opened PDF = %q", contents)
		}
		return nil
	}

	executeQuestionShow(t, opener, "--name", "ArRaYs")
	if got := downloads.Load(); got != 1 {
		t.Fatalf("downloads after first show = %d, want 1", got)
	}
	if len(openedPaths) != 1 || filepath.Ext(openedPaths[0]) != ".pdf" {
		t.Fatalf("opened paths = %#v", openedPaths)
	}

	session.ExpiresAt = time.Now().Add(-time.Minute)
	if err := saveSession(session); err != nil {
		t.Fatal(err)
	}
	executeQuestionShow(t, opener, "673")
	if got := downloads.Load(); got != 1 {
		t.Fatalf("cached show downloaded again; count = %d", got)
	}

	session.ExpiresAt = time.Now().Add(time.Hour)
	if err := saveSession(session); err != nil {
		t.Fatal(err)
	}
	pdf = []byte("%PDF-1.7\nsecond version")
	executeQuestionShow(t, opener, "673", "--refresh")
	if got := downloads.Load(); got != 2 {
		t.Fatalf("downloads after refresh = %d, want 2", got)
	}
}

func TestQuestionNewCreatesSourceAndOpensPDF(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	pdf := []byte("%PDF-1.7\nnew question statement")
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/v1/problems":
			_, _ = fmt.Fprintln(w, `[{"id":673,"name":"arrays","full_name":"Array Problem","has_attachment":false}]`)
		case "/api/v1/problems/673/files/pdf":
			if got := r.Header.Get("Accept"); got != "application/pdf" {
				t.Errorf("Accept = %q", got)
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(pdf)
		default:
			t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
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

	var openedPath string
	opener := func(path string) error {
		openedPath = path
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(contents, pdf) {
			return fmt.Errorf("opened PDF = %q", contents)
		}
		return nil
	}

	command := newRootCommandWithOpener(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("question new unexpectedly prompted for credentials")
		return "", "", nil
	}, opener)
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"question", "new", "--id", "673"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question new: %v", err)
	}

	source, err := os.ReadFile(filepath.Join(workingDirectory, "arrays", "673.cpp"))
	if err != nil {
		t.Fatalf("read created source file: %v", err)
	}
	if got, want := string(source), "// --- Automatically Created by yoel ---\n#include <iostream>\nusing namespace std;\n\nint main(){\n\n}"; got != want {
		t.Fatalf("created source = %q, want %q", got, want)
	}
	if openedPath == "" || filepath.Ext(openedPath) != ".pdf" {
		t.Fatalf("opened path = %q, want cached PDF path", openedPath)
	}
}

func TestUserCommandShowsActivityWithoutExposingToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	latestSubmission := time.Date(2030, time.January, 2, 10, 30, 0, 0, time.UTC)
	expiresAt := time.Now().Add(time.Hour).Round(time.Second)
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/problems" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-user-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = fmt.Fprintf(w, `[
			{"id":1,"name":"one","full_name":"One","submission_count":2,"last_submission_time":"2029-12-01T01:00:00Z"},
			{"id":2,"name":"two","full_name":"Two","submission_count":0,"last_submission_time":null},
			{"id":3,"name":"three","full_name":"Three","submission_count":3,"last_submission_time":%q}
		]`, latestSubmission.Format(time.RFC3339))
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-user-token",
		ExpiresAt: expiresAt,
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	output := executeUserCommand(t)
	for _, expected := range []string{
		"Username:            fake-login\n",
		"Full name:           Fake Student\n",
		"Last submission:     " + latestSubmission.Local().Format(userTimeFormat) + "\n",
		"Problems attempted:  2\n",
		"Total submissions:   5\n",
		"Token expires:       " + expiresAt.Local().Format(userTimeFormat) + "\n",
		"Token status:        unexpired\n",
		"Cookie status:       not used\n",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("output %q does not contain %q", output, expected)
		}
	}
	if strings.Contains(output, "fake-user-token") {
		t.Fatal("output contains bearer token")
	}
}

func TestUserCommandReportsExpiredTokenWithoutRequest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := newCLITestServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("expired user command unexpectedly contacted grader")
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-expired-token",
		ExpiresAt: time.Now().Add(-time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	output := executeUserCommand(t)
	for _, expected := range []string{
		"Last submission:     unavailable\n",
		"Problems attempted:  unavailable\n",
		"Total submissions:   unavailable\n",
		"Token status:        expired\n",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("output %q does not contain %q", output, expected)
		}
	}
	if strings.Contains(output, "fake-expired-token") {
		t.Fatal("output contains expired bearer token")
	}
}

func TestUserCommandReportsRejectedToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, "fake-rejected-token")
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-rejected-token",
		ExpiresAt: time.Now().Add(time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	output := executeUserCommand(t)
	if !strings.Contains(output, "Token status:        rejected\n") {
		t.Fatalf("output = %q, want rejected token status", output)
	}
	if strings.Contains(output, "fake-rejected-token") {
		t.Fatal("output contains rejected bearer token")
	}
}

func executeQuestionShow(t *testing.T, opener fileOpener, args ...string) {
	t.Helper()
	command := newRootCommandWithOpener(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("question show unexpectedly prompted for credentials")
		return "", "", nil
	}, opener)
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs(append([]string{"question", "show"}, args...))
	if err := command.Execute(); err != nil {
		t.Fatalf("question show: %v", err)
	}
}

func executeQuestionList(t *testing.T) string {
	t.Helper()
	command := newRootCommandWithOpener(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("question list unexpectedly prompted for credentials")
		return "", "", nil
	}, func(string) error { return nil })
	output := new(bytes.Buffer)
	command.SetIn(strings.NewReader("1\n"))
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"question", "list"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question list: %v", err)
	}
	return output.String()
}

func executeUserCommand(t *testing.T) string {
	t.Helper()
	command := newRootCommand(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("user command unexpectedly prompted for credentials")
		return "", "", nil
	})
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"user"})
	if err := command.Execute(); err != nil {
		t.Fatalf("user command: %v", err)
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
