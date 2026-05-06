package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"neo-pkg-llm/fixer"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/tools"
)

// AgentState exposes the agent state needed by guards.
type AgentState interface {
	Messages() []llm.Message
	IsAdvanced() bool
	IsReport() bool
	LLM() llm.LLMProvider
	Registry() *tools.Registry
}

// Guard is a single check that may modify a message before or after the tool loop.
type Guard interface {
	Name() string
	Check(ctx context.Context, state AgentState, msg llm.Message) llm.Message
}

// Pipeline holds ordered lists of pre-tool and post-loop guards.
type Pipeline struct {
	preTool  []Guard
	postLoop []Guard
}

// NewPipeline creates a Pipeline with the given guards.
func NewPipeline(preTool, postLoop []Guard) *Pipeline {
	return &Pipeline{preTool: preTool, postLoop: postLoop}
}

// RunPreTool runs all pre-tool guards in order.
func (p *Pipeline) RunPreTool(ctx context.Context, state AgentState, msg llm.Message) llm.Message {
	for _, g := range p.preTool {
		msg = g.Check(ctx, state, msg)
	}
	return msg
}

// RunPostLoop runs all post-loop guards in order.
func (p *Pipeline) RunPostLoop(ctx context.Context, state AgentState, msg llm.Message) llm.Message {
	for _, g := range p.postLoop {
		msg = g.Check(ctx, state, msg)
	}
	return msg
}

// --- Exported template constants ---

var TemplateExpected = map[string]int{"1": 6, "2": 7, "3": 4}

var TemplateAllIDs = map[string][]string{
	"1": {"1-1", "1-2", "1-3", "1-4", "1-5", "1-6"},
	"2": {"2-1", "2-2", "2-3", "2-4", "2-5", "2-6", "2-7"},
	"3": {"3-1", "3-2", "3-3", "3-4"},
}

var TemplateNames = map[string]string{
	"1-1": "평균 추세", "1-2": "변동성", "1-3": "가격 밴드",
	"1-4": "태그 비교", "1-5": "거래량 추세", "1-6": "로그 가격",
	"2-1": "RMS 진동", "2-2": "FFT 스펙트럼", "2-3": "피크 엔벨로프",
	"2-4": "Peak-to-Peak", "2-5": "Crest Factor", "2-6": "데이터 밀도", "2-7": "3D 스펙트럼",
	"3-1": "롤업 평균", "3-2": "태그 비교", "3-3": "카운트 추세", "3-4": "MIN/MAX 엔벨로프",
}

// DashboardTools and TemplateIDRE are defined in the fixer package.

// --- Exported helper functions ---

// SavedTQL holds a template ID and file path for a saved TQL file.
type SavedTQL struct {
	ID   string
	Path string
}

// GetSavedTemplateIDs scans messages for successfully saved TQL template IDs.
func GetSavedTemplateIDs(msgs []llm.Message) map[string]bool {
	ids := map[string]bool{}
	var pendingTIDs []string

	for _, msg := range msgs {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			pendingTIDs = nil
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "save_tql_file" {
					fn, _ := tc.Function.Arguments["filename"].(string)
					if m := fixer.TemplateIDRE.FindString(fn); m != "" {
						pendingTIDs = append(pendingTIDs, strings.ReplaceAll(m, "_", "-"))
					} else {
						pendingTIDs = append(pendingTIDs, "")
					}
				} else {
					pendingTIDs = append(pendingTIDs, "")
				}
			}
		} else if msg.Role == "tool" && len(pendingTIDs) > 0 {
			tid := pendingTIDs[0]
			pendingTIDs = pendingTIDs[1:]
			if tid != "" {
				content := strings.ToLower(msg.Content)
				if !strings.Contains(content, "error") && !strings.Contains(content, "fail") {
					ids[tid] = true
				}
			}
		}
	}
	return ids
}

// GetSavedTQLPaths returns all successfully saved TQL file paths from messages.
func GetSavedTQLPaths(msgs []llm.Message) []SavedTQL {
	var saved []SavedTQL
	var pendingPaths []SavedTQL

	for _, msg := range msgs {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			pendingPaths = nil
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "save_tql_file" {
					fn, _ := tc.Function.Arguments["filename"].(string)
					tid := ""
					if m := fixer.TemplateIDRE.FindString(fn); m != "" {
						tid = strings.ReplaceAll(m, "_", "-")
					}
					pendingPaths = append(pendingPaths, SavedTQL{ID: tid, Path: fn})
				} else {
					pendingPaths = append(pendingPaths, SavedTQL{})
				}
			}
		} else if msg.Role == "tool" && len(pendingPaths) > 0 {
			p := pendingPaths[0]
			pendingPaths = pendingPaths[1:]
			if p.Path != "" {
				content := strings.ToLower(msg.Content)
				if !strings.Contains(content, "error") && !strings.Contains(content, "fail") {
					saved = append(saved, p)
				}
			}
		}
	}
	return saved
}

// GetDashboardFilename finds the first dashboard filename from messages.
func GetDashboardFilename(msgs []llm.Message) string {
	for _, msg := range msgs {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if fixer.DashboardTools[tc.Function.Name] {
					if fn, ok := tc.Function.Arguments["filename"].(string); ok {
						return fn
					}
				}
			}
		}
	}
	return ""
}

// CountAddChartCalls counts how many chart additions are in the messages.
func CountAddChartCalls(msgs []llm.Message) int {
	count := 0
	for _, msg := range msgs {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "add_chart_to_dashboard" {
					count++
				} else if tc.Function.Name == "create_dashboard_with_charts" {
					count += countChartsInArgs(tc.Function.Arguments)
				}
			}
		}
	}
	return count
}

// countChartsInArgs extracts the number of charts from create_dashboard_with_charts arguments.
func countChartsInArgs(args map[string]any) int {
	charts, ok := args["charts"]
	if !ok {
		return 0
	}
	switch c := charts.(type) {
	case []any:
		return len(c)
	case string:
		var arr []any
		if json.Unmarshal([]byte(c), &arr) == nil {
			return len(arr)
		}
	}
	return 0
}

// CountConsecutiveFailures counts how many times the given tool failed consecutively at the end of messages.
func CountConsecutiveFailures(msgs []llm.Message, toolName string) int {
	count := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Role == "tool" {
			content := strings.ToLower(msg.Content)
			if strings.Contains(content, "failed") || strings.Contains(content, "error") || strings.Contains(content, "failure") {
				count++
			} else {
				break
			}
		} else if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			hasTargetTool := false
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == toolName {
					hasTargetTool = true
				}
			}
			if !hasTargetTool {
				break
			}
		} else if msg.Role == "user" {
			continue
		} else {
			break
		}
	}
	return count
}

// FixToolCallsFunc is a function type for fixing tool calls (avoids circular dependency).
type FixToolCallsFunc func(msg llm.Message) llm.Message

// SetFixToolCalls stores the fixToolCalls callback for guards to use after re-prompting LLM.
var FixToolCalls FixToolCallsFunc

// AppendMessages is a callback to append messages to the agent state.
// Guards need this to inject retry messages.
type AppendMessagesFunc func(msgs ...llm.Message)

// SetAppendMessages stores the callback for guards.
var AppendMessages AppendMessagesFunc

// Re-prompt helper used by multiple guards: appends the original msg + cancel results + user hint,
// calls LLM, and returns the fixed response.
func rePrompt(ctx context.Context, state AgentState, msg llm.Message, hint string) llm.Message {
	msgs := make([]llm.Message, len(state.Messages()))
	copy(msgs, state.Messages())

	msgs = append(msgs, msg)
	for range msg.ToolCalls {
		msgs = append(msgs, llm.Message{
			Role:    "tool",
			Content: "cancelled: redirecting",
		})
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: hint})

	// Update agent messages
	AppendMessages(msg)
	for range msg.ToolCalls {
		AppendMessages(llm.Message{Role: "tool", Content: "cancelled: redirecting"})
	}
	AppendMessages(llm.Message{Role: "user", Content: hint})

	resp, err := state.LLM().Chat(ctx, msgs, state.Registry().AllToolDefs())
	if err != nil {
		return msg
	}
	if FixToolCalls != nil {
		return FixToolCalls(resp.Message)
	}
	return resp.Message
}

// rePromptNoToolCalls is like rePrompt but for post-loop guards (msg has no tool calls).
func rePromptNoToolCalls(ctx context.Context, state AgentState, msg llm.Message, hint string) llm.Message {
	AppendMessages(msg)
	AppendMessages(llm.Message{Role: "user", Content: hint})

	msgs := make([]llm.Message, len(state.Messages()))
	copy(msgs, state.Messages())

	resp, err := state.LLM().Chat(ctx, msgs, state.Registry().AllToolDefs())
	if err != nil {
		return msg
	}
	if FixToolCalls != nil {
		return FixToolCalls(resp.Message)
	}
	return resp.Message
}

func init() {
	// Default no-op to avoid nil panics
	FixToolCalls = func(msg llm.Message) llm.Message { return msg }
	AppendMessages = func(msgs ...llm.Message) {}
	_ = fmt.Sprintf // ensure fmt is used
}
