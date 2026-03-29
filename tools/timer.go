package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (r *Registry) registerTimerTools() {

	// ── list_timers ──────────────────────────────────────────────
	r.register(&Tool{
		Name:        "list_timers",
		Description: "List all timers (schedulers) registered in Machbase Neo. Returns name, state (RUNNING/STOP), schedule, and TQL path for each timer. Use this tool when the user asks about timers, timer list, timer status, or scheduled tasks. This is different from list_tables which lists database tables.",
		Parameters:  ToolParameters{Type: "object", Properties: map[string]ToolProperty{}},
		Fn: func(args map[string]any) (string, error) {
			data, err := r.client.WebGet("/web/api/timers")
			if err != nil {
				return "", fmt.Errorf("list timers failed: %w", err)
			}

			var resp struct {
				Success bool              `json:"success"`
				Reason  string            `json:"reason"`
				Data    []json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", fmt.Errorf("parse response failed: %w", err)
			}
			if !resp.Success {
				return fmt.Sprintf("Error: %s", resp.Reason), nil
			}

			if len(resp.Data) == 0 {
				return "No timers registered.", nil
			}

			out, _ := json.MarshalIndent(resp.Data, "", "  ")
			return string(out), nil
		},
	})

	// ── add_timer ────────────────────────────────────────────────
	r.register(&Tool{
		Name: "add_timer",
		Description: `Create a new timer (scheduler) in Machbase Neo that runs a TQL script on a schedule.

Schedule formats: CRON ("0 30 * * * *"), interval ("@every 5s", "@every 10ms"), or predefined ("@daily", "@hourly").
Valid time units for @every: "ms", "s", "m", "h".

NAMING RULE: Use the SAME name for timer, table, and TQL folder. Example: if the user asks for "sensor data", use NAME=SENSOR_DATA everywhere: table=SENSOR_DATA, TQL path=SENSOR_DATA/SENSOR_DATA.tql, timer=SENSOR_DATA. Derive the name from what the user requests.

IMPORTANT workflow - you MUST follow these steps in order:
1) Create the target TAG TABLE using execute_sql_query.
   TAG TABLE syntax: CREATE TAG TABLE IF NOT EXISTS NAME (name VARCHAR(80) PRIMARY KEY, time DATETIME BASETIME, value DOUBLE SUMMARIZED) WITH ROLLUP
2) Create the TQL script using save_tql_file. TQL is NOT SQL. Never use INSERT INTO, SELECT, or any SQL statements in TQL files.
   TQL uses a pipeline: SRC -> MAP -> SINK. Only ONE SRC and ONE SINK (INSERT) per file.
   Data generation TQL example (100 records/sec for one sensor, timer @every 1s):
   FAKE( oscillator( freq(3, 1.0), range("now-1s", "1s", "10ms") ))
   PUSHVALUE(0, 'sensor-01')
   INSERT("name", "time", "value", table("NAME"))
   For multiple sensors, create separate TQL files and timers per sensor.
   Save as: NAME/NAME.tql
3) Call add_timer with name=NAME, path=NAME/NAME.tql
4) Call start_timer to begin execution. Creating a timer does NOT start it automatically.

To clean up: stop_timer(NAME) -> delete_timer(NAME) -> delete TQL folder(NAME/) -> DROP TABLE NAME CASCADE;`,
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"name":       {Type: "string", Description: "Timer name (unique identifier)"},
				"schedule":   {Type: "string", Description: "Execution schedule. Examples: \"@every 10s\", \"0 30 * * * *\", \"@daily\""},
				"path":       {Type: "string", Description: "TQL script path to execute (e.g., \"timer.tql\")"},
				"auto_start": {Type: "boolean", Description: "Auto-start when Machbase Neo server restarts. Always set to false unless the user explicitly requests auto-start.", Default: false},
			},
			Required: []string{"name", "schedule", "path"},
		},
		Fn: func(args map[string]any) (string, error) {
			name := argStr(args, "name", "")
			schedule := argStr(args, "schedule", "")
			path := argStr(args, "path", "")
			autoStart := argBool(args, "auto_start", false)

			if name == "" || schedule == "" || path == "" {
				return "", fmt.Errorf("name, schedule, and path are required")
			}

			payload := map[string]any{
				"name":      name,
				"schedule":  schedule,
				"path":      path,
				"autoStart": autoStart,
			}

			data, err := r.client.WebPost("/web/api/timers", payload)
			if err != nil {
				return "", fmt.Errorf("add timer failed: %w", err)
			}

			var resp struct {
				Success bool   `json:"success"`
				Reason  string `json:"reason"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", fmt.Errorf("parse response failed: %w", err)
			}
			if !resp.Success {
				return fmt.Sprintf("Error: %s", resp.Reason), nil
			}

			return fmt.Sprintf("Timer '%s' created successfully. (schedule: %s, path: %s, autoStart: %v)\nNOTE: The timer is NOT running yet. Call start_timer with name='%s' to begin execution.", name, schedule, path, autoStart, name), nil
		},
	})

	// ── start_timer ──────────────────────────────────────────────
	r.register(&Tool{
		Name:        "start_timer",
		Description: "Start (run/execute) an existing timer in Machbase Neo. Use this when the user asks to run, start, or execute a timer that already exists. Check list_timers first if unsure whether the timer exists.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"name": {Type: "string", Description: "Timer name to start"},
			},
			Required: []string{"name"},
		},
		Fn: func(args map[string]any) (string, error) {
			name := argStr(args, "name", "")
			if name == "" {
				return "", fmt.Errorf("name is required")
			}

			payload := map[string]any{"state": "start"}
			data, err := r.client.WebPost("/web/api/timers/"+strings.ToUpper(name)+"/state", payload)
			if err != nil {
				return "", fmt.Errorf("start timer failed: %w", err)
			}

			var resp struct {
				Success bool   `json:"success"`
				Reason  string `json:"reason"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", fmt.Errorf("parse response failed: %w", err)
			}
			if !resp.Success {
				return fmt.Sprintf("Error: %s", resp.Reason), nil
			}

			return fmt.Sprintf("Timer '%s' started.", name), nil
		},
	})

	// ── stop_timer ───────────────────────────────────────────────
	r.register(&Tool{
		Name:        "stop_timer",
		Description: "Stop a running timer in Machbase Neo.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"name": {Type: "string", Description: "Timer name to stop"},
			},
			Required: []string{"name"},
		},
		Fn: func(args map[string]any) (string, error) {
			name := argStr(args, "name", "")
			if name == "" {
				return "", fmt.Errorf("name is required")
			}

			payload := map[string]any{"state": "stop"}
			data, err := r.client.WebPost("/web/api/timers/"+strings.ToUpper(name)+"/state", payload)
			if err != nil {
				return "", fmt.Errorf("stop timer failed: %w", err)
			}

			var resp struct {
				Success bool   `json:"success"`
				Reason  string `json:"reason"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", fmt.Errorf("parse response failed: %w", err)
			}
			if !resp.Success {
				return fmt.Sprintf("Error: %s", resp.Reason), nil
			}

			return fmt.Sprintf("Timer '%s' stopped.", name), nil
		},
	})

	// ── delete_timer ─────────────────────────────────────────────
	r.register(&Tool{
		Name:        "delete_timer",
		Description: "Delete a timer from Machbase Neo. The timer must be stopped before deletion. By naming convention, timer/table/TQL folder share the same NAME. Full cleanup: stop_timer(NAME) -> delete_timer(NAME) -> delete_file(NAME/NAME.tql) -> delete_file(NAME/) -> execute_sql_query(DROP TABLE NAME CASCADE).",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"name": {Type: "string", Description: "Timer name to delete"},
			},
			Required: []string{"name"},
		},
		Fn: func(args map[string]any) (string, error) {
			name := argStr(args, "name", "")
			if name == "" {
				return "", fmt.Errorf("name is required")
			}

			upperName := strings.ToUpper(name)

			// Check timer state and auto-stop if RUNNING
			infoData, err := r.client.WebGet("/web/api/timers/" + upperName)
			if err == nil {
				var infoResp struct {
					Success bool `json:"success"`
					Data    []struct {
						State string `json:"state"`
					} `json:"data"`
				}
				if json.Unmarshal(infoData, &infoResp) == nil && infoResp.Success && len(infoResp.Data) > 0 {
					if infoResp.Data[0].State == "RUNNING" || infoResp.Data[0].State == "STARTING" {
						stopPayload := map[string]any{"state": "stop"}
						r.client.WebPost("/web/api/timers/"+upperName+"/state", stopPayload)
					}
				}
			}

			data, err := r.client.WebDelete("/web/api/timers/" + upperName)
			if err != nil {
				return "", fmt.Errorf("delete timer failed: %w", err)
			}

			var resp struct {
				Success bool   `json:"success"`
				Reason  string `json:"reason"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", fmt.Errorf("parse response failed: %w", err)
			}
			if !resp.Success {
				return fmt.Sprintf("Error: %s", resp.Reason), nil
			}

			return fmt.Sprintf("Timer '%s' deleted.", name), nil
		},
	})
}
