package llm

import (
	"context"
	"encoding/json"
)

// LLMProvider is the interface that all LLM clients implement.
type LLMProvider interface {
	Chat(ctx context.Context, messages []Message, tools []map[string]any) (*ChatResponse, error)
	ChatStream(ctx context.Context, messages []Message, tools []map[string]any, cb StreamCallback) (*ChatResponse, error)
}

// StreamCallback is called for each streaming chunk.
type StreamCallback func(resp *ChatResponse)

// Message represents a chat message shared across all LLM providers.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// RawModelParts preserves the raw JSON of Gemini model response parts.
	// This ensures exact echo-back including thoughtSignature and other fields.
	RawModelParts json.RawMessage `json:"-"`
}

// ToolCall represents a function call requested by the LLM.
type ToolCall struct {
	Function         ToolCallFunction `json:"function"`
	ThoughtSignature string           `json:"thought_signature,omitempty"` // Gemini thinking models
}

// ToolCallFunction holds the function name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ChatResponse is the unified response type for all LLM providers.
type ChatResponse struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}
