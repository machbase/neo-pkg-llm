package tools

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"neo-pkg-llm/machbase"
)

// Dashboard grid constants (matching Python Machbase.py)
const (
	gridCols      = 36
	chartWLarge   = 17
	chartWSmall   = 7
	chartHDefault = 7
)

var largeChartTypes = map[string]bool{
	"Line": true, "Bar": true, "Scatter": true, "Adv scatter": true, "Tql chart": true,
}

var seriesColors = []string{
	"#5470c6", "#91cc75", "#fac858", "#ee6666", "#73c0de",
	"#3ba272", "#fc8452", "#9a60b4", "#ea7ccc", "#FADE2A",
}

func chartWidth(chartType string) int {
	if largeChartTypes[chartType] {
		return chartWLarge
	}
	return chartWSmall
}

// generateID creates a timestamp-based unique ID (matching Python _generate_id)
func generateID() string {
	return strconv.FormatInt(time.Now().UnixMicro(), 10)
}

// generatePanelID creates a UUID v4 string (matching Python _generate_panel_id)
func generatePanelID() string {
	var uuid [16]byte
	rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// parseTimeValue converts digit strings to int, else returns as-is (matching Python _parse_time_value)
func parseTimeValue(val string) any {
	if val == "" {
		return val
	}
	allDigit := true
	for _, r := range val {
		if !unicode.IsDigit(r) {
			allDigit = false
			break
		}
	}
	if allDigit {
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return val
}

// isEnglishFilename checks that filename contains only ASCII letters, digits, underscores, hyphens, dots, slashes.
func isEnglishFilename(filename string) bool {
	for _, r := range filename {
		if r > 127 {
			return false
		}
	}
	return true
}

func (r *Registry) registerDashboardTools() {
	r.register(&Tool{
		Name:        "list_dashboards",
		Description: "List all dashboards in Machbase Neo Web UI.",
		Parameters:  ToolParameters{Type: "object", Properties: map[string]ToolProperty{}},
		Fn:          r.listDashboards,
	})
	r.register(&Tool{
		Name:        "get_dashboard",
		Description: "Get a dashboard's full configuration.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename": {Type: "string", Description: "Dashboard filename"},
			},
			Required: []string{"filename"},
		},
		Fn: r.getDashboard,
	})
	r.register(&Tool{
		Name:        "create_dashboard",
		Description: "Create a new empty dashboard in Machbase Neo Web UI.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename":   {Type: "string", Description: "Dashboard path (e.g., 'BEARING/analysis.dsh')"},
				"title":      {Type: "string", Description: "Dashboard title", Default: "New dashboard"},
				"time_start": {Type: "string", Description: "Time range start", Default: "now-1h"},
				"time_end":   {Type: "string", Description: "Time range end", Default: "now"},
			},
			Required: []string{"filename"},
		},
		Fn: r.createDashboard,
	})
	r.register(&Tool{
		Name:        "create_dashboard_with_charts",
		Description: "Create a dashboard with multiple chart panels in a single call.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename":   {Type: "string", Description: "Dashboard path"},
				"title":      {Type: "string", Description: "Dashboard title", Default: "Dashboard"},
				"time_start": {Type: "string", Description: "Time range start", Default: "now-1h"},
				"time_end":   {Type: "string", Description: "Time range end", Default: "now"},
				"charts": {Type: "string", Description: `JSON array of chart definitions. Each object: {"title":"Chart Title","type":"Line|Bar|Scatter|Pie|Gauge","table":"TABLE_NAME","tag":"tag1,tag2","column":"VALUE","color":"#hex","tql_path":"path.tql"}`},
			},
			Required: []string{"filename"},
		},
		Fn: r.createDashboardWithCharts,
	})
	r.register(&Tool{
		Name:        "add_chart_to_dashboard",
		Description: "Add a chart panel to an existing dashboard.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename":    {Type: "string", Description: "Dashboard filename"},
				"chart_title": {Type: "string", Description: "Chart title", Default: "New chart"},
				"chart_type":  {Type: "string", Description: "Chart type (Line, Bar, Scatter, Pie, Gauge, Tql chart)", Default: "Line"},
				"table":       {Type: "string", Description: "Tag table name"},
				"tag":         {Type: "string", Description: "Tag name(s), comma-separated"},
				"column":      {Type: "string", Description: "Column name", Default: "VALUE"},
				"tql_path":    {Type: "string", Description: "TQL file path for TQL charts"},
				"color":       {Type: "string", Description: "Chart color hex", Default: "#367FEB"},
				"w":           {Type: "integer", Description: "Panel width (0=auto)", Default: 0},
				"h":           {Type: "integer", Description: "Panel height (0=default 7)", Default: 0},
			},
			Required: []string{"filename"},
		},
		Fn: r.addChartToDashboard,
	})
	r.register(&Tool{
		Name:        "remove_chart_from_dashboard",
		Description: "Remove a chart panel from a dashboard.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename":    {Type: "string", Description: "Dashboard filename"},
				"panel_id":    {Type: "string", Description: "Panel UUID to remove"},
				"panel_title": {Type: "string", Description: "Panel title to remove"},
			},
			Required: []string{"filename"},
		},
		Fn: r.removeChartFromDashboard,
	})
	r.register(&Tool{
		Name:        "update_chart_in_dashboard",
		Description: "Update an existing chart panel in a dashboard.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename":       {Type: "string", Description: "Dashboard filename"},
				"panel_id":       {Type: "string", Description: "Panel UUID"},
				"panel_title":    {Type: "string", Description: "Panel title (first match)"},
				"new_title":      {Type: "string", Description: "New panel title"},
				"new_chart_type": {Type: "string", Description: "New chart type"},
				"new_table":      {Type: "string", Description: "New table name"},
				"new_tag":        {Type: "string", Description: "New tag name(s)"},
				"new_column":     {Type: "string", Description: "New column name"},
				"new_color":      {Type: "string", Description: "New color"},
			},
			Required: []string{"filename"},
		},
		Fn: r.updateChartInDashboard,
	})
	r.register(&Tool{
		Name:        "delete_dashboard",
		Description: "Delete a dashboard file from Machbase Neo.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename": {Type: "string", Description: "Dashboard filename to delete"},
			},
			Required: []string{"filename"},
		},
		Fn: r.deleteDashboard,
	})
	r.register(&Tool{
		Name:        "update_dashboard_time_range",
		Description: "Update the time range of a dashboard.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename":   {Type: "string", Description: "Dashboard filename"},
				"time_start": {Type: "string", Description: "Start time", Default: "now-1h"},
				"time_end":   {Type: "string", Description: "End time", Default: "now"},
				"refresh":    {Type: "string", Description: "Auto-refresh interval", Default: "Off"},
			},
			Required: []string{"filename"},
		},
		Fn: r.updateDashboardTimeRange,
	})
	r.register(&Tool{
		Name:        "preview_dashboard",
		Description: "Get a dashboard preview: summary and direct Neo Web UI link.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename": {Type: "string", Description: "Dashboard filename"},
			},
			Required: []string{"filename"},
		},
		Fn: r.previewDashboard,
	})
}

// --- Dashboard implementations ---

func (r *Registry) listDashboards(args map[string]any) (string, error) {
	var dashboards []string
	entries, err := r.client.ListDir("/")
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e["type"] == "dir" {
			subEntries, _ := r.client.ListDir(e["name"])
			for _, se := range subEntries {
				if strings.HasSuffix(se["name"], ".dsh") {
					dashboards = append(dashboards, e["name"]+"/"+se["name"])
				}
			}
		}
		if strings.HasSuffix(e["name"], ".dsh") {
			dashboards = append(dashboards, e["name"])
		}
	}
	if len(dashboards) == 0 {
		return "No dashboards found", nil
	}
	return "Dashboards:\n" + strings.Join(dashboards, "\n"), nil
}


func (r *Registry) getDashboard(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}
	raw, err := r.loadDashFile(filename)
	if err != nil {
		return "", err
	}

	panels := getDashPanels(raw)
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Dashboard: %s\nPanels: %d\n", filename, len(panels)))
	for i, p := range panels {
		panel, _ := p.(map[string]any)
		if panel == nil {
			continue
		}
		id, _ := panel["id"].(string)
		title, _ := panel["title"].(string)
		pType, _ := panel["type"].(string)

		// Extract tags from blockList
		var tags []string
		if bl, ok := panel["blockList"].([]any); ok {
			for _, b := range bl {
				if block, ok := b.(map[string]any); ok {
					if tag, ok := block["tag"].(string); ok && tag != "" {
						tags = append(tags, tag)
					}
				}
			}
		}
		tagStr := ""
		if len(tags) > 0 {
			tagStr = fmt.Sprintf(" tags=%s", strings.Join(tags, ","))
		}
		result.WriteString(fmt.Sprintf("  %d. [%s] \"%s\" (id: %s)%s\n", i+1, pType, title, id, tagStr))
	}
	return strings.TrimSpace(result.String()), nil
}

func (r *Registry) createDashboard(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	title := argStr(args, "title", "New dashboard")
	timeStart := argStr(args, "time_start", "now-1h")
	timeEnd := argStr(args, "time_end", "now")

	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}
	if !isEnglishFilename(filename) {
		return "", fmt.Errorf("filename must contain only English characters: %s", filename)
	}

	ensureParentFolder(r.client, filename)

	dshFile := buildDSHFile(filename, title, timeStart, timeEnd, nil)
	return r.saveDashFile(filename, dshFile)
}

func (r *Registry) createDashboardWithCharts(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	title := argStr(args, "title", "Dashboard")
	timeStart := argStr(args, "time_start", "now-1h")
	timeEnd := argStr(args, "time_end", "now")
	chartsStr := argStr(args, "charts", "[]")

	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}
	if !isEnglishFilename(filename) {
		return "", fmt.Errorf("filename must contain only English characters: %s", filename)
	}

	var charts []map[string]any
	if err := json.Unmarshal([]byte(chartsStr), &charts); err != nil {
		return "", fmt.Errorf("invalid charts JSON: %w", err)
	}

	// Normalize LLM parameter variations (chart_type→type, "Line chart"→"Line", etc.)
	normalizeCharts(charts)

	// First pass: find any table name
	var firstTable string
	for _, c := range charts {
		if t, _ := c["table"].(string); t != "" {
			firstTable = t
			break
		}
	}

	// Second pass: auto-fill missing table + validate
	for i, c := range charts {
		tql, _ := c["tql_path"].(string)
		if tql != "" {
			continue
		}
		table, _ := c["table"].(string)
		if table == "" && firstTable != "" {
			c["table"] = firstTable
			table = firstTable
		}
		tag, _ := c["tag"].(string)
		if table == "" || tag == "" {
			return "", fmt.Errorf("chart[%d] missing required 'table' and/or 'tag'. Each chart must have: {\"title\":\"...\",\"type\":\"Line\",\"table\":\"TABLE_NAME\",\"tag\":\"tag_name\"}", i)
		}
		if firstTable == "" {
			firstTable = table
		}
	}

	// Auto-detect time range if relative values (now-*) and table is known
	if firstTable != "" && strings.Contains(timeStart, "now") {
		timeStart, timeEnd = r.detectTimeRange(firstTable, timeStart, timeEnd)
	}

	ensureParentFolder(r.client, filename)

	panels := buildPanelsFromCharts(charts)
	for _, p := range panels {
		if panel, ok := p.(map[string]any); ok {
			r.fillTableInfo(panel)
		}
	}
	dshFile := buildDSHFile(filename, title, timeStart, timeEnd, panels)
	return r.saveDashFile(filename, dshFile)
}

func (r *Registry) addChartToDashboard(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}

	raw, err := r.loadDashFile(filename)
	if err != nil {
		return "", err
	}

	panels := getDashPanels(raw)

	// Calculate auto-position based on existing panels
	cType := argStr(args, "chart_type", "Line")
	if argStr(args, "tql_path", "") != "" {
		cType = "Tql chart"
	}
	w := argInt(args, "w", 0)
	if w <= 0 {
		w = chartWidth(cType)
	}
	x, y := calculateNextPosition(panels, w)

	panel := makeChartPanel(
		argStr(args, "chart_title", "New chart"),
		cType,
		argStr(args, "table", ""),
		argStr(args, "tag", ""),
		argStr(args, "column", "VALUE"),
		argStr(args, "color", "#367FEB"),
		argStr(args, "tql_path", ""),
		x, y, w,
		argInt(args, "h", 0),
	)
	r.fillTableInfo(panel)
	panels = append(panels, panel)
	setDashPanels(raw, panels)

	_, err = r.saveDashFile(filename, raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Chart added to dashboard: %s", argStr(args, "chart_title", "New chart")), nil
}

func (r *Registry) removeChartFromDashboard(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	panelID := argStr(args, "panel_id", "")
	panelTitle := argStr(args, "panel_title", "")
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}

	raw, err := r.loadDashFile(filename)
	if err != nil {
		return "", err
	}

	panels := getDashPanels(raw)
	var newPanels []any
	removed := false
	for _, p := range panels {
		panel, _ := p.(map[string]any)
		if panel == nil {
			continue
		}
		if panelID != "" && panel["id"] == panelID {
			removed = true
			continue
		}
		if panelTitle != "" && panel["title"] == panelTitle && !removed {
			removed = true
			continue
		}
		newPanels = append(newPanels, p)
	}
	setDashPanels(raw, newPanels)
	r.saveDashFile(filename, raw)

	if removed {
		return "Chart removed successfully", nil
	}
	return "No matching chart found", nil
}

func (r *Registry) updateChartInDashboard(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}

	raw, err := r.loadDashFile(filename)
	if err != nil {
		return "", err
	}

	panels := getDashPanels(raw)
	panelID := argStr(args, "panel_id", "")
	panelTitle := argStr(args, "panel_title", "")
	updated := false

	for i, p := range panels {
		panel, ok := p.(map[string]any)
		if !ok {
			continue
		}
		match := false
		if panelID != "" && panel["id"] == panelID {
			match = true
		} else if panelTitle != "" && panel["title"] == panelTitle {
			match = true
		}
		if match {
			if v := argStr(args, "new_title", ""); v != "" {
				panel["title"] = v
			}
			if v := argStr(args, "new_chart_type", ""); v != "" {
				panel["type"] = v
				panel["chartInfo"] = getChartTypeDefaults(v)
				panel["chartOptions"] = getChartTypeDefaults(v)
			}
			if argStr(args, "new_table", "") != "" {
				r.fillTableInfo(panel)
			}
			panels[i] = panel
			updated = true
			break
		}
	}
	setDashPanels(raw, panels)
	r.saveDashFile(filename, raw)

	if updated {
		return "Chart updated successfully", nil
	}
	return "No matching chart found", nil
}

func (r *Registry) deleteDashboard(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}

	if err := r.client.DeleteFile(filename); err != nil {
		return "", err
	}
	return fmt.Sprintf("Dashboard deleted: %s", filename), nil
}

func (r *Registry) updateDashboardTimeRange(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}
	timeStart := argStr(args, "time_start", "now-1h")
	timeEnd := argStr(args, "time_end", "now")
	refresh := argStr(args, "refresh", "Off")

	raw, err := r.loadDashFile(filename)
	if err != nil {
		return "", err
	}

	raw["range_bgn"] = timeStart
	raw["range_end"] = timeEnd
	if dash, ok := raw["dashboard"].(map[string]any); ok {
		dash["timeRange"] = map[string]any{
			"start":   parseTimeValue(timeStart),
			"end":     parseTimeValue(timeEnd),
			"refresh": refresh,
		}
	}
	r.saveDashFile(filename, raw)

	return fmt.Sprintf("Time range updated: %s ~ %s", timeStart, timeEnd), nil
}

func (r *Registry) previewDashboard(args map[string]any) (string, error) {
	filename := fixDashFilename(argStr(args, "filename", ""))
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}

	raw, err := r.loadDashFile(filename)
	if err != nil {
		return "", err
	}

	panels := getDashPanels(raw)
	title, _ := raw["name"].(string)
	if title == "" {
		title = filename
	}

	result := fmt.Sprintf("Dashboard: %s\nPanels: %d\n", filename, len(panels))
	for i, p := range panels {
		panel, _ := p.(map[string]any)
		if panel == nil {
			continue
		}
		pTitle, _ := panel["title"].(string)
		pType, _ := panel["type"].(string)
		result += fmt.Sprintf("  %d. [%s] %s\n", i+1, pType, pTitle)
	}

	boardPath := strings.TrimSuffix(filename, ".dsh")
	dashURL := r.client.BaseURL + "/web/ui/board/" + boardPath
	result += fmt.Sprintf("\nDashboard URL: %s", dashURL)

	return result, nil
}

// --- Helper functions ---

// fixDashFilename ensures the filename has a .dsh extension.
func fixDashFilename(filename string) string {
	if filename == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".dsh") {
		filename = filename + ".dsh"
	}
	return filename
}

func ensureParentFolder(client *machbase.Client, filename string) {
	if idx := strings.LastIndex(filename, "/"); idx > 0 {
		client.CreateFolder(filename[:idx])
	}
}

// loadDashFile loads a .dsh file and returns the raw file content.
func (r *Registry) loadDashFile(filename string) (map[string]any, error) {
	data, err := r.client.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load dashboard: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard: %w", err)
	}
	return raw, nil
}

// saveDashFile saves the complete .dsh file content.
func (r *Registry) saveDashFile(filename string, content map[string]any) (string, error) {
	dashData, _ := json.Marshal(content)
	if err := r.client.WriteFile(filename, dashData); err != nil {
		return "", fmt.Errorf("save dashboard failed: %w", err)
	}
	return fmt.Sprintf("Dashboard saved: %s", filename), nil
}

// getDashPanels extracts panels from the dashboard.panels path.
func getDashPanels(raw map[string]any) []any {
	if dash, ok := raw["dashboard"].(map[string]any); ok {
		if panels, ok := dash["panels"].([]any); ok {
			return panels
		}
	}
	return nil
}

// setDashPanels sets panels at dashboard.panels.
func setDashPanels(raw map[string]any, panels []any) {
	dash, ok := raw["dashboard"].(map[string]any)
	if !ok {
		dash = map[string]any{}
		raw["dashboard"] = dash
	}
	if panels == nil {
		panels = []any{}
	}
	dash["panels"] = panels
}

// calculateNextPosition finds the next auto-layout position (matching Python _calculate_next_position)
func calculateNextPosition(existingPanels []any, neededW int) (int, int) {
	if len(existingPanels) == 0 {
		return 0, 0
	}

	maxBottom := 0
	lastRowY := 0

	for _, p := range existingPanels {
		panel, ok := p.(map[string]any)
		if !ok {
			continue
		}
		py := intFromAny(panel["y"])
		ph := intFromAny(panel["h"])
		if ph == 0 {
			ph = chartHDefault
		}
		bottom := py + ph
		if bottom > maxBottom {
			maxBottom = bottom
		}
		if py > lastRowY {
			lastRowY = py
		}
	}

	lastRowRight := 0
	for _, p := range existingPanels {
		panel, ok := p.(map[string]any)
		if !ok {
			continue
		}
		py := intFromAny(panel["y"])
		if py == lastRowY {
			px := intFromAny(panel["x"])
			pw := intFromAny(panel["w"])
			if pw == 0 {
				pw = chartWLarge
			}
			right := px + pw
			if right > lastRowRight {
				lastRowRight = right
			}
		}
	}

	if lastRowRight+neededW <= gridCols {
		return lastRowRight, lastRowY
	}
	return 0, maxBottom
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// buildDSHFile creates a complete .dsh file structure matching Python _make_empty_dashboard exactly.
func buildDSHFile(filename, title, timeStart, timeEnd string, panels []any) map[string]any {
	if panels == nil {
		panels = []any{}
	}

	name := path.Base(filename)
	dir := path.Dir(filename)
	if dir == "." {
		dir = "/"
	} else {
		dir = "/" + dir + "/"
	}

	return map[string]any{
		"id":        generateID(),
		"type":      "dsh",
		"name":      name,
		"path":      dir,
		"code":      "",
		"panels":    []any{},
		"range_bgn": "",
		"range_end": "",
		"savedCode": false,
		"sheet":     []any{},
		"shell": map[string]any{
			"icon":  "dashboard",
			"theme": "",
			"id":    "DSH",
		},
		"dashboard": map[string]any{
			"variables": []any{},
			"timeRange": map[string]any{
				"start":   parseTimeValue(timeStart),
				"end":     parseTimeValue(timeEnd),
				"refresh": "Off",
			},
			"title":  title,
			"panels": panels,
		},
	}
}

// makeBlock creates a single blockList entry (matching Python _make_block)
func makeBlock(table, tag, column, color, userName, aggregator string) map[string]any {
	if column == "" {
		column = "VALUE"
	}
	if color == "" {
		color = "#367FEB"
	}
	if userName == "" {
		userName = "sys"
	}
	if aggregator == "" {
		aggregator = "value"
	}

	return map[string]any{
		"id":       generatePanelID(),
		"table":    table,
		"userName": userName,
		"color":    color,
		"type":     "tag",
		"filter": []any{
			map[string]any{
				"id":          generatePanelID(),
				"column":      "NAME",
				"operator":    "in",
				"value":       tag,
				"useFilter":   true,
				"useTyping":   false,
				"typingValue": fmt.Sprintf(`NAME in ("%s")`, tag),
			},
		},
		"values": []any{
			map[string]any{
				"id":         generatePanelID(),
				"alias":      "",
				"value":      column,
				"aggregator": aggregator,
			},
		},
		"useRollup":    false,
		"name":         "NAME",
		"time":         "TIME",
		"useCustom":    false,
		"aggregator":   aggregator,
		"diff":         "none",
		"tag":          tag,
		"value":        column,
		"alias":        "",
		"math":         "",
		"isValidMath":  true,
		"duration":     map[string]any{"from": "", "to": ""},
		"customFullTyping": map[string]any{"use": false, "text": ""},
		"isVisible":    true,
		"tableInfo":    []any{},
	}
}

// getChartTypeDefaults returns default chart options for a given chart type (matching Python _CHART_TYPE_DEFAULTS)
func getChartTypeDefaults(chartType string) map[string]any {
	switch chartType {
	case "Line":
		return map[string]any{
			"areaStyle": false, "smooth": false, "isStep": false, "isStack": false,
			"connectNulls": true, "isSymbol": false, "symbol": "circle", "symbolSize": 4,
			"isSampling": false, "fillOpacity": 0.3, "tagLimit": 12,
			"markLine": map[string]any{
				"symbol": []any{"none", "none"},
				"label":  map[string]any{"show": false},
				"data":   []any{},
			},
			"visualMap": map[string]any{
				"type": "piecewise", "show": false, "dimension": 0,
				"seriesIndex": 0, "pieces": []any{},
			},
		}
	case "Bar":
		return map[string]any{
			"isStack": false, "isLarge": false, "isPolar": false, "polarRadius": 30,
			"polarSize": 80, "startAngle": 90, "maxValue": 100, "tagLimit": 12, "polarAxis": "time",
		}
	case "Scatter":
		return map[string]any{
			"isLarge": false, "symbol": "circle", "symbolSize": 4, "tagLimit": 12,
		}
	case "Pie":
		return map[string]any{
			"doughnutRatio": 50, "roseType": false, "tagLimit": 12,
		}
	case "Gauge":
		return map[string]any{
			"isAxisTick": true, "axisLabelDistance": 25, "valueFontSize": 15,
			"valueAnimation": false, "alignCenter": 30, "isAnchor": true, "anchorSize": 25,
			"min": 0, "max": 100, "tagLimit": 1, "digit": 0,
			"axisLineStyleWidth": 10, "isAxisLineStyleColor": false,
			"axisLineStyleColor": []any{
				[]any{0.5, "#c2c2c2"}, []any{1, "#F44E3B"},
			},
		}
	case "Tql chart":
		return map[string]any{
			"theme": "white",
		}
	default:
		return map[string]any{
			"areaStyle": false, "smooth": false, "isStep": false, "isStack": false,
			"connectNulls": true, "isSymbol": false, "symbol": "circle", "symbolSize": 4,
			"isSampling": false, "fillOpacity": 0.3, "tagLimit": 12,
			"markLine": map[string]any{
				"symbol": []any{"none", "none"},
				"label":  map[string]any{"show": false},
				"data":   []any{},
			},
			"visualMap": map[string]any{
				"type": "piecewise", "show": false, "dimension": 0,
				"seriesIndex": 0, "pieces": []any{},
			},
		}
	}
}

// makeChartPanel creates a complete chart panel matching Python _make_chart_panel exactly.
func makeChartPanel(title, chartType, table, tag, column, color, tqlPath string, x, y, w, h int) map[string]any {
	panelID := generatePanelID()

	if tqlPath != "" {
		chartType = "Tql chart"
	}
	if w <= 0 {
		w = chartWidth(chartType)
	}
	if h <= 0 {
		h = chartHDefault
	}

	chartOptions := getChartTypeDefaults(chartType)

	// Determine aggregator based on chart type (matching Python: default "value" for raw data)
	agg := "value"
	switch chartType {
	case "Pie":
		agg = "count"
	case "Gauge", "Liquid fill":
		agg = "last"
	}

	panel := map[string]any{
		"id":         panelID,
		"title":      title,
		"titleColor": "",
		"type":       chartType,
		"x":          x,
		"y":          y,
		"w":          w,
		"h":          h,
		"theme":      "white",
		"useCustomTime":   false,
		"isAxisInterval":  false,
		"timeRange": map[string]any{
			"start":   "",
			"end":     "",
			"refresh": "Off",
		},
		"blockList":          []any{},
		"transformBlockList": []any{},
		"chartInfo":          chartOptions,
		"chartOptions":       getChartTypeDefaults(chartType),
		"commonOptions": map[string]any{
			"isLegend":        true,
			"legendTop":       "bottom",
			"legendLeft":      "center",
			"legendOrient":    "horizontal",
			"isTooltip":       true,
			"tooltipTrigger":  "axis",
			"tooltipBgColor":  "#FFFFFF",
			"tooltipTxtColor": "#333",
			"tooltipDecimals": 3,
			"tooltipUnit":     "",
			"isInsideTitle":   true,
			"isDataZoom":      false,
			"title":           title,
			"gridTop":         50,
			"gridBottom":      50,
			"gridLeft":        35,
			"gridRight":       35,
		},
		"xAxisOptions": []any{
			map[string]any{
				"type":      "time",
				"axisTick":  map[string]any{"alignWithLabel": true},
				"axisLabel": map[string]any{"hideOverlap": true},
				"axisLine":  map[string]any{"onZero": false},
				"scale":     true,
				"useMinMax": false,
				"useBlockList": []any{0},
				"label": map[string]any{
					"name": "value", "key": "value", "title": "",
					"unit": "", "decimals": nil, "squared": 0,
				},
			},
		},
		"yAxisOptions": []any{
			map[string]any{
				"type":       "value",
				"position":   "left",
				"offset":     "",
				"alignTicks": true,
				"scale":      true,
				"useMinMax":  false,
				"axisLine":   map[string]any{"onZero": false},
				"label": map[string]any{
					"name": "value", "key": "value", "title": "",
					"unit": "", "decimals": nil, "squared": 0,
				},
			},
		},
		"axisInterval": map[string]any{"IntervalType": "", "IntervalValue": ""},
	}

	// TQL chart: set tqlInfo + default block
	if tqlPath != "" {
		if !strings.HasPrefix(tqlPath, "/") {
			tqlPath = "/" + tqlPath
		}
		panel["tqlInfo"] = map[string]any{
			"path": tqlPath,
			"params": []any{
				map[string]any{"name": "", "value": "", "format": ""},
			},
			"chart_id": "",
		}
		panel["blockList"] = []any{makeBlock("", "", "VALUE", color, "sys", "avg")}
	} else if table != "" && tag != "" {
		// Table-based: build blockList with multi-tag support
		tags := strings.Split(tag, ",")
		blocks := make([]any, 0, len(tags))
		for i, t := range tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			c := color
			if len(tags) > 1 {
				c = seriesColors[i%len(seriesColors)]
			}
			blocks = append(blocks, makeBlock(table, t, column, c, "sys", agg))
		}
		panel["blockList"] = blocks
	}

	return panel
}

// detectTimeRange queries MIN/MAX TIME from a table and returns epoch ms strings.
// Falls back to the original values on any error.
func (r *Registry) detectTimeRange(table, origStart, origEnd string) (string, string) {
	rawResp, err := r.client.QuerySQL(
		"SELECT MIN(TIME), MAX(TIME) FROM "+table,
		"ms", "", "",
	)
	if err != nil {
		return origStart, origEnd
	}
	var resp struct {
		Data struct {
			Rows [][]any `json:"rows"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(rawResp), &resp) != nil || len(resp.Data.Rows) == 0 || len(resp.Data.Rows[0]) < 2 {
		return origStart, origEnd
	}
	minT := anyToString(resp.Data.Rows[0][0])
	maxT := anyToString(resp.Data.Rows[0][1])
	if minT == "" || maxT == "" {
		return origStart, origEnd
	}
	return minT, maxT
}

func anyToString(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case int64:
		return strconv.FormatInt(n, 10)
	case string:
		return n
	case json.Number:
		return n.String()
	}
	return fmt.Sprintf("%v", v)
}

// fillTableInfo queries column metadata from Machbase and fills tableInfo in each blockList entry.
// Without tableInfo, Machbase Neo dashboard viewer cannot render table-based charts.
func (r *Registry) fillTableInfo(panel map[string]any) {
	blockList, ok := panel["blockList"].([]any)
	if !ok || len(blockList) == 0 {
		return
	}
	firstBlock, _ := blockList[0].(map[string]any)
	if firstBlock == nil {
		return
	}
	table, _ := firstBlock["table"].(string)
	if table == "" {
		return
	}

	rawResp, err := r.client.QuerySQL("SELECT * FROM "+table+" LIMIT 0", "", "", "")
	if err != nil {
		return
	}

	var resp struct {
		Data struct {
			Columns []string `json:"columns"`
			Types   []string `json:"types"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(rawResp), &resp) != nil || len(resp.Data.Columns) == 0 {
		return
	}

	typeMap := map[string]int{
		"string": 5, "varchar": 5, "datetime": 6,
		"double": 20, "float": 16, "int32": 8, "int64": 12,
	}
	sizeMap := map[string]int{
		"string": 32, "varchar": 32, "datetime": 8,
		"double": 8, "float": 4, "int32": 4, "int64": 8,
	}

	var tableInfo []any
	for i, col := range resp.Data.Columns {
		t := "string"
		if i < len(resp.Data.Types) {
			t = resp.Data.Types[i]
		}
		typeNum := 5
		if v, ok := typeMap[t]; ok {
			typeNum = v
		}
		sizeNum := 32
		if v, ok := sizeMap[t]; ok {
			sizeNum = v
		}
		tableInfo = append(tableInfo, []any{col, typeNum, sizeNum, i})
	}
	tableInfo = append(tableInfo, []any{"_RID", 12, 8, 65534})

	for _, bl := range blockList {
		if block, ok := bl.(map[string]any); ok {
			block["tableInfo"] = tableInfo
		}
	}
}

// normalizeCharts fixes common LLM parameter naming variations in chart definitions.
func normalizeCharts(charts []map[string]any) {
	for _, c := range charts {
		// chart_type → type
		if _, ok := c["type"]; !ok {
			if ct, ok := c["chart_type"]; ok {
				c["type"] = ct
				delete(c, "chart_type")
			}
		}
		// "Line chart" → "Line" etc.
		if t, ok := c["type"].(string); ok {
			c["type"] = normalizeChartType(t)
		}
		// table_name → table
		if _, ok := c["table"]; !ok {
			if tn, ok := c["table_name"]; ok {
				c["table"] = tn
				delete(c, "table_name")
			}
		}
		// tag_name / tags → tag
		if _, ok := c["tag"]; !ok {
			for _, alias := range []string{"tag_name", "tags", "tag_names"} {
				if v, ok := c[alias]; ok {
					c["tag"] = v
					delete(c, alias)
					break
				}
			}
		}
	}
}

func normalizeChartType(t string) string {
	low := strings.ToLower(strings.TrimSpace(t))
	switch {
	case strings.Contains(low, "line"):
		return "Line"
	case strings.Contains(low, "bar"):
		return "Bar"
	case strings.Contains(low, "scatter"):
		return "Scatter"
	case strings.Contains(low, "pie"):
		return "Pie"
	case strings.Contains(low, "gauge"):
		return "Gauge"
	case strings.Contains(low, "tql"):
		return "Tql chart"
	}
	return t
}

// buildPanelsFromCharts builds panels from chart definitions with auto-layout.
func buildPanelsFromCharts(charts []map[string]any) []any {
	panels := make([]any, 0, len(charts))
	x, y := 0, 0
	for _, chart := range charts {
		cType, _ := chart["type"].(string)
		if cType == "" {
			cType = "Line"
		}
		tqlPath, _ := chart["tql_path"].(string)
		if tqlPath != "" {
			cType = "Tql chart"
		}

		w := chartWidth(cType)
		if x+w > gridCols {
			x = 0
			y += chartHDefault
		}

		title, _ := chart["title"].(string)
		table, _ := chart["table"].(string)
		tag, _ := chart["tag"].(string)
		column, _ := chart["column"].(string)
		if column == "" {
			column = "VALUE"
		}
		color, _ := chart["color"].(string)
		if color == "" {
			color = "#367FEB"
		}

		panel := makeChartPanel(title, cType, table, tag, column, color, tqlPath, x, y, w, 0)
		panels = append(panels, panel)
		x += w
	}
	return panels
}
