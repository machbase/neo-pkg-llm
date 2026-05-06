package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// OllamaClient implements LLMProvider for Ollama's native /api/chat API.
type OllamaClient struct {
	Model       string
	BaseURL     string
	Temperature float64
	NumPredict  int
	NumCtx      int
	NumGPU      int
	numKeep     int // dynamically computed system prompt token count
	client      *http.Client
}

type OllamaOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
	NumGPU      int     `json:"num_gpu,omitempty"`
	NumKeep     int     `json:"num_keep,omitempty"`
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	if model == "" {
		model = "llama3"
	}
	ensureOllamaRunning(baseURL)
	return &OllamaClient{
		Model:       model,
		BaseURL:     baseURL,
		Temperature: 0,
		NumPredict:  4096,
		NumCtx:      40960,
		NumGPU:      36,
		client:      &http.Client{Timeout: 10 * time.Minute},
	}
}

// ensureOllamaRunning kills any existing Ollama process and restarts it
// with OLLAMA_FLASH_ATTENTION=1.
func ensureOllamaRunning(baseURL string) {
	// Kill existing Ollama process
	kill := exec.Command("taskkill", "/f", "/im", "ollama.exe")
	kill.Run() // ignore error (may not be running)
	time.Sleep(1 * time.Second)

	fmt.Println("[Ollama] Starting with FLASH_ATTENTION=1 ...")
	cmd := exec.Command("ollama", "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_FLASH_ATTENTION=1")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		fmt.Printf("[Ollama] Failed to start: %v\n", err)
		return
	}

	// Wait until server is ready (max 15s)
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		if resp, err := client.Get(baseURL); err == nil {
			resp.Body.Close()
			fmt.Println("[Ollama] Server started successfully")
			return
		}
	}
	fmt.Println("[Ollama] WARNING: Server did not become ready in 15s")
}

// Pre-measured num_keep values per skill (Ollama compact segments + catalog, qwen3:8b).
// Based on OllamaSystemPrompt full = 8300 tokens measured, ratio-scaled per skill.
var ollamaNumKeepBySkill = map[string]int{
	"AdvancedAnalysis": 6900,
	"BasicAnalysis":    6100,
	"Report":           5500,
	"DocLookup":        5500,
}

// SetNumKeep sets num_keep based on the active skill name.
// Falls back to estimation if skill is unknown.
func (o *OllamaClient) SetNumKeep(skillName string) {
	if skillName == "" {
		o.numKeep = 6100 // default to BasicAnalysis
		fmt.Printf("[Ollama] num_keep set to %d (default)\n", o.numKeep)
		return
	}
	if v, ok := ollamaNumKeepBySkill[skillName]; ok {
		o.numKeep = v
		fmt.Printf("[Ollama] num_keep set to %d (skill: %s)\n", o.numKeep, skillName)
	} else {
		o.numKeep = 6100
		fmt.Printf("[Ollama] num_keep set to %d (unknown skill: %s, using default)\n", o.numKeep, skillName)
	}
}

// --- Ollama native API types ---

type ollamaNativeRequest struct {
	Model    string           `json:"model"`
	Messages []ollamaMessage  `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
	Options  *OllamaOptions   `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolFunc `json:"function"`
}

type ollamaToolFunc struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaNativeResponse struct {
	Message    ollamaMessage `json:"message"`
	Done       bool          `json:"done"`
	DoneReason string        `json:"done_reason,omitempty"`
	Error      string        `json:"error,omitempty"`
}

func messagesToOllama(msgs []Message) []ollamaMessage {
	var result []ollamaMessage
	for _, msg := range msgs {
		om := ollamaMessage{Role: msg.Role, Content: msg.Content}
		for _, tc := range msg.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				Function: ollamaToolFunc{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
		result = append(result, om)
	}
	return result
}

func ollamaToMessage(om ollamaMessage) Message {
	msg := Message{Role: om.Role, Content: om.Content}
	for _, tc := range om.ToolCalls {
		args := tc.Function.Arguments
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

// Chat sends a non-streaming request to Ollama via native /api/chat.
func (o *OllamaClient) Chat(ctx context.Context, messages []Message, toolDefs []map[string]any) (*ChatResponse, error) {
	reqBody := ollamaNativeRequest{
		Model:    o.Model,
		Messages: messagesToOllama(messages),
		Tools:    toolDefs,
		Stream:   false,
		Options: &OllamaOptions{
			Temperature: o.Temperature,
			NumPredict:  o.NumPredict,
			NumCtx:      o.NumCtx,
			NumGPU:      o.NumGPU,
			NumKeep:     o.numKeep,
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, newAPIError("Ollama", resp.StatusCode, string(errBody))
	}

	var nativeResp ollamaNativeResponse
	if err := json.NewDecoder(resp.Body).Decode(&nativeResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	if nativeResp.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", nativeResp.Error)
	}

	return &ChatResponse{
		Model:   o.Model,
		Message: ollamaToMessage(nativeResp.Message),
		Done:    true,
	}, nil
}

// ChatStream sends a streaming request to Ollama via native /api/chat.
func (o *OllamaClient) ChatStream(ctx context.Context, messages []Message, toolDefs []map[string]any, cb StreamCallback) (*ChatResponse, error) {
	reqBody := ollamaNativeRequest{
		Model:    o.Model,
		Messages: messagesToOllama(messages),
		Tools:    toolDefs,
		Stream:   true,
		Options: &OllamaOptions{
			Temperature: o.Temperature,
			NumPredict:  o.NumPredict,
			NumCtx:      o.NumCtx,
			NumGPU:      o.NumGPU,
			NumKeep:     o.numKeep,
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, newAPIError("Ollama", resp.StatusCode, string(errBody))
	}

	// Ollama native streaming: one JSON object per line
	var contentBuf strings.Builder
	var toolCalls []ollamaToolCall

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var chunk ollamaNativeResponse
		if json.Unmarshal([]byte(line), &chunk) != nil {
			continue
		}

		if chunk.Message.Content != "" {
			contentBuf.WriteString(chunk.Message.Content)
			if cb != nil {
				cb(&ChatResponse{
					Message: Message{Role: "assistant", Content: chunk.Message.Content},
				})
			}
		}

		if len(chunk.Message.ToolCalls) > 0 {
			toolCalls = append(toolCalls, chunk.Message.ToolCalls...)
		}

		if chunk.Done {
			break
		}
	}

	msg := Message{Role: "assistant", Content: contentBuf.String()}
	for _, tc := range toolCalls {
		args := tc.Function.Arguments
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
