package tools

import "fmt"

func (r *Registry) registerUtilTools() {
	r.register(&Tool{
		Name:        "get_version",
		Description: "Get version information of the Machbase Neo server.",
		Parameters:  ToolParameters{Type: "object", Properties: map[string]ToolProperty{}},
		Fn: func(args map[string]any) (string, error) {
			return fmt.Sprintf("Agentic Loop Go Backend v1.0.0 / Machbase Neo at %s", r.client.BaseURL), nil
		},
	})

	r.register(&Tool{
		Name:        "debug_mcp_status",
		Description: "Check current status and connectivity of the backend.",
		Parameters:  ToolParameters{Type: "object", Properties: map[string]ToolProperty{}},
		Fn: func(args map[string]any) (string, error) {
			// Quick health check: try listing tables
			result, err := r.client.QuerySQL("SELECT COUNT(*) FROM M$SYS_TABLES", "", "", "csv")
			if err != nil {
				return fmt.Sprintf("Machbase Neo connection FAILED: %v", err), nil
			}
			return fmt.Sprintf("Status: OK\nMachbase: %s\n%s", r.client.BaseURL, result), nil
		},
	})

	r.register(&Tool{
		Name:        "update_connection",
		Description: "Update Machbase Neo connection settings at runtime. Only provided fields are changed; omitted fields keep current values.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"host":     {Type: "string", Description: "Machbase Neo host (e.g., '192.168.1.100')"},
				"port":     {Type: "string", Description: "Machbase Neo port (e.g., '5654')"},
				"user":     {Type: "string", Description: "User name"},
				"password": {Type: "string", Description: "Password"},
			},
		},
		Fn: func(args map[string]any) (string, error) {
			host := argStr(args, "host", "")
			port := argStr(args, "port", "")
			user := argStr(args, "user", "")
			password := argStr(args, "password", "")

			var baseURL string
			if host != "" {
				if port == "" {
					port = "5654"
				}
				baseURL = "http://" + host + ":" + port
			}

			r.client.UpdateConnection(baseURL, user, password)

			// 연결 확인
			result, err := r.client.QuerySQL("SELECT COUNT(*) FROM M$SYS_TABLES", "", "", "csv")
			if err != nil {
				return fmt.Sprintf("Connection updated but verification FAILED: %v\nCurrent: %s", err, r.client.BaseURL), nil
			}
			return fmt.Sprintf("Connection updated successfully.\nMachbase: %s\nUser: %s\n%s", r.client.BaseURL, r.client.User, result), nil
		},
	})
}
