package machbase

import (
	"bytes"
	"encoding/base64"
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

	mu       sync.Mutex
	jwtToken string
	jwtExp   time.Time
}

func NewClient(baseURL, user, password string) *Client {
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		User:     user,
		Password: password,
		client:   &http.Client{Timeout: 60 * time.Second},
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
func (c *Client) ExecuteTQL(tqlContent string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(
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

// VerifyToken validates a Neo JWT token by calling /web/api/check.
// Returns the login name (from JWT "sub" claim) on success, or an error if invalid/expired.
func (c *Client) VerifyToken(token string) (string, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+"/web/api/check", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token verify request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Success bool   `json:"success"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("token verify decode failed: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("invalid token: %s", result.Reason)
	}

	// Extract "sub" (loginName) from JWT payload
	sub, err := jwtSubject(token)
	if err != nil {
		return "", fmt.Errorf("token parse failed: %w", err)
	}
	return sub, nil
}

// GetAccessToken performs login and returns the access token.
func (c *Client) GetAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked()
}

// jwtSubject extracts the "sub" claim from a JWT without signature verification
// (signature is already validated by Neo's /web/api/check).
func jwtSubject(token string) (string, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid JWT format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("JWT payload decode failed: %w", err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("JWT claims parse failed: %w", err)
	}
	if claims.Sub == "" {
		return "", fmt.Errorf("JWT missing sub claim")
	}
	return claims.Sub, nil
}

// EscapePath escapes each segment of a path individually,
// preserving "/" as path separators.
// e.g., "BITCOIN/chart 1.tql" → "BITCOIN/chart%201.tql"
func EscapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// CreateFolder creates a folder in Machbase Neo file system.
// Neo API: POST /web/api/files/{parent}/{name}/ (trailing slash, empty body)
func (c *Client) CreateFolder(folderPath string) error {
	apiPath := "/web/api/files/" + EscapePath(folderPath) + "/"
	data, err := c.WebPost(apiPath, nil)
	if err != nil {
		return err
	}
	// Check API response
	var resp map[string]any
	if json.Unmarshal(data, &resp) == nil {
		if success, ok := resp["success"].(bool); ok && !success {
			reason, _ := resp["reason"].(string)
			// "already exists" is not a real error
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
