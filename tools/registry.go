package tools

import (
	"neo-pkg-llm/machbase"
	"encoding/json"
	"fmt"
)

// Tool represents a callable tool with its schema for LLM tool calling.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
	Fn          func(args map[string]any) (string, error) `json:"-"`
}

type ToolParameters struct {
	Type       string                    `json:"type"`
	Properties map[string]ToolProperty   `json:"properties"`
	Required   []string                  `json:"required,omitempty"`
}

type ToolProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolDef returns the tool definition in OpenAI-compatible function calling format.
func (t *Tool) ToolDef() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		},
	}
}

// Registry holds all registered tools.
type Registry struct {
	tools  map[string]*Tool
	order  []string
	client *machbase.Client
}

func NewRegistry(client *machbase.Client) *Registry {
	r := &Registry{
		tools:  make(map[string]*Tool),
		client: client,
	}
	r.registerAll()
	return r
}

func (r *Registry) register(t *Tool) {
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
}

func (r *Registry) Get(name string) *Tool {
	return r.tools[name]
}

func (r *Registry) Execute(name string, argsJSON string) (string, error) {
	tool := r.tools[name]
	if tool == nil {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("invalid args JSON: %w", err)
		}
	} else {
		args = map[string]any{}
	}
	return tool.Fn(args)
}

func (r *Registry) ExecuteMap(name string, args map[string]any) (string, error) {
	tool := r.tools[name]
	if tool == nil {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	if args == nil {
		args = map[string]any{}
	}
	return tool.Fn(args)
}

// AllToolDefs returns all tool definitions in OpenAI-compatible format.
func (r *Registry) AllToolDefs() []map[string]any {
	defs := make([]map[string]any, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].ToolDef())
	}
	return defs
}

// ToolNames returns all registered tool names.
func (r *Registry) ToolNames() []string {
	return r.order
}

func (r *Registry) registerAll() {
	r.registerSQLTools()
	r.registerTQLTools()
	r.registerFileTools()
	r.registerDashboardTools()
	r.registerDocTools()
	r.registerUtilTools()
}

// Helper to extract string from args with default.
func argStr(args map[string]any, key, fallback string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

// argStrAny tries multiple keys in order and returns the first non-empty match.
func argStrAny(args map[string]any, fallback string, keys ...string) string {
	for _, key := range keys {
		if v := argStr(args, key, ""); v != "" {
			return v
		}
	}
	return fallback
}

func argInt(args map[string]any, key string, fallback int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return fallback
}

func argBool(args map[string]any, key string, fallback bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}
