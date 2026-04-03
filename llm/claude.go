package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Claude API types (Anthropic Messages API)

type ClaudeClient struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewClaudeClient(apiKey, model string) *ClaudeClient {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &ClaudeClient{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.anthropic.com",
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// --- Request types ---

type ClaudeRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    []ClaudeSystemBlock `json:"system,omitempty"`
	Messages  []ClaudeMessage     `json:"messages"`
	Tools     []ClaudeTool        `json:"tools,omitempty"`
	Stream    bool                `json:"stream,omitempty"`
}

type ClaudeMessage struct {
	Role    string         `json:"role"`
	Content json.RawMessage `json:"content"` // string or []ContentBlock
}

// NewTextMessage creates a message with plain text content.
func NewTextMessage(role, text string) ClaudeMessage {
	raw, _ := json.Marshal(text)
	return ClaudeMessage{Role: role, Content: raw}
}

// NewBlocksMessage creates a message with content blocks.
func NewBlocksMessage(role string, blocks []ContentBlock) ClaudeMessage {
	raw, _ := json.Marshal(blocks)
	return ClaudeMessage{Role: role, Content: raw}
}

type ContentBlock struct {
	Type      string         `json:"-"`
	Text      string         `json:"-"`
	ID        string         `json:"-"`
	Name      string         `json:"-"`
	Input     map[string]any `json:"-"`
	ToolUseID string         `json:"-"`
	Content   string         `json:"-"`
}

func (cb ContentBlock) MarshalJSON() ([]byte, error) {
	switch cb.Type {
	case "tool_use":
		input := cb.Input
		if input == nil {
			input = map[string]any{}
		}
		return json.Marshal(struct {
			Type  string         `json:"type"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		}{cb.Type, cb.ID, cb.Name, input})
	case "tool_result":
		return json.Marshal(struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
		}{cb.Type, cb.ToolUseID, cb.Content})
	default: // "text"
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{cb.Type, cb.Text})
	}
}

func (cb *ContentBlock) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type      string         `json:"type"`
		Text      string         `json:"text"`
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Input     map[string]any `json:"input"`
		ToolUseID string         `json:"tool_use_id"`
		Content   string         `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	cb.Type = raw.Type
	cb.Text = raw.Text
	cb.ID = raw.ID
	cb.Name = raw.Name
	cb.Input = raw.Input
	cb.ToolUseID = raw.ToolUseID
	cb.Content = raw.Content
	return nil
}

type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type ClaudeSystemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type ClaudeTool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	InputSchema  ClaudeToolSchema `json:"input_schema"`
	CacheControl *CacheControl    `json:"cache_control,omitempty"`
}

type ClaudeToolSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]any          `json:"properties"`
	Required   []string                `json:"required,omitempty"`
}

// --- Response types ---

type ClaudeResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	Usage        ClaudeUsage    `json:"usage"`
}

type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// --- Caching helpers ---

// buildSystemBlocks converts a plain system prompt string into a block array with cache_control.
func buildSystemBlocks(system string) []ClaudeSystemBlock {
	if system == "" {
		return nil
	}
	return []ClaudeSystemBlock{
		{
			Type:         "text",
			Text:         system,
			CacheControl: &CacheControl{Type: "ephemeral"},
		},
	}
}

// applyToolsCacheControl sets cache_control on the last tool definition.
func applyToolsCacheControl(tools []ClaudeTool) {
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = &CacheControl{Type: "ephemeral"}
	}
}

// --- Conversion helpers ---

// ToolDefsToClaudeTools converts OpenAI-compatible tool definitions to Claude format.
func ToolDefsToClaudeTools(defs []map[string]any) []ClaudeTool {
	var claudeTools []ClaudeTool
	for _, def := range defs {
		fn, _ := def["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)

		params, _ := fn["parameters"].(map[string]any)
		props := map[string]any{}
		var required []string
		if params != nil {
			if p, ok := params["properties"]; ok {
				props, _ = p.(map[string]any)
			}
			if r, ok := params["required"].([]any); ok {
				for _, v := range r {
					if s, ok := v.(string); ok {
						required = append(required, s)
					}
				}
			}
			// Also handle typed required
			if r, ok := params["required"].([]string); ok {
				required = r
			}
		}

		claudeTools = append(claudeTools, ClaudeTool{
			Name:        name,
			Description: desc,
			InputSchema: ClaudeToolSchema{
				Type:       "object",
				Properties: props,
				Required:   required,
			},
		})
	}
	return claudeTools
}

// MessagesToClaudeMessages converts internal Message format to Claude API format.
// Returns (system_prompt, messages).
func MessagesToClaudeMessages(msgs []Message) (string, []ClaudeMessage) {
	var system string
	var claudeMsgs []ClaudeMessage

	// Collect tool results to batch into a single user message with tool_result blocks
	var pendingToolResults []ContentBlock

	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			system = msg.Content

		case "user":
			// Flush pending tool results first
			if len(pendingToolResults) > 0 {
				claudeMsgs = append(claudeMsgs, NewBlocksMessage("user", pendingToolResults))
				pendingToolResults = nil
			}
			claudeMsgs = append(claudeMsgs, NewTextMessage("user", msg.Content))

		case "assistant":
			// Flush pending tool results first
			if len(pendingToolResults) > 0 {
				claudeMsgs = append(claudeMsgs, NewBlocksMessage("user", pendingToolResults))
				pendingToolResults = nil
			}

			if len(msg.ToolCalls) > 0 {
				var blocks []ContentBlock
				if msg.Content != "" {
					blocks = append(blocks, ContentBlock{Type: "text", Text: msg.Content})
				}
				for i, tc := range msg.ToolCalls {
					input := tc.Function.Arguments
					if input == nil {
						input = map[string]any{}
					}
					blocks = append(blocks, ContentBlock{
						Type:  "tool_use",
						ID:    "call_" + strconv.Itoa(len(claudeMsgs)) + "_" + strconv.Itoa(i),
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				claudeMsgs = append(claudeMsgs, NewBlocksMessage("assistant", blocks))
			} else {
				claudeMsgs = append(claudeMsgs, NewTextMessage("assistant", msg.Content))
			}

		case "tool":
			// Match tool_use ID by position: Nth tool result → Nth tool_use block
			toolUseID := "unknown_call"
			resultIdx := len(pendingToolResults) // how many results collected so far
			if len(claudeMsgs) > 0 {
				lastMsg := claudeMsgs[len(claudeMsgs)-1]
				if lastMsg.Role == "assistant" {
					var blocks []ContentBlock
					json.Unmarshal(lastMsg.Content, &blocks)
					tuIdx := 0
					for _, b := range blocks {
						if b.Type == "tool_use" {
							if tuIdx == resultIdx {
								toolUseID = b.ID
								break
							}
							tuIdx++
						}
					}
				}
			}
			pendingToolResults = append(pendingToolResults, ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   msg.Content,
			})
		}
	}

	// Flush remaining tool results
	if len(pendingToolResults) > 0 {
		claudeMsgs = append(claudeMsgs, NewBlocksMessage("user", pendingToolResults))
	}

	return system, claudeMsgs
}

// ClaudeResponseToMessage converts Claude API response to internal Message format.
func ClaudeResponseToMessage(resp *ClaudeResponse) Message {
	var msg Message
	msg.Role = "assistant"

	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				Function: ToolCallFunction{
					Name:      block.Name,
					Arguments: block.Input,
				},
			})
		}
	}
	msg.Content = strings.Join(textParts, "")
	return msg
}

// --- API calls ---

// Chat sends a non-streaming request to Claude API.
func (c *ClaudeClient) Chat(ctx context.Context, messages []Message, toolDefs []map[string]any) (*ChatResponse, error) {
	system, claudeMsgs := MessagesToClaudeMessages(messages)
	claudeTools := ToolDefsToClaudeTools(toolDefs)

	applyToolsCacheControl(claudeTools)
	reqBody := ClaudeRequest{
		Model:     c.Model,
		MaxTokens: 4096,
		System:    buildSystemBlocks(system),
		Messages:  claudeMsgs,
		Tools:     claudeTools,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, newAPIError("Claude", resp.StatusCode, string(errBody))
	}

	var claudeResp ClaudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, fmt.Errorf("failed to decode claude response: %w", err)
	}

	logClaudeCacheUsage(&claudeResp.Usage)

	msg := ClaudeResponseToMessage(&claudeResp)

	return &ChatResponse{
		Model:   c.Model,
		Message: msg,
		Done:    true,
	}, nil
}

// logClaudeCacheUsage logs prompt cache hit/creation information.
func logClaudeCacheUsage(usage *ClaudeUsage) {
	if usage == nil {
		return
	}
	if usage.CacheReadInputTokens > 0 {
		fmt.Printf("[Claude] prompt cache hit: %d tokens read from cache\n", usage.CacheReadInputTokens)
	}
	if usage.CacheCreationInputTokens > 0 {
		fmt.Printf("[Claude] prompt cache created: %d tokens cached\n", usage.CacheCreationInputTokens)
	}
}

// ChatStream sends a streaming request to Claude API.
func (c *ClaudeClient) ChatStream(ctx context.Context, messages []Message, toolDefs []map[string]any, cb StreamCallback) (*ChatResponse, error) {
	system, claudeMsgs := MessagesToClaudeMessages(messages)
	claudeTools := ToolDefsToClaudeTools(toolDefs)

	applyToolsCacheControl(claudeTools)
	reqBody := ClaudeRequest{
		Model:     c.Model,
		MaxTokens: 4096,
		System:    buildSystemBlocks(system),
		Messages:  claudeMsgs,
		Tools:     claudeTools,
		Stream:    true,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, newAPIError("Claude", resp.StatusCode, string(errBody))
	}

	// Parse SSE stream
	var contentBlocks []ContentBlock
	var inputJSONBuf []string // accumulates partial_json for current tool_use block
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event map[string]any
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		switch eventType {
		case "message_start":
			if msg, ok := event["message"].(map[string]any); ok {
				if usage, ok := msg["usage"].(map[string]any); ok {
					var u ClaudeUsage
					if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
						u.CacheCreationInputTokens = int(v)
					}
					if v, ok := usage["cache_read_input_tokens"].(float64); ok {
						u.CacheReadInputTokens = int(v)
					}
					logClaudeCacheUsage(&u)
				}
			}

		case "content_block_start":
			block, _ := event["content_block"].(map[string]any)
			if block != nil {
				cb := ContentBlock{Type: block["type"].(string)}
				if cb.Type == "tool_use" {
					cb.ID, _ = block["id"].(string)
					cb.Name, _ = block["name"].(string)
					cb.Input = map[string]any{}
					inputJSONBuf = inputJSONBuf[:0]
				}
				contentBlocks = append(contentBlocks, cb)
			}

		case "content_block_delta":
			delta, _ := event["delta"].(map[string]any)
			if delta != nil && len(contentBlocks) > 0 {
				idx := len(contentBlocks) - 1
				deltaType, _ := delta["type"].(string)
				switch deltaType {
				case "text_delta":
					text, _ := delta["text"].(string)
					contentBlocks[idx].Text += text
					if cb != nil {
						cb(&ChatResponse{
							Message: Message{Role: "assistant", Content: text},
						})
					}
				case "input_json_delta":
					partial, _ := delta["partial_json"].(string)
					inputJSONBuf = append(inputJSONBuf, partial)
				}
			}

		case "content_block_stop":
			if len(contentBlocks) > 0 {
				idx := len(contentBlocks) - 1
				if contentBlocks[idx].Type == "tool_use" && len(inputJSONBuf) > 0 {
					full := strings.Join(inputJSONBuf, "")
					var parsed map[string]any
					if json.Unmarshal([]byte(full), &parsed) == nil {
						contentBlocks[idx].Input = parsed
					}
					inputJSONBuf = inputJSONBuf[:0]
				}
			}

		case "message_stop":
			break
		}
	}

	// Build final response
	claudeResp := &ClaudeResponse{
		Role:    "assistant",
		Content: contentBlocks,
	}

	msg := ClaudeResponseToMessage(claudeResp)
	return &ChatResponse{
		Model:   c.Model,
		Message: msg,
		Done:    true,
	}, nil
}

// Verify interface compliance
var _ LLMProvider = (*ClaudeClient)(nil)
