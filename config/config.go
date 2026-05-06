package config

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"neo-pkg-llm/logger"
)

// --- JSON config structures ---

type ModelEntry struct {
	Name    string `json:"name"`
	ModelID string `json:"model_id"`
}

type MachbaseConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type APIProviderConfig struct {
	APIKey string       `json:"api_key"`
	Models []ModelEntry `json:"models"`
}

type OllamaConfig struct {
	BaseURL     string       `json:"base_url"`
	Models      []ModelEntry `json:"models"`
	Temperature *float64     `json:"temperature,omitempty"`
	NumPredict  int          `json:"num_predict,omitempty"`
	NumCtx      int          `json:"num_ctx,omitempty"`
	NumGPU      int          `json:"num_gpu,omitempty"`
}

type ServerConfig struct {
	Port string `json:"port"`
}

type Config struct {
	Server   ServerConfig      `json:"server"`
	Machbase MachbaseConfig    `json:"machbase"`
	Claude   APIProviderConfig `json:"claude"`
	ChatGPT  APIProviderConfig `json:"chatgpt"`
	Gemini   APIProviderConfig `json:"gemini"`
	Ollama   OllamaConfig      `json:"ollama"`

	// Runtime only (CLI flags / env vars, not saved to config.json)
	Provider string `json:"-"`
	Model    string `json:"-"`

	ConfigPath string `json:"-"` // internal: path used for Save
}

// --- Default config ---

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: "8884",
		},
		Machbase: MachbaseConfig{
			Host:     "127.0.0.1",
			Port:     "5654",
			User:     "sys",
			Password: "manager",
		},
		Claude: APIProviderConfig{
			APIKey: "",
			Models: []ModelEntry{
				{Name: "sonnet", ModelID: "claude-sonnet-4-20250514"},
				{Name: "haiku", ModelID: "claude-haiku-4-5-20251001"},
			},
		},
		ChatGPT: APIProviderConfig{
			APIKey: "",
			Models: []ModelEntry{
				{Name: "gpt-4o"},
				{Name: "gpt-4o-mini"},
			},
		},
		Gemini: APIProviderConfig{
			APIKey: "",
			Models: []ModelEntry{
				{Name: "gemini-2.5-flash", ModelID: "gemini-2.5-flash-preview-04-17"},
			},
		},
		Ollama: OllamaConfig{
			BaseURL: "",
			Models: []ModelEntry{
				{Name: "qwen3:8b"},
			},
		},
	}
}

// --- Load / Save ---

func LoadConfig(path string) *Config {
	LoadDotEnv(".env")

	cfg := DefaultConfig()
	cfg.ConfigPath = path

	data, err := os.ReadFile(path)
	if err != nil {
		// config.json not found → create with defaults
		logger.Infof("Config file not found (%s), creating with defaults", path)
		os.MkdirAll(filepath.Dir(path), 0755)
		cfg.Save()
		cfg.ApplyEnvOverrides()
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		logger.Infof("Config parse error: %v, using defaults", err)
		cfg = DefaultConfig()
		cfg.ConfigPath = path
	}

	cfg.ApplyEnvOverrides()
	return cfg
}

func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigPath, data, 0644)
}

// ApplyEnvOverrides lets environment variables override config.json values.
func (c *Config) ApplyEnvOverrides() {
	if v := os.Getenv("MACHBASE_HOST"); v != "" {
		c.Machbase.Host = v
	}
	if v := os.Getenv("MACHBASE_PORT"); v != "" {
		c.Machbase.Port = v
	}
	if v := os.Getenv("MACHBASE_USER"); v != "" {
		c.Machbase.User = v
	}
	if v := os.Getenv("MACHBASE_PASSWORD"); v != "" {
		c.Machbase.Password = v
	}
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		c.Model = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		c.Claude.APIKey = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		c.ChatGPT.APIKey = v
	}
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		c.Gemini.APIKey = v
	}
	if v := os.Getenv("OLLAMA_BASE_URL"); v != "" {
		c.Ollama.BaseURL = v
	}
}

// --- Resolve helpers ---

func (c *Config) MachbaseURL() string {
	return "http://" + c.Machbase.Host + ":" + c.Machbase.Port
}

// OllamaURL returns the Ollama base URL.
// If base_url is empty, uses the machbase host with default Ollama port 11434.
func (c *Config) OllamaURL() string {
	if c.Ollama.BaseURL != "" {
		return c.Ollama.BaseURL
	}
	return "http://" + c.Machbase.Host + ":11434"
}

// ResolveProvider returns the active provider.
// Priority: CLI flag > env var > first provider with API key.
func (c *Config) ResolveProvider() string {
	if c.Provider != "" {
		return c.Provider
	}
	if c.Claude.APIKey != "" {
		return "claude"
	}
	if c.ChatGPT.APIKey != "" {
		return "chatgpt"
	}
	if c.Gemini.APIKey != "" {
		return "gemini"
	}
	if c.Ollama.BaseURL != "" || len(c.Ollama.Models) > 0 {
		return "ollama"
	}
	return "gemini" // fallback
}

// ResolveModel returns the active model name.
// If not set, returns the first model name of the resolved provider.
func (c *Config) ResolveModel() string {
	if c.Model != "" {
		return c.Model
	}
	models := c.CurrentModels()
	if len(models) > 0 {
		return models[0].Name
	}
	return ""
}

// ResolveModelID looks up the model name in the current provider's models list.
// If found, returns the model_id. Otherwise returns the input as-is (raw model_id).
func (c *Config) ResolveModelID() string {
	model := c.ResolveModel()
	models := c.CurrentModels()
	for _, m := range models {
		if strings.EqualFold(m.Name, model) {
			if m.ModelID != "" {
				return m.ModelID
			}
			return m.Name // model_id 없으면 name을 model ID로 사용
		}
	}
	return model
}

// GetAPIKey returns the API key for the current provider.
func (c *Config) GetAPIKey() string {
	switch strings.ToLower(c.ResolveProvider()) {
	case "claude":
		return c.Claude.APIKey
	case "chatgpt":
		return c.ChatGPT.APIKey
	case "gemini":
		return c.Gemini.APIKey
	case "ollama":
		return "" // Ollama doesn't require API key
	}
	return ""
}

func (c *Config) CurrentModels() []ModelEntry {
	switch strings.ToLower(c.ResolveProvider()) {
	case "claude":
		return c.Claude.Models
	case "chatgpt":
		return c.ChatGPT.Models
	case "gemini":
		return c.Gemini.Models
	case "ollama":
		return c.Ollama.Models
	}
	return nil
}

// --- Sensitive field masking ---

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "********"
	}
	return s[:4] + "********" + s[len(s)-4:]
}

// MaskedCopy returns a copy of the config with sensitive fields masked.
func (c *Config) MaskedCopy() Config {
	cp := *c
	if cp.Machbase.Password != "" {
		cp.Machbase.Password = maskSecret(cp.Machbase.Password)
	}
	if cp.Claude.APIKey != "" {
		cp.Claude.APIKey = maskSecret(cp.Claude.APIKey)
	}
	if cp.ChatGPT.APIKey != "" {
		cp.ChatGPT.APIKey = maskSecret(cp.ChatGPT.APIKey)
	}
	if cp.Gemini.APIKey != "" {
		cp.Gemini.APIKey = maskSecret(cp.Gemini.APIKey)
	}
	return cp
}

// IsMasked returns true if the value looks like a masked secret.
func IsMasked(s string) bool {
	return strings.Contains(s, "********")
}

// RestoreSecrets replaces masked values in the incoming config with
// the original values from the existing config.
func (c *Config) RestoreSecrets(existing *Config) {
	if IsMasked(c.Machbase.Password) {
		c.Machbase.Password = existing.Machbase.Password
	}
	if IsMasked(c.Claude.APIKey) {
		c.Claude.APIKey = existing.Claude.APIKey
	}
	if IsMasked(c.ChatGPT.APIKey) {
		c.ChatGPT.APIKey = existing.ChatGPT.APIKey
	}
	if IsMasked(c.Gemini.APIKey) {
		c.Gemini.APIKey = existing.Gemini.APIKey
	}
}

// ValidateConfig checks required fields.
func ValidateConfig(c *Config) string {
	switch {
	case c.Machbase.Host == "":
		return "machbase.host is required"
	case c.Machbase.Port == "":
		return "machbase.port is required"
	case c.Machbase.User == "":
		return "machbase.user is required"
	case c.Machbase.Password == "":
		return "machbase.password is required"
	}
	return ""
}

// --- .env loader ---

func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
