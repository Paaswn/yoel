// Package registry persists Yoel's known grader questions and their local
// source bindings. It deliberately contains no CLI, HTTP, or runner behavior.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	applicationDirectory = "yoel"
	filename             = "questions.json"
)

var (
	ErrNotFound      = errors.New("question registry entry not found")
	ErrNoLocalSource = errors.New("question has no local source")
	ErrStaleSource   = errors.New("registered question source is stale")
)

// QuestionEntry is one canonical question record, identified by its stable
// grader ID. Local paths are only set after Yoel has created a real file.
type QuestionEntry struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DirectoryPath string `json:"directory_path,omitempty"`
	SourcePath    string `json:"source_path,omitempty"`
}

// Registry maps stable question IDs to their one canonical entry.
type Registry map[int]QuestionEntry

// ResolveResult identifies the registered question and its usable local file.
type ResolveResult struct {
	Entry      QuestionEntry
	SourcePath string
}

// AmbiguousError reports a key that exactly matches multiple entries.
type AmbiguousError struct {
	Key     string
	Entries []QuestionEntry
}

func (e *AmbiguousError) Error() string {
	if e == nil {
		return "question registry key is ambiguous"
	}
	lines := make([]string, 0, len(e.Entries))
	for _, entry := range e.Entries {
		lines = append(lines, fmt.Sprintf("  %d  %s  %s", entry.ID, entry.Name, entry.SourcePath))
	}
	return fmt.Sprintf("question key %q matches multiple registered questions:\n%s\nuse a question ID or explicit file path", e.Key, strings.Join(lines, "\n"))
}

// DefaultPath returns the platform-appropriate registry location without
// creating it.
func DefaultPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(directory, applicationDirectory, filename), nil
}

// LoadDefault loads the registry from Yoel's normal user config location.
func LoadDefault() (Registry, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Load(path)
}

// Load returns an empty registry when path does not exist.
func Load(path string) (Registry, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open question registry: %w", err)
	}
	defer file.Close()

	registry := Registry{}
	decoder := json.NewDecoder(io.LimitReader(file, 4<<20))
	if err := decoder.Decode(&registry); err != nil {
		return nil, errors.New("read question registry: invalid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("read question registry: invalid JSON")
	}
	for id, entry := range registry {
		if id <= 0 || entry.ID != id {
			return nil, errors.New("read question registry: invalid question entry")
		}
	}
	return registry, nil
}

// Save writes registry atomically enough for normal local filesystem use.
func (r Registry) Save(path string) error {
	if r == nil {
		r = Registry{}
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize question registry: %w", err)
	}
	data = append(data, '\n')

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create question registry directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("protect question registry directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".yoel-questions-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary question registry: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect temporary question registry: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write question registry: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close question registry: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("save question registry: %w", err)
	}
	return nil
}

// UpsertRemote refreshes remote fields while preserving all local bindings.
func (r Registry) UpsertRemote(id int, name, fullName string) error {
	if id <= 0 {
		return errors.New("question registry: question ID must be positive")
	}
	entry := r[id]
	entry.ID = id
	entry.Name = name
	entry.FullName = fullName
	r[id] = entry
	return nil
}

// BindLocal records the actual created directory and, when known, source file.
// It requires remote metadata to have already established the canonical entry.
func (r Registry) BindLocal(id int, directoryPath, sourcePath string) error {
	entry, exists := r[id]
	if !exists || entry.ID != id {
		return fmt.Errorf("question registry: question %d is not known", id)
	}
	directoryPath, err := normalizeDirectory(directoryPath)
	if err != nil {
		return err
	}
	if sourcePath != "" {
		sourcePath, err = normalizeRegularFile(sourcePath)
		if err != nil {
			return err
		}
	}
	entry.DirectoryPath = directoryPath
	entry.SourcePath = sourcePath
	r[id] = entry
	return nil
}

// Resolve finds an exact registered key and validates its recorded source.
// Existing explicit file paths are intentionally handled by the caller first.
func (r Registry) Resolve(key string) (ResolveResult, error) {
	if entry, found, err := selectUnique(key, r.matchSourceFilename(key)); err != nil {
		return ResolveResult{}, err
	} else if found {
		return resolveEntry(entry)
	}
	if entry, found, err := selectUnique(key, r.matchDirectory(key)); err != nil {
		return ResolveResult{}, err
	} else if found {
		return resolveEntry(entry)
	}
	if id, err := strconv.Atoi(key); err == nil && strconv.Itoa(id) == key {
		if entry, exists := r[id]; exists {
			return resolveEntry(entry)
		}
	}
	if entry, found, err := selectUnique(key, r.matchName(key)); err != nil {
		return ResolveResult{}, err
	} else if found {
		return resolveEntry(entry)
	}
	if entry, found, err := selectUnique(key, r.matchFullName(key)); err != nil {
		return ResolveResult{}, err
	} else if found {
		return resolveEntry(entry)
	}
	return ResolveResult{}, fmt.Errorf("%w: %q", ErrNotFound, key)
}

func (r Registry) matchSourceFilename(key string) []QuestionEntry {
	return r.match(func(entry QuestionEntry) bool {
		return entry.SourcePath != "" && filepath.Base(entry.SourcePath) == key
	})
}

func (r Registry) matchDirectory(key string) []QuestionEntry {
	normalized := ""
	if absolute, err := filepath.Abs(key); err == nil {
		normalized = filepath.Clean(absolute)
	}
	return r.match(func(entry QuestionEntry) bool {
		return entry.DirectoryPath != "" && (entry.DirectoryPath == normalized || filepath.Base(entry.DirectoryPath) == key)
	})
}

func (r Registry) matchName(key string) []QuestionEntry {
	return r.match(func(entry QuestionEntry) bool { return entry.Name == key })
}

func (r Registry) matchFullName(key string) []QuestionEntry {
	return r.match(func(entry QuestionEntry) bool { return entry.FullName == key })
}

func (r Registry) match(matches func(QuestionEntry) bool) []QuestionEntry {
	entries := make([]QuestionEntry, 0)
	for _, entry := range r {
		if matches(entry) {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func selectUnique(key string, entries []QuestionEntry) (QuestionEntry, bool, error) {
	switch len(entries) {
	case 0:
		return QuestionEntry{}, false, nil
	case 1:
		return entries[0], true, nil
	default:
		return QuestionEntry{}, false, &AmbiguousError{Key: key, Entries: entries}
	}
}

func resolveEntry(entry QuestionEntry) (ResolveResult, error) {
	if entry.SourcePath == "" {
		return ResolveResult{}, fmt.Errorf("%w: question %d", ErrNoLocalSource, entry.ID)
	}
	info, err := os.Stat(entry.SourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return ResolveResult{}, fmt.Errorf("%w: question %d points to %s", ErrStaleSource, entry.ID, entry.SourcePath)
	}
	if err != nil {
		return ResolveResult{}, fmt.Errorf("inspect registered source for question %d: %w", entry.ID, err)
	}
	if !info.Mode().IsRegular() {
		return ResolveResult{}, fmt.Errorf("%w: question %d points to %s", ErrStaleSource, entry.ID, entry.SourcePath)
	}
	return ResolveResult{Entry: entry, SourcePath: entry.SourcePath}, nil
}

func normalizeDirectory(path string) (string, error) {
	if path == "" {
		return "", errors.New("question registry: directory path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize question directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("inspect question directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("question registry: %s is not a directory", abs)
	}
	return filepath.Clean(abs), nil
}

func normalizeRegularFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize question source: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("inspect question source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("question registry: %s is not a regular file", abs)
	}
	return filepath.Clean(abs), nil
}
