package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestParseAndCompareVersions(t *testing.T) {
	newer, _ := parseVersion("v0.10.0")
	older, _ := parseVersion("v0.2.9")
	equal, _ := parseVersion("v0.10.0")
	if compareVersions(newer, older) <= 0 || compareVersions(newer, equal) != 0 {
		t.Fatalf("version comparison is incorrect")
	}
	for _, invalid := range []string{"dev", "v1", "v1.2", "1.2.3", "v1.2.3-beta", "v01.2.3", "v-1.2.3", "v1.2.3x"} {
		if _, ok := parseVersion(invalid); ok {
			t.Errorf("parseVersion(%q) succeeded", invalid)
		}
	}
}

func TestLatestVersionValidatesGitHubResponses(t *testing.T) {
	for name, response := range map[string]struct {
		status  int
		body    string
		wantErr bool
	}{
		"valid":         {http.StatusOK, `{"tag_name":"v0.3.0"}`, false},
		"malformed":     {http.StatusOK, `{`, true},
		"invalid tag":   {http.StatusOK, `{"tag_name":"latest"}`, true},
		"trailing JSON": {http.StatusOK, `{"tag_name":"v0.3.0"} {}`, true},
		"rate limited":  {http.StatusTooManyRequests, `{}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.Header.Get("Accept") != "application/vnd.github+json" || request.Header.Get("User-Agent") != "yoel/v0.2.0" {
					t.Errorf("request = %s %#v", request.Method, request.Header)
				}
				w.WriteHeader(response.status)
				_, _ = fmt.Fprint(w, response.body)
			}))
			defer server.Close()
			version, err := latestVersion(context.Background(), updateDependencies{latestURL: server.URL, version: "v0.2.0", httpClient: server.Client()})
			if (err != nil) != response.wantErr {
				t.Fatalf("latestVersion error = %v", err)
			}
			if !response.wantErr && formatVersion(version) != "v0.3.0" {
				t.Fatalf("version = %s", formatVersion(version))
			}
		})
	}
}

func TestLatestVersionRejectsOversizedAndCancelledResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", maxReleaseBodyBytes+1))
	}))
	defer server.Close()
	_, err := latestVersion(context.Background(), updateDependencies{latestURL: server.URL, version: "v0.2.0", httpClient: server.Client()})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = latestVersion(ctx, updateDependencies{latestURL: server.URL, version: "v0.2.0", httpClient: server.Client()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
}

func TestUpdateDeclineAndNonInteractiveDoNotRunInstaller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, `{"tag_name":"v0.3.0"}`) }))
	defer server.Close()
	var runs atomic.Int32
	for name, interactive := range map[string]bool{"decline": true, "non-interactive": false} {
		t.Run(name, func(t *testing.T) {
			output := new(bytes.Buffer)
			command := newUpdateCommand(updateDependencies{
				version: "v0.2.0", latestURL: server.URL, installerURL: server.URL, httpClient: server.Client(),
				interactive:  func(*cobra.Command) bool { return interactive },
				confirm:      func(*cobra.Command) (bool, error) { return false, nil },
				runInstaller: func(context.Context, string, string, io.Writer) error { runs.Add(1); return nil },
			})
			command.SetOut(output)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if runs.Load() != 0 {
		t.Fatalf("installer runs = %d", runs.Load())
	}
}

func TestUpdateRunsInjectedInstallerWithValidatedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, `{"tag_name":"v0.3.0"}`) }))
	defer server.Close()
	var gotURL, gotVersion string
	command := newUpdateCommand(updateDependencies{
		version: "v0.2.0", latestURL: server.URL, installerURL: "https://example.invalid/install.sh", httpClient: server.Client(),
		interactive: func(*cobra.Command) bool { return true }, confirm: func(*cobra.Command) (bool, error) { return true, nil },
		runInstaller: func(_ context.Context, url, version string, _ io.Writer) error {
			gotURL, gotVersion = url, version
			return nil
		},
	})
	command.SetOut(new(bytes.Buffer))
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://example.invalid/install.sh" || gotVersion != "v0.3.0" {
		t.Fatalf("installer got %q %q", gotURL, gotVersion)
	}
}

func TestUpdateNoticeIsInteractiveThrottledAndCanBeDisabled(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, `{"tag_name":"v0.3.0"}`) }))
	defer server.Close()
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	deps := updateDependencies{version: "v0.2.0", latestURL: server.URL, httpClient: server.Client(), now: func() time.Time { return now }, interactive: func(*cobra.Command) bool { return true }}
	command := &cobra.Command{}
	output := new(bytes.Buffer)
	command.SetOut(output)
	maybeShowUpdateNotice(command, deps)
	if !strings.Contains(output.String(), "A new Yoel version is available: v0.3.0. Run `yoel update` to install it.") {
		t.Fatalf("notice = %q", output.String())
	}
	output.Reset()
	maybeShowUpdateNotice(command, deps)
	if output.Len() != 0 {
		t.Fatalf("throttled notice = %q", output.String())
	}

	if err := os.Remove(mustUpdateCachePath(t)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YOEL_NO_UPDATE_CHECK", "1")
	maybeShowUpdateNotice(command, deps)
	if output.Len() != 0 {
		t.Fatalf("disabled notice = %q", output.String())
	}
}

func TestUpdateNoticeFailureDoesNotThrottleRetryOrExposeSecrets(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	secret := "fake-token-and-source"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, secret)
	}))
	defer server.Close()
	command := &cobra.Command{}
	output := new(bytes.Buffer)
	command.SetOut(output)
	deps := updateDependencies{version: "v0.2.0", latestURL: server.URL, httpClient: server.Client(), now: time.Now, interactive: func(*cobra.Command) bool { return true }}
	maybeShowUpdateNotice(command, deps)
	if _, err := loadUpdateCheckCache(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache error = %v, want not exist", err)
	}
	if strings.Contains(output.String(), secret) {
		t.Fatalf("notice exposes secret: %q", output.String())
	}
	deps.interactive = func(*cobra.Command) bool { return false }
	maybeShowUpdateNotice(command, deps)
	if output.Len() != 0 {
		t.Fatalf("non-interactive notice = %q", output.String())
	}
}

func TestDownloadAndRunInstallerUsesTemporaryFileAndOnlyValidatedYOELSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by the Windows installer path in CI")
	}
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	t.Setenv("YOEL_TEST_MODE", "1")
	t.Setenv("YOEL_INSTALL_DIR", "unsafe")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `test "$YOEL_VERSION" = "v0.3.0" && test -z "$YOEL_TEST_MODE" && test -z "$YOEL_INSTALL_DIR" && printf 'installer ran'`)
	}))
	defer server.Close()
	output := new(bytes.Buffer)
	if err := downloadAndRunInstaller(context.Background(), server.URL, "v0.3.0", output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "installer ran" {
		t.Fatalf("output = %q", output.String())
	}
	entries, err := os.ReadDir(temp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary installer remains: %#v", entries)
	}
}

func mustUpdateCachePath(t *testing.T) string {
	t.Helper()
	path, err := updateCheckPath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}
