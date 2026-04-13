package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (r *Registry) registerSQLTools() {
	r.register(&Tool{
		Name:        "list_tables",
		Description: "Query available table list in Machbase Neo.",
		Parameters: ToolParameters{
			Type:       "object",
			Properties: map[string]ToolProperty{},
		},
		Fn: func(args map[string]any) (string, error) {
			owner := strings.ToUpper(r.client.User)
			sql := fmt.Sprintf(
				"SELECT st.NAME FROM m$sys_tables AS st JOIN m$sys_users AS su ON st.USER_ID = su.USER_ID WHERE su.NAME = '%s' AND st.FLAG = 0 ORDER BY st.NAME",
				owner,
			)
			result, err := r.client.QuerySQL(sql, "", "", "csv")
			if err != nil {
				return "", fmt.Errorf("list_tables failed: %w", err)
			}
			return result, nil
		},
	})

	r.register(&Tool{
		Name:        "list_table_tags",
		Description: "Get tag metadata from _tablename_meta table. table_name is required.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"table_name": {Type: "string", Description: "Table name"},
			},
			Required: []string{"table_name"},
		},
		Fn: func(args map[string]any) (string, error) {
			table := argStrAny(args, "", "table_name", "table", "name")
			if table == "" {
				return "", fmt.Errorf("table_name is required")
			}
			if strings.ContainsAny(table, " \t\n\r") {
				return "", fmt.Errorf("invalid table name")
			}
			sql := fmt.Sprintf("SELECT NAME FROM _%s_meta", strings.ToLower(table))
			result, err := r.client.QuerySQL(sql, "", "", "csv")
			if err != nil {
				return "", fmt.Errorf("list_table_tags failed: %w", err)
			}
			return result, nil
		},
	})

	r.register(&Tool{
		Name:        "execute_sql_query",
		Description: "Execute SQL query directly on Machbase Neo.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"sql_query":  {Type: "string", Description: "SQL query to execute"},
				"format":     {Type: "string", Description: "Output format", Default: "csv"},
				"timeformat": {Type: "string", Description: "Time format", Default: "default"},
				"timezone":   {Type: "string", Description: "Timezone", Default: "Local"},
			},
			Required: []string{"sql_query"},
		},
		Fn: func(args map[string]any) (string, error) {
			sql := argStrAny(args, "", "sql_query", "sql", "query")
			if sql == "" {
				return "", fmt.Errorf("sql_query is required")
			}
			if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "UPDATE") {
				return "", fmt.Errorf("UPDATE 구문은 허용되지 않습니다")
			}
			format := argStr(args, "format", "csv")
			timeformat := argStr(args, "timeformat", "default")
			tz := argStr(args, "timezone", "Local")

			result, err := r.client.QuerySQL(sql, timeformat, tz, format)
			if err != nil {
				return "", fmt.Errorf("execute_sql_query failed: %w", err)
			}

			// Check for empty/error result
			trimmed := strings.TrimSpace(result)
			if trimmed == "" {
				return "No data returned", nil
			}

			// Parse JSON error response
			if strings.HasPrefix(trimmed, "{") {
				var resp map[string]any
				if json.Unmarshal([]byte(trimmed), &resp) == nil {
					if errMsg, ok := resp["error"].(string); ok {
						return fmt.Sprintf("SQL Error: %s", errMsg), nil
					}
				}
			}

			return result, nil
		},
	})
}
