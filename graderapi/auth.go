package graderapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Session is the authenticated session returned by the grader.
type Session struct {
	Token     string
	ExpiresAt time.Time
	User      User
}

// User contains the identity fields returned by the login endpoint.
type User struct {
	ID       int    `json:"id"`
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

func (c *Client) Login(ctx context.Context, username, password string) (Session, error) {
	if strings.TrimSpace(username) == "" || password == "" {
		return Session{}, fmt.Errorf("login: %w: username and password are required", ErrInvalidInput)
	}

	payload := struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}{
		Login:    username,
		Password: password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Session{}, fmt.Errorf("login: %w: could not encode request", ErrInvalidInput)
	}

	responseBody, err := c.do(ctx, "login", http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body), false)
	if err != nil {
		return Session{}, err
	}

	var response struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
		User      *User     `json:"user"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return Session{}, fmt.Errorf("login: %w: malformed response", ErrInvalidResponse)
	}
	if response.Token == "" || response.ExpiresAt.IsZero() || response.User == nil {
		return Session{}, fmt.Errorf("login: %w: response is missing required fields", ErrInvalidResponse)
	}

	return Session{
		Token:     response.Token,
		ExpiresAt: response.ExpiresAt,
		User:      *response.User,
	}, nil
}
