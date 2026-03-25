package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"strings"
)

// --- JSON config structures ---

type ModelEntry struct {
	Name    string `json:"name"`
	ModelID string `json:"model_id"`
}

type MachbaseConfig struct {
	Host    string `json:"host"`
	Port    string `json:"port"`
	User    string `json:"user"`
	WorkDir string `json:"work_dir"`
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

	configPath string // internal: path used for Save
}

// --- Default config ---

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: "8884",
		},
		Machbase: MachbaseConfig{
			Host: "127.0.0.1",
			Port: "5654",
			User: "sys",
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
	loadDotEnv(".env")

	cfg := defaultConfig()
	cfg.configPath = path

	data, err := os.ReadFile(path)
	if err != nil {
		// config.json not found → create with defaults
		log.Printf("Config file not found (%s), creating with defaults", path)
		cfg.Save()
		cfg.applyEnvOverrides()
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		log.Printf("Config parse error: %v, using defaults", err)
		cfg = defaultConfig()
		cfg.configPath = path
	}

	cfg.applyEnvOverrides()
	return cfg
}

func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configPath, data, 0644)
}

// applyEnvOverrides lets environment variables override config.json values.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("MACHBASE_HOST"); v != "" {
		c.Machbase.Host = v
	}
	if v := os.Getenv("MACHBASE_PORT"); v != "" {
		c.Machbase.Port = v
	}
	if v := os.Getenv("MACHBASE_USER"); v != "" {
		c.Machbase.User = v
	}
	if v := os.Getenv("MACHBASE_WORK_DIR"); v != "" {
		c.Machbase.WorkDir = v
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
	models := c.currentModels()
	if len(models) > 0 {
		return models[0].Name
	}
	return ""
}

// ResolveModelID looks up the model name in the current provider's models list.
// If found, returns the model_id. Otherwise returns the input as-is (raw model_id).
func (c *Config) ResolveModelID() string {
	model := c.ResolveModel()
	models := c.currentModels()
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

func (c *Config) currentModels() []ModelEntry {
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

// --- .env loader (unchanged) ---

func loadDotEnv(path string) {
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
