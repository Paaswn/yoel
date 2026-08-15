package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"yoel/internal/runner"
)

const maxLocalReplayCacheFile = 4 << 20

type localReplayMetadata struct {
	TestcaseID   int      `json:"testcase_id"`
	RemoteStatus string   `json:"remote_status"`
	Score        *float64 `json:"score,omitempty"`
	RemoteTime   *int     `json:"remote_time,omitempty"`
	RemoteMemory *int     `json:"remote_memory,omitempty"`
	LocalStatus  string   `json:"local_status"`
	ExitCode     int      `json:"exit_code"`
	DurationMS   int64    `json:"duration_ms"`
	TimedOut     bool     `json:"timed_out,omitempty"`
	HasInput     bool     `json:"has_input,omitempty"`
	HasExpected  bool     `json:"has_expected,omitempty"`
	HasStdout    bool     `json:"has_stdout,omitempty"`
	HasStderr    bool     `json:"has_stderr,omitempty"`
}

type localReplayCacheData struct {
	Metadata localReplayMetadata
	Input    []byte
	Expected []byte
	Stdout   []byte
	Stderr   []byte
}

func localReplayCacheDirectory(sourcePath string, submissionID, testcaseID int) (string, error) {
	if sourcePath == "" || submissionID <= 0 || testcaseID <= 0 {
		return "", errors.New("invalid local replay cache key")
	}
	return filepath.Join(
		filepath.Dir(sourcePath),
		".yoel",
		"testcases",
		strconv.Itoa(submissionID),
		strconv.Itoa(testcaseID),
	), nil
}

func binaryCacheDirectory(sourcePath string) (string, error) {
	if sourcePath == "" {
		return "", errors.New("invalid binary cache path")
	}
	return filepath.Join(filepath.Dir(sourcePath), ".yoel", "bin"), nil
}

func writeLocalReplayCache(sourcePath string, submissionID int, evaluation remoteEvaluationSnapshot, testcase runner.TestCase, result runner.RunResult, localStatus string) error {
	directory, err := localReplayCacheDirectory(sourcePath, submissionID, testcase.ID)
	if err != nil {
		return err
	}
	if localStatus == "" {
		localStatus = localRunStatus(result)
	}
	metadata := localReplayMetadata{
		TestcaseID:   testcase.ID,
		RemoteStatus: evaluation.Status,
		Score:        evaluation.Score,
		RemoteTime:   evaluation.Time,
		RemoteMemory: evaluation.Memory,
		LocalStatus:  localStatus,
		ExitCode:     result.ExitCode,
		DurationMS:   result.Duration.Milliseconds(),
		TimedOut:     result.TimedOut,
		HasInput:     true,
		HasExpected:  true,
		HasStdout:    true,
		HasStderr:    true,
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	metadataBytes = append(metadataBytes, '\n')
	files := []struct {
		name string
		data []byte
	}{
		{"input.txt", testcase.Input},
		{"expected.txt", testcase.Expected},
		{"stdout.txt", result.Stdout},
		{"stderr.txt", result.Stderr},
		// Metadata is written last, so its presence indicates that all data
		// files in this cache entry were replaced successfully.
		{"meta.json", metadataBytes},
	}
	for _, file := range files {
		if err := writePrivateFile(filepath.Join(directory, file.name), file.data); err != nil {
			return fmt.Errorf("write local replay cache: %w", err)
		}
	}
	return nil
}

// writeLocalReplayInputCache makes downloaded testcase input available for
// later inspection even if solution download, compilation, or execution fails.
func writeLocalReplayInputCache(sourcePath string, submissionID int, evaluation remoteEvaluationSnapshot, testcase runner.TestCase) error {
	directory, err := localReplayCacheDirectory(sourcePath, submissionID, testcase.ID)
	if err != nil {
		return err
	}
	metadata, err := loadLocalReplayMetadata(directory, testcase.ID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	metadata.TestcaseID = testcase.ID
	metadata.RemoteStatus = evaluation.Status
	metadata.Score = evaluation.Score
	metadata.RemoteTime = evaluation.Time
	metadata.RemoteMemory = evaluation.Memory
	metadata.HasInput = true
	if err := writePrivateFile(filepath.Join(directory, "input.txt"), testcase.Input); err != nil {
		return fmt.Errorf("write local replay input cache: %w", err)
	}
	return writeLocalReplayMetadata(directory, metadata)
}

func writeLocalReplayExpectedCache(sourcePath string, submissionID, testcaseID int, expected []byte) error {
	directory, err := localReplayCacheDirectory(sourcePath, submissionID, testcaseID)
	if err != nil {
		return err
	}
	metadata, err := loadLocalReplayMetadata(directory, testcaseID)
	if err != nil {
		return err
	}
	if err := writePrivateFile(filepath.Join(directory, "expected.txt"), expected); err != nil {
		return fmt.Errorf("write local replay expected cache: %w", err)
	}
	metadata.HasExpected = true
	return writeLocalReplayMetadata(directory, metadata)
}

func readLocalReplayCache(sourcePath string, submissionID, testcaseID int) (localReplayCacheData, error) {
	directory, err := localReplayCacheDirectory(sourcePath, submissionID, testcaseID)
	if err != nil {
		return localReplayCacheData{}, err
	}
	metadata, err := loadLocalReplayMetadata(directory, testcaseID)
	if err != nil {
		return localReplayCacheData{}, err
	}
	data := localReplayCacheData{Metadata: metadata}
	// Cache entries written before availability markers existed always contained
	// the complete four-file set. Keep those existing private caches readable.
	legacyComplete := !metadata.HasInput && !metadata.HasExpected && !metadata.HasStdout && !metadata.HasStderr
	for _, file := range []struct {
		name string
		has  bool
		to   *[]byte
	}{
		{"input.txt", metadata.HasInput, &data.Input},
		{"expected.txt", metadata.HasExpected, &data.Expected},
		{"stdout.txt", metadata.HasStdout, &data.Stdout},
		{"stderr.txt", metadata.HasStderr, &data.Stderr},
	} {
		if !file.has && !legacyComplete {
			continue
		}
		*file.to, err = readBoundedLocalFile(filepath.Join(directory, file.name))
		if err != nil {
			return localReplayCacheData{}, err
		}
	}
	return data, nil
}

func loadLocalReplayMetadata(directory string, testcaseID int) (localReplayMetadata, error) {
	metadataBytes, err := readBoundedLocalFile(filepath.Join(directory, "meta.json"))
	if err != nil {
		return localReplayMetadata{}, err
	}
	var metadata localReplayMetadata
	decoder := json.NewDecoder(bytes.NewReader(metadataBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil || metadata.TestcaseID != testcaseID {
		return localReplayMetadata{}, errors.New("invalid local replay cache")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return localReplayMetadata{}, errors.New("invalid local replay cache")
	}
	return metadata, nil
}

func writeLocalReplayMetadata(directory string, metadata localReplayMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writePrivateFile(filepath.Join(directory, "meta.json"), data); err != nil {
		return fmt.Errorf("write local replay cache: %w", err)
	}
	return nil
}

func readBoundedLocalFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxLocalReplayCacheFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxLocalReplayCacheFile {
		return nil, errors.New("local replay cache file is too large")
	}
	return data, nil
}
