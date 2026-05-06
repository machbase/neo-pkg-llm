package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"neo-pkg-llm/llm"
	"neo-pkg-llm/logger"
	"neo-pkg-llm/tools"
)

// SubAgent runs an isolated task with its own context window.
// It shares the same LLM provider but maintains a separate conversation history.
type SubAgent struct {
	LLM      llm.LLMProvider
	Registry *tools.Registry
	Prompt   string // task-specific system prompt (built via context.PromptBuilder)
	MaxSteps int    // limited iterations (typically 10-15)
}

// SubAgentResult holds the structured output from a sub-agent execution.
type SubAgentResult struct {
	Summary   string         `json:"summary"`   // natural language summary
	Artifacts []string       `json:"artifacts"` // created file paths
	Data      map[string]any `json:"data"`      // structured data (table, tags, time range, etc.)
	Error     string         `json:"error,omitempty"`
}

// Run executes the sub-agent with the given task description.
// The sub-agent uses its own system prompt and independent message history.
func (s *SubAgent) Run(ctx context.Context, task string) (*SubAgentResult, error) {
	if s.MaxSteps <= 0 {
		s.MaxSteps = 15
	}

	// Create a full agent (with fixerCtx, guardPipeline) but without sub-agent delegation
	ag := NewAgent(s.LLM, s.Registry)
	ag.maxSteps = s.MaxSteps

	// Override messages with sub-agent specific prompt (before Run triggers initMessages)
	ag.messages = []llm.Message{
		{Role: "system", Content: s.Prompt},
		{Role: "user", Content: task + "\n\n응답은 반드시 JSON 형태로 해주세요: {\"summary\": \"...\", \"artifacts\": [...], \"data\": {...}}"},
	}

	LoadTemplates()

	logger.Infof("[SubAgent] Starting task: %s", truncate(task, 100))

	// Run the agent loop directly (skip initMessages since messages are already set)
	result, err := ag.runLoop(ctx)
	if err != nil {
		return &SubAgentResult{Error: err.Error()}, err
	}

	return parseSubAgentResult(result), nil
}

// RunStream executes the sub-agent with streaming events.
func (s *SubAgent) RunStream(ctx context.Context, task string) (<-chan Event, *SubAgentResult) {
	if s.MaxSteps <= 0 {
		s.MaxSteps = 15
	}

	ag := &Agent{
		llm:      s.LLM,
		registry: s.Registry,
		maxSteps: s.MaxSteps,
	}

	ag.messages = []llm.Message{
		{Role: "system", Content: s.Prompt},
		{Role: "user", Content: task},
	}

	LoadTemplates()

	events := ag.RunStream(ctx, task)
	// Caller reads events; final result captured in last event
	return events, nil
}

// parseSubAgentResult attempts to parse the LLM response as a SubAgentResult.
// Falls back to treating the entire response as a summary if JSON parsing fails.
func parseSubAgentResult(response string) *SubAgentResult {
	result := &SubAgentResult{
		Data: make(map[string]any),
	}

	// Try to extract JSON from the response
	jsonStr := extractJSON(response)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), result); err == nil {
			if result.Data == nil {
				result.Data = make(map[string]any)
			}
			return result
		}
	}

	// Fallback: use the entire response as summary
	result.Summary = response
	return result
}

// extractJSON finds the first JSON object in a string.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// SubAgentToolSubset defines which tools each sub-agent type can access.
var SubAgentToolSubset = map[string][]string{
	"data_discovery":      {"list_tables", "list_table_tags", "execute_sql_query"},
	"tql_generator":       {"create_folder", "save_tql_file"},
	"dashboard_assembler": {"create_dashboard_with_charts", "preview_dashboard"},
	"report_writer":       {"list_tables", "list_table_tags", "execute_sql_query", "save_html_report"},
	"doc_researcher":      {"list_available_documents", "get_full_document_content", "get_document_sections", "extract_code_blocks"},
}

// ParallelResult holds per-table results from parallel sub-agent execution.
type ParallelResult struct {
	Table  string          `json:"table"`
	Result *SubAgentResult `json:"result"`
	Error  error           `json:"error,omitempty"`
}

// RunParallel executes sub-agents in parallel, one per table.
// Each sub-agent independently performs: tag lookup → stats → time range → dashboard creation.
func RunParallel(ctx context.Context, llmClient llm.LLMProvider, registry *tools.Registry, tables []string, query string, skillName string, buildPrompt func(groups ...string) string) ([]ParallelResult, error) {
	if len(tables) == 0 {
		return nil, fmt.Errorf("no tables specified for parallel execution")
	}

	logger.Infof("[Orchestrator] Parallel fan-out: %d tables %v", len(tables), tables)

	// Determine tool subset and prompt groups based on skill
	toolSubset := []string{
		"list_table_tags", "execute_sql_query",
		"create_folder", "save_tql_file", "validate_chart_tql",
		"create_dashboard_with_charts", "preview_dashboard",
		"list_available_documents", "get_full_document_content",
	}
	promptGroups := []string{"sql_tools", "dashboard_tools"}

	if skillName == "AdvancedAnalysis" {
		promptGroups = append(promptGroups, "tql_tools")
	}

	prompt := buildPrompt(promptGroups...)

	results := make([]ParallelResult, len(tables))
	var wg sync.WaitGroup

	for i, table := range tables {
		wg.Add(1)
		go func(idx int, tbl string) {
			defer wg.Done()

			sub := &SubAgent{
				LLM:      llmClient,
				Registry: registry.Subset(toolSubset),
				Prompt:   prompt,
				MaxSteps: 20,
			}

			var task string
			switch skillName {
			case "AdvancedAnalysis":
				task = fmt.Sprintf(
					"테이블 '%s'에 대해 심층 분석을 수행하세요.\n"+
						"1. list_table_tags로 태그 목록 확인\n"+
						"2. execute_sql_query로 태그별 통계(COUNT/AVG/MIN/MAX) GROUP BY NAME\n"+
						"3. execute_sql_query로 시간 범위 확인 (SELECT MIN(TIME), MAX(TIME) FROM %s, timeformat='ms')\n"+
						"4. TQL 템플릿으로 차트 파일 저장 (save_tql_file)\n"+
						"5. create_dashboard_with_charts로 대시보드 생성\n"+
						"6. preview_dashboard로 URL 확인\n"+
						"결과를 JSON으로 반환: {\"summary\":\"...\", \"artifacts\":[...], \"data\":{\"table\":\"%s\", \"tags\":[...], \"stats\":{...}, \"dashboard\":\"...\"}}",
					tbl, tbl, tbl)
			default: // BasicAnalysis
				task = fmt.Sprintf(
					"테이블 '%s'에 대해 기본 분석을 수행하세요.\n"+
						"1. list_table_tags로 태그 목록 확인\n"+
						"2. execute_sql_query로 태그별 통계(COUNT/AVG/MIN/MAX) GROUP BY NAME\n"+
						"3. execute_sql_query로 시간 범위 확인 (SELECT MIN(TIME), MAX(TIME) FROM %s, timeformat='ms')\n"+
						"4. create_dashboard_with_charts로 대시보드 생성 (table-based 차트)\n"+
						"5. preview_dashboard로 URL 확인\n"+
						"결과를 JSON으로 반환: {\"summary\":\"...\", \"artifacts\":[...], \"data\":{\"table\":\"%s\", \"tags\":[...], \"stats\":{...}, \"dashboard\":\"...\"}}",
					tbl, tbl, tbl)
			}

			logger.Infof("[SubAgent-%d] Starting: %s", idx, tbl)
			result, err := sub.Run(ctx, task)
			results[idx] = ParallelResult{Table: tbl, Result: result, Error: err}

			if err != nil {
				logger.Infof("[SubAgent-%d] %s failed: %v", idx, tbl, err)
			} else {
				logger.Infof("[SubAgent-%d] %s complete", idx, tbl)
			}
		}(i, table)
	}

	wg.Wait()
	logger.Infof("[Orchestrator] All %d sub-agents complete", len(tables))

	return results, nil
}

// FormatParallelResults composes a combined response from parallel sub-agent results.
func FormatParallelResults(results []ParallelResult, query string) string {
	var sb strings.Builder

	for _, r := range results {
		sb.WriteString(fmt.Sprintf("## %s\n", r.Table))
		if r.Error != nil {
			sb.WriteString(fmt.Sprintf("분석 실패: %v\n\n", r.Error))
			continue
		}
		if r.Result != nil {
			sb.WriteString(r.Result.Summary)
			if dash, ok := r.Result.Data["dashboard"]; ok {
				sb.WriteString(fmt.Sprintf("\n대시보드: %v", dash))
			}
		}
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// RunReportWithSubAgent orchestrates report generation via a sub-agent.
// The report sub-agent handles data discovery + report generation in one session.
func RunReportWithSubAgent(ctx context.Context, llmClient llm.LLMProvider, registry *tools.Registry, query string, buildPrompt func(groups ...string) string) (string, error) {
	logger.Infof("[Orchestrator] Report sub-agent starting")

	report := &SubAgent{
		LLM:      llmClient,
		Registry: registry.Subset(SubAgentToolSubset["report_writer"]),
		Prompt:   buildPrompt("sql_tools", "report_tools"),
		MaxSteps: 10,
	}

	result, err := report.Run(ctx, fmt.Sprintf("다음 요청에 대해 HTML 분석 리포트를 생성하세요. save_html_report 도구를 사용하세요: %s", query))
	if err != nil {
		return "", fmt.Errorf("report generation failed: %w", err)
	}

	logger.Infof("[Orchestrator] Report sub-agent complete")
	return result.Summary, nil
}
