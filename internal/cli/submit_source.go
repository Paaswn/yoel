package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"yoel/internal/registry"
)

type submissionSourceResolution struct {
	Path       string
	Filename   string
	ProblemID  int
	UsedLegacy bool
}

var (
	errSubmissionSourceNotFound = errors.New("submission source file was not found")
	errAmbiguousSubmissionFile  = errors.New("multiple submission source files were found")
)

func resolveSubmissionSource(argument string) (path string, filename string, problemID int, err error) {
	info, statErr := os.Stat(argument)
	if statErr == nil {
		if info.IsDir() {
			path, err = findSubmissionSourceInDirectory(argument)
			if err != nil {
				return "", "", 0, err
			}
			filename = filepath.Base(path)
			problemID, err = problemIDFromFilename(filename)
			return path, filename, problemID, err
		}
		if !info.Mode().IsRegular() {
			return "", "", 0, fmt.Errorf("submission source %q is not a regular file", argument)
		}
		filename = filepath.Base(argument)
		problemID, err = problemIDFromFilename(filename)
		if err != nil {
			return "", "", 0, err
		}
		return filepath.Clean(argument), filename, problemID, nil
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return "", "", 0, fmt.Errorf("inspect submission source: %w", statErr)
	}

	filename = filepath.Base(argument)
	problemID, err = problemIDFromFilename(filename)
	if err != nil {
		return "", "", 0, err
	}
	currentDir, err := os.Getwd()
	if err != nil {
		return "", "", 0, fmt.Errorf("find current directory: %w", err)
	}
	matches, err := findRegularFiles(currentDir, func(_ string, entry fs.DirEntry) bool {
		return entry.Name() == filename
	})
	if err != nil {
		return "", "", 0, err
	}
	path, err = selectSubmissionSource(matches, filename)
	if err != nil {
		return "", "", 0, err
	}
	return path, filename, problemID, nil
}

func resolveSubmissionSourceWithRegistry(argument string, questions registry.Registry) (submissionSourceResolution, error) {
	info, statErr := os.Stat(argument)
	if statErr == nil && !info.IsDir() {
		path, filename, problemID, err := resolveSubmissionSource(argument)
		if err != nil {
			return submissionSourceResolution{}, err
		}
		return submissionSourceResolution{Path: path, Filename: filename, ProblemID: problemID}, nil
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return submissionSourceResolution{}, fmt.Errorf("inspect submission source: %w", statErr)
	}

	resolved, err := questions.Resolve(argument)
	if err == nil {
		return submissionSourceResolution{
			Path:      resolved.SourcePath,
			Filename:  filepath.Base(resolved.SourcePath),
			ProblemID: resolved.Entry.ID,
		}, nil
	}
	if !errors.Is(err, registry.ErrNotFound) {
		if errors.Is(err, registry.ErrNoLocalSource) {
			return submissionSourceResolution{}, fmt.Errorf("%w; run yoel question new --id <question-id> or submit an explicit file path", err)
		}
		return submissionSourceResolution{}, err
	}

	path, filename, problemID, err := resolveSubmissionSource(argument)
	if err != nil {
		return submissionSourceResolution{}, err
	}
	return submissionSourceResolution{
		Path:       path,
		Filename:   filename,
		ProblemID:  problemID,
		UsedLegacy: statErr != nil || info.IsDir(),
	}, nil
}

func findSubmissionSourceInDirectory(directory string) (string, error) {
	matches, err := findRegularFiles(directory, func(_ string, entry fs.DirEntry) bool {
		if filepath.Ext(entry.Name()) != ".cpp" {
			return false
		}
		_, err := problemIDFromFilename(entry.Name())
		return err == nil
	})
	if err != nil {
		return "", err
	}
	return selectSubmissionSource(matches, directory)
}

func findRegularFiles(root string, match func(path string, entry fs.DirEntry) bool) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !match(path, entry) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			matches = append(matches, filepath.Clean(path))
			if len(matches) == 2 {
				return fs.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search submission source in %q: %w", root, err)
	}
	return matches, nil
}

func selectSubmissionSource(matches []string, searchedFor string) (string, error) {
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: %s", errSubmissionSourceNotFound, searchedFor)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%w for %q: %q and %q", errAmbiguousSubmissionFile, searchedFor, matches[0], matches[1])
	}
}
