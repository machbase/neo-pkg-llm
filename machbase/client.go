package machbase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	BaseURL  string
	User     string
	Password string
	client   *http.Client
	tqlClient *http.Client

	mu       sync.Mutex
	jwtToken string
	jwtExp   time.Time
}

func NewClient(baseURL, user, password string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		User:      user,
		Password:  password,
		client:    &http.Client{Timeout: 60 * time.Second},
		tqlClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// UpdateConnection changes connection settings at runtime and resets cached JWT.
func (c *Client) UpdateConnection(baseURL, user, password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if baseURL != "" {
		c.BaseURL = strings.TrimRight(baseURL, "/")
	}
	if user != "" {
		c.User = user
	}
	if password != "" {
		c.Password = password
	}
	c.jwtToken = ""
	c.jwtExp = time.Time{}
}

// getJWT authenticates and caches a JWT token for /web/api/ endpoints.
func (c *Client) getJWT() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.jwtToken != "" && time.Now().Before(c.jwtExp) {
		return c.jwtToken, nil
	}

	return c.loginLocked()
}

// invalidateJWT clears the cached token so the next call to getJWT will re-login.
func (c *Client) invalidateJWT() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jwtToken = ""
	c.jwtExp = time.Time{}
}

// loginLocked performs the actual login. Must be called with c.mu held.
func (c *Client) loginLocked() (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"loginName": c.User,
		"password":  c.Password,
	})

	resp, err := c.client.Post(c.BaseURL+"/web/api/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success      bool   `json:"success"`
		Reason       string `json:"reason"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("login response decode failed: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("login failed: %s", result.Reason)
	}

	c.jwtToken = result.AccessToken
	c.jwtExp = time.Now().Add(5 * time.Minute)
	return c.jwtToken, nil
}

func (c *Client) authHeaders() (http.Header, error) {
	token, err := c.getJWT()
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	return h, nil
}

// isTokenExpiredResponse checks if a response indicates JWT token expiration.
func isTokenExpiredResponse(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "token") && strings.Contains(s, "expired")
}

// doWithRetry executes an HTTP request, retrying once on JWT expiration.
func (c *Client) doWithRetry(buildReq func() (*http.Request, error)) ([]byte, error) {
	req, err := buildReq()
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// If token expired, invalidate and retry once
	if isTokenExpiredResponse(body) {
		c.invalidateJWT()
		headers, err := c.authHeaders()
		if err != nil {
			return nil, fmt.Errorf("re-login failed: %w", err)
		}
		req2, err := buildReq()
		if err != nil {
			return nil, err
		}
		req2.Header = headers
		resp2, err := c.client.Do(req2)
		if err != nil {
			return nil, err
		}
		defer resp2.Body.Close()
		return io.ReadAll(resp2.Body)
	}

	return body, nil
}

// --- DB API (no auth) ---

// QuerySQL executes a SQL query via /db/query endpoint.
func (c *Client) QuerySQL(sql, timeformat, tz, format string) (string, error) {
	params := url.Values{}
	params.Set("q", sql)
	if timeformat != "" {
		params.Set("timeformat", timeformat)
	}
	if tz != "" {
		params.Set("tz", tz)
	}
	if format != "" {
		params.Set("format", format)
	}

	u := c.BaseURL + "/db/query?" + params.Encode()
	resp, err := c.client.Get(u)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// ExecuteTQL executes a TQL script via POST /db/tql.
func (c *Client) ExecuteTQL(tqlContent string) (string, error) {
	resp, err := c.tqlClient.Post(
		c.BaseURL+"/db/tql",
		"text/plain",
		strings.NewReader(tqlContent),
	)
	if err != nil {
		return "", fmt.Errorf("tql execution failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// --- Web API (JWT auth) ---

// WebGet makes an authenticated GET request to /web/api/.
func (c *Client) WebGet(path string) ([]byte, error) {
	return c.doWithRetry(func() (*http.Request, error) {
		headers, err := c.authHeaders()
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequest("GET", c.BaseURL+path, nil)
		req.Header = headers
		return req, nil
	})
}

// WebPost makes an authenticated POST request with JSON body.
func (c *Client) WebPost(path string, payload any) ([]byte, error) {
	return c.doWithRetry(func() (*http.Request, error) {
		headers, err := c.authHeaders()
		if err != nil {
			return nil, err
		}
		var body io.Reader
		if payload != nil {
			data, _ := json.Marshal(payload)
			body = bytes.NewReader(data)
		}
		req, _ := http.NewRequest("POST", c.BaseURL+path, body)
		req.Header = headers
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}

// WebDelete makes an authenticated DELETE request.
func (c *Client) WebDelete(path string) ([]byte, error) {
	return c.doWithRetry(func() (*http.Request, error) {
		headers, err := c.authHeaders()
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequest("DELETE", c.BaseURL+path, nil)
		req.Header = headers
		return req, nil
	})
}

// WebPut makes an authenticated PUT request with JSON body.
func (c *Client) WebPut(path string, payload any) ([]byte, error) {
	return c.doWithRetry(func() (*http.Request, error) {
		headers, err := c.authHeaders()
		if err != nil {
			return nil, err
		}
		var body io.Reader
		if payload != nil {
			data, _ := json.Marshal(payload)
			body = bytes.NewReader(data)
		}
		req, _ := http.NewRequest("PUT", c.BaseURL+path, body)
		req.Header = headers
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}

// WebPostRaw posts raw text content (for file saving).
func (c *Client) WebPostRaw(path, contentType string, data []byte) ([]byte, error) {
	return c.doWithRetry(func() (*http.Request, error) {
		headers, err := c.authHeaders()
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader(data))
		req.Header = headers
		req.Header.Set("Content-Type", contentType)
		return req, nil
	})
}

// EscapePath escapes each segment of a path individually,
// preserving "/" as path separators.
func EscapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// --- File operations via Web API ---

// CreateFolder creates a folder in Machbase Neo file system.
func (c *Client) CreateFolder(folderPath string) error {
	apiPath := "/web/api/files/" + EscapePath(folderPath) + "/"
	data, err := c.WebPost(apiPath, nil)
	if err != nil {
		return err
	}
	var resp map[string]any
	if json.Unmarshal(data, &resp) == nil {
		if success, ok := resp["success"].(bool); ok && !success {
			reason, _ := resp["reason"].(string)
			if strings.Contains(strings.ToLower(reason), "already exist") {
				return nil
			}
			return fmt.Errorf("create folder failed: %s", reason)
		}
	}
	return nil
}

// FileExists checks if a file exists.
func (c *Client) FileExists(filePath string) bool {
	data, err := c.WebGet("/web/api/files/" + EscapePath(filePath))
	if err != nil {
		return false
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		return false
	}
	if success, ok := result["success"].(bool); ok {
		return success
	}
	return false
}

// WriteFile saves content to a file via the Web API.
func (c *Client) WriteFile(relPath string, data []byte) error {
	apiPath := "/web/api/files/" + EscapePath(relPath)
	_, err := c.WebPostRaw(apiPath, "application/octet-stream", data)
	return err
}

// ReadFile reads a file via the Web API.
func (c *Client) ReadFile(relPath string) ([]byte, error) {
	return c.WebGet("/web/api/files/" + EscapePath(relPath))
}

// DeleteFile removes a file or empty folder via the Web API.
func (c *Client) DeleteFile(relPath string) error {
	_, err := c.WebDelete("/web/api/files/" + EscapePath(relPath))
	return err
}

// ListDir lists files and folders via the Web API.
func (c *Client) ListDir(relPath string) ([]map[string]string, error) {
	data, err := c.WebGet("/web/api/files/" + EscapePath(relPath))
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("list_dir response parse failed: %w", err)
	}
	var result []map[string]string
	if d, ok := resp["data"].(map[string]any); ok {
		if children, ok := d["children"].([]any); ok {
			for _, c := range children {
				child, _ := c.(map[string]any)
				name, _ := child["name"].(string)
				cType, _ := child["type"].(string)
				result = append(result, map[string]string{"name": name, "type": cType})
			}
		}
	}
	return result, nil
}
