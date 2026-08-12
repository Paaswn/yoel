package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	compileTimeout = 30 * time.Second
	outputLimit    = 256 << 10
)

var cppCompilerArguments = []string{"-std=c++17", "-O2", "-pipe"}

// CompileCPP compiles one C++ source file and safely reuses a binary only when
// source contents, compiler identity, flags, OS, and architecture all match.
func CompileCPP(ctx context.Context, sourcePath, cacheDir string) (CompileResult, error) {
	if ctx == nil || strings.ToLower(filepath.Ext(sourcePath)) != ".cpp" || cacheDir == "" {
		return CompileResult{}, errors.New("invalid local compilation input")
	}
	compiler, err := exec.LookPath("g++")
	if err != nil {
		return CompileResult{}, errors.New("C++ compiler g++ was not found")
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return CompileResult{}, fmt.Errorf("read local source: %w", err)
	}
	compilerVersion, err := commandOutput(ctx, compiler, "--version")
	if err != nil {
		return CompileResult{}, fmt.Errorf("identify C++ compiler: %w", err)
	}

	hash := sha256.New()
	_, _ = hash.Write(source)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(compiler))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(compilerVersion)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strings.Join(cppCompilerArguments, "\x00")))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(runtime.GOOS + "/" + runtime.GOARCH))
	buildID := hex.EncodeToString(hash.Sum(nil))
	filename := buildID
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return CompileResult{}, fmt.Errorf("create binary cache: %w", err)
	}
	binaryPath := filepath.Join(cacheDir, filename)
	if validCachedBinary(binaryPath) {
		return CompileResult{BinaryPath: binaryPath, Cached: true}, nil
	}

	temporary, err := os.CreateTemp(cacheDir, ".yoel-build-*")
	if err != nil {
		return CompileResult{}, fmt.Errorf("create temporary binary: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return CompileResult{}, fmt.Errorf("close temporary binary: %w", err)
	}
	defer os.Remove(temporaryPath)

	compileContext, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()
	arguments := append([]string{}, cppCompilerArguments...)
	arguments = append(arguments, sourcePath, "-o", temporaryPath)
	command := exec.CommandContext(compileContext, compiler, arguments...)
	output := &limitedBuffer{limit: outputLimit}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		if compileContext.Err() != nil {
			return CompileResult{}, fmt.Errorf("local compilation: %w", compileContext.Err())
		}
		return CompileResult{}, &CompileError{Output: string(output.Bytes())}
	}
	if err := os.Chmod(temporaryPath, 0o700); err != nil {
		return CompileResult{}, fmt.Errorf("make local binary executable: %w", err)
	}
	if err := os.Rename(temporaryPath, binaryPath); err != nil {
		return CompileResult{}, fmt.Errorf("store local binary: %w", err)
	}
	return CompileResult{BinaryPath: binaryPath}, nil
}

func commandOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	commandContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(commandContext, name, args...)
	output := &limitedBuffer{limit: outputLimit}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		if commandContext.Err() != nil {
			return nil, commandContext.Err()
		}
		return nil, err
	}
	return output.Bytes(), nil
}

func validCachedBinary(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o100 != 0
}
