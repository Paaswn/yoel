package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	latestReleaseURL     = "https://api.github.com/repos/Paaswn/yoel/releases/latest"
	unixInstallerURL     = "https://raw.githubusercontent.com/Paaswn/yoel/master/scripts/install.sh"
	windowsInstallerURL  = "https://raw.githubusercontent.com/Paaswn/yoel/master/scripts/install.ps1"
	maxReleaseBodyBytes  = 64 << 10
	maxInstallerBytes    = 512 << 10
	updateCheckInterval  = 24 * time.Hour
	updateRequestTimeout = 3 * time.Second
)

type semanticVersion struct{ major, minor, patch int }

type updateDependencies struct {
	version      string
	latestURL    string
	installerURL string
	httpClient   *http.Client
	now          func() time.Time
	interactive  func(*cobra.Command) bool
	confirm      func(*cobra.Command) (bool, error)
	runInstaller func(context.Context, string, string, io.Writer) error
}

func defaultUpdateDependencies(version string) updateDependencies {
	return updateDependencies{
		version:      version,
		latestURL:    latestReleaseURL,
		installerURL: installerURLForPlatform(runtime.GOOS),
		httpClient:   &http.Client{Timeout: updateRequestTimeout},
		now:          time.Now,
		interactive:  interactiveTerminal,
		confirm:      confirmUpdate,
		runInstaller: downloadAndRunInstaller,
	}
}

func newUpdateCommand(deps updateDependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Install the latest Yoel release",
		Long:  "Install the latest release with Yoel's official installer. Package-managed installations should be updated with their package manager.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			latest, err := latestVersion(command.Context(), deps)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(command.OutOrStdout(), "Current version: %s\nLatest version:  %s\n", deps.version, formatVersion(latest)); err != nil {
				return err
			}

			current, validCurrent := parseVersion(deps.version)
			if !validCurrent {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "This is a development build. Install a release with the official installer instead.")
				return nil
			}
			if compareVersions(latest, current) <= 0 {
				_, err := fmt.Fprintln(command.OutOrStdout(), "Yoel is already up to date.")
				return err
			}
			if !deps.interactive(command) {
				_, err := fmt.Fprintln(command.OutOrStdout(), "Run `yoel update` in an interactive terminal to confirm the update.")
				return err
			}
			if _, err := fmt.Fprintln(command.OutOrStdout()); err != nil {
				return err
			}
			accepted, err := deps.confirm(command)
			if err != nil {
				return fmt.Errorf("confirm update: %w", err)
			}
			if !accepted {
				return nil
			}
			if deps.installerURL == "" {
				return errors.New("update: this operating system is not supported by the official installer")
			}
			return deps.runInstaller(command.Context(), deps.installerURL, formatVersion(latest), command.OutOrStdout())
		},
	}
}

func latestVersion(ctx context.Context, deps updateDependencies) (semanticVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, updateRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, deps.latestURL, nil)
	if err != nil {
		return semanticVersion{}, fmt.Errorf("check latest version: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "yoel/"+deps.version)
	client := deps.httpClient
	if client == nil {
		client = &http.Client{Timeout: updateRequestTimeout}
	}
	response, err := client.Do(req)
	if err != nil {
		return semanticVersion{}, fmt.Errorf("check latest version: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return semanticVersion{}, fmt.Errorf("check latest version: GitHub returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseBodyBytes+1))
	if err != nil {
		return semanticVersion{}, fmt.Errorf("check latest version: read response: %w", err)
	}
	if len(body) > maxReleaseBodyBytes {
		return semanticVersion{}, errors.New("check latest version: response is too large")
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	if err := decoder.Decode(&release); err != nil {
		return semanticVersion{}, errors.New("check latest version: invalid response")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return semanticVersion{}, errors.New("check latest version: invalid response")
	}
	version, ok := parseVersion(release.TagName)
	if !ok {
		return semanticVersion{}, errors.New("check latest version: response contains an invalid release tag")
	}
	return version, nil
}

func parseVersion(value string) (semanticVersion, bool) {
	var version semanticVersion
	if _, err := fmt.Sscanf(value, "v%d.%d.%d", &version.major, &version.minor, &version.patch); err != nil || version.major < 0 || version.minor < 0 || version.patch < 0 || formatVersion(version) != value {
		return semanticVersion{}, false
	}
	return version, true
}

func formatVersion(version semanticVersion) string {
	return fmt.Sprintf("v%d.%d.%d", version.major, version.minor, version.patch)
}

func compareVersions(left, right semanticVersion) int {
	if left.major != right.major {
		return left.major - right.major
	}
	if left.minor != right.minor {
		return left.minor - right.minor
	}
	return left.patch - right.patch
}

func interactiveTerminal(command *cobra.Command) bool {
	input, inputOK := command.InOrStdin().(*os.File)
	output, outputOK := command.OutOrStdout().(*os.File)
	if !inputOK || !outputOK {
		return false
	}
	inputInfo, inputErr := input.Stat()
	outputInfo, outputErr := output.Stat()
	return inputErr == nil && outputErr == nil && inputInfo.Mode()&os.ModeCharDevice != 0 && outputInfo.Mode()&os.ModeCharDevice != 0
}

func confirmUpdate(command *cobra.Command) (bool, error) {
	return huhYesNo(command, "Update Yoel?")
}

func installerURLForPlatform(goos string) string {
	switch goos {
	case "darwin", "linux":
		return unixInstallerURL
	case "windows":
		return windowsInstallerURL
	default:
		return ""
	}
}

func downloadAndRunInstaller(ctx context.Context, installerURL, version string, output io.Writer) error {
	if _, ok := parseVersion(version); !ok {
		return errors.New("update: invalid version")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, installerURL, nil)
	if err != nil {
		return fmt.Errorf("download updater: %w", err)
	}
	response, err := (&http.Client{Timeout: updateRequestTimeout}).Do(request)
	if err != nil {
		return fmt.Errorf("download updater: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download updater: server returned %s", response.Status)
	}
	script, err := io.ReadAll(io.LimitReader(response.Body, maxInstallerBytes+1))
	if err != nil {
		return fmt.Errorf("download updater: read response: %w", err)
	}
	if len(script) > maxInstallerBytes {
		return errors.New("download updater: installer is too large")
	}
	file, err := os.CreateTemp("", "yoel-update-*")
	if err != nil {
		return fmt.Errorf("download updater: create temporary file: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(script); err != nil {
		file.Close()
		return fmt.Errorf("download updater: write temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("download updater: close temporary file: %w", err)
	}

	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-File", path)
	} else {
		command = exec.CommandContext(ctx, "sh", path)
	}
	command.Env = installerEnvironment(version)
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return fmt.Errorf("run official installer: %w", err)
	}
	return nil
}

func installerEnvironment(version string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "YOEL_VERSION=") || strings.HasPrefix(entry, "YOEL_INSTALL_DIR=") || strings.HasPrefix(entry, "YOEL_TEST_MODE=") || strings.HasPrefix(entry, "YOEL_RELEASE_") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "YOEL_VERSION="+version)
}

func maybeShowUpdateNotice(command *cobra.Command, deps updateDependencies) {
	if deps.version == "dev" || os.Getenv("YOEL_NO_UPDATE_CHECK") == "1" || !deps.interactive(command) {
		return
	}
	now := deps.now()
	if cache, err := loadUpdateCheckCache(); err == nil && now.Sub(cache.CheckedAt) < updateCheckInterval {
		return
	}
	latest, err := latestVersion(command.Context(), deps)
	if err != nil {
		return
	}
	_ = saveUpdateCheckCache(updateCheckCache{CheckedAt: now})
	current, ok := parseVersion(deps.version)
	if !ok || compareVersions(latest, current) <= 0 {
		return
	}
	_, _ = fmt.Fprintf(command.OutOrStdout(), "A new Yoel version is available: %s. Run `yoel update` to install it.\n", formatVersion(latest))
}
