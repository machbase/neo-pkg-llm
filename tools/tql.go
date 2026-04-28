package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func (r *Registry) registerTQLTools() {
	r.register(&Tool{
		Name:        "execute_tql_script",
		Description: "Execute a TQL script on Machbase Neo. Returns chart HTML or CSV data.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"tql_content":     {Type: "string", Description: "TQL script content"},
				"timeout_seconds": {Type: "integer", Description: "Execution timeout in seconds", Default: 60},
			},
			Required: []string{"tql_content"},
		},
		Fn: func(args map[string]any) (string, error) {
			tql := argStrAny(args, "", "tql_content", "script", "content")
			if tql == "" {
				return "", fmt.Errorf("tql_content is required")
			}
			result, err := r.client.ExecuteTQL(tql)
			if err != nil {
				return "", fmt.Errorf("execute_tql_script failed: %w", err)
			}

			trimmed := strings.TrimSpace(result)
			if trimmed == "" {
				return "TQL execution returned no data", nil
			}

			return result, nil
		},
	})

	r.register(&Tool{
		Name:        "validate_chart_tql",
		Description: "Validate TQL chart script for data existence and column reference errors.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"tql_script": {Type: "string", Description: "TQL script to validate"},
			},
			Required: []string{"tql_script"},
		},
		Fn: func(args map[string]any) (string, error) {
			tql := argStrAny(args, "", "tql_script", "script", "tql_content", "content")
			if tql == "" {
				return "", fmt.Errorf("tql_script is required")
			}
			result, err := r.client.ExecuteTQL(tql)
			if err != nil {
				return fmt.Sprintf("VALIDATION FAILED: %v", err), nil
			}
			if strings.TrimSpace(result) == "" {
				return "VALIDATION WARNING: TQL returned empty result", nil
			}
			return fmt.Sprintf("VALIDATION OK: TQL executed successfully (%d bytes output)", len(result)), nil
		},
	})

	r.register(&Tool{
		Name:        "save_tql_file",
		Description: "Save a TQL or SQL script file to Machbase Neo. TQL files are validated by execution before saving.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename":    {Type: "string", Description: "File path (e.g., 'GOLD/chart.tql')"},
				"tql_content": {Type: "string", Description: "TQL script content"},
			},
			Required: []string{"filename", "tql_content"},
		},
		Fn: func(args map[string]any) (string, error) {
			filename := argStrAny(args, "", "filename", "path", "file_path", "filepath", "name")
			tqlContent := argStrAny(args, "", "tql_content", "script", "content", "code")
			if filename == "" || tqlContent == "" {
				return "", fmt.Errorf("filename and tql_content are required")
			}

			// Ensure parent folder exists
			if idx := strings.Index(filename, "/"); idx > 0 {
				r.client.CreateFolder(filename[:idx])
			}

			isTQL := strings.HasSuffix(strings.ToLower(filename), ".tql")

			// Check for invalid ROLLUP units before execution
			if isTQL {
				lower := strings.ToLower(tqlContent)
				if strings.Contains(lower, "rollup('ms'") || strings.Contains(lower, "rollup(\"ms\"") {
					return "TQL validation failed (not saved): ROLLUP('ms')는 지원하지 않습니다. 최소 단위는 'sec'입니다. 지원 단위: 'sec', 'min', 'hour', 'day', 'week', 'month'", nil
				}
			}

			// Validate TQL by execution first
			if isTQL {
				testResult, err := r.client.ExecuteTQL(tqlContent)
				if err != nil {
					return fmt.Sprintf("TQL validation failed (not saved): %v", err), nil
				}
				trimmed := strings.TrimSpace(testResult)
				if trimmed == "" {
					return "TQL validation failed: execution returned empty result (not saved)", nil
				}

				// Check for ROLLUP column error → auto-create rollup and retry
				if strings.Contains(trimmed, "MACH-ERR 2264") || strings.Contains(trimmed, "not a ROLLUP column") {
					tableName := extractTableFromTQL(tqlContent)
					if tableName != "" {
						if created := r.createRollupForTable(tableName); created {
							// Retry execution after rollup creation
							testResult, err = r.client.ExecuteTQL(tqlContent)
							if err != nil {
								return fmt.Sprintf("TQL validation failed after rollup creation (not saved): %v", err), nil
							}
							trimmed = strings.TrimSpace(testResult)
							if trimmed == "" {
								return "TQL validation failed: execution returned empty result after rollup creation (not saved)", nil
							}
						}
					}
				}

				if strings.Contains(strings.ToLower(trimmed), "error") {
					var resp map[string]any
					if json.Unmarshal([]byte(trimmed), &resp) == nil {
						if errMsg, ok := resp["error"].(string); ok {
							return fmt.Sprintf("TQL validation failed: %s (not saved)", errMsg), nil
						}
					}
				}
			}

			// Save the file
			if err := r.client.WriteFile(filename, []byte(tqlContent)); err != nil {
				return fmt.Sprintf("File save failed: %v", err), nil
			}
			return fmt.Sprintf("File saved successfully: %s", filename), nil
		},
	})
}

// extractTableFromTQL extracts the table name from "FROM tablename" in TQL SQL content.
func extractTableFromTQL(tql string) string {
	re := regexp.MustCompile(`(?i)FROM\s+([A-Za-z_][A-Za-z0-9_]*)`)
	m := re.FindStringSubmatch(tql)
	if len(m) > 1 {
		return strings.ToUpper(m[1])
	}
	return ""
}

// createRollupForTable creates SEC/MIN/HOUR custom rollup tables and forces aggregation.
// Returns true if rollup was created successfully.
func (r *Registry) createRollupForTable(table string) bool {
	upper := strings.ToUpper(table)

	// Check if rollup already exists
	checkSQL := fmt.Sprintf("SELECT NAME FROM M$SYS_TABLES WHERE NAME LIKE '_%s_ROLLUP_%%'", upper)
	result, err := r.client.QuerySQL(checkSQL, "", "", "csv")
	if err == nil && strings.Contains(result, "_ROLLUP_") {
		return false // rollup already exists
	}

	type rollupStep struct {
		name string
		sql  string
	}

	secName := fmt.Sprintf("_%s_ROLLUP_SEC", upper)
	minName := fmt.Sprintf("_%s_ROLLUP_MIN", upper)
	hourName := fmt.Sprintf("_%s_ROLLUP_HOUR", upper)

	steps := []rollupStep{
		{secName, fmt.Sprintf("CREATE ROLLUP %s ON %s(VALUE) INTERVAL 1 SEC", secName, upper)},
		{minName, fmt.Sprintf("CREATE ROLLUP %s ON %s INTERVAL 1 MIN", minName, secName)},
		{hourName, fmt.Sprintf("CREATE ROLLUP %s ON %s INTERVAL 1 HOUR", hourName, minName)},
	}

	for _, s := range steps {
		r.client.QuerySQL(s.sql, "", "", "csv")
	}

	// Force aggregation
	for _, name := range []string{secName, minName, hourName} {
		r.client.QuerySQL(fmt.Sprintf("EXEC ROLLUP_FORCE('%s')", name), "", "", "csv")
	}

	fmt.Printf("[TQL] Auto-created rollup for table %s: %s, %s, %s\n", upper, secName, minName, hourName)
	return true
}
