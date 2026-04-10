package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reportTemplateRE = regexp.MustCompile("(?s)###\\s*(R-\\d+)\\.[^\\n]*\\r?\\n.*?```html\\r?\\n(.*?)\\r?\\n```")
	reportTmplDir    string
)

func init() {
	const dirName = "report"
	candidates := []string{
		filepath.Join("neo", dirName),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{
			filepath.Join(filepath.Dir(exe), "neo", dirName),
		}, candidates...)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "neo", dirName))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			reportTmplDir, _ = filepath.Abs(c)
			fmt.Printf("[ReportTemplates] Found dir: %s\n", reportTmplDir)
			break
		}
	}
}

// LoadReportTemplates reads all report templates from neo/report/*.md every time (no cache).
func LoadReportTemplates() map[string]string {
	templates := map[string]string{}
	if reportTmplDir == "" {
		fmt.Println("[ReportTemplates] WARNING: report template directory not found")
		return templates
	}
	entries, err := os.ReadDir(reportTmplDir)
	if err != nil {
		fmt.Printf("[ReportTemplates] Failed to read dir: %v\n", err)
		return templates
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(reportTmplDir, entry.Name()))
		if err != nil {
			continue
		}
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		matches := reportTemplateRE.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			templates[m[1]] = strings.TrimSpace(m[2])
		}
	}
	if len(templates) > 0 {
		ids := make([]string, 0, len(templates))
		for id := range templates {
			ids = append(ids, id)
		}
		fmt.Printf("[ReportTemplates] Loaded %d templates: %v\n", len(templates), ids)
	}
	return templates
}

// ExpandReportTemplate substitutes placeholders in an HTML report template.
func ExpandReportTemplate(templateID string, params map[string]string) (string, error) {
	tmpl := LoadReportTemplates()
	code, ok := tmpl[templateID]
	if !ok {
		// Fallback to R-0 (범용)
		code, ok = tmpl["R-0"]
		if ok {
			fmt.Printf("[ReportTemplates] Template '%s' not found, falling back to R-0\n", templateID)
		}
	}
	if !ok {
		ids := make([]string, 0, len(tmpl))
		for id := range tmpl {
			ids = append(ids, id)
		}
		return "", fmt.Errorf("report template '%s' not found. available: %v", templateID, ids)
	}

	for key, val := range params {
		code = strings.ReplaceAll(code, "{"+key+"}", val)
	}

	unmatched := regexp.MustCompile(`\{(TABLE|GENERATED_DATE|TAG_COUNT|DATA_COUNT|TIME_RANGE|TAG_STATS_ROWS|CHART_DATA_JSON|TREND_DATA_JSON|TAG_LIST_JSON|PER_TAG_DATA_JSON|ROLLUP_LABEL|ANALYSIS|RECOMMENDATIONS)\}`).FindAllString(code, -1)
	if len(unmatched) > 0 {
		fmt.Printf("[ReportTemplates] WARNING: unsubstituted placeholders: %v\n", unmatched)
	}

	return code, nil
}
