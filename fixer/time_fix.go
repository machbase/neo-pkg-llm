package fixer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CaptureDataTimeRange intercepts execute_sql_query results containing MIN(TIME)/MAX(TIME).
func CaptureDataTimeRange(args map[string]interface{}, result string, fctx *FixerContext) {
	sqlQuery, _ := args["sql_query"].(string)
	upperQuery := strings.ToUpper(sqlQuery)
	hasMinMax := strings.Contains(upperQuery, "MAX(TIME)") || strings.Contains(upperQuery, "MIN(TIME)")
	hasOrderLimit := strings.Contains(upperQuery, "ORDER BY TIME") && strings.Contains(upperQuery, "LIMIT 1")
	if !hasMinMax && !hasOrderLimit {
		return
	}

	var minMs, maxMs int64
	var foundMin, foundMax bool

	trimmed := strings.TrimSpace(result)

	if strings.HasPrefix(trimmed, "{") {
		type jsonResult struct {
			Data struct {
				Columns []string        `json:"columns"`
				Rows    [][]interface{} `json:"rows"`
			} `json:"data"`
		}
		var jr jsonResult
		if err := json.Unmarshal([]byte(trimmed), &jr); err == nil && len(jr.Data.Rows) > 0 {
			row := jr.Data.Rows[0]
			for i, col := range jr.Data.Columns {
				upper := strings.ToUpper(col)
				if i < len(row) {
					var val int64
					switch v := row[i].(type) {
					case float64:
						val = int64(v)
					case json.Number:
						val, _ = v.Int64()
					}
					if (strings.Contains(upper, "MIN") || strings.HasSuffix(upper, "MIN_TIME")) && !foundMin {
						minMs = val
						foundMin = true
					}
					if (strings.Contains(upper, "MAX") || strings.HasSuffix(upper, "MAX_TIME")) && !foundMax {
						maxMs = val
						foundMax = true
					}
				}
			}
		}
	} else {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			headers := strings.Split(lines[0], ",")
			values := strings.Split(lines[1], ",")
			for i, h := range headers {
				upper := strings.ToUpper(strings.TrimSpace(h))
				if i < len(values) {
					if strings.Contains(upper, "MIN") && !foundMin {
						if ms, ok := ParseTimeValue(strings.TrimSpace(values[i])); ok {
							minMs = ms
							foundMin = true
						}
					}
					if strings.Contains(upper, "MAX") && !foundMax {
						if ms, ok := ParseTimeValue(strings.TrimSpace(values[i])); ok {
							maxMs = ms
							foundMax = true
						}
					}
					if hasOrderLimit && upper == "TIME" && !foundMax && !foundMin {
						if ms, ok := ParseTimeValue(strings.TrimSpace(values[i])); ok {
							if strings.Contains(upperQuery, "DESC") {
								maxMs = ms
								foundMax = true
							} else {
								minMs = ms
								foundMin = true
							}
						}
					}
				}
			}
		}
	}

	if foundMin {
		fctx.DataMinDt = time.UnixMilli(minMs).Format(DtFormat)
		fmt.Printf("  [capture] MIN(TIME): %d → %s\n", minMs, fctx.DataMinDt)
	}
	if foundMax {
		maxTime := time.UnixMilli(maxMs)
		fctx.DataMaxDt = maxTime.Format(DtFormat)
		fmt.Printf("  [capture] MAX(TIME): %d → %s\n", maxMs, fctx.DataMaxDt)

		if fctx.TimeStartDt != "" && fctx.TimeEndDt != "" {
			if currentEnd, err := time.Parse(DtFormat, fctx.TimeEndDt); err == nil {
				fmt.Printf("  [capture] compare: maxTime=%s, currentEnd=%s, before=%v\n",
					maxTime.Format(DtFormat), currentEnd.Format(DtFormat), maxTime.Before(currentEnd))
				if maxTime.Before(currentEnd) {
					currentStart, _ := time.Parse(DtFormat, fctx.TimeStartDt)
					dur := currentEnd.Sub(currentStart)
					fctx.TimeStartDt = maxTime.Add(-dur).Format(DtFormat)
					fctx.TimeEndDt = maxTime.Format(DtFormat)
					fmt.Printf("  [TimeRange] 데이터 기반 갱신: %s ~ %s\n", fctx.TimeStartDt, fctx.TimeEndDt)
				}
			}
		} else {
			fmt.Printf("  [capture] skip recalc: timeStartDt=%q, timeEndDt=%q\n", fctx.TimeStartDt, fctx.TimeEndDt)
		}
	}
	if !foundMin && !foundMax {
		fmt.Printf("  [capture] no MIN/MAX found in result\n")
	}
}

// ParseTimeValue parses a time value as epoch milliseconds (int64) or datetime string.
func ParseTimeValue(s string) (int64, bool) {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, true
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), true
		}
	}
	return 0, false
}
