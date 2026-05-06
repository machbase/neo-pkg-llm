package agent

import (
	"context"
	"fmt"
	"strings"

	promptctx "neo-pkg-llm/context"
	"neo-pkg-llm/fixer"
	"neo-pkg-llm/guard"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/skill"
	"neo-pkg-llm/tools"
)

// Event types yielded during streaming execution.
const (
	EventStatus     = "status"
	EventStream     = "stream"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
	EventFinal      = "final"
	EventError      = "error"
)

// Event represents a streaming event from the agent loop.
type Event struct {
	Type    string         `json:"type"`
	Step    int            `json:"step,omitempty"`
	Name    string         `json:"name,omitempty"`
	Args    map[string]any `json:"args,omitempty"`
	Status  string         `json:"status,omitempty"`
	Content string         `json:"content,omitempty"`
}

// Agent orchestrates the LLM ↔ tools loop.
type Agent struct {
	llm           llm.LLMProvider
	registry      *tools.Registry
	messages      []llm.Message
	maxSteps      int
	advanced      bool // true = 고급 분석 (TQL templates), false = 기본 분석 (table-based)
	reportMode    bool // true = HTML 리포트 생성 모드
	useSubAgents  bool // true = 서브에이전트 모드 활성화
	skillName     string
	toolDefs      []map[string]any // filtered tool definitions for LLM
	docCatalog    string           // cached document catalog (loaded once, reused across turns)
	fixerCtx      *fixer.FixerContext
	guardPipeline *guard.Pipeline
}

// --- guard.AgentState interface ---

// getToolDefs returns the filtered tool definitions (skill-based) or all tools as fallback.
func (a *Agent) getToolDefs() []map[string]any {
	if a.toolDefs != nil {
		return a.toolDefs
	}
	return a.registry.AllToolDefs()
}

func (a *Agent) Messages() []llm.Message       { return a.messages }
func (a *Agent) IsAdvanced() bool               { return a.advanced }
func (a *Agent) IsReport() bool                 { return a.reportMode }
func (a *Agent) LLM() llm.LLMProvider           { return a.llm }
func (a *Agent) Registry() *tools.Registry       { return a.registry }

// NewAgent creates an agent. Set useSubAgents=true to enable sub-agent delegation for advanced/report tasks.
func NewAgent(llmClient llm.LLMProvider, registry *tools.Registry) *Agent {
	return NewAgentWithOptions(llmClient, registry, false)
}

// NewAgentWithSubAgents creates an agent with sub-agent delegation enabled.
func NewAgentWithSubAgents(llmClient llm.LLMProvider, registry *tools.Registry) *Agent {
	return NewAgentWithOptions(llmClient, registry, true)
}

func NewAgentWithOptions(llmClient llm.LLMProvider, registry *tools.Registry, useSubAgents bool) *Agent {
	a := &Agent{
		llm:          llmClient,
		registry:     registry,
		maxSteps:     60,
		useSubAgents: useSubAgents,
		fixerCtx:     &fixer.FixerContext{},
	}

	// Wire up fixer's template expansion callback
	fixer.ExpandTemplateFunc = ExpandTemplate

	// Wire up fixer's table name inference callback
	a.fixerCtx.InferTableName = func() string {
		return fixer.InferTableName(a.messages)
	}

	// Build guard pipeline
	a.guardPipeline = guard.NewPipeline(
		[]guard.Guard{
			&guard.DashboardEarlyGuard{},
			&guard.ConsecutiveFailureGuard{},
		},
		[]guard.Guard{
			&guard.ChartOmissionGuard{},
			&guard.ReportOmissionGuard{},
		},
	)

	// Wire up guard callbacks
	guard.FixToolCalls = func(msg llm.Message) llm.Message {
		return fixer.Fix(msg, a.fixerCtx)
	}
	guard.AppendMessages = func(msgs ...llm.Message) {
		a.messages = append(a.messages, msgs...)
	}

	return a
}

// detectTables checks the user query against known table names from list_tables.
// Returns matched table names (uppercase). Only useful when 2+ tables are found.
func detectTables(query string, registry *tools.Registry) []string {
	tableList, err := registry.ExecuteMap("list_tables", nil)
	if err != nil || tableList == "" {
		return nil
	}

	upper := strings.ToUpper(query)
	var matched []string
	for _, line := range strings.Split(tableList, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || name == "NAME" {
			continue
		}
		if strings.Contains(upper, name) {
			matched = append(matched, name)
		}
	}
	return matched
}

// compactHistory removes tool_calls and tool results from previous turns,
// keeping only system, user, and final assistant messages.
func compactHistory(messages []llm.Message) []llm.Message {
	var result []llm.Message
	for _, msg := range messages {
		if msg.Role == "tool" {
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			continue
		}
		result = append(result, msg)
	}
	return result
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("... (총 %d자)", len(s))
}

// Run executes the agent loop and returns the final text response.
func (a *Agent) Run(ctx context.Context, query string) (string, error) {
	isFirstTurn := !a.HasHistory()
	if isFirstTurn {
		a.initMessages(query)
	} else {
		a.ContinueMessages(query)
	}
	LoadTemplates()

	// Sub-agent parallel delegation: only when 2+ tables detected
	if a.useSubAgents && isFirstTurn {
		tables := detectTables(query, a.registry)
		if len(tables) >= 2 {
			buildPrompt := func(groups ...string) string {
				b := promptctx.NewBuilder().AddCore()
				b.AddToolPrompts(groups...)
				docList, _ := a.registry.ExecuteMap("list_available_documents", nil)
				if docList != "" {
					b.SetCatalog(promptctx.FormatCatalog(docList))
				}
				return b.Build()
			}

			fmt.Printf("[Agent] 병렬 모드: %d개 테이블 %v\n", len(tables), tables)
			results, err := RunParallel(ctx, a.llm, a.registry, tables, query, a.skillName, buildPrompt)
			if err != nil {
				return "", err
			}
			return FormatParallelResults(results, query), nil
		}
	}

	return a.runLoop(ctx)
}

// runLoop is the core agent loop, separated so SubAgent can call it directly.
func (a *Agent) runLoop(ctx context.Context) (string, error) {
	fmt.Printf("\n[Agent] Agentic Loop 시작...\n")
	fmt.Println(strings.Repeat("=", 60))

	step := 0
	for iter := 0; iter < a.maxSteps; iter++ {
		if ctx.Err() != nil {
			return "사용자에 의해 중단되었습니다.", nil
		}
		resp, err := a.llm.Chat(ctx, a.messages, a.getToolDefs())
		if err != nil {
			return "", fmt.Errorf("LLM call failed at step %d: %w", step, err)
		}

		msg := resp.Message
		msg = fixer.Fix(msg, a.fixerCtx)
		msg = a.guardPipeline.RunPreTool(ctx, a, msg)

		if len(msg.ToolCalls) == 0 {
			msg = a.guardPipeline.RunPostLoop(ctx, a, msg)

			if len(msg.ToolCalls) == 0 {
				// Guard: empty response
				if msg.Content == "" {
					fmt.Println("\n[Agent] 빈 응답 → 재시도")
					a.messages = append(a.messages, llm.Message{
						Role:    "user",
						Content: "작업이 완료되지 않았습니다. 다음 단계를 계속 진행하세요.",
					})
					continue
				}
				// Save final assistant response for multi-turn context
				a.messages = append(a.messages, llm.Message{
					Role:    "assistant",
					Content: msg.Content,
				})
				fmt.Println(strings.Repeat("=", 60))
				return appendMissingReportURL(msg.Content, a.messages), nil
			}

			// PostLoop guard re-prompted with tool calls → apply fixer + pre-tool guards
			msg = fixer.Fix(msg, a.fixerCtx)
			msg = a.guardPipeline.RunPreTool(ctx, a, msg)
		}

		// Execute tool calls
		a.messages = append(a.messages, msg)
		for _, tc := range msg.ToolCalls {
			if ctx.Err() != nil {
				return "사용자에 의해 중단되었습니다.", nil
			}
			step++
			fmt.Printf("\n[Step %d] 도구 호출: %s\n", step, tc.Function.Name)
			args := tc.Function.Arguments
			if args != nil {
				for k, v := range args {
					vs := fmt.Sprintf("%v", v)
					if len(vs) > 200 {
						vs = vs[:200] + "..."
					}
					fmt.Printf("  ├─ %s: %s\n", k, vs)
				}
			}

			// Dashboard time correction
			fixer.FixDashboardTime(&tc, a.fixerCtx)

			// TAG validation
			if tagErr := fixer.ValidateTagInArgs(tc.Function.Name, tc.Function.Arguments, a.fixerCtx.KnownTags); tagErr != "" {
				result := tagErr
				fmt.Printf("  └─ ✗ TAG ERROR: %s\n", truncate(result, 500))
				fmt.Println(strings.Repeat("-", 60))
				a.messages = append(a.messages, llm.Message{
					Role:    "tool",
					Content: result,
				})
				continue
			}

			// TQL time range fix
			fixer.FixTQLTimeRange(&tc, a.fixerCtx)

			result, err := a.registry.ExecuteMap(tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				fmt.Printf("  └─ ✗ ERROR: %s\n", truncate(result, 500))
			} else {
				fmt.Printf("  └─ ✓: %s\n", truncate(result, 500))
			}
			fmt.Println(strings.Repeat("-", 60))

			// Post-execution capture
			fixer.CaptureResults(tc, result, err, a.fixerCtx)

			a.messages = append(a.messages, llm.Message{
				Role:    "tool",
				Content: result,
			})
		}
	}

	fmt.Println(strings.Repeat("=", 60))
	return "최대 실행 횟수에 도달했습니다.", nil
}

// RunStream executes the agent loop and yields events via a channel.
func (a *Agent) RunStream(ctx context.Context, query string) <-chan Event {
	ch := make(chan Event, 100)

	go func() {
		defer close(ch)

		isFirstTurn := !a.HasHistory()
		if isFirstTurn {
			a.initMessages(query)
		} else {
			a.ContinueMessages(query)
		}

		// Sub-agent parallel delegation for streaming (2+ tables only)
		if a.useSubAgents && isFirstTurn {
			tables := detectTables(query, a.registry)
			if len(tables) >= 2 {
				buildPrompt := func(groups ...string) string {
					b := promptctx.NewBuilder().AddCore()
					b.AddToolPrompts(groups...)
					docList, _ := a.registry.ExecuteMap("list_available_documents", nil)
					if docList != "" {
						b.SetCatalog(promptctx.FormatCatalog(docList))
					}
					return b.Build()
				}

				ch <- Event{Type: EventStatus, Content: fmt.Sprintf("병렬 분석 모드: %d개 테이블 %v", len(tables), tables)}
				results, err := RunParallel(ctx, a.llm, a.registry, tables, query, a.skillName, buildPrompt)
				if err != nil {
					ch <- Event{Type: EventError, Content: err.Error()}
				} else {
					ch <- Event{Type: EventFinal, Content: FormatParallelResults(results, query)}
				}
				return
			}
		}
		LoadTemplates()
		ch <- Event{Type: EventStatus, Content: "답변 생성중..."}

		step := 0
		for step < a.maxSteps {
			if ctx.Err() != nil {
				ch <- Event{Type: EventFinal, Content: "사용자에 의해 중단되었습니다."}
				return
			}
			resp, err := a.llm.ChatStream(ctx, a.messages, a.getToolDefs(), func(partial *llm.ChatResponse) {
				if partial.Message.Content != "" {
					ch <- Event{Type: EventStream, Content: partial.Message.Content}
				}
			})
			if err != nil {
				ch <- Event{Type: EventError, Content: fmt.Sprintf("LLM 오류: %v", err)}
				return
			}

			msg := resp.Message
			msg = fixer.Fix(msg, a.fixerCtx)
			msg = a.guardPipeline.RunPreTool(ctx, a, msg)

			if len(msg.ToolCalls) == 0 {
				msg = a.guardPipeline.RunPostLoop(ctx, a, msg)

				if len(msg.ToolCalls) == 0 {
					if msg.Content == "" {
						a.messages = append(a.messages, llm.Message{
							Role:    "user",
							Content: "작업이 완료되지 않았습니다. 다음 단계를 계속 진행하세요.",
						})
						continue
					}
					// Save final assistant response to messages for multi-turn context
					a.messages = append(a.messages, llm.Message{
						Role:    "assistant",
						Content: msg.Content,
					})
					ch <- Event{Type: EventFinal, Content: appendMissingReportURL(msg.Content, a.messages)}
					return
				}

				// PostLoop guard re-prompted with tool calls → apply fixer + pre-tool guards
				msg = fixer.Fix(msg, a.fixerCtx)
				msg = a.guardPipeline.RunPreTool(ctx, a, msg)
			}

			// Execute tool calls
			a.messages = append(a.messages, msg)
			for _, tc := range msg.ToolCalls {
				if ctx.Err() != nil {
					ch <- Event{Type: EventFinal, Content: "사용자에 의해 중단되었습니다."}
					return
				}
				step++
				fmt.Printf("\n[Step %d] 도구 호출: %s\n", step, tc.Function.Name)
				for k, v := range tc.Function.Arguments {
					vs := fmt.Sprintf("%v", v)
					if len(vs) > 200 {
						vs = vs[:200] + "..."
					}
					fmt.Printf("  ├─ %s: %s\n", k, vs)
				}
				ch <- Event{
					Type: EventToolCall,
					Step: step,
					Name: tc.Function.Name,
					Args: tc.Function.Arguments,
				}

				// TQL time range fix
				fixer.FixTQLTimeRange(&tc, a.fixerCtx)

				// Dashboard time correction
				fixer.FixDashboardTime(&tc, a.fixerCtx)

				// TAG validation
				if tagErr := fixer.ValidateTagInArgs(tc.Function.Name, tc.Function.Arguments, a.fixerCtx.KnownTags); tagErr != "" {
					ch <- Event{
						Type:    EventToolResult,
						Status:  "error",
						Content: tagErr,
					}
					a.messages = append(a.messages, llm.Message{
						Role:    "tool",
						Content: tagErr,
					})
					continue
				}

				result, err := a.registry.ExecuteMap(tc.Function.Name, tc.Function.Arguments)
				status := "success"
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
					status = "error"
				}

				ch <- Event{
					Type:    EventToolResult,
					Status:  status,
					Content: result,
				}

				// Post-execution capture
				fixer.CaptureResults(tc, result, err, a.fixerCtx)

				a.messages = append(a.messages, llm.Message{
					Role:    "tool",
					Content: result,
				})
			}
		}

		ch <- Event{Type: EventFinal, Content: "최대 실행 횟수에 도달했습니다."}
	}()

	return ch
}

// buildSystemPrompt constructs the system prompt for a given skill.
// This is called on first turn and whenever skill changes.
func (a *Agent) buildSystemPrompt(activeSkill *skill.Skill) string {
	isOllama := false
	if _, ok := a.llm.(*llm.OllamaClient); ok {
		isOllama = true
	}

	builder := promptctx.NewBuilder()
	if isOllama {
		builder.SetOllama()
	}
	if !activeSkill.SkipCore {
		builder.AddCore()
	} else {
		builder.AddSegment("Role")
	}
	builder.AddWorkflow(activeSkill.Workflows...)
	builder.AddToolPrompts(activeSkill.ToolGroups...)

	// Inject document catalog
	if a.docCatalog != "" {
		builder.SetCatalog(a.docCatalog)
	}

	systemPrompt := builder.Build()

	if isOllama {
		systemPrompt += "\n/no_think"
	}
	return systemPrompt
}

// applySkill updates toolDefs and provider-specific settings for the active skill.
func (a *Agent) applySkill(activeSkill *skill.Skill) {
	if activeSkill.AllowTools != nil {
		allowed := make(map[string]bool, len(activeSkill.AllowTools))
		for _, t := range activeSkill.AllowTools {
			allowed[t] = true
		}
		allDefs := a.registry.AllToolDefs()
		filtered := make([]map[string]any, 0, len(activeSkill.AllowTools))
		for _, def := range allDefs {
			fn := def["function"].(map[string]any)
			if allowed[fn["name"].(string)] {
				filtered = append(filtered, def)
			}
		}
		a.toolDefs = filtered
	} else {
		a.toolDefs = a.registry.AllToolDefs()
	}
}

func (a *Agent) initMessages(query string) {
	// 1. Skill classification
	skillRegistry := skill.NewRegistry()
	activeSkill := skillRegistry.Classify(query)

	// 2. Set agent mode flags from skill
	a.skillName = activeSkill.Name
	a.advanced = (activeSkill.Name == "AdvancedAnalysis")
	a.reportMode = (activeSkill.Name == "Report")

	// 3. Load document catalog (cached for reuse across turns)
	docList, _ := a.registry.ExecuteMap("list_available_documents", nil)
	if docList != "" {
		a.docCatalog = promptctx.FormatCatalog(docList)
	}

	// 4. Build system prompt for current skill
	systemPrompt := a.buildSystemPrompt(activeSkill)

	// 5. Build user content with skill hint + time range
	userContent := query
	if tr := parseTimeRange(query); tr != nil {
		a.fixerCtx.TimeStartDt = tr.StartDt
		a.fixerCtx.TimeEndDt = tr.EndDt
	} else {
		a.fixerCtx.TimeStartDt = ""
		a.fixerCtx.TimeEndDt = ""
	}

	if hint := buildSkillHint(query); hint != "" {
		userContent += "\n\n" + hint
	}

	// 6. Set messages
	a.messages = []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	// 7. Provider-specific optimizations
	if ollamaClient, ok := a.llm.(*llm.OllamaClient); ok {
		ollamaClient.SetNumKeep(activeSkill.Name)
	}
	if geminiClient, ok := a.llm.(*llm.GeminiClient); ok {
		geminiClient.SetupCache(systemPrompt, a.getToolDefs())
	}

	// 8. Filter tool definitions based on skill
	a.applySkill(activeSkill)

	fmt.Printf("[Agent] Skill: %s | Workflows: %v | ToolGroups: %v | Tools: %d/%d\n",
		activeSkill.Name, activeSkill.Workflows, activeSkill.ToolGroups,
		len(a.toolDefs), len(a.getToolDefs()))
}

// HasHistory returns true if the agent has prior conversation messages.
func (a *Agent) HasHistory() bool {
	return len(a.messages) > 0
}

// buildSkillHint returns a skill-specific hint string for the given query.
// Used by both initMessages and ContinueMessages to guide the LLM.
func buildSkillHint(query string) string {
	skillRegistry := skill.NewRegistry()
	activeSkill := skillRegistry.Classify(query)

	timeHint := " 반드시 SELECT MIN(TIME), MAX(TIME) FROM 테이블 (timeformat='ms')로 시간 범위를 먼저 조회하고, " +
		"그 결과를 time_start/time_end에 문자열로 전달하세요. now-1h 등 상대값 사용 금지!"
	if tr := parseTimeRange(query); tr != nil {
		timeHint = fmt.Sprintf(" 시간 범위가 지정되었습니다. time_start=%s, time_end=%s (%s). "+
			"이 값을 time_start, time_end에 그대로 사용하세요. "+
			"통계 SQL 조회 시에도 WHERE TIME BETWEEN TO_DATE('%s') AND TO_DATE('%s') 조건을 추가하세요. "+
			"ROLLUP UNIT은 %s을 사용하세요.", tr.StartMs, tr.EndMs, tr.Label, tr.StartDt, tr.EndDt, tr.Unit)
	}

	switch activeSkill.Name {
	case "Report":
		templateHint := ""
		queryLower := strings.ToLower(query)
		if containsAny(queryLower, []string{"금융", "주식", "종목", "주가", "환율", "원자재", "finance", "stock"}) {
			templateHint = " template_id='R-1'로 지정하세요."
		} else if containsAny(queryLower, []string{"진동", "vibration", "베어링", "bearing"}) {
			templateHint = " template_id='R-2'로 지정하세요."
		} else if containsAny(queryLower, []string{"운전", "운행", "주행", "차량", "driving", "드라이빙"}) {
			templateHint = " template_id='R-3'로 지정하세요."
		}
		return "[시스템 힌트: HTML 분석 리포트 요청입니다. " +
			"사전 쿼리(execute_sql_query, list_table_tags) 없이 save_html_report(table=테이블명)을 바로 호출하세요." +
			templateHint + " " +
			"통계/태그/시간범위는 도구가 내부에서 처리합니다. " +
			"create_dashboard_with_charts, create_dashboard, add_chart_to_dashboard, save_tql_file, create_folder 사용 절대 금지! " +
			"오직 save_html_report만 사용하세요. " +
			"리포트 생성 후 도구 결과에 포함된 URL 링크를 반드시 ��종 답변에 포함하세요!" + timeHint + "]"
	case "AdvancedAnalysis":
		return "[시스템 힌트: 고급 분석 키워드가 감지되었습니다. 고급 분석(TQL 템플릿) 절차를 따르세요." + timeHint + "]"
	case "BasicAnalysis":
		return "[시스템 힌트: 기본 분석 요청입니다. 반드시 기본 분석(table-based 차트, create_dashboard_with_charts) 절차를 따르세요. " +
			"TQL 파일/템플릿/save_tql_file/create_folder를 절대 사용하지 마세요." + timeHint + "]"
	}
	return ""
}

// ContinueMessages appends a new user query to existing conversation history.
func (a *Agent) ContinueMessages(query string) {
	// Reset time range values from previous turn
	a.fixerCtx.TimeStartDt = ""
	a.fixerCtx.TimeEndDt = ""
	a.fixerCtx.DataMinDt = ""
	a.fixerCtx.DataMaxDt = ""

	// Re-parse time range for new query
	if tr := parseTimeRange(query); tr != nil {
		a.fixerCtx.TimeStartDt = tr.StartDt
		a.fixerCtx.TimeEndDt = tr.EndDt
	}

	// Re-classify skill
	skillRegistry := skill.NewRegistry()
	activeSkill := skillRegistry.Classify(query)
	prevSkill := a.skillName

	// Update agent mode flags
	a.skillName = activeSkill.Name
	a.reportMode = (activeSkill.Name == "Report")
	a.advanced = (activeSkill.Name == "AdvancedAnalysis")

	// Log skill classification every turn
	fmt.Printf("[Agent] Skill: %s | Workflows: %v | ToolGroups: %v | Tools: %d\n",
		activeSkill.Name, activeSkill.Workflows, activeSkill.ToolGroups, len(a.toolDefs))

	// Skill changed → compact history + rebuild system prompt
	if activeSkill.Name != prevSkill {
		a.messages = compactHistory(a.messages)
		newPrompt := a.buildSystemPrompt(activeSkill)
		a.messages[0] = llm.Message{Role: "system", Content: newPrompt}
		fmt.Printf("[Agent] Skill switch: %s → %s (system prompt rebuilt, history compacted: %d messages)\n",
			prevSkill, activeSkill.Name, len(a.messages))
	}

	// Update toolDefs based on new skill
	a.applySkill(activeSkill)

	// Build user content with skill hint
	userContent := query
	if hint := buildSkillHint(query); hint != "" {
		userContent += "\n\n" + hint
	}

	if _, ok := a.llm.(*llm.OllamaClient); ok {
		a.messages = []llm.Message{a.messages[0], {Role: "user", Content: userContent}}
	} else {
		a.messages = append(a.messages, llm.Message{
			Role:    "user",
			Content: userContent,
		})
	}
}

// appendMissingReportURL checks if a report was generated in this session
// and the final response is missing the URL. If so, appends it.
func appendMissingReportURL(content string, messages []llm.Message) string {
	// Find the last report URL from tool results (scan in reverse)
	var lastURL string
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "tool" && strings.Contains(msg.Content, "Report saved:") {
			// Extract URL from markdown link: [리포트 열기](http://...)
			start := strings.Index(msg.Content, "](")
			end := strings.LastIndex(msg.Content, ")")
			if start != -1 && end > start {
				lastURL = msg.Content[start+2 : end]
			}
			break
		}
	}

	if lastURL == "" || strings.Contains(content, lastURL) {
		return content
	}

	fmt.Printf("[Agent] 리포트 URL 누락 → 자동 추가: %s\n", lastURL)
	return content + fmt.Sprintf("\n\n[리포트 열기](%s)", lastURL)
}
