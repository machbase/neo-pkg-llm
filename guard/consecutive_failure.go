package guard

import (
	"context"
	"fmt"

	"neo-pkg-llm/llm"
)

// ConsecutiveFailureGuard redirects the LLM when a tool fails repeatedly.
type ConsecutiveFailureGuard struct{}

func (g *ConsecutiveFailureGuard) Name() string { return "consecutive_failure" }

func (g *ConsecutiveFailureGuard) Check(ctx context.Context, state AgentState, msg llm.Message) llm.Message {
	if len(msg.ToolCalls) == 0 {
		return msg
	}

	toolName := msg.ToolCalls[0].Function.Name
	failCount := CountConsecutiveFailures(state.Messages(), toolName)

	if failCount < 2 {
		return msg
	}

	fmt.Printf("[Agent] %s failed %d times → redirecting\n", toolName, failCount)

	hint := fmt.Sprintf(
		"⚠ %s이(가) %d회 연속 실패했습니다. "+
			"같은 방식을 반복하지 마세요! "+
			"에러 메시지를 읽고 원인을 파악한 후 완전히 다른 접근법을 사용하세요.",
		toolName, failCount,
	)
	if toolName == "save_tql_file" {
		hint = fmt.Sprintf(
			"⚠ save_tql_file가 %d회 연속 실패했습니다. "+
				"TQL 코드를 직접 작성하지 마세요! "+
				"반드시 TEMPLATE:ID TABLE:테이블 TAG:태그 UNIT:단위 형식을 사용하세요.",
			failCount,
		)
	}

	AppendMessages(msg)
	for range msg.ToolCalls {
		AppendMessages(llm.Message{
			Role:    "tool",
			Content: "cancelled: redirecting due to consecutive failures",
		})
	}
	AppendMessages(llm.Message{Role: "user", Content: hint})

	resp, err := state.LLM().Chat(ctx, state.Messages(), state.Registry().AllToolDefs())
	if err != nil {
		return msg
	}
	if FixToolCalls != nil {
		return FixToolCalls(resp.Message)
	}
	return resp.Message
}
