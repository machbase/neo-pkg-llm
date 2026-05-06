package fixer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"neo-pkg-llm/llm"
)

const DtFormat = "2006-01-02 15:04:05"

// FixerContext holds state accumulated during the agent loop for fixers to use.
type FixerContext struct {
	TimeStartDt string   // parsed time range start (datetime string)
	TimeEndDt   string   // parsed time range end (datetime string)
	DataMinDt   string   // captured MIN(TIME) from SQL results
	DataMaxDt   string   // captured MAX(TIME) from SQL results
	KnownTags   []string // captured tag names from list_table_tags

	// InferTableName is a callback to infer the table name from messages.
	InferTableName func() string
}

// TqlFuncRE matches TQL function calls that need line breaks inserted.
var TqlFuncRE = regexp.MustCompile(`\)[ \t]*(SQL_SELECT|SQL|SCRIPT|CHART_LINE|CHART_BAR3D|CHART|MAPVALUE|POPVALUE|MAPKEY|GROUPBYKEY|FFT|FLATTEN|PUSHKEY|CSV)\(`)

// TemplateRefRE matches TEMPLATE: references in TQL content (just detects the pattern).
var TemplateRefRE = regexp.MustCompile(`TEMPLATE:\s*(\d+-\d+)`)

// Individual param extractors (order-independent)
var templateTableRE = regexp.MustCompile(`TABLE:\s*(\S+)`)
var templateTagRE = regexp.MustCompile(`(?:^|\s)TAG:\s*(\S+)`)
var templateTag1RE = regexp.MustCompile(`TAG1:\s*(\S+)`)
var templateTag2RE = regexp.MustCompile(`TAG2:\s*(\S+)`)
var templateUnitRE = regexp.MustCompile(`UNIT:\s*(\S+)`)

// TemplateIDRE matches template IDs like "1-1", "2_3".
var TemplateIDRE = regexp.MustCompile(`(\d+[-_]\d+)`)

// DashboardTools is the set of dashboard-related tool names.
var DashboardTools = map[string]bool{
	"create_dashboard":             true,
	"create_dashboard_with_charts": true,
	"add_chart_to_dashboard":       true,
	"remove_chart_from_dashboard":  true,
	"update_chart_in_dashboard":    true,
	"delete_dashboard":             true,
	"update_dashboard_time_range":  true,
	"preview_dashboard":            true,
	"get_dashboard":                true,
	"save_html_report":             true,
}

// ExpandTemplateFunc is a callback to expand TQL templates (avoids circular dep with agent/templates.go).
var ExpandTemplateFunc func(templateID string, params map[string]string) (string, error)

// Fix applies all fixers to a message's tool calls. This is the main orchestrator.
func Fix(msg llm.Message, fctx *FixerContext) llm.Message {
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

		// Step 1: Normalize parameter aliases
		NormalizeArgs(name, args)

		// Step 2: list_table_tags table_name inference
		if name == "list_table_tags" {
			table, _ := args["table_name"].(string)
			if table == "" && fctx.InferTableName != nil {
				if inferred := fctx.InferTableName(); inferred != "" {
					args["table_name"] = inferred
					fmt.Printf("  [fix] list_table_tags table_name 자동 삽입: %s\n", inferred)
				}
			}
		}

		// Step 3: save_html_report time injection
		if name == "save_html_report" && fctx.TimeStartDt != "" {
			args["time_start"] = fctx.TimeStartDt
			args["time_end"] = fctx.TimeEndDt
			fmt.Printf("  [fix] save_html_report time injected: %s ~ %s\n", fctx.TimeStartDt, fctx.TimeEndDt)
		}

		// Step 4: time_start/time_end float → string
		fixTimeFloats(args)

		// Step 5: Literal \n → real newline
		fixEscapedNewlines(args)

		// Step 6: charts normalization
		fixCharts(args, fctx)

		// Step 7: time_start/time_end normalize to epoch ms
		fixTimeValues(args)

		// Step 8: save_tql_file / delete_file folder merge
		fixFolderMerge(name, args)

		// Step 9: Dashboard filename/title auto-fix
		fixDashboardFilename(name, args)
		fixDashboardTitle(name, args)

		// Step 10: TQL content fixes (template expansion, line breaks)
		fixTQLContent(name, args, fctx)
	}
	return msg
}

// FixDashboardTime applies dashboard time correction before tool execution.
func FixDashboardTime(tc *llm.ToolCall, fctx *FixerContext) {
	if !DashboardTools[tc.Function.Name] {
		return
	}

	existingStart, hasStart := tc.Function.Arguments["time_start"]
	existingEnd, hasEnd := tc.Function.Arguments["time_end"]
	_, startIsStr := existingStart.(string)
	_, endIsStr := existingEnd.(string)
	needsStartFix := !hasStart || !startIsStr || existingStart.(string) == ""
	needsEndFix := !hasEnd || !endIsStr || existingEnd.(string) == ""

	startDt, endDt := fctx.TimeStartDt, fctx.TimeEndDt
	if startDt == "" && fctx.DataMinDt != "" {
		startDt = fctx.DataMinDt
	}
	if endDt == "" && fctx.DataMaxDt != "" {
		endDt = fctx.DataMaxDt
	}
	if startDt != "" && endDt != "" {
		if needsStartFix {
			if startTime, err := time.ParseInLocation(DtFormat, startDt, time.Local); err == nil {
				tc.Function.Arguments["time_start"] = strconv.FormatInt(startTime.UnixMilli(), 10)
			}
		}
		if needsEndFix {
			if endTime, err := time.ParseInLocation(DtFormat, endDt, time.Local); err == nil {
				tc.Function.Arguments["time_end"] = strconv.FormatInt(endTime.UnixMilli(), 10)
			}
		}
		if needsStartFix || needsEndFix {
			fmt.Printf("  [fix] dashboard time → %s ~ %s\n", startDt, endDt)
		}
	}
}

// FixTQLTimeRange replaces TO_DATE time ranges in save_tql_file calls.
func FixTQLTimeRange(tc *llm.ToolCall, fctx *FixerContext) {
	if tc.Function.Name != "save_tql_file" || fctx.TimeStartDt == "" || fctx.TimeEndDt == "" {
		return
	}
	content, ok := tc.Function.Arguments["tql_content"].(string)
	if !ok {
		return
	}
	toDateRE := regexp.MustCompile(`TIME\s+BETWEEN\s+TO_DATE\('([^']+)'\)\s+AND\s+TO_DATE\('([^']+)'\)`)
	m := toDateRE.FindStringSubmatch(content)
	if m == nil {
		return
	}
	if m[1] != fctx.TimeStartDt || m[2] != fctx.TimeEndDt {
		replacement := fmt.Sprintf("TIME BETWEEN TO_DATE('%s') AND TO_DATE('%s')", fctx.TimeStartDt, fctx.TimeEndDt)
		tc.Function.Arguments["tql_content"] = strings.Replace(content, m[0], replacement, 1)
		fmt.Printf("  [fix] TO_DATE: %s~%s → %s~%s\n", m[1], m[2], fctx.TimeStartDt, fctx.TimeEndDt)
	}
}

// CaptureResults runs post-execution capture logic.
func CaptureResults(tc llm.ToolCall, result string, err error, fctx *FixerContext) {
	if err != nil {
		return
	}
	if tc.Function.Name == "list_table_tags" {
		CaptureKnownTags(result, fctx)
	}
	if tc.Function.Name == "execute_sql_query" {
		CaptureDataTimeRange(tc.Function.Arguments, result, fctx)
	}
}

// ValidateTagInArgs checks if TAG parameters in TQL-related tool calls use valid tag names.
// Returns error string if invalid, empty string if OK.
func ValidateTagInArgs(toolName string, args map[string]any, knownTags []string) string {
	if len(knownTags) == 0 {
		return ""
	}

	if toolName != "save_tql_file" && toolName != "execute_tql_script" && toolName != "validate_chart_tql" {
		return ""
	}

	tql, _ := args["tql_content"].(string)
	if tql == "" {
		tql, _ = args["tql_script"].(string)
	}
	if tql == "" {
		return ""
	}

	// Detect unsubstituted placeholders
	placeholderRE := regexp.MustCompile(`\{(TAG\d?|TABLE|UNIT)\}`)
	if found := placeholderRE.FindAllString(tql, -1); len(found) > 0 {
		return fmt.Sprintf(
			"Error: 플레이스홀더 %v가 치환되지 않았습니다. "+
				"tql_content에 raw TQL을 직접 쓰지 마세요! "+
				"반드시 TEMPLATE:ID TABLE:테이블 TAG:태그 UNIT:단위 형식을 사용하세요. "+
				"예: TEMPLATE:3-2 TABLE:STAT TAG1:machbase:http:latency TAG2:machbase:ps:cpu_percent",
			found)
	}

	nameRE := regexp.MustCompile(`NAME\s*=\s*'([^']+)'`)
	matches := nameRE.FindAllStringSubmatch(tql, -1)

	tagSet := make(map[string]bool)
	for _, t := range knownTags {
		tagSet[t] = true
	}

	var invalidTags []string
	for _, m := range matches {
		if len(m) > 1 && !tagSet[m[1]] {
			invalidTags = append(invalidTags, m[1])
		}
	}

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
		invalidTags, knownTags,
	)
}

// InferTableName scans messages to find a table name.
func InferTableName(messages []llm.Message) string {
	var knownTables []string
	for i, m := range messages {
		if m.Role == "tool" && i > 0 {
			prev := messages[i-1]
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

	var searchText string
	for _, m := range messages {
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

// --- internal helpers ---

func fixTimeFloats(args map[string]any) {
	for _, tk := range []string{"time_start", "time_end"} {
		if v, ok := args[tk]; ok {
			switch n := v.(type) {
			case float64:
				args[tk] = strconv.FormatInt(int64(n), 10)
			}
		}
	}
}

func fixEscapedNewlines(args map[string]any) {
	for k, v := range args {
		if s, ok := v.(string); ok && strings.Contains(s, "\\n") {
			args[k] = strings.ReplaceAll(s, "\\n", "\n")
		}
	}
}

func fixCharts(args map[string]any, fctx *FixerContext) {
	if charts, ok := args["charts"]; ok {
		switch c := charts.(type) {
		case []any, map[string]any:
			data, _ := jsonMarshal(c)
			args["charts"] = string(data)
		case string:
			if strings.Contains(c, "'") && !strings.Contains(c, "\"") {
				args["charts"] = strings.ReplaceAll(c, "'", "\"")
				fmt.Printf("  [fix] charts single quotes → double quotes\n")
			}
		}
	}

	// charts: table field missing → auto insert
	if chartsStr, ok := args["charts"].(string); ok && chartsStr != "" && fctx.InferTableName != nil {
		if inferred := fctx.InferTableName(); inferred != "" {
			var chartList []map[string]any
			if jsonUnmarshal([]byte(chartsStr), &chartList) == nil {
				fixed := false
				for i := range chartList {
					t, _ := chartList[i]["table"].(string)
					if t == "" {
						chartList[i]["table"] = inferred
						fixed = true
					}
				}
				if fixed {
					data, _ := jsonMarshal(chartList)
					args["charts"] = string(data)
					fmt.Printf("  [fix] charts table 자동 삽입: %s\n", inferred)
				}
			}
		}
	}
}

func fixTimeValues(args map[string]any) {
	for _, key := range []string{"time_start", "time_end"} {
		if v, ok := args[key].(string); ok {
			if len(v) > 15 && isAllDigits(v) {
				args[key] = v[:len(v)-6]
			} else if !isAllDigits(v) {
				if ms, ok := ParseTimeValue(v); ok {
					args[key] = strconv.FormatInt(ms, 10)
					fmt.Printf("  [fix] %s: %s → %d\n", key, v, ms)
				}
			}
		}
	}
}

func fixFolderMerge(name string, args map[string]any) {
	if name == "save_tql_file" || name == "delete_file" {
		fn, _ := args["filename"].(string)
		folder, _ := args["folder_name"].(string)
		if fn != "" && folder != "" && !strings.Contains(fn, "/") {
			args["filename"] = folder + "/" + fn
			delete(args, "folder_name")
			fmt.Printf("  [fix] Merged folder into filename: %s\n", args["filename"])
		}
	}
}

func fixDashboardFilename(name string, args map[string]any) {
	if !DashboardTools[name] {
		return
	}
	fn, _ := args["filename"].(string)
	if fn == "" {
		return
	}
	if !strings.HasSuffix(strings.ToLower(fn), ".dsh") {
		fn = fn + ".dsh"
	}
	if !strings.Contains(fn, "/") {
		base := strings.TrimSuffix(fn, ".dsh")
		parts := strings.SplitN(base, "_", 2)
		folder := strings.ToUpper(parts[0])
		fn = folder + "/" + fn
	}

	// create_dashboard / create_dashboard_with_charts: add timestamp like report
	if name == "create_dashboard" || name == "create_dashboard_with_charts" {
		ts := time.Now().Format("20060102_150405")
		base := strings.TrimSuffix(fn, ".dsh")
		// Avoid double timestamp (if already contains one like _20260428_)
		if !regexp.MustCompile(`_\d{8}_\d{6}$`).MatchString(base) {
			fn = base + "_" + ts + ".dsh"
		}
	}

	args["filename"] = fn
	fmt.Printf("  [fix] Dashboard filename → %s\n", fn)
}

func fixDashboardTitle(name string, args map[string]any) {
	if name != "create_dashboard" && name != "create_dashboard_with_charts" {
		return
	}
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

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
