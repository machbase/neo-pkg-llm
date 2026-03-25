package machbase

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	User    string
	WorkDir string
	client  *http.Client
}

func NewClient(baseURL, user, workDir string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		User:    user,
		WorkDir: workDir,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// UpdateConnection changes connection settings at runtime.
func (c *Client) UpdateConnection(baseURL, user string) {
	if baseURL != "" {
		c.BaseURL = strings.TrimRight(baseURL, "/")
	}
	if user != "" {
		c.User = user
	}
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

// --- Local filesystem methods ---

// localPath joins WorkDir with a relative file path.
func (c *Client) localPath(relPath string) string {
	return filepath.Join(c.WorkDir, filepath.FromSlash(relPath))
}

// CreateFolder creates a folder under WorkDir.
func (c *Client) CreateFolder(folderPath string) error {
	return os.MkdirAll(c.localPath(folderPath), 0755)
}

// FileExists checks if a file exists under WorkDir.
func (c *Client) FileExists(filePath string) bool {
	_, err := os.Stat(c.localPath(filePath))
	return err == nil
}

// ReadFile reads a file from WorkDir.
func (c *Client) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(c.localPath(relPath))
}

// WriteFile writes a file to WorkDir, creating parent dirs as needed.
func (c *Client) WriteFile(relPath string, data []byte) error {
	full := c.localPath(relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0644)
}

// DeleteFile removes a file or empty directory from WorkDir.
func (c *Client) DeleteFile(relPath string) error {
	return os.Remove(c.localPath(relPath))
}

// ListDir lists files and folders in a directory under WorkDir.
func (c *Client) ListDir(relPath string) ([]map[string]string, error) {
	entries, err := os.ReadDir(c.localPath(relPath))
	if err != nil {
		return nil, err
	}
	var result []map[string]string
	for _, e := range entries {
		typ := "file"
		if e.IsDir() {
			typ = "dir"
		}
		result = append(result, map[string]string{"name": e.Name(), "type": typ})
	}
	return result, nil
}
