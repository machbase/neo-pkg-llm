package tools

import (
	"neo-pkg-llm/machbase"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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
			timeout := argInt(args, "timeout_seconds", 60)

			result, err := r.client.ExecuteTQL(tql, time.Duration(timeout)*time.Second)
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
			// Basic validation: try executing the TQL
			result, err := r.client.ExecuteTQL(tql, 30*time.Second)
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
			filename := argStrAny(args, "", "filename", "path", "file_path", "name")
			tqlContent := argStrAny(args, "", "tql_content", "script", "content", "code")
			if filename == "" || tqlContent == "" {
				return "", fmt.Errorf("filename and tql_content are required")
			}

			// Ensure parent folder exists
			if idx := strings.Index(filename, "/"); idx > 0 {
				folder := filename[:idx]
				r.client.CreateFolder(folder)
			}

			isTQL := strings.HasSuffix(strings.ToLower(filename), ".tql")

			// Validate TQL by execution first
			if isTQL {
				testResult, err := r.client.ExecuteTQL(tqlContent, 30*time.Second)
				if err != nil {
					return fmt.Sprintf("TQL validation failed (not saved): %v", err), nil
				}
				trimmed := strings.TrimSpace(testResult)
				if trimmed == "" {
					return "TQL validation failed: execution returned empty result (not saved)", nil
				}
				// Check for error in response
				if strings.Contains(strings.ToLower(trimmed), "error") {
					var resp map[string]any
					if json.Unmarshal([]byte(trimmed), &resp) == nil {
						if errMsg, ok := resp["error"].(string); ok {
							return fmt.Sprintf("TQL validation failed: %s (not saved)", errMsg), nil
						}
					}
				}
			}

			// Save the file via Web API (always /web/api/files/)
			savePath := "/web/api/files/" + machbase.EscapePath(filename)

			respData, err := r.client.WebPostRaw(savePath, "text/plain", []byte(tqlContent))
			if err != nil {
				return fmt.Sprintf("File save failed: %v", err), nil
			}

			// Check API response for success
			var resp map[string]any
			if json.Unmarshal(respData, &resp) == nil {
				if success, ok := resp["success"].(bool); ok && !success {
					reason, _ := resp["reason"].(string)
					return fmt.Sprintf("File save failed: %s", reason), nil
				}
			}

			return fmt.Sprintf("File saved successfully: %s", filename), nil
		},
	})
}
