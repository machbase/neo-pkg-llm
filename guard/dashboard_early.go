package guard

import (
	"context"
	"fmt"
	"strings"

	"neo-pkg-llm/fixer"
	"neo-pkg-llm/llm"
)

// DashboardEarlyGuard prevents dashboard creation before enough TQL templates are saved.
type DashboardEarlyGuard struct{}

func (g *DashboardEarlyGuard) Name() string { return "dashboard_early" }

func (g *DashboardEarlyGuard) Check(ctx context.Context, state AgentState, msg llm.Message) llm.Message {
	if !state.IsAdvanced() {
		return msg
	}
	if len(msg.ToolCalls) == 0 || !fixer.DashboardTools[msg.ToolCalls[0].Function.Name] {
		return msg
	}

	savedIDs := GetSavedTemplateIDs(state.Messages())

	var templateType string
	if len(savedIDs) > 0 {
		for id := range savedIDs {
			templateType = strings.Split(id, "-")[0]
			break
		}
	}

	expected := 4
	if templateType != "" {
		if e, ok := TemplateExpected[templateType]; ok {
			expected = e
		}
	}

	if len(savedIDs) >= expected {
		return msg
	}

	allIDs, _ := TemplateAllIDs[templateType]
	var missingIDs []string
	for _, id := range allIDs {
		if !savedIDs[id] {
			missingIDs = append(missingIDs, id)
		}
	}

	nextID := "?"
	if len(missingIDs) > 0 {
		nextID = missingIDs[0]
	}

	fmt.Printf("[Agent] TQL %d/%d saved (type %s) → missing: %v\n", len(savedIDs), expected, templateType, missingIDs)

	hint := fmt.Sprintf(
		"⚠ 아직 TQL 파일이 %d/%d개만 저장되었습니다. "+
			"미저장 템플릿: %v. "+
			"지금 바로 save_tql_file을 호출하세요! "+
			"tql_content: TEMPLATE:%s TABLE:(테이블명) TAG:(태그명) UNIT:(단위)",
		len(savedIDs), expected, missingIDs, nextID,
	)

	// Inject cancel results for each tool call + user hint, then re-prompt
	AppendMessages(msg)
	for range msg.ToolCalls {
		AppendMessages(llm.Message{
			Role:    "tool",
			Content: "cancelled: dashboard creation deferred",
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
