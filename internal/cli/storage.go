package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"yoel/internal/graderapi"
)

const (
	applicationDir      = "yoel"
	sessionFilename     = "session.json"
	questionsFilename   = "questions.json"
	updateCheckFilename = "update-check.json"
	questionCacheTTL    = 5 * time.Minute
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

type updateCheckCache struct {
	CheckedAt time.Time `json:"checked_at"`
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
	session, err := loadStoredSession()
	if err != nil {
		return storedSession{}, err
	}
	if !now.Before(session.ExpiresAt) {
		return storedSession{}, errors.New("saved session expired; run yoel login")
	}
	return session, nil
}

func loadStoredSession() (storedSession, error) {
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

func updateCheckPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	return filepath.Join(dir, applicationDir, updateCheckFilename), nil
}

func loadUpdateCheckCache() (updateCheckCache, error) {
	path, err := updateCheckPath()
	if err != nil {
		return updateCheckCache{}, err
	}
	var cache updateCheckCache
	if err := readJSON(path, &cache); err != nil {
		return updateCheckCache{}, err
	}
	if cache.CheckedAt.IsZero() {
		return updateCheckCache{}, errors.New("update check cache is invalid")
	}
	return cache, nil
}

func saveUpdateCheckCache(cache updateCheckCache) error {
	if cache.CheckedAt.IsZero() {
		return errors.New("update check cache is invalid")
	}
	path, err := updateCheckPath()
	if err != nil {
		return err
	}
	return writePrivateJSON(path, cache)
}

func statementPDFPath(baseURL string, userID, problemID int) (string, error) {
	if baseURL == "" || userID <= 0 || problemID <= 0 {
		return "", errors.New("invalid statement cache key")
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find user cache directory: %w", err)
	}
	serverKey := fmt.Sprintf("%x", sha256.Sum256([]byte(baseURL)))
	return filepath.Join(dir, applicationDir, "statements", serverKey, fmt.Sprintf("user-%d", userID), fmt.Sprintf("%d.pdf", problemID)), nil
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
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writePrivateFile(path, data)
}

func writePrivateFile(path string, data []byte) error {
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
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporaryPath, path)
}
