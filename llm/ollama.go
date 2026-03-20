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

// OllamaClient implements LLMProvider for Ollama's OpenAI-compatible API.
type OllamaClient struct {
	Model      string
	BaseURL    string
	Temperature float64
	NumPredict int
	NumCtx     int
	NumGPU     int
	client     *http.Client
}

type OllamaOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
	NumGPU      int     `json:"num_gpu,omitempty"`
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	if model == "" {
		model = "llama3"
	}
	return &OllamaClient{
		Model:       model,
		BaseURL:     baseURL,
		Temperature: 0,
		NumPredict:  4096,
		NumCtx:      32768,
		NumGPU:      36,
		client:      &http.Client{Timeout: 10 * time.Minute},
	}
}

// ollamaRequest extends openaiRequest with Ollama-specific options.
type ollamaRequest struct {
	Model    string           `json:"model"`
	Messages []openaiMessage  `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream,omitempty"`
	Options  *OllamaOptions   `json:"options,omitempty"`
}

// Chat sends a non-streaming request to Ollama.
func (o *OllamaClient) Chat(ctx context.Context, messages []Message, toolDefs []map[string]any) (*ChatResponse, error) {
	reqBody := ollamaRequest{
		Model:    o.Model,
		Messages: messagesToOpenAI(messages),
		Tools:    toolDefs,
		Options: &OllamaOptions{
			Temperature: o.Temperature,
			NumPredict:  o.NumPredict,
			NumCtx:      o.NumCtx,
			NumGPU:      o.NumGPU,
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ollama")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(errBody))
	}

	var openaiResp openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	if openaiResp.Error != nil {
		return nil, fmt.Errorf("ollama error: %s", openaiResp.Error.Message)
	}

	msg := openaiResponseToMessage(&openaiResp)
	return &ChatResponse{
		Model:   o.Model,
		Message: msg,
		Done:    true,
	}, nil
}

// ChatStream sends a streaming request to Ollama.
func (o *OllamaClient) ChatStream(ctx context.Context, messages []Message, toolDefs []map[string]any, cb StreamCallback) (*ChatResponse, error) {
	reqBody := ollamaRequest{
		Model:    o.Model,
		Messages: messagesToOpenAI(messages),
		Tools:    toolDefs,
		Stream:   true,
		Options: &OllamaOptions{
			Temperature: o.Temperature,
			NumPredict:  o.NumPredict,
			NumCtx:      o.NumCtx,
			NumGPU:      o.NumGPU,
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ollama")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(errBody))
	}

	// Parse SSE stream (same format as OpenAI)
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

		if delta.Content != "" {
			contentBuf.WriteString(delta.Content)
			if cb != nil {
				cb(&ChatResponse{
					Message: Message{Role: "assistant", Content: delta.Content},
				})
			}
		}

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
		Model:   o.Model,
		Message: msg,
		Done:    true,
	}, nil
}

// Verify interface compliance
var _ LLMProvider = (*OllamaClient)(nil)
