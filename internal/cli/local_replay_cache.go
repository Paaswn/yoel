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

func readLocalReplayCache(sourcePath string, submissionID, testcaseID int) (localReplayCacheData, error) {
	directory, err := localReplayCacheDirectory(sourcePath, submissionID, testcaseID)
	if err != nil {
		return localReplayCacheData{}, err
	}
	metadataBytes, err := readBoundedLocalFile(filepath.Join(directory, "meta.json"))
	if err != nil {
		return localReplayCacheData{}, err
	}
	var data localReplayCacheData
	decoder := json.NewDecoder(bytes.NewReader(metadataBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data.Metadata); err != nil || data.Metadata.TestcaseID != testcaseID {
		return localReplayCacheData{}, errors.New("invalid local replay cache")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return localReplayCacheData{}, errors.New("invalid local replay cache")
	}
	for filename, target := range map[string]*[]byte{
		"input.txt":    &data.Input,
		"expected.txt": &data.Expected,
		"stdout.txt":   &data.Stdout,
		"stderr.txt":   &data.Stderr,
	} {
		*target, err = readBoundedLocalFile(filepath.Join(directory, filename))
		if err != nil {
			return localReplayCacheData{}, err
		}
	}
	return data, nil
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
