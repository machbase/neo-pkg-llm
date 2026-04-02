package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"neo-pkg-llm/llm"
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

// Template constants (mirroring Python graph.py)
var (
	templateExpected = map[string]int{"1": 6, "2": 7, "3": 4}

	templateAllIDs = map[string][]string{
		"1": {"1-1", "1-2", "1-3", "1-4", "1-5", "1-6"},
		"2": {"2-1", "2-2", "2-3", "2-4", "2-5", "2-6", "2-7"},
		"3": {"3-1", "3-2", "3-3", "3-4"},
	}

	templateNames = map[string]string{
		"1-1": "평균 추세", "1-2": "변동성", "1-3": "가격 밴드",
		"1-4": "태그 비교", "1-5": "거래량 추세", "1-6": "로그 가격",
		"2-1": "RMS 진동", "2-2": "FFT 스펙트럼", "2-3": "피크 엔벨로프",
		"2-4": "Peak-to-Peak", "2-5": "Crest Factor", "2-6": "데이터 밀도", "2-7": "3D 스펙트럼",
		"3-1": "롤업 평균", "3-2": "태그 비교", "3-3": "카운트 추세", "3-4": "MIN/MAX 엔벨로프",
	}

	dashboardTools = map[string]bool{
		"create_dashboard":             true,
		"create_dashboard_with_charts": true,
		"add_chart_to_dashboard":       true,
		"remove_chart_from_dashboard":  true,
		"update_chart_in_dashboard":    true,
		"delete_dashboard":             true,
		"update_dashboard_time_range":  true,
		"preview_dashboard":            true,
		"get_dashboard":                true,
	}

	tqlFuncRE     = regexp.MustCompile(`\)[ \t]*(SQL_SELECT|SQL|SCRIPT|CHART_LINE|CHART_BAR3D|CHART|MAPVALUE|POPVALUE|MAPKEY|GROUPBYKEY|FFT|FLATTEN|PUSHKEY|CSV)\(`)
	templateRefRE = regexp.MustCompile(`TEMPLATE:\s*(\d+-\d+)\s+TABLE:\s*(\S+)(?:\s+TAG:\s*(\S+))?(?:\s+UNIT:\s*(\S+))?(?:\s+TAG1:\s*(\S+))?(?:\s+TAG2:\s*(\S+))?`)
	templateIDRE  = regexp.MustCompile(`(\d+[-_]\d+)`)
)

// Agent orchestrates the LLM ↔ tools loop.
type Agent struct {
	llm       llm.LLMProvider
	registry  *tools.Registry
	messages  []llm.Message
	maxSteps  int
	advanced  bool     // true = 고급 분석 (TQL templates), false = 기본 분석 (table-based)
	knownTags []string // list_table_tags 결과를 저장하여 TAG 검증에 사용
}

func NewAgent(llmClient llm.LLMProvider, registry *tools.Registry) *Agent {
	return &Agent{
		llm:      llmClient,
		registry: registry,
		maxSteps: 60,
	}
}

// Run executes the agent loop and returns the final text response.
func (a *Agent) Run(ctx context.Context, query string) (string, error) {
	if a.HasHistory() {
		a.ContinueMessages(query)
	} else {
		a.initMessages(query)
	}
	LoadTemplates()

	fmt.Printf("\n[Agent] 쿼리: %s\n", query)
	fmt.Println("[Agent] Agentic Loop 시작...")
	fmt.Println(strings.Repeat("=", 60))

	step := 0
	for iter := 0; iter < a.maxSteps; iter++ {
		if ctx.Err() != nil {
			return "사용자에 의해 중단되었습니다.", nil
		}
		resp, err := a.llm.Chat(ctx, a.messages, a.registry.AllToolDefs())
		if err != nil {
			return "", fmt.Errorf("LLM call failed at step %d: %w", step, err)
		}

		msg := resp.Message
		msg = a.fixToolCalls(msg)

		if a.advanced {
			msg = a.guardDashboardEarlyCall(ctx, msg)
		}
		msg = a.guardConsecutiveFailure(ctx, msg)

		if len(msg.ToolCalls) == 0 {
			if a.advanced {
				msg = a.guardChartOmission(ctx, msg)
			}

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
				fmt.Println(strings.Repeat("=", 60))
				return msg.Content, nil
			}
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

			// TAG 검증: TQL 관련 도구 호출 시 태그명 확인
			if tagErr := a.validateTagInArgs(tc.Function.Name, tc.Function.Arguments); tagErr != "" {
				result := tagErr
				fmt.Printf("  └─ ✗ TAG ERROR: %s\n", truncate(result, 500))
				fmt.Println(strings.Repeat("-", 60))
				a.messages = append(a.messages, llm.Message{
					Role:    "tool",
					Content: result,
				})
				continue
			}

			result, err := a.registry.ExecuteMap(tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				fmt.Printf("  └─ ✗ ERROR: %s\n", truncate(result, 500))
			} else {
				fmt.Printf("  └─ ✓: %s\n", truncate(result, 500))
			}
			fmt.Println(strings.Repeat("-", 60))

			// list_table_tags 결과에서 태그 목록 캡처
			if tc.Function.Name == "list_table_tags" && err == nil {
				a.captureKnownTags(result)
			}

			a.messages = append(a.messages, llm.Message{
				Role:    "tool",
				Content: result,
			})
		}
	}

	fmt.Println(strings.Repeat("=", 60))
	return "최대 실행 횟수에 도달했습니다.", nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("... (총 %d자)", len(s))
}

// RunStream executes the agent loop and yields events via a channel.
func (a *Agent) RunStream(ctx context.Context, query string) <-chan Event {
	ch := make(chan Event, 100)

	go func() {
		defer close(ch)

		if a.HasHistory() {
			a.ContinueMessages(query)
		} else {
			a.initMessages(query)
		}
		LoadTemplates()
		ch <- Event{Type: EventStatus, Content: "답변 생성중..."}

		step := 0
		for step < a.maxSteps {
			if ctx.Err() != nil {
				ch <- Event{Type: EventFinal, Content: "사용자에 의해 중단되었습니다."}
				return
			}
			resp, err := a.llm.ChatStream(ctx, a.messages, a.registry.AllToolDefs(), func(partial *llm.ChatResponse) {
				if partial.Message.Content != "" {
					ch <- Event{Type: EventStream, Content: partial.Message.Content}
				}
			})
			if err != nil {
				ch <- Event{Type: EventError, Content: fmt.Sprintf("LLM 오류: %v", err)}
				return
			}

			msg := resp.Message
			msg = a.fixToolCalls(msg)
			if a.advanced {
				msg = a.guardDashboardEarlyCall(ctx, msg)
			}
			msg = a.guardConsecutiveFailure(ctx, msg)

			if len(msg.ToolCalls) == 0 {
				if a.advanced {
					msg = a.guardChartOmission(ctx, msg)
				}

				if len(msg.ToolCalls) == 0 {
					if msg.Content == "" {
						a.messages = append(a.messages, llm.Message{
							Role:    "user",
							Content: "작업이 완료되지 않았습니다. 다음 단계를 계속 진행하세요.",
						})
						continue
					}
					ch <- Event{Type: EventFinal, Content: msg.Content}
					return
				}
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

				// TAG 검증: TQL 관련 도구 호출 시 태그명 확인
				if tagErr := a.validateTagInArgs(tc.Function.Name, tc.Function.Arguments); tagErr != "" {
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

				// list_table_tags 결과에서 태그 목록 캡처
				if tc.Function.Name == "list_table_tags" && err == nil {
					a.captureKnownTags(result)
				}

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

// advancedKeywords triggers advanced (TQL-based) analysis.
var advancedKeywords = []string{"심층", "다각도", "고급", "fft", "rms"}

func isAdvancedQuery(query string) bool {
	q := strings.ToLower(query)
	for _, kw := range advancedKeywords {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func (a *Agent) initMessages(query string) {
	// Inject document catalog into system prompt
	docList, _ := a.registry.ExecuteMap("list_available_documents", nil)
	systemPrompt := llm.SystemPrompt
	if docList != "" {
		systemPrompt += "\n\n## 문서 카탈로그 (경로 | 한국어 제목 | 키워드)\n" + docList + "\n"
	}

	// Detect analysis type and set agent mode
	userContent := query
	if strings.Contains(query, "분석") || strings.Contains(query, "대시보드") {
		a.advanced = isAdvancedQuery(query)
		if a.advanced {
			userContent += "\n\n[시스템 힌트: 고급 분석 키워드가 감지되었습니다. 고급 분석(TQL 템플릿) 절차를 따르세요. " +
				"반드시 SELECT MIN(TIME), MAX(TIME) FROM 테이블 (timeformat='ms')로 시간 범위를 먼저 조회하고, " +
				"그 결과를 time_start/time_end에 문자열로 전달하세요. now-1h 등 상대값 사용 금지!]"
		} else {
			userContent += "\n\n[시스템 힌트: 기본 분석 요청입니다. 반드시 기본 분석(table-based 차트, create_dashboard_with_charts) 절차를 따르세요. " +
				"TQL 파일/템플릿/save_tql_file/create_folder를 절대 사용하지 마세요. create_dashboard_with_charts 한 번으로 대시보드를 완성하세요. " +
				"반드시 SELECT MIN(TIME), MAX(TIME) FROM 테이블 (timeformat='ms')로 시간 범위를 먼저 조회하고, " +
				"그 결과를 time_start/time_end에 문자열로 전달하세요. now-1h 등 상대값 사용 금지!]"
		}
	}

	a.messages = []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	// Set num_keep for Ollama to pin system prompt in KV cache
	if ollamaClient, ok := a.llm.(*llm.OllamaClient); ok {
		ollamaClient.SetNumKeep(systemPrompt)
	}
}

// HasHistory returns true if the agent has prior conversation messages.
func (a *Agent) HasHistory() bool {
	return len(a.messages) > 0
}

// ContinueMessages appends a new user query to existing conversation history.
func (a *Agent) ContinueMessages(query string) {
	a.messages = append(a.messages, llm.Message{
		Role:    "user",
		Content: query,
	})
}

// inferTableName scans previous messages to find a table name.
// 1) Find list_tables result → extract known table names
// 2) Match against user query or LLM text mentioning a table
// 3) If only one table exists, use it directly
func (a *Agent) inferTableName() string {
	// Collect known table names from list_tables results
	var knownTables []string
	for i, m := range a.messages {
		if m.Role == "tool" && i > 0 {
			// Check if previous message had a list_tables call
			prev := a.messages[i-1]
			for _, tc := range prev.ToolCalls {
				if tc.Function.Name == "list_tables" {
					for _, line := range strings.Split(strings.TrimSpace(m.Content), "\n") {
						t := strings.TrimSpace(line)
						if t != "" && t != "NAME" && !strings.Contains(t, " ") {
							knownTables = append(knownTables, t)
						}
					}
				}
			}
		}
	}

	if len(knownTables) == 0 {
		return ""
	}
	if len(knownTables) == 1 {
		return knownTables[0]
	}

	// Find user query and LLM text to match against table names
	var searchText string
	for _, m := range a.messages {
		if m.Role == "user" || m.Role == "assistant" {
			searchText += " " + strings.ToUpper(m.Content)
		}
	}

	for _, t := range knownTables {
		if strings.Contains(searchText, t) {
			return t
		}
	}

	return ""
}

// --- Tool call auto-fixing (mirrors Python _fix_tool_calls) ---

func (a *Agent) fixToolCalls(msg llm.Message) llm.Message {
	if len(msg.ToolCalls) == 0 {
		return msg
	}

	for i := range msg.ToolCalls {
		tc := &msg.ToolCalls[i]
		args := tc.Function.Arguments
		if args == nil {
			args = map[string]any{}
			tc.Function.Arguments = args
		}
		name := tc.Function.Name

		// Normalize alias keys to canonical names BEFORE any key-specific logic.
		// This ensures template detection, dashboard title fix, etc. all work
		// regardless of which parameter name the LLM chose to use.
		normalizeArgs(name, args)

		// list_table_tags: table_name 누락 시 이전 messages에서 자동 추론
		if name == "list_table_tags" {
			table, _ := args["table_name"].(string)
			if table == "" {
				if inferred := a.inferTableName(); inferred != "" {
					args["table_name"] = inferred
					fmt.Printf("  [fix] list_table_tags table_name 자동 삽입: %s\n", inferred)
				}
			}
		}

		// Literal \n → real newline
		for k, v := range args {
			if s, ok := v.(string); ok && strings.Contains(s, "\\n") {
				args[k] = strings.ReplaceAll(s, "\\n", "\n")
			}
		}

		// charts: list/dict → JSON string
		if charts, ok := args["charts"]; ok {
			switch c := charts.(type) {
			case []any, map[string]any:
				data, _ := json.Marshal(c)
				args["charts"] = string(data)
			}
		}

		// charts: table 필드 누락 시 자동 삽입
		if chartsStr, ok := args["charts"].(string); ok && chartsStr != "" {
			if inferred := a.inferTableName(); inferred != "" {
				var chartList []map[string]any
				if json.Unmarshal([]byte(chartsStr), &chartList) == nil {
					fixed := false
					for i := range chartList {
						t, _ := chartList[i]["table"].(string)
						if t == "" {
							chartList[i]["table"] = inferred
							fixed = true
						}
					}
					if fixed {
						data, _ := json.Marshal(chartList)
						args["charts"] = string(data)
						fmt.Printf("  [fix] charts table 자동 삽입: %s\n", inferred)
					}
				}
			}
		}

		// time_start/time_end: nanoseconds → milliseconds
		for _, key := range []string{"time_start", "time_end"} {
			if v, ok := args[key].(string); ok && len(v) > 15 && isAllDigits(v) {
				// Convert nanoseconds to milliseconds
				args[key] = v[:len(v)-6]
			}
		}

		// save_tql_file / delete_file: merge folder_name into filename
		if name == "save_tql_file" || name == "delete_file" {
			fn, _ := args["filename"].(string)
			folder, _ := args["folder_name"].(string)
			if fn != "" && folder != "" && !strings.Contains(fn, "/") {
				args["filename"] = folder + "/" + fn
				delete(args, "folder_name")
				fmt.Printf("  [fix] Merged folder into filename: %s\n", args["filename"])
			}
		}

		// Dashboard filename/title auto-fix for all dashboard tools
		if dashboardTools[name] {
			fn, _ := args["filename"].(string)
			if fn != "" {
				// Auto-append .dsh extension
				if !strings.HasSuffix(strings.ToLower(fn), ".dsh") {
					fn = fn + ".dsh"
				}
				// If no folder, infer from filename or table context
				if !strings.Contains(fn, "/") {
					// e.g. "BITCOIN_analysis.dsh" → "BITCOIN/BITCOIN_analysis.dsh"
					base := strings.TrimSuffix(fn, ".dsh")
					parts := strings.SplitN(base, "_", 2)
					folder := strings.ToUpper(parts[0])
					fn = folder + "/" + fn
				}
				args["filename"] = fn
				fmt.Printf("  [fix] Dashboard filename → %s\n", fn)
			}
		}

		// Dashboard title auto-fix
		if name == "create_dashboard" || name == "create_dashboard_with_charts" {
			title, _ := args["title"].(string)
			if title == "" || title == "New dashboard" || title == "Dashboard" || title == "dashboard" {
				fn, _ := args["filename"].(string)
				table := strings.Split(fn, "/")[0]
				if table == "" {
					table = "데이터"
				}
				args["title"] = table + " 심층 분석 대시보드"
			}
		}

		// TQL content: template reference detection → expansion
		if tql, ok := args["tql_content"].(string); ok {
			tql = strings.TrimSpace(tql)
			match := templateRefRE.FindStringSubmatch(tql)
			if match != nil {
				params := map[string]string{"TABLE": match[2]}
				if match[3] != "" {
					params["TAG"] = match[3]
				}
				if match[4] != "" {
					params["UNIT"] = match[4]
				}
				if match[5] != "" {
					params["TAG1"] = match[5]
				}
				if match[6] != "" {
					params["TAG2"] = match[6]
				}
				expanded, err := ExpandTemplate(match[1], params)
				if err == nil {
					args["tql_content"] = expanded
					fmt.Printf("  [fix] Template %s expanded\n", match[1])
				}
			} else {
				// Try to auto-detect template from filename
				autoExpanded := false
				if fn, ok := args["filename"].(string); ok {
					if idMatch := templateIDRE.FindString(fn); idMatch != "" {
						idMatch = strings.ReplaceAll(idMatch, "_", "-") // normalize 1_1 → 1-1
						table := strings.Split(fn, "/")[0]
						nameRE := regexp.MustCompile(`NAME\s*=\s*'([^']+)'`)
						unitRE := regexp.MustCompile(`ROLLUP\('(\w+)'`)
						tag := ""
						unit := "'day'"
						if m := nameRE.FindStringSubmatch(tql); m != nil {
							tag = m[1]
						}
						if m := unitRE.FindStringSubmatch(tql); m != nil {
							unit = "'" + m[1] + "'"
						}
						params := map[string]string{"TABLE": table, "UNIT": unit}
						if idMatch == "1-4" || idMatch == "3-2" {
							tagsRE := regexp.MustCompile(`'([^']+)'`)
							allTags := tagsRE.FindAllStringSubmatch(tql, -1)
							var names []string
							skipUnits := map[string]bool{"day": true, "hour": true, "sec": true, "min": true, "week": true, "month": true}
							for _, t := range allTags {
								if !skipUnits[t[1]] {
									names = append(names, t[1])
								}
							}
							if len(names) >= 2 {
								params["TAG1"] = names[0]
								params["TAG2"] = names[1]
							}
						} else if tag != "" {
							params["TAG"] = tag
						}
						expanded, err := ExpandTemplate(idMatch, params)
						if err == nil {
							args["tql_content"] = expanded
							autoExpanded = true
							fmt.Printf("  [fix] Raw TQL → template %s auto-expanded\n", idMatch)
						}
					}
				}
				if !autoExpanded {
					// Fix TQL line breaks
					args["tql_content"] = tqlFuncRE.ReplaceAllStringFunc(tql, func(s string) string {
						// Insert newline before function name
						idx := strings.Index(s, ")")
						return s[:idx+1] + "\n" + strings.TrimSpace(s[idx+1:])
					})
				}
			}
		}
	}
	return msg
}

// --- Guard: dashboard early-call defense ---

func (a *Agent) guardDashboardEarlyCall(ctx context.Context, msg llm.Message) llm.Message {
	if len(msg.ToolCalls) == 0 || !dashboardTools[msg.ToolCalls[0].Function.Name] {
		return msg
	}

	savedIDs := a.getSavedTemplateIDs()

	var templateType string
	if len(savedIDs) > 0 {
		for id := range savedIDs {
			templateType = strings.Split(id, "-")[0]
			break
		}
	}

	expected := 4
	if templateType != "" {
		if e, ok := templateExpected[templateType]; ok {
			expected = e
		}
	}

	if len(savedIDs) >= expected {
		return msg // Enough templates saved, allow dashboard creation
	}

	allIDs, _ := templateAllIDs[templateType]
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

	// Inject retry message (must add tool_result for each tool_use to satisfy Claude API)
	a.messages = append(a.messages, msg)
	for range msg.ToolCalls {
		a.messages = append(a.messages, llm.Message{
			Role:    "tool",
			Content: "cancelled: dashboard creation deferred",
		})
	}
	a.messages = append(a.messages, llm.Message{
		Role: "user",
		Content: fmt.Sprintf(
			"⚠ 아직 TQL 파일이 %d/%d개만 저장되었습니다. "+
				"미저장 템플릿: %v. "+
				"지금 바로 save_tql_file을 호출하세요! "+
				"tql_content: TEMPLATE:%s TABLE:(테이블명) TAG:(태그명) UNIT:(단위)",
			len(savedIDs), expected, missingIDs, nextID,
		),
	})

	resp, err := a.llm.Chat(ctx, a.messages, a.registry.AllToolDefs())
	if err != nil {
		return msg
	}
	return a.fixToolCalls(resp.Message)
}

// --- Guard: consecutive failure detection ---

func (a *Agent) guardConsecutiveFailure(ctx context.Context, msg llm.Message) llm.Message {
	if len(msg.ToolCalls) == 0 {
		return msg
	}

	toolName := msg.ToolCalls[0].Function.Name
	failCount := a.countConsecutiveFailures(toolName)

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

	a.messages = append(a.messages, msg)
	for range msg.ToolCalls {
		a.messages = append(a.messages, llm.Message{
			Role:    "tool",
			Content: "cancelled: redirecting due to consecutive failures",
		})
	}
	a.messages = append(a.messages, llm.Message{Role: "user", Content: hint})

	resp, err := a.llm.Chat(ctx, a.messages, a.registry.AllToolDefs())
	if err != nil {
		return msg
	}
	return a.fixToolCalls(resp.Message)
}

// --- Guard: chart omission check ---

func (a *Agent) guardChartOmission(ctx context.Context, msg llm.Message) llm.Message {
	if msg.Content == "" || len(msg.ToolCalls) > 0 {
		return msg
	}

	allMsgs := append(a.messages, msg)
	savedTQLs := a.getSavedTQLPaths(allMsgs)
	dashFn := a.getDashboardFilename(allMsgs)
	chartCalls := a.countAddChartCalls(allMsgs)

	if len(savedTQLs) == 0 || dashFn == "" || chartCalls >= len(savedTQLs) {
		return msg
	}

	fmt.Printf("[Agent] Dashboard has %d/%d charts → prompting chart addition\n", chartCalls, len(savedTQLs))

	var cmds []string
	for _, st := range savedTQLs {
		table := strings.Split(st.Path, "/")[0]
		name := st.ID
		if n, ok := templateNames[st.ID]; ok {
			name = table + " " + n
		}
		cmds = append(cmds, fmt.Sprintf(
			`add_chart_to_dashboard(filename="%s", chart_type="Tql chart", tql_path="%s", chart_title="%s")`,
			dashFn, st.Path, name,
		))
	}

	a.messages = append(a.messages, msg)
	a.messages = append(a.messages, llm.Message{
		Role: "user",
		Content: fmt.Sprintf(
			"⚠ 대시보드가 생성되었지만 차트가 추가되지 않았습니다! "+
				"저장된 TQL 파일 %d개를 모두 대시보드에 추가하세요.\n"+
				"대시보드: %s\n아래 호출을 모두 실행하세요:\n%s\n그 후 preview_dashboard를 호출하세요.",
			len(savedTQLs), dashFn, strings.Join(cmds, "\n"),
		),
	})

	resp, err := a.llm.Chat(ctx, a.messages, a.registry.AllToolDefs())
	if err != nil {
		return msg
	}
	return a.fixToolCalls(resp.Message)
}

// --- Message analysis helpers ---

type savedTQL struct {
	ID   string
	Path string
}

func (a *Agent) getSavedTemplateIDs() map[string]bool {
	ids := map[string]bool{}
	var pendingTIDs []string // queue: one entry per tool call in an assistant message

	for _, msg := range a.messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			pendingTIDs = nil // reset for new batch
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "save_tql_file" {
					fn, _ := tc.Function.Arguments["filename"].(string)
					if m := templateIDRE.FindString(fn); m != "" {
						pendingTIDs = append(pendingTIDs, strings.ReplaceAll(m, "_", "-"))
					} else {
						pendingTIDs = append(pendingTIDs, "") // placeholder
					}
				} else {
					pendingTIDs = append(pendingTIDs, "") // non-save tool call
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

func (a *Agent) getSavedTQLPaths(msgs []llm.Message) []savedTQL {
	var saved []savedTQL
	var pendingPaths []savedTQL // queue: one entry per tool call

	for _, msg := range msgs {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			pendingPaths = nil // reset for new batch
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "save_tql_file" {
					fn, _ := tc.Function.Arguments["filename"].(string)
					tid := ""
					if m := templateIDRE.FindString(fn); m != "" {
						tid = strings.ReplaceAll(m, "_", "-")
					}
					pendingPaths = append(pendingPaths, savedTQL{ID: tid, Path: fn})
				} else {
					pendingPaths = append(pendingPaths, savedTQL{}) // placeholder
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

func (a *Agent) getDashboardFilename(msgs []llm.Message) string {
	for _, msg := range msgs {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if dashboardTools[tc.Function.Name] {
					if fn, ok := tc.Function.Arguments["filename"].(string); ok {
						return fn
					}
				}
			}
		}
	}
	return ""
}

func (a *Agent) countAddChartCalls(msgs []llm.Message) int {
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

func (a *Agent) countConsecutiveFailures(toolName string) int {
	count := 0
	for i := len(a.messages) - 1; i >= 0; i-- {
		msg := a.messages[i]
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

// captureKnownTags parses list_table_tags result and stores tag names.
// Result format: "NAME\ntag1\ntag2\n..."
func (a *Agent) captureKnownTags(result string) {
	a.knownTags = nil
	for _, line := range strings.Split(strings.TrimSpace(result), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "NAME" {
			continue
		}
		// Format: "[TABLE] tag1, tag2, tag3" (fallback 형식)
		if strings.HasPrefix(line, "[") {
			if idx := strings.Index(line, "] "); idx >= 0 {
				for _, t := range strings.Split(line[idx+2:], ",") {
					if tag := strings.TrimSpace(t); tag != "" {
						a.knownTags = append(a.knownTags, tag)
					}
				}
			}
			continue
		}
		// Format: "tag" (기본 형식)
		a.knownTags = append(a.knownTags, line)
	}
	if len(a.knownTags) > 0 {
		fmt.Printf("  [guard] Known tags captured: %d tags\n", len(a.knownTags))
	}
}

// validateTagInArgs checks if TAG parameters in TQL-related tool calls use valid tag names.
// Returns error string if invalid, empty string if OK.
func (a *Agent) validateTagInArgs(toolName string, args map[string]any) string {
	if len(a.knownTags) == 0 {
		return "" // no tags captured yet, skip validation
	}

	// Only validate TQL-related tools
	if toolName != "save_tql_file" && toolName != "execute_tql_script" && toolName != "validate_chart_tql" {
		return ""
	}

	// Extract TQL content
	tql, _ := args["tql_content"].(string)
	if tql == "" {
		tql, _ = args["tql_script"].(string)
	}
	if tql == "" {
		return ""
	}

	// Detect unsubstituted placeholders like {TAG}, {TAG1}, {TAG2}, {TABLE}, {UNIT}
	placeholderRE := regexp.MustCompile(`\{(TAG\d?|TABLE|UNIT)\}`)
	if found := placeholderRE.FindAllString(tql, -1); len(found) > 0 {
		return fmt.Sprintf(
			"Error: 플레이스홀더 %v가 치환되지 않았습니다. "+
				"tql_content에 raw TQL을 직접 쓰지 마세요! "+
				"반드시 TEMPLATE:ID TABLE:테이블 TAG:태그 UNIT:단위 형식을 사용하세요. "+
				"예: TEMPLATE:3-2 TABLE:STAT TAG1:machbase:http:latency TAG2:machbase:ps:cpu_percent",
			found)
	}

	// Check for NAME = 'tag' patterns in TQL
	nameRE := regexp.MustCompile(`NAME\s*=\s*'([^']+)'`)
	matches := nameRE.FindAllStringSubmatch(tql, -1)

	tagSet := make(map[string]bool)
	for _, t := range a.knownTags {
		tagSet[t] = true
	}

	var invalidTags []string
	for _, m := range matches {
		if len(m) > 1 && !tagSet[m[1]] {
			invalidTags = append(invalidTags, m[1])
		}
	}

	// Also check NAME IN ('tag1', 'tag2') patterns
	inRE := regexp.MustCompile(`NAME\s+IN\s*\(([^)]+)\)`)
	inMatches := inRE.FindAllStringSubmatch(tql, -1)
	for _, m := range inMatches {
		if len(m) > 1 {
			tagListRE := regexp.MustCompile(`'([^']+)'`)
			tags := tagListRE.FindAllStringSubmatch(m[1], -1)
			for _, t := range tags {
				if len(t) > 1 && !tagSet[t[1]] {
					invalidTags = append(invalidTags, t[1])
				}
			}
		}
	}

	if len(invalidTags) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"Error: 존재하지 않는 태그명이 사용되었습니다: %v\n사용 가능한 태그 목록: %v",
		invalidTags, a.knownTags,
	)
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// aliasMap maps tool name → { aliasKey → canonicalKey }.
// When the LLM sends a parameter under an alias name, we rename it
// to the canonical key so that downstream logic (template expansion,
// dashboard title fix, etc.) always finds the expected key.
var aliasMap = map[string]map[string]string{
	"save_tql_file": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"script": "tql_content", "content": "tql_content", "code": "tql_content", "tql_script": "tql_content", "tql": "tql_content",
	},
	"execute_tql_script": {
		"script": "tql_content", "content": "tql_content", "code": "tql_content",
	},
	"validate_chart_tql": {
		"script": "tql_script", "tql_content": "tql_script", "content": "tql_script",
	},
	"execute_sql_query": {
		"sql": "sql_query", "query": "sql_query",
	},
	"list_table_tags": {
		"table": "table_name", "name": "table_name", "table_id": "table_name",
	},
	"get_full_document_content": {
		"file_path": "file_identifier", "doc_name": "file_identifier", "path": "file_identifier", "document_path": "file_identifier", "doc_path": "file_identifier",
	},
	"get_document_sections": {
		"file_path": "file_identifier", "doc_name": "file_identifier", "path": "file_identifier", "document_path": "file_identifier", "doc_path": "file_identifier",
	},
	"extract_code_blocks": {
		"file_path": "file_identifier", "doc_name": "file_identifier", "path": "file_identifier", "document_path": "file_identifier", "doc_path": "file_identifier",
	},
	"create_folder": {
		"name": "folder_name", "path": "folder_name",
	},
	"delete_file": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
	},
	"create_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"create_dashboard_with_charts": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"add_chart_to_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
		"title": "chart_title", "type": "chart_type", "tql": "tql_path",
	},
	"remove_chart_from_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
		"chart_id": "panel_id", "id": "panel_id",
		"chart_title": "panel_title", "title": "panel_title", "chart_name": "panel_title",
	},
	"update_chart_in_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"chart_id": "panel_id", "id": "panel_id",
		"chart_title": "panel_title", "title": "panel_title", "chart_name": "panel_title",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"get_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"preview_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"delete_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
}

// canonicalKeys defines the expected parameter names per tool.
// Any unrecognized key whose value is a string will be matched
// to a missing canonical key by simple heuristics (contains check).
var canonicalKeys = map[string][]string{
	"save_tql_file":                {"filename", "tql_content"},
	"execute_tql_script":           {"tql_content"},
	"validate_chart_tql":           {"tql_script"},
	"execute_sql_query":            {"sql_query"},
	"list_table_tags":              {"table_name"},
	"get_full_document_content":    {"file_identifier"},
	"get_document_sections":        {"file_identifier"},
	"extract_code_blocks":          {"file_identifier"},
	"create_folder":                {"folder_name"},
	"delete_file":                  {"filename"},
	"create_dashboard":             {"filename"},
	"create_dashboard_with_charts": {"filename"},
	"add_chart_to_dashboard":       {"filename"},
	"remove_chart_from_dashboard":  {"filename"},
	"update_chart_in_dashboard":    {"filename"},
	"get_dashboard":                {"filename"},
	"preview_dashboard":            {"filename"},
	"delete_dashboard":             {"filename"},
	"update_connection":            {},
}

// normalizeArgs renames alias keys to their canonical names in-place.
// Two-pass: (1) known aliases from aliasMap, (2) fuzzy fallback for unknowns.
func normalizeArgs(toolName string, args map[string]any) {
	// Pass 1: explicit alias mapping
	if mapping, ok := aliasMap[toolName]; ok {
		for alias, canonical := range mapping {
			if _, hasCanonical := args[canonical]; hasCanonical {
				continue
			}
			if v, hasAlias := args[alias]; hasAlias {
				args[canonical] = v
				delete(args, alias)
			}
		}
	}

	// Strip leading "/" from filename/folder_name — LLM often sends "/FOLDER/file.tql"
	// but the Machbase Neo API expects relative paths like "FOLDER/file.tql".
	for _, key := range []string{"filename", "folder_name", "tql_path"} {
		if v, ok := args[key].(string); ok && strings.HasPrefix(v, "/") {
			args[key] = strings.TrimLeft(v, "/")
			fmt.Printf("  [fix] Stripped leading '/' from %s: %q\n", key, args[key])
		}
	}

	// Pass 2: fuzzy fallback — if a canonical key is still missing,
	// try to find an unrecognized arg key that looks like a match.
	expected, ok := canonicalKeys[toolName]
	if !ok {
		return
	}
	for _, canonical := range expected {
		if _, present := args[canonical]; present {
			continue // already resolved
		}
		// Find best candidate among remaining args
		for key, val := range args {
			if isKnownParam(key) {
				continue // skip standard params like "format", "timeout_seconds", etc.
			}
			if _, isStr := val.(string); !isStr {
				continue
			}
			// Heuristic: key contains part of canonical or vice versa
			if fuzzyKeyMatch(key, canonical) {
				args[canonical] = val
				delete(args, key)
				fmt.Printf("  [fix] Fuzzy alias: %q → %q\n", key, canonical)
				break
			}
		}
	}
}

// fuzzyKeyMatch returns true if key is likely an alias for canonical.
func fuzzyKeyMatch(key, canonical string) bool {
	k := strings.ToLower(strings.ReplaceAll(key, "_", ""))
	c := strings.ToLower(strings.ReplaceAll(canonical, "_", ""))
	// Direct substring match
	if strings.Contains(k, c) || strings.Contains(c, k) {
		return true
	}
	// Partial word match: "file"↔"filename", "tql"↔"tql_content", "sql"↔"sql_query"
	parts := strings.FieldsFunc(canonical, func(r rune) bool { return r == '_' })
	for _, part := range parts {
		if strings.Contains(k, strings.ToLower(part)) {
			return true
		}
	}
	return false
}

// isKnownParam returns true for standard parameter names that should not be remapped.
var knownParams = map[string]bool{
	"format": true, "timeformat": true, "timezone": true,
	"timeout_seconds": true, "limit": true,
	"section_filter": true, "language": true,
	"host": true, "port": true, "user": true,
	"time_start": true, "time_end": true, "refresh": true,
	"charts": true, "chart_title": true, "chart_type": true,
	"table": true, "tag": true, "column": true, "color": true,
	"tql_path": true, "user_name": true,
	"x": true, "y": true, "w": true, "h": true,
	"smooth": true, "area_style": true, "is_stack": true,
	"panel_id": true, "panel_title": true,
	"new_title": true, "new_chart_type": true, "new_table": true,
	"new_tag": true, "new_column": true, "new_color": true,
	"title": true, "parent": true,
	"auto_fix": true, "add_validation_script": true,
}

func isKnownParam(key string) bool {
	return knownParams[key]
}
