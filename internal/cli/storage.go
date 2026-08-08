package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"yoel/internal/graderapi"
)

const (
	applicationDir    = "yoel"
	sessionFilename   = "session.json"
	questionsFilename = "questions.json"
	questionCacheTTL  = 5 * time.Minute
)

type storedSession struct {
	BaseURL   string         `json:"base_url"`
	Token     string         `json:"token"`
	ExpiresAt time.Time      `json:"expires_at"`
	User      graderapi.User `json:"user"`
}

type questionCache struct {
	BaseURL   string              `json:"base_url"`
	UserID    int                 `json:"user_id"`
	FetchedAt time.Time           `json:"fetched_at"`
	Problems  []graderapi.Problem `json:"problems"`
}

func saveSession(session storedSession) error {
	if session.BaseURL == "" || session.Token == "" || session.ExpiresAt.IsZero() {
		return errors.New("session is missing required fields")
	}
	path, err := sessionPath()
	if err != nil {
		return err
	}
	return writePrivateJSON(path, session)
}

func loadSession(now time.Time) (storedSession, error) {
	path, err := sessionPath()
	if err != nil {
		return storedSession{}, err
	}
	var session storedSession
	if err := readJSON(path, &session); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storedSession{}, errors.New("no saved session; run yoel login")
		}
		return storedSession{}, fmt.Errorf("read saved session: %w", err)
	}
	if session.BaseURL == "" || session.Token == "" || session.ExpiresAt.IsZero() {
		return storedSession{}, errors.New("saved session is invalid; run yoel login")
	}
	if !now.Before(session.ExpiresAt) {
		return storedSession{}, errors.New("saved session expired; run yoel login")
	}
	return session, nil
}

func saveQuestionCache(cache questionCache) error {
	path, err := questionCachePath()
	if err != nil {
		return err
	}
	return writePrivateJSON(path, cache)
}

func loadQuestionCache() (questionCache, error) {
	path, err := questionCachePath()
	if err != nil {
		return questionCache{}, err
	}
	var cache questionCache
	if err := readJSON(path, &cache); err != nil {
		return questionCache{}, err
	}
	return cache, nil
}

func sessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, applicationDir, sessionFilename), nil
}

func questionCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(dir, applicationDir, questionsFilename), nil
}

func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid JSON")
	}
	return nil
}

func writePrivateJSON(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}

	file, err := os.CreateTemp(dir, ".yoel-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(value)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporaryPath, path)
}
