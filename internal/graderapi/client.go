package graderapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout  = 15 * time.Second
	maxResponseBodySize = 1 << 20
)

var errCrossOriginRedirect = errors.New("cross-origin redirect rejected")

// Client is a small HTTP client for the grader API. It contains no command,
// persistence, or polling state.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      string
}

// NewClient validates baseURL and returns a client using httpClient. HTTP is
// accepted only for loopback hosts so local httptest servers can be used while
// production requests are required to use HTTPS.
func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("new client: %w: base URL is empty", ErrInvalidInput)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("new client: %w: invalid base URL", ErrInvalidInput)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("new client: %w: invalid base URL", ErrInvalidInput)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "https":
	case "http":
		if !isLoopbackHost(parsed.Hostname()) {
			return nil, fmt.Errorf("new client: %w: HTTP is allowed only for loopback test servers", ErrInvalidInput)
		}
	default:
		return nil, fmt.Errorf("new client: %w: base URL must use HTTPS", ErrInvalidInput)
	}

	configuredClient := httpClient
	if configuredClient == nil {
		configuredClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	clientCopy := *configuredClient
	configuredRedirect := configuredClient.CheckRedirect
	clientCopy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(parsed, req.URL) {
			return errCrossOriginRedirect
		}
		if configuredRedirect != nil {
			return configuredRedirect(req, via)
		}
		return nil
	}

	return &Client{
		baseURL:    parsed,
		httpClient: &clientCopy,
	}, nil
}

// WithToken returns a copy of c configured with token. It does not mutate c.
func (c *Client) WithToken(token string) *Client {
	if c == nil {
		return nil
	}
	copy := *c
	copy.token = token
	return &copy
}

func (c *Client) do(ctx context.Context, operation, method, apiPath string, body io.Reader, authenticated bool) ([]byte, error) {
	request, err := c.newRequest(ctx, method, apiPath, body, authenticated)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s: %w", operation, &HTTPError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
		})
	}

	responseBody, err := readBounded(response.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return responseBody, nil
}

func (c *Client) newRequest(ctx context.Context, method, apiPath string, body io.Reader, authenticated bool) (*http.Request, error) {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return nil, ErrInvalidInput
	}
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", ErrInvalidInput)
	}

	resolved, err := c.resolvePath(apiPath)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, resolved.String(), body)
	if err != nil {
		return nil, fmt.Errorf("%w: could not create request", ErrInvalidInput)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated && c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	return request, nil
}

func (c *Client) resolvePath(apiPath string) (*url.URL, error) {
	parsedPath, err := url.Parse(apiPath)
	if err != nil || parsedPath.IsAbs() || parsedPath.Host != "" || !strings.HasPrefix(parsedPath.Path, "/") {
		return nil, fmt.Errorf("%w: invalid API path", ErrInvalidInput)
	}
	return c.baseURL.ResolveReference(parsedPath), nil
}

func readBounded(body io.Reader) ([]byte, error) {
	return readBoundedLimit(body, maxResponseBodySize)
}

func readBoundedLimit(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: could not read response body", ErrInvalidResponse)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: response body is too large", ErrInvalidResponse)
	}
	return data, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) || !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}
