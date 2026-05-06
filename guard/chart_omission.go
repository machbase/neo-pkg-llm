package guard

import (
	"context"
	"fmt"
	"strings"

	"neo-pkg-llm/llm"
)

// ChartOmissionGuard checks whether all saved TQL files have been added to the dashboard.
type ChartOmissionGuard struct{}

func (g *ChartOmissionGuard) Name() string { return "chart_omission" }

func (g *ChartOmissionGuard) Check(ctx context.Context, state AgentState, msg llm.Message) llm.Message {
	if !state.IsAdvanced() {
		return msg
	}
	if msg.Content == "" || len(msg.ToolCalls) > 0 {
		return msg
	}

	allMsgs := append(state.Messages(), msg)
	savedTQLs := GetSavedTQLPaths(allMsgs)
	dashFn := GetDashboardFilename(allMsgs)
	chartCalls := CountAddChartCalls(allMsgs)

	if len(savedTQLs) == 0 || dashFn == "" || chartCalls >= len(savedTQLs) {
		return msg
	}

	fmt.Printf("[Agent] Dashboard has %d/%d charts → prompting chart addition\n", chartCalls, len(savedTQLs))

	var cmds []string
	for _, st := range savedTQLs {
		table := strings.Split(st.Path, "/")[0]
		name := st.ID
		if n, ok := TemplateNames[st.ID]; ok {
			name = table + " " + n
		}
		cmds = append(cmds, fmt.Sprintf(
			`add_chart_to_dashboard(filename="%s", chart_type="Tql chart", tql_path="%s", chart_title="%s")`,
			dashFn, st.Path, name,
		))
	}

	hint := fmt.Sprintf(
		"⚠ 대시보드가 생성되었지만 차트가 추가되지 않았습니다! "+
			"저장된 TQL 파일 %d개를 모두 대시보드에 추가하세요.\n"+
			"대시보드: %s\n아래 호출을 모두 실행하세요:\n%s\n그 후 preview_dashboard를 호출하세요.",
		len(savedTQLs), dashFn, strings.Join(cmds, "\n"),
	)

	AppendMessages(msg)
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
