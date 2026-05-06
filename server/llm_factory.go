package server

import (
	"fmt"
	"strings"

	"neo-pkg-llm/config"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/logger"
)

// NewLLMSafe creates the appropriate LLM client based on config, returning an error instead of fatal.
func NewLLMSafe(cfg *config.Config) (llm.LLMProvider, error) {
	modelID := cfg.ResolveModelID()

	switch strings.ToLower(cfg.ResolveProvider()) {
	case "claude":
		if cfg.Claude.APIKey == "" {
			return nil, fmt.Errorf("Claude API key is required")
		}
		logger.Infof("LLM: Claude (%s)", modelID)
		return llm.NewClaudeClient(cfg.Claude.APIKey, modelID), nil

	case "chatgpt":
		if cfg.ChatGPT.APIKey == "" {
			return nil, fmt.Errorf("ChatGPT API key is required")
		}
		logger.Infof("LLM: ChatGPT (%s)", modelID)
		return llm.NewChatGPTClient(cfg.ChatGPT.APIKey, modelID), nil

	case "gemini":
		if cfg.Gemini.APIKey == "" {
			return nil, fmt.Errorf("Gemini API key is required")
		}
		logger.Infof("LLM: Gemini (%s)", modelID)
		return llm.NewGeminiClient(cfg.Gemini.APIKey, modelID), nil

	case "ollama":
		ollamaURL := cfg.OllamaURL()
		logger.Infof("LLM: Ollama (%s) at %s", modelID, ollamaURL)
		return llm.NewOllamaClient(ollamaURL, modelID), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s (use 'claude', 'chatgpt', 'gemini', or 'ollama')", cfg.ResolveProvider())
	}
}

// NewLLM creates the LLM client, fataling on error (used at startup).
func NewLLM(cfg *config.Config) llm.LLMProvider {
	client, err := NewLLMSafe(cfg)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	return client
}
