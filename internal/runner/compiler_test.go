package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompileCPPBuildsAndReusesSourceSpecificBinary(t *testing.T) {
	requireGPP(t)
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "673.cpp")
	if err := os.WriteFile(sourcePath, []byte("int main() { return 0; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(directory, ".yoel", "bin")

	first, err := CompileCPP(context.Background(), sourcePath, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached || !validCachedBinary(first.BinaryPath) {
		t.Fatalf("first compile = %#v", first)
	}
	second, err := CompileCPP(context.Background(), sourcePath, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || second.BinaryPath != first.BinaryPath {
		t.Fatalf("second compile = %#v, first = %#v", second, first)
	}

	if err := os.WriteFile(sourcePath, []byte("int main() { return 1; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := CompileCPP(context.Background(), sourcePath, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if changed.BinaryPath == first.BinaryPath {
		t.Fatal("changed source reused the old binary path")
	}
	wantExtension := ""
	if runtime.GOOS == "windows" {
		wantExtension = ".exe"
	}
	if filepath.Ext(changed.BinaryPath) != wantExtension {
		t.Fatalf("binary extension = %q, want %q", filepath.Ext(changed.BinaryPath), wantExtension)
	}
}

func TestCompileCPPReturnsSanitizedCompileError(t *testing.T) {
	requireGPP(t)
	directory := t.TempDir()
	const source = "int main() { fake-private-source }\n"
	sourcePath := filepath.Join(directory, "673.cpp")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := CompileCPP(context.Background(), sourcePath, filepath.Join(directory, ".yoel", "bin"))
	var compileErr *CompileError
	if !errors.As(err, &compileErr) {
		t.Fatalf("error = %v, want CompileError", err)
	}
	if compileErr.Output == "" {
		t.Fatal("compiler output is empty")
	}
	if strings.Contains(err.Error(), "fake-private-source") {
		t.Fatalf("ordinary error exposes source: %v", err)
	}
}

func TestCompileCPPRejectsUnsupportedSource(t *testing.T) {
	if _, err := CompileCPP(context.Background(), "673.py", t.TempDir()); err == nil {
		t.Fatal("CompileCPP accepted a non-C++ source")
	}
}

func requireGPP(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("g++ is not installed")
	}
}
