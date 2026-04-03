package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatGPTClient implements LLMProvider for OpenAI Chat Completions API.
type ChatGPTClient struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewChatGPTClient(apiKey, model string) *ChatGPTClient {
	if model == "" {
		model = "gpt-4o"
	}
	return &ChatGPTClient{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.openai.com",
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// --- OpenAI request/response types ---

type openaiRequest struct {
	Model    string           `json:"model"`
	Messages []openaiMessage  `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream,omitempty"`
}

type openaiMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content,omitempty"`
	ToolCalls  []openaiToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type openaiResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      openaiMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage *openaiUsage `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type openaiUsage struct {
	PromptTokens        int                   `json:"prompt_tokens"`
	CompletionTokens    int                   `json:"completion_tokens"`
	PromptTokensDetails *openaiPromptDetails  `json:"prompt_tokens_details,omitempty"`
}

type openaiPromptDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// --- Conversion helpers ---

func messagesToOpenAI(msgs []Message) []openaiMessage {
	var result []openaiMessage
	for _, msg := range msgs {
		switch msg.Role {
		case "system", "user":
			result = append(result, openaiMessage{Role: msg.Role, Content: msg.Content})

		case "assistant":
			om := openaiMessage{Role: "assistant", Content: msg.Content}
			for i, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				om.ToolCalls = append(om.ToolCalls, openaiToolCall{
					ID:   fmt.Sprintf("call_%d", i),
					Type: "function",
					Function: openaiToolFunction{
						Name:      tc.Function.Name,
						Arguments: string(argsJSON),
					},
				})
			}
			result = append(result, om)

		case "tool":
			// Match tool_call_id from previous assistant message
			toolCallID := "call_0"
			// Count how many tool messages since last assistant
			toolIdx := 0
			for j := len(result) - 1; j >= 0; j-- {
				if result[j].Role == "tool" {
					toolIdx++
				} else {
					break
				}
			}
			// Find matching assistant tool_call
			for j := len(result) - 1; j >= 0; j-- {
				if result[j].Role == "assistant" && len(result[j].ToolCalls) > toolIdx {
					toolCallID = result[j].ToolCalls[toolIdx].ID
					break
				}
			}
			result = append(result, openaiMessage{
				Role:       "tool",
				Content:    msg.Content,
				ToolCallID: toolCallID,
			})
		}
	}
	return result
}

func openaiResponseToMessage(resp *openaiResponse) Message {
	if len(resp.Choices) == 0 {
		return Message{Role: "assistant", Content: ""}
	}
	choice := resp.Choices[0].Message
	msg := Message{Role: "assistant", Content: choice.Content}

	for _, tc := range choice.ToolCalls {
		var args map[string]any
		json.Unmarshal([]byte(tc.Function.Arguments), &args)
		if args == nil {
			args = map[string]any{}
		}
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			Function: ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}
	return msg
}

// --- API calls ---

func (c *ChatGPTClient) Chat(ctx context.Context, messages []Message, toolDefs []map[string]any) (*ChatResponse, error) {
	reqBody := openaiRequest{
		Model:    c.Model,
		Messages: messagesToOpenAI(messages),
		Tools:    toolDefs,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatgpt request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, newAPIError("ChatGPT", resp.StatusCode, string(errBody))
	}

	var openaiResp openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode chatgpt response: %w", err)
	}

	if openaiResp.Error != nil {
		return nil, fmt.Errorf("chatgpt error: %s", openaiResp.Error.Message)
	}

	logOpenAICacheUsage(&openaiResp)

	msg := openaiResponseToMessage(&openaiResp)
	return &ChatResponse{
		Model:   c.Model,
		Message: msg,
		Done:    true,
	}, nil
}

// logOpenAICacheUsage logs prompt cache hit information from the OpenAI response.
func logOpenAICacheUsage(resp *openaiResponse) {
	if resp.Usage != nil && resp.Usage.PromptTokensDetails != nil && resp.Usage.PromptTokensDetails.CachedTokens > 0 {
		fmt.Printf("[ChatGPT] prompt cache hit: %d/%d tokens cached\n",
			resp.Usage.PromptTokensDetails.CachedTokens, resp.Usage.PromptTokens)
	}
}

func (c *ChatGPTClient) ChatStream(ctx context.Context, messages []Message, toolDefs []map[string]any, cb StreamCallback) (*ChatResponse, error) {
	reqBody := openaiRequest{
		Model:    c.Model,
		Messages: messagesToOpenAI(messages),
		Tools:    toolDefs,
		Stream:   true,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatgpt stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, newAPIError("ChatGPT", resp.StatusCode, string(errBody))
	}

	// Parse SSE stream
	var contentBuf strings.Builder
	var toolCalls []openaiToolCall

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

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// Accumulate text content
		if delta.Content != "" {
			contentBuf.WriteString(delta.Content)
			if cb != nil {
				cb(&ChatResponse{
					Message: Message{Role: "assistant", Content: delta.Content},
				})
			}
		}

		// Accumulate tool calls
		for _, tc := range delta.ToolCalls {
			for len(toolCalls) <= tc.Index {
				toolCalls = append(toolCalls, openaiToolCall{Type: "function"})
			}
			if tc.ID != "" {
				toolCalls[tc.Index].ID = tc.ID
			}
			if tc.Function.Name != "" {
				toolCalls[tc.Index].Function.Name = tc.Function.Name
			}
			toolCalls[tc.Index].Function.Arguments += tc.Function.Arguments
		}
	}

	// Build final message
	msg := Message{Role: "assistant", Content: contentBuf.String()}
	for _, tc := range toolCalls {
		var args map[string]any
		json.Unmarshal([]byte(tc.Function.Arguments), &args)
		if args == nil {
			args = map[string]any{}
		}
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			Function: ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}

	return &ChatResponse{
		Model:   c.Model,
		Message: msg,
		Done:    true,
	}, nil
}

// Verify interface compliance
var _ LLMProvider = (*ChatGPTClient)(nil)
