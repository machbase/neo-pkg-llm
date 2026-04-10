package tools

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"math/cmplx"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// argAnyToString converts any value to string.
func argAnyToString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int:
		return fmt.Sprintf("%d", val)
	case bool:
		return fmt.Sprintf("%v", val)
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(data)
	}
}

func argAnyStr(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		return argAnyToString(v)
	}
	return ""
}

func (r *Registry) registerReportTools() {
	LoadReportTemplates()

	r.register(&Tool{
		Name: "save_html_report",
		Description: `HTML 분석 리포트를 생성하여 저장합니다.

** 통계/차트 데이터는 도구가 자동 SQL 조회합니다! LLM이 직접 조회할 필요 없음! **

** 이 도구 호출 전에 실행할 것 **
1. list_tables → 테이블 확인
2. list_table_tags → 태그 확인
3. execute_sql_query: SELECT NAME, COUNT(*) as cnt, ROUND(AVG(VALUE),2) as avg, ROUND(MIN(VALUE),2) as min, ROUND(MAX(VALUE),2) as max FROM 테이블 GROUP BY NAME
   ※ 시간 제한 필요시 WHERE 추가하지 마세요! 도구가 time_start/time_end로 자동 필터링합니다.

** LLM이 전달할 파라미터 **
- table: 테이블명 (필수!)
- analysis: 3번 SQL 통계를 해석한 심층 분석 (한국어, **소제목** 포함 5~7문단)
- recommendations: 종합 소견 및 권고 (한국어, 5개 이상 번호 항목)
template_id 선택 기준 (데이터 성격에 맞게):
- 'R-2': 진동 분석 (가속도, 속도, 변위 — 센서 진동 데이터)
- 'R-1': 금융 (OHLC, 환율, 원자재 — close/open/high/low/volume 등)
- 'R-0': 범용 (위에 해당하지 않는 모든 데이터)`,
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"template_id": {
					Type:        "string",
					Description: "템플릿 ID. 진동: 'R-2', 금융: 'R-1', 범용: 'R-0' (기본값)",
				},
				"table": {
					Type:        "string",
					Description: "테이블명 (필수! 예: 'GOLD', 'SILVER')",
				},
				"tag_count": {
					Type:        "string",
					Description: "태그 수 (예: '5')",
				},
				"data_count": {
					Type:        "string",
					Description: "총 데이터 건수 (예: '68530')",
				},
				"time_range": {
					Type:        "string",
					Description: "시간 범위 (예: '2023-09-20 ~ 2025-12-13')",
				},
				"analysis": {
					Type:        "string",
					Description: "심층 분석 (한국어). **소제목** 또는 ## 소제목 포함, 5~7문단",
				},
				"recommendations": {
					Type:        "string",
					Description: "종합 소견 및 권고 (한국어). 5개 이상 번호 항목!",
				},
				"rollup_unit": {
					Type:        "string",
					Description: "ROLLUP 시간 단위. 기본값: 자동(시간 범위 기반). 사용자가 단위를 지정하면 그 값 사용. sec/min/hour/day/week/month",
					Enum:        []string{"sec", "min", "hour", "day", "week", "month"},
				},
			},
			Required: []string{"table", "analysis", "recommendations"},
		},
		Fn: r.saveHtmlReport,
	})
}

func (r *Registry) saveHtmlReport(args map[string]any) (string, error) {
	// Normalize args keys to lowercase
	normalizedArgs := make(map[string]any, len(args))
	for k, v := range args {
		normalizedArgs[strings.ToLower(k)] = v
	}

	fmt.Printf("[Report] Received args keys: %v\n", func() []string {
		keys := make([]string, 0, len(args))
		for k := range args {
			keys = append(keys, k)
		}
		return keys
	}())

	// Extract table name
	tableName := ""
	for _, k := range []string{"table", "table_name", "tablename", "name"} {
		tableName = argAnyStr(normalizedArgs, k)
		if tableName != "" {
			break
		}
	}
	if tableName == "" {
		// Scan all string values for ALL CAPS value
		for _, v := range normalizedArgs {
			if s, ok := v.(string); ok && len(s) >= 2 && len(s) <= 30 && s == strings.ToUpper(s) && !strings.Contains(s, " ") {
				tableName = s
				break
			}
		}
	}
	if tableName == "" {
		return "table 파라미터가 필요합니다. 예: table=\"GOLD\"", nil
	}
	tableName = strings.ToUpper(tableName)

	templateID := argAnyStr(normalizedArgs, "template_id")
	if templateID == "" {
		templateID = argAnyStr(normalizedArgs, "templateid")
	}
	if templateID == "" {
		templateID = "R-0"
	}

	filename := argAnyStr(normalizedArgs, "filename")
	if filename == "" {
		filename = tableName + "/" + tableName + "_Analysis_Report.html"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".html") {
		filename += ".html"
	}
	if !strings.Contains(filename, "/") {
		filename = tableName + "/" + filename
	}

	loc := time.Local
	params := map[string]string{
		"GENERATED_DATE": time.Now().In(loc).Format("2006-01-02 15:04:05 (MST)"),
		"TABLE":          tableName,
	}

	// --- Build time filter clause ---
	timeWhere := ""
	timeStart := argAnyStr(normalizedArgs, "time_start")
	if timeStart == "" {
		timeStart = argAnyStr(normalizedArgs, "timestart")
	}
	timeEnd := argAnyStr(normalizedArgs, "time_end")
	if timeEnd == "" {
		timeEnd = argAnyStr(normalizedArgs, "timeend")
	}
	if timeStart != "" && timeEnd != "" {
		// Convert epoch milliseconds (13 digits) to nanoseconds (19 digits) for Machbase
		tsNano := msToNano(timeStart)
		teNano := msToNano(timeEnd)
		timeWhere = fmt.Sprintf(" AND TIME BETWEEN %s AND %s", tsNano, teNano)
		fmt.Printf("[Report] Time filter: %s ~ %s (ms: %s ~ %s)\n", tsNano, teNano, timeStart, timeEnd)
	}
	timeWhereBase := ""
	if timeWhere != "" {
		timeWhereBase = " WHERE" + timeWhere[4:] // strip leading " AND"
	}

	// --- Server-side SQL queries ---
	fmt.Printf("[Report] Fetching data for table: %s\n", tableName)

	// 1. Tag stats
	statsSQL := fmt.Sprintf("SELECT NAME, COUNT(*) as cnt, ROUND(AVG(VALUE),2) as avg, ROUND(MIN(VALUE),2) as min, ROUND(MAX(VALUE),2) as max FROM %s%s GROUP BY NAME", tableName, timeWhereBase)
	statsCSV, err := r.client.QuerySQL(statsSQL, "", "", "csv")
	if err == nil {
		rows, chartItems := parseStatsCSV(statsCSV)
		if len(rows) > 0 {
			params["TAG_STATS_ROWS"] = strings.Join(rows, "\n")
			params["TAG_COUNT"] = fmt.Sprintf("%d", len(rows))
			chartJSON, _ := json.Marshal(chartItems)
			params["CHART_DATA_JSON"] = string(chartJSON)
			fmt.Printf("[Report] Fetched %d tag stats\n", len(rows))
		}
	} else {
		fmt.Printf("[Report] Stats query failed: %v\n", err)
	}

	// 2. Time range
	timeSQL := fmt.Sprintf("SELECT MIN(TIME), MAX(TIME) FROM %s%s", tableName, timeWhereBase)
	timeCSV, err := r.client.QuerySQL(timeSQL, "Default", "", "csv")
	if err == nil {
		if tr := parseTimeRangeCSV(timeCSV); tr != "" {
			params["TIME_RANGE"] = convertTimeRangeToLocal(tr, loc)
		}
	}

	// 3. Get tag list
	tagListSQL := fmt.Sprintf("SELECT NAME FROM %s%s GROUP BY NAME", tableName, timeWhereBase)
	tagCSV, _ := r.client.QuerySQL(tagListSQL, "", "", "csv")
	tags := parseTagList(tagCSV)

	// 4. Pick ROLLUP unit (user-specified or auto)
	rollupUnit := argAnyStr(normalizedArgs, "rollup_unit")
	if rollupUnit == "" {
		rollupUnit = pickRollupUnit(timeStart, timeEnd)
	}
	fmt.Printf("[Report] ROLLUP unit: %s\n", rollupUnit)
	rollupLabels := map[string]string{"sec": "초별", "min": "분별", "hour": "시간별", "day": "일별", "week": "주별", "month": "월별"}
	params["ROLLUP_LABEL"] = rollupLabels[rollupUnit]

	if templateID == "R-2" {
		// --- R-2: Vibration-specific per-tag data ---
		tagListJSON, _ := json.Marshal(tags)
		params["TAG_LIST_JSON"] = string(tagListJSON)

		maxVibTags := 10
		vibTags := tags
		if len(vibTags) > maxVibTags {
			vibTags = vibTags[:maxVibTags]
		}

		perTagData := map[string]interface{}{}
		for _, tag := range vibTags {
			tagData := map[string]interface{}{}

			// ROLLUP trend with SUMSQ for RMS/P2P/Crest
			rollupSQL := fmt.Sprintf(
				"SELECT ROLLUP('%s',1,TIME) as t, ROUND(AVG(VALUE),6) as avg_val, "+
					"ROUND(MIN(VALUE),6) as min_val, ROUND(MAX(VALUE),6) as max_val, "+
					"SUMSQ(VALUE) as sumsq, COUNT(VALUE) as cnt "+
					"FROM %s WHERE NAME='%s'%s "+
					"GROUP BY ROLLUP('%s',1,TIME) ORDER BY t",
				rollupUnit, tableName, tag, timeWhere, rollupUnit)
			rollupCSV, err := r.client.QuerySQL(rollupSQL, "Default", "", "csv")
			if err == nil {
				tagData["rollup"] = parseVibRollupCSV(rollupCSV, rollupUnit)
			} else {
				fmt.Printf("[Report] Vibration rollup query failed for tag %s: %v\n", tag, err)
			}

			// Raw waveform (4096 points for chart display)
			rawSQL := fmt.Sprintf(
				"SELECT TIME, VALUE FROM %s WHERE NAME='%s'%s ORDER BY TIME LIMIT 4096",
				tableName, tag, timeWhere)
			rawCSV, err := r.client.QuerySQL(rawSQL, "", "", "csv")
			if err == nil {
				tagData["raw"] = parseVibRawCSV(rawCSV)
			} else {
				fmt.Printf("[Report] Vibration raw query failed for tag %s: %v\n", tag, err)
			}

			// FFT: fetch ALL raw data in range, compute server-side, send spectrum only
			fftSQL := fmt.Sprintf(
				"SELECT TIME, VALUE FROM %s WHERE NAME='%s'%s ORDER BY TIME",
				tableName, tag, timeWhere)
			fftCSV, err := r.client.QuerySQL(fftSQL, "", "", "csv")
			if err == nil {
				if spectrum := computeFFTSpectrum(fftCSV, 4096); spectrum != nil {
					tagData["fft"] = spectrum
					fmt.Printf("[Report] FFT computed for tag %s: %d points → %d bins\n",
						tag, spectrum["total_points"], len(spectrum["mags"].([]float64)))
				}
			} else {
				fmt.Printf("[Report] FFT raw query failed for tag %s: %v\n", tag, err)
			}

			// Per-tag summary stats
			tagData["stats"] = computeVibStats(tagData)

			perTagData[tag] = tagData
		}

		perTagJSON, _ := json.Marshal(perTagData)
		params["PER_TAG_DATA_JSON"] = string(perTagJSON)
		fmt.Printf("[Report] Fetched vibration data for %d tags\n", len(vibTags))
	} else {
		// --- R-0/R-1: primary + volume trend ---
		primaryTag, volumeTag := pickTrendTags(tags)
		fmt.Printf("[Report] Tags: %v → primary=%q, volume=%q\n", tags, primaryTag, volumeTag)

		closeTrend := []map[string]interface{}{}
		if primaryTag != "" {
			trendSQL := fmt.Sprintf("SELECT ROLLUP('%s',1,TIME) as t, ROUND(AVG(VALUE),2) as v FROM %s WHERE NAME='%s'%s GROUP BY ROLLUP('%s',1,TIME) ORDER BY t",
				rollupUnit, tableName, primaryTag, timeWhere, rollupUnit)
			trendCSV, err := r.client.QuerySQL(trendSQL, "Default", "", "csv")
			if err == nil {
				closeTrend = parseTrendCSV(trendCSV, "close", rollupUnit)
			}
		}

		volTrend := []map[string]interface{}{}
		if volumeTag != "" {
			volSQL := fmt.Sprintf("SELECT ROLLUP('%s',1,TIME) as t, ROUND(AVG(VALUE),0) as v FROM %s WHERE NAME='%s'%s GROUP BY ROLLUP('%s',1,TIME) ORDER BY t",
				rollupUnit, tableName, volumeTag, timeWhere, rollupUnit)
			volCSV, err := r.client.QuerySQL(volSQL, "Default", "", "csv")
			if err == nil {
				volTrend = parseTrendCSV(volCSV, "volume", rollupUnit)
			}
		}

		trendData := mergeTrend(closeTrend, volTrend)
		if len(trendData) > 0 {
			trendJSON, _ := json.Marshal(trendData)
			params["TREND_DATA_JSON"] = string(trendJSON)
			fmt.Printf("[Report] Fetched %d trend data points\n", len(trendData))
		}
	}

	// --- LLM-provided params ---
	if v := argAnyStr(normalizedArgs, "tag_count"); v != "" {
		params["TAG_COUNT"] = v
	}
	if v := argAnyStr(normalizedArgs, "data_count"); v != "" {
		params["DATA_COUNT"] = v
	}
	if v := argAnyStr(normalizedArgs, "time_range"); v != "" {
		params["TIME_RANGE"] = convertTimeRangeToLocal(v, loc)
	}
	if v := argAnyStr(normalizedArgs, "analysis"); v != "" {
		params["ANALYSIS"] = mdToHTML(v)
	}
	if v := argAnyStr(normalizedArgs, "recommendations"); v != "" {
		params["RECOMMENDATIONS"] = mdToHTML(v)
	}

	// Calculate data_count from stats if not provided
	if _, ok := params["DATA_COUNT"]; !ok {
		if statsCSV != "" {
			total := calcTotalCount(statsCSV)
			if total > 0 {
				params["DATA_COUNT"] = fmt.Sprintf("%d", total)
			}
		}
	}

	// If analysis is missing, return filtered stats so LLM can write proper analysis
	hasAnalysis := params["ANALYSIS"] != ""
	hasReco := params["RECOMMENDATIONS"] != ""
	if !hasAnalysis || !hasReco {
		// Build stats summary from the filtered data
		var summary strings.Builder
		summary.WriteString(fmt.Sprintf("테이블: %s\n", tableName))
		if tr, ok := params["TIME_RANGE"]; ok {
			summary.WriteString(fmt.Sprintf("조회 기간: %s\n", tr))
		}
		if dc, ok := params["DATA_COUNT"]; ok {
			summary.WriteString(fmt.Sprintf("총 데이터 건수: %s\n", dc))
		}
		if statsCSV != "" {
			summary.WriteString(fmt.Sprintf("태그별 통계 (필터 적용됨):\n%s\n", statsCSV))
		}
		msg := fmt.Sprintf("데이터를 조회했습니다. 아래 **필터된 통계**를 기반으로 analysis와 recommendations를 작성하여 다시 호출하세요.\n\n%s", summary.String())
		if !hasAnalysis {
			msg += "\n※ analysis 파라미터가 누락되었습니다."
		}
		if !hasReco {
			msg += "\n※ recommendations 파라미터가 누락되었습니다."
		}
		return msg, nil
	}

	// Expand template
	html, err := ExpandReportTemplate(templateID, params)
	if err != nil {
		return fmt.Sprintf("Template error: %v", err), nil
	}

	// Create folder + save
	if idx := strings.Index(filename, "/"); idx > 0 {
		_ = r.client.CreateFolder(filename[:idx])
	}
	if err := r.client.WriteFile(filename, []byte(html)); err != nil {
		return fmt.Sprintf("File save failed: %v", err), nil
	}

	reportURL := r.client.BaseURL + "/db/tql/" + filename
	return fmt.Sprintf("Report saved: %s\n[리포트 열기](%s)", filename, reportURL), nil
}

// --- CSV parsing helpers ---

func parseStatsCSV(csvData string) ([]string, []map[string]interface{}) {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return nil, nil
	}
	var rows []string
	var items []map[string]interface{}
	for _, rec := range records[1:] { // skip header
		if len(rec) < 5 {
			continue
		}
		name := rec[0]
		cnt := roundCSVNum(rec[1], 0)
		avg := roundCSVNum(rec[2], 2)
		min := roundCSVNum(rec[3], 2)
		max := roundCSVNum(rec[4], 2)
		rows = append(rows, fmt.Sprintf(
			`<tr><td>%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td></tr>`,
			name, cnt, avg, min, max))
		items = append(items, map[string]interface{}{
			"name": name, "count": cnt, "avg": avg, "min": min, "max": max,
		})
	}
	return rows, items
}

func parseTimeRangeCSV(csvData string) string {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	if len(records) < 2 || len(records[1]) < 2 {
		return ""
	}
	minT := strings.TrimSpace(records[1][0])
	maxT := strings.TrimSpace(records[1][1])
	// If same date, keep time portion (HH:MM:SS); otherwise truncate to date
	minDate := minT
	maxDate := maxT
	if len(minDate) > 10 {
		minDate = minDate[:10]
	}
	if len(maxDate) > 10 {
		maxDate = maxDate[:10]
	}
	if minDate == maxDate {
		// Same day — show with time
		if len(minT) > 19 {
			minT = minT[:19]
		}
		if len(maxT) > 19 {
			maxT = maxT[:19]
		}
	} else {
		minT = minDate
		maxT = maxDate
	}
	return minT + " ~ " + maxT
}

func parseTrendCSV(csvData string, valueKey string, rollupUnit string) []map[string]interface{} {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	if len(records) < 2 {
		return nil
	}
	// Determine time label length based on rollup unit
	trimLen := 7 // "YYYY-MM" for month
	switch rollupUnit {
	case "sec":
		trimLen = 19 // "YYYY-MM-DD HH:MM:SS"
	case "min":
		trimLen = 16 // "YYYY-MM-DD HH:MM"
	case "hour":
		trimLen = 13 // "YYYY-MM-DD HH"
	case "day", "week":
		trimLen = 10 // "YYYY-MM-DD"
	}
	var items []map[string]interface{}
	for _, rec := range records[1:] {
		if len(rec) < 2 {
			continue
		}
		t := strings.TrimSpace(rec[0])
		if len(t) > trimLen {
			t = t[:trimLen]
		}
		items = append(items, map[string]interface{}{
			"time":   t,
			valueKey: rec[1],
		})
	}
	return items
}

func mergeTrend(closeTrend, volTrend []map[string]interface{}) []map[string]interface{} {
	volMap := map[string]string{}
	for _, v := range volTrend {
		t, _ := v["time"].(string)
		vol, _ := v["volume"].(string)
		volMap[t] = vol
	}
	var result []map[string]interface{}
	for _, c := range closeTrend {
		t, _ := c["time"].(string)
		item := map[string]interface{}{
			"time":  t,
			"close": c["close"],
		}
		if vol, ok := volMap[t]; ok {
			item["volume"] = vol
		}
		result = append(result, item)
	}
	return result
}

// pickRollupUnit selects ROLLUP time unit based on the time range span.
// msToNano converts epoch milliseconds string to nanoseconds string.
// Machbase TIME column stores nanoseconds (19 digits), agent provides milliseconds (13 digits).
func msToNano(ms string) string {
	var v int64
	fmt.Sscanf(ms, "%d", &v)
	if v == 0 {
		return ms
	}
	// If already nanoseconds (>= 16 digits), return as-is
	if v > 1e15 {
		return ms
	}
	return fmt.Sprintf("%d", v*1000000)
}

func pickRollupUnit(startMs, endMs string) string {
	if startMs == "" || endMs == "" {
		return "month"
	}
	var s, e int64
	fmt.Sscanf(startMs, "%d", &s)
	fmt.Sscanf(endMs, "%d", &e)
	if s == 0 || e == 0 {
		return "month"
	}
	diffHours := (e - s) / 1000 / 3600
	switch {
	case diffHours < 1: // < 1 hour
		return "sec"
	case diffHours < 48: // < 2 days
		return "min"
	case diffHours < 720: // < 30 days
		return "hour"
	case diffHours < 8760: // < 1 year
		return "day"
	default:
		return "month"
	}
}

// roundCSVNum formats a CSV number string to the given decimal places.
func roundCSVNum(s string, decimals int) string {
	s = strings.TrimSpace(s)
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return s
	}
	if decimals == 0 {
		return fmt.Sprintf("%d", int64(f+0.5))
	}
	return fmt.Sprintf("%.*f", decimals, f)
}

func calcTotalCount(csvData string) int {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	if len(records) < 2 {
		return 0
	}
	total := 0
	for _, rec := range records[1:] {
		if len(rec) >= 2 {
			var cnt int
			fmt.Sscanf(rec[1], "%d", &cnt)
			total += cnt
		}
	}
	return total
}

func parseTagList(csvData string) []string {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	var tags []string
	for _, rec := range records[1:] { // skip header
		if len(rec) > 0 && rec[0] != "" {
			tags = append(tags, strings.TrimSpace(rec[0]))
		}
	}
	return tags
}

// pickTrendTags selects which tags to use for price trend and volume chart.
// Priority for primary: close > value > first non-volume tag
// Priority for volume: volume > vol > count > (empty if none found)
func pickTrendTags(tags []string) (primary string, volume string) {
	lower := map[string]string{} // lowercase → original
	for _, t := range tags {
		lower[strings.ToLower(t)] = t
	}

	// Primary tag (price/value)
	for _, candidate := range []string{"close", "value", "avg", "price", "temp", "temperature"} {
		if orig, ok := lower[candidate]; ok {
			primary = orig
			break
		}
	}
	if primary == "" && len(tags) > 0 {
		// Pick first tag that isn't volume-like
		for _, t := range tags {
			tl := strings.ToLower(t)
			if tl != "volume" && tl != "vol" && tl != "count" {
				primary = t
				break
			}
		}
		if primary == "" {
			primary = tags[0]
		}
	}

	// Volume tag
	for _, candidate := range []string{"volume", "vol", "count", "qty", "quantity"} {
		if orig, ok := lower[candidate]; ok {
			volume = orig
			break
		}
	}

	return
}

// --- Time + Markdown helpers ---

func convertTimeRangeToLocal(s string, loc *time.Location) string {
	parts := strings.Split(s, "~")
	if len(parts) != 2 {
		return s
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var parsed []time.Time
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		converted := false
		for _, fmt := range formats {
			if t, err := time.Parse(fmt, part); err == nil {
				parsed = append(parsed, t.In(loc))
				converted = true
				break
			}
		}
		if !converted {
			result = append(result, part)
			parsed = append(parsed, time.Time{})
		}
	}
	// If same date, show with time; otherwise date only
	outFmt := "2006-01-02"
	if len(parsed) == 2 && !parsed[0].IsZero() && !parsed[1].IsZero() {
		if parsed[0].Format("2006-01-02") == parsed[1].Format("2006-01-02") {
			outFmt = "2006-01-02 15:04:05"
		}
	}
	result = nil
	for _, t := range parsed {
		if t.IsZero() {
			result = append(result, "?")
		} else {
			result = append(result, t.Format(outFmt))
		}
	}
	return strings.Join(result, " ~ ")
}

var (
	mdBoldRE       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	circledNumRE   = regexp.MustCompile(`([①②③④⑤⑥⑦⑧⑨⑩])`)
	numParenInline = regexp.MustCompile(`\s+(\d+)\)\s+`)
	boldHeadingRE  = regexp.MustCompile(`(?m)^\*\*(.+?)\*\*\s+(.+)$`)
)

// --- Vibration CSV parsing helpers ---

func parseVibRollupCSV(csvData string, rollupUnit string) []map[string]interface{} {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	if len(records) < 2 {
		return nil
	}
	trimLen := 7
	switch rollupUnit {
	case "sec":
		trimLen = 19 // "YYYY-MM-DD HH:MM:SS"
	case "min":
		trimLen = 16 // "YYYY-MM-DD HH:MM"
	case "hour":
		trimLen = 13
	case "day", "week":
		trimLen = 10
	}
	var items []map[string]interface{}
	for _, rec := range records[1:] {
		if len(rec) < 6 {
			continue
		}
		t := strings.TrimSpace(rec[0])
		if len(t) > trimLen {
			t = t[:trimLen]
		}
		var avg, minV, maxV, sumsq float64
		var cnt float64
		fmt.Sscanf(strings.TrimSpace(rec[1]), "%f", &avg)
		fmt.Sscanf(strings.TrimSpace(rec[2]), "%f", &minV)
		fmt.Sscanf(strings.TrimSpace(rec[3]), "%f", &maxV)
		fmt.Sscanf(strings.TrimSpace(rec[4]), "%f", &sumsq)
		fmt.Sscanf(strings.TrimSpace(rec[5]), "%f", &cnt)

		rms := 0.0
		if cnt > 0 {
			rms = math.Sqrt(sumsq / cnt)
		}
		p2p := maxV - minV
		peak := math.Max(math.Abs(minV), math.Abs(maxV))
		crest := 0.0
		if rms > 0 {
			crest = peak / rms
		}

		items = append(items, map[string]interface{}{
			"t":     t,
			"rms":   math.Round(rms*1e6) / 1e6,
			"p2p":   math.Round(p2p*1e6) / 1e6,
			"crest": math.Round(crest*1e4) / 1e4,
			"avg":   avg,
			"min":   minV,
			"max":   maxV,
		})
	}
	return items
}

func parseVibRawCSV(csvData string) map[string]interface{} {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	if len(records) < 2 {
		return map[string]interface{}{"times_ms": []float64{}, "values": []float64{}}
	}
	dataRows := records[1:]
	timesMs := make([]float64, 0, len(dataRows))
	values := make([]float64, 0, len(dataRows))
	for _, rec := range dataRows {
		if len(rec) < 2 {
			continue
		}
		var ns float64
		fmt.Sscanf(strings.TrimSpace(rec[0]), "%f", &ns)
		ms := ns / 1e6 // nanoseconds → milliseconds
		var val float64
		fmt.Sscanf(strings.TrimSpace(rec[1]), "%f", &val)
		timesMs = append(timesMs, ms)
		values = append(values, val)
	}
	return map[string]interface{}{
		"times_ms": timesMs,
		"values":   values,
	}
}

func computeVibStats(tagData map[string]interface{}) map[string]interface{} {
	stats := map[string]interface{}{
		"count": 0, "avg": 0.0, "min": 0.0, "max": 0.0,
		"rms": 0.0, "p2p": 0.0, "crest": 0.0,
	}
	rollupRaw, ok := tagData["rollup"]
	if !ok {
		return stats
	}
	rollup, ok := rollupRaw.([]map[string]interface{})
	if !ok || len(rollup) == 0 {
		return stats
	}

	// Aggregate across all rollup buckets
	var totalSumSq, totalCount float64
	globalMin := math.MaxFloat64
	globalMax := -math.MaxFloat64
	var sumAvg float64
	for _, r := range rollup {
		rms, _ := r["rms"].(float64)
		avg, _ := r["avg"].(float64)
		minV, _ := r["min"].(float64)
		maxV, _ := r["max"].(float64)
		// Approximate: reconstruct sumsq from rms^2 * estimated count per bucket
		// Instead, use per-bucket min/max/avg for global stats
		sumAvg += avg
		if minV < globalMin {
			globalMin = minV
		}
		if maxV > globalMax {
			globalMax = maxV
		}
		totalSumSq += rms * rms
		totalCount++
	}

	overallRMS := 0.0
	if totalCount > 0 {
		overallRMS = math.Sqrt(totalSumSq / totalCount)
	}
	overallP2P := globalMax - globalMin
	peak := math.Max(math.Abs(globalMin), math.Abs(globalMax))
	overallCrest := 0.0
	if overallRMS > 0 {
		overallCrest = peak / overallRMS
	}

	return map[string]interface{}{
		"count": int(totalCount),
		"avg":   math.Round(sumAvg/totalCount*1e4) / 1e4,
		"min":   globalMin,
		"max":   globalMax,
		"rms":   math.Round(overallRMS*1e6) / 1e6,
		"p2p":   math.Round(overallP2P*1e6) / 1e6,
		"crest": math.Round(overallCrest*1e4) / 1e4,
	}
}

// --- Server-side FFT ---

func nextPow2(n int) int {
	p := 1
	for p < n {
		p *= 2
	}
	return p
}

// fftRadix2 performs in-place Cooley-Tukey FFT. len(data) must be power of 2.
func fftRadix2(data []complex128) {
	n := len(data)
	// Bit-reversal permutation
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			data[i], data[j] = data[j], data[i]
		}
	}
	// Butterfly
	for l := 2; l <= n; l *= 2 {
		ang := -2.0 * math.Pi / float64(l)
		wn := cmplx.Rect(1, ang)
		for i := 0; i < n; i += l {
			w := complex(1, 0)
			for j := 0; j < l/2; j++ {
				u := data[i+j]
				v := data[i+j+l/2] * w
				data[i+j] = u + v
				data[i+j+l/2] = u - v
				w *= wn
			}
		}
	}
}

// computeFFTSpectrum computes FFT on all values, then bins the magnitude spectrum down to maxBins points.
// Returns {freqs: [...], mags: [...], sample_rate: float64}
func computeFFTSpectrum(rawCSV string, maxBins int) map[string]interface{} {
	reader := csv.NewReader(strings.NewReader(rawCSV))
	records, _ := reader.ReadAll()
	if len(records) < 9 { // header + at least 8 data rows
		return nil
	}
	dataRows := records[1:]
	N := len(dataRows)

	// Parse values + compute sample rate from first and last timestamp
	values := make([]float64, N)
	var firstNs, lastNs float64
	for i, rec := range dataRows {
		if len(rec) < 2 {
			continue
		}
		if i == 0 {
			fmt.Sscanf(strings.TrimSpace(rec[0]), "%f", &firstNs)
		}
		if i == N-1 {
			fmt.Sscanf(strings.TrimSpace(rec[0]), "%f", &lastNs)
		}
		fmt.Sscanf(strings.TrimSpace(rec[1]), "%f", &values[i])
	}
	dtSec := (lastNs - firstNs) / 1e9 / float64(N-1)
	if dtSec <= 0 {
		return nil
	}
	sampleRate := 1.0 / dtSec

	// Zero-pad to next power of 2
	n2 := nextPow2(N)
	data := make([]complex128, n2)
	// Apply Hanning window
	for i := 0; i < N; i++ {
		w := 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(N-1)))
		data[i] = complex(values[i]*w, 0)
	}

	fftRadix2(data)

	// Magnitude spectrum (first half)
	half := n2 / 2
	rawMags := make([]float64, half-1)
	for k := 1; k < half; k++ {
		rawMags[k-1] = cmplx.Abs(data[k]) / float64(N) * 2
	}

	// Bin down to maxBins
	freqs := make([]float64, 0, maxBins)
	mags := make([]float64, 0, maxBins)
	if len(rawMags) <= maxBins {
		// No binning needed
		for k := 0; k < len(rawMags); k++ {
			freqs = append(freqs, float64(k+1)*sampleRate/float64(n2))
			mags = append(mags, rawMags[k])
		}
	} else {
		binSize := float64(len(rawMags)) / float64(maxBins)
		for b := 0; b < maxBins; b++ {
			start := int(float64(b) * binSize)
			end := int(float64(b+1) * binSize)
			if end > len(rawMags) {
				end = len(rawMags)
			}
			sum := 0.0
			for i := start; i < end; i++ {
				sum += rawMags[i]
			}
			avg := sum / float64(end-start)
			midK := (start + end) / 2
			freqs = append(freqs, float64(midK+1)*sampleRate/float64(n2))
			mags = append(mags, math.Round(avg*1e6)/1e6)
		}
	}

	return map[string]interface{}{
		"freqs":       freqs,
		"mags":        mags,
		"sample_rate": math.Round(sampleRate*100) / 100,
		"total_points": N,
	}
}

// formatFreqLabel formats frequency value for display
func formatFreqLabel(f float64) string {
	if f >= 1000 {
		return strconv.FormatFloat(f/1000, 'f', 1, 64) + "KHz"
	}
	return strconv.FormatFloat(f, 'f', 1, 64) + "Hz"
}

func mdToHTML(text string) string {
	// Split inline numbered lists: "1. xxx 2. xxx" → separate lines
	text = regexp.MustCompile(`\.\s+(\d+)\.\s+`).ReplaceAllString(text, ".\n$1. ")
	// ## headings
	text = regexp.MustCompile(`(?m)^##\s+(.+)$`).ReplaceAllString(text, `<h4 style="color:#1a365d;margin:20px 0 8px;font-size:15px;">$1</h4>`)
	// **Title** content → split
	text = boldHeadingRE.ReplaceAllString(text, "**$1**\n$2")
	// Standalone **bold** lines → h4
	text = regexp.MustCompile(`(?m)^\*\*(.+?)\*\*$`).ReplaceAllString(text, `<h4 style="color:#1a365d;margin:20px 0 8px;font-size:15px;">$1</h4>`)
	// Inline bold
	text = mdBoldRE.ReplaceAllString(text, "<strong>$1</strong>")
	text = circledNumRE.ReplaceAllString(text, "\n$1")
	text = numParenInline.ReplaceAllString(text, "\n$1) ")

	lines := strings.Split(text, "\n")
	var result []string
	inOL := false
	inUL := false

	numRE := regexp.MustCompile(`^(\d+[.)]\s+|[①②③④⑤⑥⑦⑧⑨⑩])`)
	numStripRE := regexp.MustCompile(`^(\d+[.)]\s+|[①②③④⑤⑥⑦⑧⑨⑩]\s*)`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Numbered list
		if numRE.MatchString(trimmed) {
			if !inOL {
				if inUL {
					result = append(result, "</ul>")
					inUL = false
				}
				result = append(result, "<ol>")
				inOL = true
			}
			content := numStripRE.ReplaceAllString(trimmed, "")
			result = append(result, "<li>"+content+"</li>")
			continue
		}

		// Bullet list
		if strings.HasPrefix(trimmed, "- ") {
			if !inUL {
				if inOL {
					result = append(result, "</ol>")
					inOL = false
				}
				result = append(result, "<ul>")
				inUL = true
			}
			result = append(result, "<li>"+trimmed[2:]+"</li>")
			continue
		}

		// Empty line inside a list: peek ahead — if next non-empty line is also a list item, keep list open
		if trimmed == "" && (inOL || inUL) {
			// Look ahead for next non-empty line
			keepOpen := false
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" {
					continue
				}
				if inOL && numRE.MatchString(next) {
					keepOpen = true
				}
				if inUL && strings.HasPrefix(next, "- ") {
					keepOpen = true
				}
				break
			}
			if keepOpen {
				continue // skip blank line, keep list open
			}
		}

		if inOL {
			result = append(result, "</ol>")
			inOL = false
		}
		if inUL {
			result = append(result, "</ul>")
			inUL = false
		}

		if trimmed == "" {
			result = append(result, "")
		} else if strings.HasPrefix(trimmed, "<h4") {
			result = append(result, trimmed)
		} else {
			result = append(result, "<p>"+trimmed+"</p>")
		}
	}

	if inOL {
		result = append(result, "</ol>")
	}
	if inUL {
		result = append(result, "</ul>")
	}

	return strings.Join(result, "\n")
}
