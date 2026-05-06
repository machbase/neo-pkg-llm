package agent

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"neo-pkg-llm/fixer"
)

func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// timeRangeREPattern matches patterns like "최근 3시간", "지난 7일", "최근 30분", "최근 1년", "최근 6개월"
var timeRangeREPattern = regexp.MustCompile(`(최근|지난)\s*(\d+)\s*(시간|분|일|주|개월|년)`)

// TimeRangeResult holds parsed time range info.
type TimeRangeResult struct {
	StartMs string // epoch milliseconds
	EndMs   string // epoch milliseconds
	StartDt string // datetime string e.g. "2026-04-07 13:00:00"
	EndDt   string // datetime string e.g. "2026-04-07 14:00:00"
	Label   string // e.g. "최근 1시간"
	Unit    string // recommended ROLLUP unit: 'sec', 'min', 'hour', 'day'
}

// parseTimeRange detects relative time expressions in the query.
// Returns nil if no time keyword is found.
func parseTimeRange(query string) *TimeRangeResult {
	now := time.Now()

	var startTime time.Time
	var label string

	// "오늘" → today 00:00 ~ now
	if strings.Contains(query, "오늘") {
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		label = "오늘"
	}

	// "최근 N시간/분/일/주", "지난 N시간/분/일/주"
	if label == "" {
		m := timeRangeREPattern.FindStringSubmatch(query)
		if m != nil {
			n, _ := strconv.Atoi(m[2])
			unit := m[3]
			var d time.Duration
			switch unit {
			case "분":
				d = time.Duration(n) * time.Minute
			case "시간":
				d = time.Duration(n) * time.Hour
			case "일":
				d = time.Duration(n) * 24 * time.Hour
			case "주":
				d = time.Duration(n) * 7 * 24 * time.Hour
			case "개월":
				startTime = now.AddDate(0, -n, 0)
			case "년":
				startTime = now.AddDate(-n, 0, 0)
			}
			if unit != "개월" && unit != "년" {
				startTime = now.Add(-d)
			}
			label = m[0]
		}
	}

	if label == "" {
		return nil
	}

	// Select appropriate ROLLUP unit based on duration
	dur := now.Sub(startTime)
	rollupUnit := "'day'"
	switch {
	case dur <= 2*time.Hour:
		rollupUnit = "'sec'"
	case dur <= 24*time.Hour:
		rollupUnit = "'min'"
	case dur <= 7*24*time.Hour:
		rollupUnit = "'hour'"
	}

	return &TimeRangeResult{
		StartMs: strconv.FormatInt(startTime.UnixMilli(), 10),
		EndMs:   strconv.FormatInt(now.UnixMilli(), 10),
		StartDt: startTime.Format(fixer.DtFormat),
		EndDt:   now.Format(fixer.DtFormat),
		Label:   label,
		Unit:    rollupUnit,
	}
}
