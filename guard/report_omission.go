package guard

import (
	"context"
	"fmt"
	"strings"

	"neo-pkg-llm/llm"
)

// ReportOmissionGuard checks whether save_html_report was called in report mode.
type ReportOmissionGuard struct{}

func (g *ReportOmissionGuard) Name() string { return "report_omission" }

func (g *ReportOmissionGuard) Check(ctx context.Context, state AgentState, msg llm.Message) llm.Message {
	if !state.IsReport() {
		return msg
	}
	if msg.Content == "" || len(msg.ToolCalls) > 0 {
		return msg
	}

	// Check if save_html_report was ever called (success or fail)
	allMsgs := append(state.Messages(), msg)
	for _, m := range allMsgs {
		for _, tc := range m.ToolCalls {
			if tc.Function.Name == "save_html_report" {
				return msg
			}
		}
		if strings.Contains(m.Content, "Report saved") || strings.Contains(m.Content, "INCOMPLETE") {
			return msg
		}
	}

	fmt.Println("[Agent] Report mode but save_html_report not called → prompting")

	AppendMessages(msg)
	AppendMessages(llm.Message{
		Role:    "user",
		Content: "아직 HTML 리포트가 저장되지 않았습니다. save_html_report를 사용하여 분석 결과를 HTML 보고서로 저장하세요.",
	})

	resp, err := state.LLM().Chat(ctx, state.Messages(), state.Registry().AllToolDefs())
	if err != nil {
		return msg
	}
	if FixToolCalls != nil {
		return FixToolCalls(resp.Message)
	}
	return resp.Message
}
