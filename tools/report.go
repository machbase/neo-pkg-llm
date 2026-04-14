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
		Description: `데이터를 분석하여 HTML 리포트를 생성합니다. 차트와 심층 분석이 포함된 보고서를 자동으로 만들어줍니다. 통계/태그/시간범위 조회를 직접 하지 마세요. 이 도구가 내부에서 모두 처리합니다. table만 지정하여 바로 호출하세요.`,
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"template_id": {
					Type:        "string",
					Description: "템플릿 ID. 운전/차량: 'R-3', 진동: 'R-2', 금융: 'R-1', 범용: 'R-0' (기본값)",
					Enum:        []string{"R-0", "R-1", "R-2", "R-3"},
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
					Description: "심층 분석 (한국어). ★1차 호출 시 이 파라미터를 비워두세요! 도구가 차트 분석 요약을 반환합니다. 2차 호출 시 그 요약을 기반으로 작성하세요.★ 5~7문단. 데이터 구조/품질 설명 금지! 차트 인사이트와 실질적 해석만 작성.",
				},
				"recommendations": {
					Type:        "string",
					Description: "종합 소견 및 권고 (한국어). ★1차 호출 시 비워두세요!★ 5개 이상 번호 항목. 금융: 투자 관점 행동 지침. 진동: 설비 관리 조치 사항.",
				},
				"rollup_unit": {
					Type:        "string",
					Description: "ROLLUP 시간 단위. 기본값: 자동(시간 범위 기반). 사용자가 단위를 지정하면 그 값 사용. sec/min/hour/day/week/month",
					Enum:        []string{"sec", "min", "hour", "day", "week", "month"},
				},
				"stock": {
					Type:        "string",
					Description: "Finance multi-stock table: stock code to analyze (e.g. AAPL). Used when tags follow AAPL_close pattern.",
				},
			},
			Required: []string{"table"},
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

	// 1. Get tag list first (needed for multi-stock detection before stats query)
	tagListSQL := fmt.Sprintf("SELECT NAME FROM V$%s_STAT LIMIT 2600", tableName)
	tagCSV, _ := r.client.QuerySQL(tagListSQL, "", "", "csv")
	tags := parseTagList(tagCSV)
	if len(tags) == 0 {
		// Fallback: GROUP BY (slower but works for all tables)
		tagListSQL = fmt.Sprintf("SELECT NAME FROM %s%s GROUP BY NAME", tableName, timeWhereBase)
		tagCSV, _ = r.client.QuerySQL(tagListSQL, "", "", "csv")
		tags = parseTagList(tagCSV)
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("failed to retrieve tags from table %s (query may have timed out due to large data volume)", tableName)
	}

	// 2. Multi-stock detection (before stats query to filter)
	stock := argAnyStr(normalizedArgs, "stock")
	// LLM often sends the stock name via "tag"/"tags" param instead of "stock"
	if stock == "" {
		tagVal := argAnyStr(normalizedArgs, "tag")
		if tagVal == "" {
			tagVal = argAnyStr(normalizedArgs, "tags")
		}
		if tagVal != "" {
			// Strip common suffixes like _close, _open etc. to extract stock name
			candidate := strings.Split(tagVal, ",")[0] // take first if comma-separated
			candidate = strings.TrimSpace(candidate)
			for _, suffix := range []string{"_close", "_open", "_high", "_low", "_volume", "_adj_close"} {
				if idx := strings.Index(strings.ToLower(candidate), suffix); idx > 0 {
					candidate = candidate[:idx]
					break
				}
			}
			// Verify this looks like a stock name (all caps, no underscore remaining)
			upper := strings.ToUpper(candidate)
			if len(upper) >= 1 && len(upper) <= 10 && !strings.Contains(upper, "_") {
				stock = upper
				fmt.Printf("[Report] Extracted stock '%s' from tag param\n", stock)
			}
		}
	}
	autoSelectedStock := ""
	if detectMultiStock(tags) && stock == "" {
		stocks := extractStockNames(tags)
		if len(stocks) > 0 {
			stock = stocks[0]
			autoSelectedStock = stock
			if templateID == "R-0" {
				templateID = "R-1"
			}
			fmt.Printf("[Report] Multi-stock detected, auto-selected first stock: %s (template: %s)\n", stock, templateID)
		}
	}

	// Filter tags by stock prefix if applicable
	stockWhere := ""
	if stock != "" {
		prefix := strings.ToUpper(stock) + "_"
		filtered := make([]string, 0)
		for _, t := range tags {
			if strings.HasPrefix(strings.ToUpper(t), prefix) {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) > 0 {
			tags = filtered
			// Build IN clause for stats query
			quoted := make([]string, len(filtered))
			for i, t := range filtered {
				quoted[i] = fmt.Sprintf("'%s'", t)
			}
			stockWhere = fmt.Sprintf(" AND NAME IN (%s)", strings.Join(quoted, ","))
			fmt.Printf("[Report] Filtered to stock '%s': %d tags\n", stock, len(filtered))
		}
	}

	// Auto-detect template: if OHLCV tags found, use R-1
	if templateID == "R-0" {
		ohlcvCheck := findOHLCVTags(tags, stock)
		if ohlcvCheck["close"] != "" && ohlcvCheck["open"] != "" {
			templateID = "R-1"
			fmt.Printf("[Report] Auto-detected OHLCV tags, switching to R-1\n")
		}
	}

	// 3. Tag stats (filtered by stock if multi-stock, exclude non-IMU for R-3)
	excludeWhere := ""
	if templateID == "R-3" {
		excludeWhere = " AND NAME NOT IN ('Class','class','Label','label','Target','target')"
	}
	statsSQL := fmt.Sprintf("SELECT NAME, COUNT(*) as cnt, ROUND(AVG(VALUE),2) as avg, ROUND(MIN(VALUE),2) as min, ROUND(MAX(VALUE),2) as max FROM %s%s%s%s GROUP BY NAME", tableName, timeWhereBase, stockWhere, excludeWhere)
	if stockWhere == "" && timeWhereBase == "" && excludeWhere == "" {
		statsSQL = fmt.Sprintf("SELECT NAME, COUNT(*) as cnt, ROUND(AVG(VALUE),2) as avg, ROUND(MIN(VALUE),2) as min, ROUND(MAX(VALUE),2) as max FROM %s GROUP BY NAME", tableName)
	} else if stockWhere == "" && timeWhereBase == "" && excludeWhere != "" {
		statsSQL = fmt.Sprintf("SELECT NAME, COUNT(*) as cnt, ROUND(AVG(VALUE),2) as avg, ROUND(MIN(VALUE),2) as min, ROUND(MAX(VALUE),2) as max FROM %s WHERE%s GROUP BY NAME", tableName, excludeWhere[4:])
	} else if stockWhere != "" && timeWhereBase == "" {
		statsSQL = fmt.Sprintf("SELECT NAME, COUNT(*) as cnt, ROUND(AVG(VALUE),2) as avg, ROUND(MIN(VALUE),2) as min, ROUND(MAX(VALUE),2) as max FROM %s WHERE%s%s GROUP BY NAME", tableName, stockWhere[4:], excludeWhere)
	}
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

	// 4. Time range
	timeSQL := fmt.Sprintf("SELECT MIN(TIME), MAX(TIME) FROM %s%s", tableName, timeWhereBase)
	timeCSV, err := r.client.QuerySQL(timeSQL, "Default", "", "csv")
	if err == nil {
		if tr := parseTimeRangeCSV(timeCSV); tr != "" {
			params["TIME_RANGE"] = convertTimeRangeToLocal(tr, loc)
		}
	}

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

			// FFT: fetch limited raw data (131072 = 2^17 points, ~0.2Hz resolution at 25kHz sampling)
			fftSQL := fmt.Sprintf(
				"SELECT TIME, VALUE FROM %s WHERE NAME='%s'%s ORDER BY TIME LIMIT 131072",
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
	} else if templateID == "R-3" {
		// --- R-3: Driving behavior analysis ---
		tagListJSON, _ := json.Marshal(tags)
		params["TAG_LIST_JSON"] = string(tagListJSON)

		// Filter out non-IMU tags (e.g. Class, Label)
		imuTags := []string{}
		for _, tag := range tags {
			tl := strings.ToLower(tag)
			if tl == "class" || tl == "label" || tl == "target" {
				continue
			}
			imuTags = append(imuTags, tag)
		}
		if len(imuTags) > 10 {
			imuTags = imuTags[:10]
		}

		perTagData := map[string]interface{}{}
		for _, tag := range imuTags {
			tagData := map[string]interface{}{}

			// Rollup: AVG, MIN, MAX
			rollupSQL := fmt.Sprintf(
				"SELECT ROLLUP('%s',1,TIME) as t, ROUND(AVG(VALUE),6) as avg_val, "+
					"ROUND(MIN(VALUE),6) as min_val, ROUND(MAX(VALUE),6) as max_val "+
					"FROM %s WHERE NAME='%s'%s "+
					"GROUP BY ROLLUP('%s',1,TIME) ORDER BY t",
				rollupUnit, tableName, tag, timeWhere, rollupUnit)
			rollupCSV, err := r.client.QuerySQL(rollupSQL, "Default", "", "csv")
			if err == nil {
				tagData["rollup"] = parseDrivingRollupCSV(rollupCSV, rollupUnit)
			} else {
				fmt.Printf("[Report] Driving rollup query failed for tag %s: %v\n", tag, err)
			}

			// Raw waveform (4096 points)
			rawSQL := fmt.Sprintf(
				"SELECT TIME, VALUE FROM %s WHERE NAME='%s'%s ORDER BY TIME LIMIT 4096",
				tableName, tag, timeWhere)
			rawCSV, err := r.client.QuerySQL(rawSQL, "", "", "csv")
			if err == nil {
				tagData["raw"] = parseVibRawCSV(rawCSV) // reuse existing parser
			}

			perTagData[tag] = tagData
		}

		// Compute adaptive thresholds: mean ± 2*stddev per axis
		thresholds := map[string][2]float64{} // tag -> [upper, lower]
		for _, axis := range []string{"AccX", "AccY"} {
			actualTag := ""
			for _, t := range tags {
				if strings.EqualFold(t, axis) {
					actualTag = t
					break
				}
			}
			if actualTag == "" {
				continue
			}
			statSQL := fmt.Sprintf(
				"SELECT ROUND(AVG(VALUE),6), ROUND(STDDEV(VALUE),6) FROM %s WHERE NAME='%s'%s",
				tableName, actualTag, timeWhere)
			statCSV, err := r.client.QuerySQL(statSQL, "", "", "csv")
			if err == nil {
				recs, _ := csv.NewReader(strings.NewReader(statCSV)).ReadAll()
				if len(recs) >= 2 && len(recs[1]) >= 2 {
					var avg, sd float64
					fmt.Sscanf(strings.TrimSpace(recs[1][0]), "%f", &avg)
					fmt.Sscanf(strings.TrimSpace(recs[1][1]), "%f", &sd)
					thresholds[axis] = [2]float64{avg + 2*sd, avg - 2*sd}
					fmt.Printf("[Report] Threshold %s: upper=%.4f, lower=%.4f (avg=%.4f, sd=%.4f)\n", axis, avg+2*sd, avg-2*sd, avg, sd)
				}
			}
		}

		// Event detection from AccX/AccY raw data (up to 50000 points)
		eventsData := map[string]interface{}{
			"accel": []map[string]interface{}{},
			"brake": []map[string]interface{}{},
			"turn":  []map[string]interface{}{},
		}
		for _, axis := range []struct {
			tag    string
			events []string
		}{
			{"AccX", []string{"accel", "brake"}},
			{"AccY", []string{"turn"}},
		} {
			actualTag := ""
			for _, t := range tags {
				if strings.EqualFold(t, axis.tag) {
					actualTag = t
					break
				}
			}
			if actualTag == "" {
				continue
			}
			th, hasTh := thresholds[axis.tag]
			if !hasTh {
				continue
			}
			eventSQL := fmt.Sprintf(
				"SELECT TIME, VALUE FROM %s WHERE NAME='%s'%s ORDER BY TIME LIMIT 50000",
				tableName, actualTag, timeWhere)
			eventCSV, err := r.client.QuerySQL(eventSQL, "", "", "csv")
			if err != nil {
				continue
			}
			parsed := parseVibRawCSV(eventCSV)
			timesRaw, _ := parsed["times_ms"].([]float64)
			valsRaw, _ := parsed["values"].([]float64)
			for i, v := range valsRaw {
				if i >= len(timesRaw) {
					break
				}
				tMs := timesRaw[i]
				if strings.EqualFold(axis.tag, "AccX") {
					if v > th[0] { // upper threshold → 급가속
						eventsData["accel"] = append(eventsData["accel"].([]map[string]interface{}), map[string]interface{}{"t_ms": tMs, "value": v})
					} else if v < th[1] { // lower threshold → 급제동
						eventsData["brake"] = append(eventsData["brake"].([]map[string]interface{}), map[string]interface{}{"t_ms": tMs, "value": v})
					}
				} else if strings.EqualFold(axis.tag, "AccY") {
					if v > th[0] || v < th[1] { // 양방향 → 급회전
						eventsData["turn"] = append(eventsData["turn"].([]map[string]interface{}), map[string]interface{}{"t_ms": tMs, "value": v})
					}
				}
			}
		}
		accelCount := len(eventsData["accel"].([]map[string]interface{}))
		brakeCount := len(eventsData["brake"].([]map[string]interface{}))
		turnCount := len(eventsData["turn"].([]map[string]interface{}))
		totalEvents := accelCount + brakeCount + turnCount

		// Count total samples scanned for rate-based scoring
		totalSamples := 0
		for _, axis := range []string{"AccX", "AccY"} {
			for _, t := range tags {
				if strings.EqualFold(t, axis) {
					actualTag := t
					cntSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE NAME='%s'%s", tableName, actualTag, timeWhere)
					cntCSV, err := r.client.QuerySQL(cntSQL, "", "", "csv")
					if err == nil {
						cntRecs, _ := csv.NewReader(strings.NewReader(cntCSV)).ReadAll()
						if len(cntRecs) >= 2 && len(cntRecs[1]) >= 1 {
							var cnt int
							fmt.Sscanf(strings.TrimSpace(cntRecs[1][0]), "%d", &cnt)
							totalSamples += cnt
						}
					}
					break
				}
			}
		}
		if totalSamples == 0 {
			totalSamples = 1 // avoid division by zero
		}

		// Event rates (percentage of total samples)
		accelRate := float64(accelCount) / float64(totalSamples) * 100
		brakeRate := float64(brakeCount) / float64(totalSamples) * 100
		turnRate := float64(turnCount) / float64(totalSamples) * 100

		fmt.Printf("[Report] Driving events: accel=%d(%.1f%%), brake=%d(%.1f%%), turn=%d(%.1f%%) out of %d samples\n",
			accelCount, accelRate, brakeCount, brakeRate, turnCount, turnRate, totalSamples)

		// Safety score: 1 - (totalEvents / totalSamples)
		safetyScore := computeSafetyScore(totalEvents, totalSamples)

		// Threshold info for chart display
		thresholdInfo := map[string]interface{}{}
		if th, ok := thresholds["AccX"]; ok {
			thresholdInfo["accel_upper"] = math.Round(th[0]*1e4) / 1e4
			thresholdInfo["brake_lower"] = math.Round(th[1]*1e4) / 1e4
		}
		if th, ok := thresholds["AccY"]; ok {
			thresholdInfo["turn_upper"] = math.Round(th[0]*1e4) / 1e4
			thresholdInfo["turn_lower"] = math.Round(th[1]*1e4) / 1e4
		}

		drivingData := map[string]interface{}{
			"per_tag":      perTagData,
			"events":       eventsData,
			"safety_score": safetyScore,
			"thresholds":   thresholdInfo,
			"summary": map[string]interface{}{
				"total_events":  totalEvents,
				"accel_count":   accelCount,
				"brake_count":   brakeCount,
				"turn_count":    turnCount,
				"accel_rate":    math.Round(accelRate*10) / 10,
				"brake_rate":    math.Round(brakeRate*10) / 10,
				"turn_rate":     math.Round(turnRate*10) / 10,
				"total_samples": totalSamples,
			},
		}
		drivingJSON, _ := json.Marshal(drivingData)
		params["DRIVING_DATA_JSON"] = string(drivingJSON)
		fmt.Printf("[Report] Driving data: score=%.1f, events=%d\n", safetyScore, totalEvents)
	} else if templateID == "R-1" {
		// --- R-1: Finance OHLCV analysis ---
		// Note: multi-stock detection is handled earlier (before template branching)

		// Filter and normalize tags for selected stock
		ohlcvTags := findOHLCVTags(tags, stock)
		if ohlcvTags["close"] == "" {
			primaryTag, _ := pickTrendTags(tags)
			if primaryTag != "" {
				ohlcvTags["close"] = primaryTag
			}
		}
		stockName := stock
		if stockName == "" {
			stockName = tableName
		}
		params["STOCK_NAME"] = stockName
		fmt.Printf("[Report] Finance OHLCV tags: %v (stock=%q)\n", ohlcvTags, stock)

		// Query ROLLUP for each OHLCV field
		ohlcvData := map[string][]map[string]interface{}{}
		for field, tagName := range ohlcvTags {
			if tagName == "" {
				continue
			}
			decimals := 2
			if field == "volume" {
				decimals = 0
			}
			sql := fmt.Sprintf("SELECT ROLLUP('%s',1,TIME) as t, ROUND(AVG(VALUE),%d) as v FROM %s WHERE NAME='%s'%s GROUP BY ROLLUP('%s',1,TIME) ORDER BY t",
				rollupUnit, decimals, tableName, tagName, timeWhere, rollupUnit)
			csvResult, err := r.client.QuerySQL(sql, "Default", "", "csv")
			if err == nil {
				ohlcvData[field] = parseTrendCSV(csvResult, field, rollupUnit)
			} else {
				fmt.Printf("[Report] OHLCV query failed for %s (%s): %v\n", field, tagName, err)
			}
		}

		// Merge OHLCV data by time
		trendData := mergeOHLCV(ohlcvData)
		if len(trendData) > 0 {
			trendJSON, _ := json.Marshal(trendData)
			params["TREND_DATA_JSON"] = string(trendJSON)
			params["_FINANCE_SUMMARY"] = computeFinanceSummary(trendData)
			fmt.Printf("[Report] Fetched %d OHLCV trend data points\n", len(trendData))
		}
	} else {
		// --- R-0: Generic primary + volume trend ---
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
		// Add chart analysis guidance per template type
		if templateID == "R-1" {
			if fs, ok := params["_FINANCE_SUMMARY"]; ok && fs != "" {
				summary.WriteString("\n[차트 데이터 기반 분석 요약]\n")
				summary.WriteString(fs)
				summary.WriteString("\n")
			}
			summary.WriteString("\n★★ analysis 작성 규칙 (반드시 준수!) ★★\n")
			summary.WriteString("데이터 구조, 정합성, 품질 설명은 쓰지 마세요. 리포트를 읽는 사람은 투자자입니다.\n")
			summary.WriteString("통계 수치를 해석하여 아래 관점으로 실질적 인사이트를 작성하세요:\n\n")
			summary.WriteString("1. 가격 추세 판단: 현재 상승/하락/횡보 중 어디인지, 최근 가격이 과거 대비 어느 위치인지\n")
			summary.WriteString("2. 캔들스틱 패턴: 통계에서 open과 close 관계로 양봉/음봉 우세 판단\n")
			summary.WriteString("3. 이동평균 분석: 단기(MA5) vs 중기(MA20) vs 장기(MA60) 배열 상태, 골든크로스/데드크로스 가능성\n")
			summary.WriteString("4. 변동성 분석: high-low 스프레드, 볼린저밴드 폭으로 현재 변동성 수준 평가\n")
			summary.WriteString("5. 거래량-가격 상관: 가격 움직임에 거래량이 동반되는지 (추세 신뢰도)\n")
			summary.WriteString("6. 투자 시 고려사항: 현재 시점에서 주의할 리스크, 주목할 기회\n\n")
			summary.WriteString("★★ recommendations도 투자 관점에서 구체적 행동 지침으로 작성하세요 ★★\n")
		} else if templateID == "R-2" {
			summary.WriteString("\n★★ analysis 작성 규칙 (반드시 준수!) ★★\n")
			summary.WriteString("데이터 구조, 정합성, 품질 설명은 쓰지 마세요. 리포트를 읽는 사람은 설비 관리자입니다.\n")
			summary.WriteString("통계 수치를 해석하여 아래 관점으로 실질적 인사이트를 작성하세요:\n\n")
			summary.WriteString("1. 진동 수준 평가: RMS/Peak 값이 정상 범위인지, ISO 10816 기준 등급\n")
			summary.WriteString("2. 원시 파형 분석: 충격성 신호, 주기적 패턴, 비정상 파형 특징\n")
			summary.WriteString("3. RMS 추이: 시간에 따른 진동 에너지 변화 추세, 악화 징후 유무\n")
			summary.WriteString("4. Peak-to-Peak/Crest Factor: 충격성 진동 정도, 베어링/기어 결함 가능성\n")
			summary.WriteString("5. FFT 스펙트럼: 주요 주파수 성분 해석, 회전체 결함 주파수 패턴\n")
			summary.WriteString("6. 설비 관리 시 주의사항: 현재 상태에서의 리스크, 점검 필요 여부\n\n")
			summary.WriteString("★★ recommendations도 설비 관리 관점에서 구체적 조치 사항으로 작성하세요 ★★\n")
		} else if templateID == "R-3" {
			summary.WriteString("\n★★ analysis 작성 규칙 (반드시 준수!) ★★\n")
			summary.WriteString("데이터 구조, 정합성, 품질 설명은 쓰지 마세요. 리포트를 읽는 사람은 차량 관리자/운전자입니다.\n")
			summary.WriteString("통계 수치를 해석하여 아래 관점으로 실질적 인사이트를 작성하세요:\n\n")
			summary.WriteString("1. 안전 점수 해석: 현재 점수 수준과 의미, 개선 필요 여부\n")
			summary.WriteString("2. 급가속/급제동 분석: 이벤트 빈도, 패턴, 운전 습관과의 연관성\n")
			summary.WriteString("3. 급회전 분석: 횡가속도 패턴, 코너링 안정성 평가\n")
			summary.WriteString("4. IMU 패턴: 3축 가속도/자이로 값의 범위와 안정성, 비정상 패턴\n")
			summary.WriteString("5. 종합 운전 행태 진단: 안전 운전 수준, 주요 위험 요인\n\n")
			summary.WriteString("★★ recommendations도 운전 습관 개선 관점에서 구체적 행동 지침으로 작성하세요 ★★\n")
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
	result := fmt.Sprintf("Report saved: %s\n[리포트 열기](%s)", filename, reportURL)
	if autoSelectedStock != "" {
		result = fmt.Sprintf("종목이 지정되지 않아 첫 번째 종목 '%s'을(를) 자동 선택하여 분석했습니다.\n\n%s", autoSelectedStock, result)
	}
	return result, nil
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
	if csvData == "" {
		return nil
	}
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	if len(records) < 2 {
		return nil
	}
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

// detectMultiStock checks if tags follow the XXX_close pattern (multi-stock table).
func detectMultiStock(tags []string) bool {
	for _, t := range tags {
		if strings.HasSuffix(strings.ToLower(t), "_close") {
			return true
		}
	}
	return false
}

// extractStockNames extracts unique stock prefixes from tags like AAPL_close.
func extractStockNames(tags []string) []string {
	seen := map[string]bool{}
	var stocks []string
	for _, t := range tags {
		idx := strings.LastIndex(t, "_")
		if idx > 0 {
			prefix := t[:idx]
			if !seen[prefix] {
				seen[prefix] = true
				stocks = append(stocks, prefix)
			}
		}
	}
	return stocks
}

// findOHLCVTags maps OHLCV field names to actual tag names.
// If stock is non-empty, filters tags by prefix (e.g., "AAPL" matches "AAPL_close").
// If stock is empty, looks for tags named "close", "open", etc. directly.
func findOHLCVTags(tags []string, stock string) map[string]string {
	result := map[string]string{}
	fields := []string{"open", "high", "low", "close", "volume"}

	if stock != "" {
		prefix := strings.ToUpper(stock) + "_"
		for _, t := range tags {
			upper := strings.ToUpper(t)
			if strings.HasPrefix(upper, prefix) {
				suffix := strings.ToLower(t[len(prefix):])
				for _, f := range fields {
					if suffix == f {
						result[f] = t
						break
					}
				}
			}
		}
	} else {
		lower := map[string]string{}
		for _, t := range tags {
			lower[strings.ToLower(t)] = t
		}
		for _, f := range fields {
			if orig, ok := lower[f]; ok {
				result[f] = orig
			}
		}
	}
	return result
}

// mergeOHLCV merges separate OHLCV trend arrays into unified time-series.
func mergeOHLCV(ohlcvData map[string][]map[string]interface{}) []map[string]interface{} {
	// Build time-indexed map from all fields
	timeMap := map[string]map[string]interface{}{}
	var timeOrder []string

	// Use close as primary time source, fallback to any available field
	primaryField := "close"
	if _, ok := ohlcvData[primaryField]; !ok {
		for f := range ohlcvData {
			primaryField = f
			break
		}
	}

	// Collect all time points from primary field
	if primary, ok := ohlcvData[primaryField]; ok {
		for _, item := range primary {
			t, _ := item["time"].(string)
			if t == "" {
				continue
			}
			if _, exists := timeMap[t]; !exists {
				timeMap[t] = map[string]interface{}{"time": t}
				timeOrder = append(timeOrder, t)
			}
			if v, ok := item[primaryField]; ok {
				timeMap[t][primaryField] = v
			}
		}
	}

	// Merge other fields
	for field, items := range ohlcvData {
		if field == primaryField {
			continue
		}
		for _, item := range items {
			t, _ := item["time"].(string)
			if t == "" {
				continue
			}
			if _, exists := timeMap[t]; !exists {
				timeMap[t] = map[string]interface{}{"time": t}
				timeOrder = append(timeOrder, t)
			}
			if v, ok := item[field]; ok {
				timeMap[t][field] = v
			}
		}
	}

	// Build result in time order
	var result []map[string]interface{}
	for _, t := range timeOrder {
		result = append(result, timeMap[t])
	}
	return result
}

// computeFinanceSummary generates a text summary of OHLCV trend data for LLM analysis.
func computeFinanceSummary(trendData []map[string]interface{}) string {
	if len(trendData) == 0 {
		return ""
	}
	var b strings.Builder

	// Helper to parse float from interface
	toFloat := func(v interface{}) float64 {
		switch val := v.(type) {
		case float64:
			return val
		case string:
			var f float64
			fmt.Sscanf(val, "%f", &f)
			return f
		}
		return 0
	}

	// Collect close prices
	type point struct {
		time  string
		close float64
		open  float64
		high  float64
		low   float64
		vol   float64
	}
	var pts []point
	for _, d := range trendData {
		p := point{}
		if t, ok := d["time"]; ok {
			p.time, _ = t.(string)
		}
		if v, ok := d["close"]; ok {
			p.close = toFloat(v)
		}
		if v, ok := d["open"]; ok {
			p.open = toFloat(v)
		}
		if v, ok := d["high"]; ok {
			p.high = toFloat(v)
		}
		if v, ok := d["low"]; ok {
			p.low = toFloat(v)
		}
		if v, ok := d["volume"]; ok {
			p.vol = toFloat(v)
		}
		if p.close > 0 {
			pts = append(pts, p)
		}
	}
	if len(pts) == 0 {
		return ""
	}

	// 1. Trend direction
	first := pts[0]
	last := pts[len(pts)-1]
	changeRate := 0.0
	if first.close > 0 {
		changeRate = (last.close - first.close) / first.close * 100
	}
	direction := "횡보"
	if changeRate > 5 {
		direction = "상승"
	} else if changeRate < -5 {
		direction = "하락"
	}
	b.WriteString(fmt.Sprintf("- 추세: %s → %s (%.1f → %.1f, %.1f%% %s)\n",
		first.time, last.time, first.close, last.close, changeRate, direction))

	// 2. Recent candle pattern (last 20 bars)
	recentN := 20
	if recentN > len(pts) {
		recentN = len(pts)
	}
	recent := pts[len(pts)-recentN:]
	bullish := 0
	bearish := 0
	for _, p := range recent {
		if p.open > 0 {
			if p.close >= p.open {
				bullish++
			} else {
				bearish++
			}
		}
	}
	if bullish+bearish > 0 {
		dominant := "중립"
		if bullish > bearish+2 {
			dominant = "강세 우위"
		} else if bearish > bullish+2 {
			dominant = "약세 우위"
		}
		b.WriteString(fmt.Sprintf("- 최근 %d봉: 양봉 %d개, 음봉 %d개 (%s)\n", recentN, bullish, bearish, dominant))
	}

	// 3. Moving averages
	calcMA := func(data []point, period int) float64 {
		if len(data) < period {
			return 0
		}
		sum := 0.0
		for i := len(data) - period; i < len(data); i++ {
			sum += data[i].close
		}
		return sum / float64(period)
	}
	ma5 := calcMA(pts, 5)
	ma20 := calcMA(pts, 20)
	ma60 := calcMA(pts, 60)
	if ma5 > 0 && ma20 > 0 {
		arrangement := ""
		if ma60 > 0 {
			if ma5 > ma20 && ma20 > ma60 {
				arrangement = "정배열 (강세)"
			} else if ma5 < ma20 && ma20 < ma60 {
				arrangement = "역배열 (약세)"
			} else {
				arrangement = "혼조"
			}
			b.WriteString(fmt.Sprintf("- 이동평균: MA5(%.1f) / MA20(%.1f) / MA60(%.1f) → %s\n", ma5, ma20, ma60, arrangement))
		} else {
			if ma5 > ma20 {
				arrangement = "단기 우위"
			} else {
				arrangement = "단기 열위"
			}
			b.WriteString(fmt.Sprintf("- 이동평균: MA5(%.1f) / MA20(%.1f) → %s\n", ma5, ma20, arrangement))
		}
	}

	// 4. Volatility (high-low spread)
	hasHL := false
	totalSpread := 0.0
	recentSpread := 0.0
	spreadCount := 0
	recentSpreadCount := 0
	for i, p := range pts {
		if p.high > 0 && p.low > 0 {
			hasHL = true
			spread := p.high - p.low
			totalSpread += spread
			spreadCount++
			if i >= len(pts)-recentN {
				recentSpread += spread
				recentSpreadCount++
			}
		}
	}
	if hasHL && spreadCount > 0 {
		avgSpread := totalSpread / float64(spreadCount)
		avgRecent := recentSpread / float64(recentSpreadCount)
		volState := "보합"
		if avgRecent > avgSpread*1.2 {
			volState = "확대"
		} else if avgRecent < avgSpread*0.8 {
			volState = "축소"
		}
		b.WriteString(fmt.Sprintf("- 변동성: 전체 평균 스프레드 %.1f, 최근 %.1f → 변동성 %s\n", avgSpread, avgRecent, volState))
	}

	// 5. High/Low points
	maxClose := pts[0]
	minClose := pts[0]
	for _, p := range pts {
		if p.close > maxClose.close {
			maxClose = p
		}
		if p.close < minClose.close {
			minClose = p
		}
	}
	b.WriteString(fmt.Sprintf("- 최고가 구간: %s (%.1f)\n", maxClose.time, maxClose.close))
	b.WriteString(fmt.Sprintf("- 최저가 구간: %s (%.1f)\n", minClose.time, minClose.close))

	// 6. Volume trend (if available)
	hasVol := false
	totalVol := 0.0
	recentVol := 0.0
	volCount := 0
	recentVolCount := 0
	for i, p := range pts {
		if p.vol > 0 {
			hasVol = true
			totalVol += p.vol
			volCount++
			if i >= len(pts)-recentN {
				recentVol += p.vol
				recentVolCount++
			}
		}
	}
	if hasVol && volCount > 0 && recentVolCount > 0 {
		avgVol := totalVol / float64(volCount)
		avgRecentVol := recentVol / float64(recentVolCount)
		ratio := avgRecentVol / avgVol
		volTrend := "보합"
		if ratio > 1.3 {
			volTrend = "급증"
		} else if ratio > 1.1 {
			volTrend = "증가"
		} else if ratio < 0.7 {
			volTrend = "급감"
		} else if ratio < 0.9 {
			volTrend = "감소"
		}
		b.WriteString(fmt.Sprintf("- 거래량: 전체 평균 %.0f, 최근 평균 %.0f (%.1f배, %s)\n", avgVol, avgRecentVol, ratio, volTrend))
	}

	return b.String()
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
		fmt.Printf("[FFT] Insufficient records: %d (need >= 9)\n", len(records))
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
		fmt.Printf("[FFT] Invalid dtSec: %.6f (firstNs=%.0f, lastNs=%.0f, N=%d)\n", dtSec, firstNs, lastNs, N)
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

// --- R-3 Driving behavior helpers ---

func parseDrivingRollupCSV(csvData string, rollupUnit string) []map[string]interface{} {
	reader := csv.NewReader(strings.NewReader(csvData))
	records, _ := reader.ReadAll()
	if len(records) < 2 {
		return nil
	}
	trimLen := 7
	switch rollupUnit {
	case "sec":
		trimLen = 19
	case "min":
		trimLen = 16
	case "hour":
		trimLen = 13
	case "day", "week":
		trimLen = 10
	}
	var items []map[string]interface{}
	for _, rec := range records[1:] {
		if len(rec) < 4 {
			continue
		}
		t := strings.TrimSpace(rec[0])
		if len(t) > trimLen {
			t = t[:trimLen]
		}
		var avg, minV, maxV float64
		fmt.Sscanf(strings.TrimSpace(rec[1]), "%f", &avg)
		fmt.Sscanf(strings.TrimSpace(rec[2]), "%f", &minV)
		fmt.Sscanf(strings.TrimSpace(rec[3]), "%f", &maxV)
		items = append(items, map[string]interface{}{
			"t":   t,
			"avg": math.Round(avg*1e6) / 1e6,
			"min": math.Round(minV*1e6) / 1e6,
			"max": math.Round(maxV*1e6) / 1e6,
		})
	}
	return items
}

func computeSafetyScore(totalEvents, totalSamples int) float64 {
	if totalSamples == 0 {
		return 100.0
	}
	ratio := float64(totalEvents) / float64(totalSamples)
	score := (1.0 - ratio) * 100.0
	if score < 0 {
		score = 0
	}
	return math.Round(score*10) / 10
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
	// ### headings (must come before ## to avoid partial match)
	text = regexp.MustCompile(`(?m)^###\s+(.+)$`).ReplaceAllString(text, `<h4 style="color:#1a365d;margin:20px 0 8px;font-size:15px;">$1</h4>`)
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
	inTable := false

	tableRowRE := regexp.MustCompile(`^\|(.+)\|$`)
	tableSepRE := regexp.MustCompile(`^\|[\s:_-]+(\|[\s:_-]+)*\|$`)

	numRE := regexp.MustCompile(`^(\d+[.)]\s+|[①②③④⑤⑥⑦⑧⑨⑩])`)
	numStripRE := regexp.MustCompile(`^(\d+[.)]\s+|[①②③④⑤⑥⑦⑧⑨⑩]\s*)`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Markdown table
		if tableRowRE.MatchString(trimmed) {
			// Skip separator row (|---|---|)
			if tableSepRE.MatchString(trimmed) {
				continue
			}
			cells := strings.Split(strings.Trim(trimmed, "|"), "|")
			if !inTable {
				// Close any open list
				if inOL {
					result = append(result, "</ol>")
					inOL = false
				}
				if inUL {
					result = append(result, "</ul>")
					inUL = false
				}
				inTable = true
				result = append(result, `<table style="width:100%;border-collapse:collapse;margin:12px 0;font-size:14px;">`)
				// First row is header
				result = append(result, "<thead><tr>")
				for _, cell := range cells {
					result = append(result, `<th style="border:1px solid #d0d5dd;padding:8px 12px;background:#f2f4f7;text-align:left;">`+strings.TrimSpace(cell)+"</th>")
				}
				result = append(result, "</tr></thead><tbody>")
				continue
			}
			result = append(result, "<tr>")
			for _, cell := range cells {
				result = append(result, `<td style="border:1px solid #d0d5dd;padding:8px 12px;">`+strings.TrimSpace(cell)+"</td>")
			}
			result = append(result, "</tr>")
			continue
		}

		// Close table if we were in one
		if inTable {
			result = append(result, "</tbody></table>")
			inTable = false
		}

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

		// Bullet list (- or *)
		isBullet := strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")
		if isBullet {
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
				if inUL && (strings.HasPrefix(next, "- ") || strings.HasPrefix(next, "* ")) {
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

	if inTable {
		result = append(result, "</tbody></table>")
	}
	if inOL {
		result = append(result, "</ol>")
	}
	if inUL {
		result = append(result, "</ul>")
	}

	return strings.Join(result, "\n")
}
