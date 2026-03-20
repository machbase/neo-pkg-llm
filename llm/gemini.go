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

// GeminiClient implements LLMProvider for Google Gemini API.
type GeminiClient struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewGeminiClient(apiKey, model string) *GeminiClient {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiClient{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://generativelanguage.googleapis.com",
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// --- Gemini request/response types ---

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts,omitempty"`

	// RawParts holds the raw JSON of model parts for exact echo-back.
	// When set, MarshalJSON uses this instead of the typed Parts.
	RawParts json.RawMessage `json:"-"`
}

// MarshalJSON uses RawParts (exact echo-back) when available, otherwise typed Parts.
func (c geminiContent) MarshalJSON() ([]byte, error) {
	if c.RawParts != nil {
		return json.Marshal(struct {
			Role  string          `json:"role,omitempty"`
			Parts json.RawMessage `json:"parts"`
		}{
			Role:  c.Role,
			Parts: c.RawParts,
		})
	}
	type plain geminiContent // prevents recursion
	return json.Marshal(plain(c))
}

type geminiPart struct {
	Text             string          `json:"text,omitempty"`
	Thought          bool            `json:"thought,omitempty"`
	FunctionCall     *geminiFnCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFnResult `json:"functionResponse,omitempty"`
}

type geminiFnCall struct {
	Name             string         `json:"name"`
	Args             map[string]any `json:"args"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

type geminiFnResult struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFnDecl `json:"functionDeclarations"`
}

type geminiFnDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// --- Conversion helpers ---

func toolDefsToGemini(defs []map[string]any) []geminiTool {
	var decls []geminiFnDecl
	for _, def := range defs {
		fn, _ := def["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)
		decls = append(decls, geminiFnDecl{
			Name:        name,
			Description: desc,
			Parameters:  params,
		})
	}
	if len(decls) == 0 {
		return nil
	}
	return []geminiTool{{FunctionDeclarations: decls}}
}

func messagesToGemini(msgs []Message) (*geminiContent, []geminiContent) {
	var system *geminiContent
	var contents []geminiContent

	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			system = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}

		case "user":
			contents = append(contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: msg.Content}},
			})

		case "assistant":
			if len(msg.RawModelParts) > 0 {
				// Use preserved raw parts for exact echo-back (preserves thoughtSignature, thought, etc.)
				contents = append(contents, geminiContent{
					Role:     "model",
					RawParts: msg.RawModelParts,
				})
			} else {
				// Reconstruct from typed fields (non-Gemini providers or first message)
				var parts []geminiPart
				if msg.Content != "" {
					parts = append(parts, geminiPart{Text: msg.Content})
				}
				for _, tc := range msg.ToolCalls {
					args := tc.Function.Arguments
					if args == nil {
						args = map[string]any{}
					}
					parts = append(parts, geminiPart{
						FunctionCall: &geminiFnCall{
							Name:             tc.Function.Name,
							Args:             args,
							ThoughtSignature: tc.ThoughtSignature,
						},
					})
				}
				contents = append(contents, geminiContent{Role: "model", Parts: parts})
			}

		case "tool":
			// Gemini expects function responses as user role
			// Find the matching function name from the previous assistant message
			fnName := "unknown"
			for j := len(contents) - 1; j >= 0; j-- {
				c := contents[j]
				if c.Role == "model" {
					// Check typed parts first
					for _, p := range c.Parts {
						if p.FunctionCall != nil {
							fnName = p.FunctionCall.Name
							break
						}
					}
					// If we used RawParts, extract function name from raw JSON
					if fnName == "unknown" && c.RawParts != nil {
						fnName = extractFnNameFromRaw(c.RawParts)
					}
					break
				}
			}
			contents = append(contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{
					FunctionResponse: &geminiFnResult{
						Name:     fnName,
						Response: map[string]any{"result": msg.Content},
					},
				}},
			})
		}
	}
	return system, contents
}

// extractFnNameFromRaw extracts the function name from raw parts JSON.
func extractFnNameFromRaw(rawParts json.RawMessage) string {
	var parts []struct {
		FunctionCall *struct {
			Name string `json:"name"`
		} `json:"functionCall,omitempty"`
	}
	if json.Unmarshal(rawParts, &parts) == nil {
		for _, p := range parts {
			if p.FunctionCall != nil && p.FunctionCall.Name != "" {
				return p.FunctionCall.Name
			}
		}
	}
	return "unknown"
}

func geminiResponseToMessage(resp *geminiResponse) Message {
	msg := Message{Role: "assistant"}
	if len(resp.Candidates) == 0 {
		return msg
	}

	content := resp.Candidates[0].Content
	var textParts []string

	for _, part := range content.Parts {
		if part.Text != "" && !part.Thought {
			// Only include non-thought text in visible content
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			args := part.FunctionCall.Args
			if args == nil {
				args = map[string]any{}
			}
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				Function: ToolCallFunction{
					Name:      part.FunctionCall.Name,
					Arguments: args,
				},
				ThoughtSignature: part.FunctionCall.ThoughtSignature,
			})
		}
	}

	if len(textParts) > 0 {
		msg.Content = textParts[0]
		for i := 1; i < len(textParts); i++ {
			msg.Content += textParts[i]
		}
	}
	return msg
}

// --- API calls ---

func (c *GeminiClient) Chat(ctx context.Context, messages []Message, toolDefs []map[string]any) (*ChatResponse, error) {
	system, contents := messagesToGemini(messages)
	tools := toolDefsToGemini(toolDefs)

	reqBody := geminiRequest{
		Contents:          contents,
		Tools:             tools,
		SystemInstruction: system,
	}

	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.BaseURL, c.Model, c.APIKey)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini returned %d: %s", resp.StatusCode, string(errBody))
	}

	// Read raw body for both typed parsing and raw parts extraction
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gemini response: %w", err)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(rawBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("gemini error: %s", geminiResp.Error.Message)
	}

	msg := geminiResponseToMessage(&geminiResp)

	// Extract raw model parts for exact echo-back (preserves thoughtSignature, thought, etc.)
	var rawResp struct {
		Candidates []struct {
			Content struct {
				Parts json.RawMessage `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(rawBody, &rawResp) == nil && len(rawResp.Candidates) > 0 {
		msg.RawModelParts = rawResp.Candidates[0].Content.Parts
	}

	return &ChatResponse{
		Model:   c.Model,
		Message: msg,
		Done:    true,
	}, nil
}

func (c *GeminiClient) ChatStream(ctx context.Context, messages []Message, toolDefs []map[string]any, cb StreamCallback) (*ChatResponse, error) {
	system, contents := messagesToGemini(messages)
	tools := toolDefsToGemini(toolDefs)

	reqBody := geminiRequest{
		Contents:          contents,
		Tools:             tools,
		SystemInstruction: system,
	}

	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", c.BaseURL, c.Model, c.APIKey)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini returned %d: %s", resp.StatusCode, string(errBody))
	}

	// Parse SSE stream, accumulating both typed and raw parts
	var allParts []geminiPart
	var allRawParts []json.RawMessage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk geminiResponse
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Error != nil {
			return nil, fmt.Errorf("gemini stream error: %s", chunk.Error.Message)
		}
		if len(chunk.Candidates) == 0 {
			continue
		}

		// Extract raw parts from this chunk for echo-back
		var rawChunk struct {
			Candidates []struct {
				Content struct {
					Parts []json.RawMessage `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if json.Unmarshal([]byte(data), &rawChunk) == nil && len(rawChunk.Candidates) > 0 {
			allRawParts = append(allRawParts, rawChunk.Candidates[0].Content.Parts...)
		}

		for _, part := range chunk.Candidates[0].Content.Parts {
			allParts = append(allParts, part)
			if part.Text != "" && !part.Thought && cb != nil {
				cb(&ChatResponse{
					Message: Message{Role: "assistant", Content: part.Text},
				})
			}
		}
	}

	// Build final response from accumulated parts
	fullResp := &geminiResponse{
		Candidates: []struct {
			Content geminiContent `json:"content"`
		}{
			{Content: geminiContent{Role: "model", Parts: allParts}},
		},
	}

	msg := geminiResponseToMessage(fullResp)

	// Store raw parts for echo-back
	if len(allRawParts) > 0 {
		rawPartsJSON, _ := json.Marshal(allRawParts)
		msg.RawModelParts = rawPartsJSON
	}

	return &ChatResponse{
		Model:   c.Model,
		Message: msg,
		Done:    true,
	}, nil
}

// Verify interface compliance
var _ LLMProvider = (*GeminiClient)(nil)
