package context

import "strings"

type PromptBuilder struct {
	parts   []string
	catalog string
	ollama  bool
}

func NewBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// SetOllama enables compact Ollama-optimized segments.
func (b *PromptBuilder) SetOllama() *PromptBuilder {
	b.ollama = true
	return b
}

// AddCore adds Role + TableSchema + sql_tools + common_prohibitions + ErrorHandling
func (b *PromptBuilder) AddCore() *PromptBuilder {
	if b.ollama {
		b.parts = append(b.parts, OllamaSegRole, OllamaSegTableSchema, OllamaSegErrorHandling)
	} else {
		b.parts = append(b.parts, SegRole, SegTableSchema, ToolPrompts["sql_tools"], SegErrorHandling, ToolPrompts["common_prohibitions"])
	}
	return b
}

// AddSegment adds a single named segment (Role, TableSchema, etc.)
func (b *PromptBuilder) AddSegment(name string) *PromptBuilder {
	b.parts = append(b.parts, b.resolveSegment(name))
	return b
}

// AddWorkflow adds workflow segments by name
func (b *PromptBuilder) AddWorkflow(names ...string) *PromptBuilder {
	for _, name := range names {
		if seg := b.resolveSegment(name); seg != "" {
			b.parts = append(b.parts, seg)
		}
	}
	return b
}

func (b *PromptBuilder) resolveSegment(name string) string {
	if b.ollama {
		ollamaMap := map[string]string{
			"Role":                OllamaSegRole,
			"QueryClassification": OllamaSegQueryClassification,
			"TableSchema":         OllamaSegTableSchema,
			"AdvancedWorkflow":    OllamaSegAdvancedWorkflow,
			"BasicWorkflow":       OllamaSegBasicWorkflow,
			"HTMLReportWorkflow":  OllamaSegHTMLReportWorkflow,
			"ErrorHandling":       OllamaSegErrorHandling,
		}
		if seg, ok := ollamaMap[name]; ok {
			return seg
		}
	}
	standardMap := map[string]string{
		"Role":                SegRole,
		"QueryClassification": SegQueryClassification,
		"TableSchema":         SegTableSchema,
		"AdvancedWorkflow":    SegAdvancedWorkflow,
		"BasicWorkflow":       SegBasicWorkflow,
		"HTMLReportWorkflow":  SegHTMLReportWorkflow,
		"ErrorHandling":       SegErrorHandling,
	}
	if seg, ok := standardMap[name]; ok {
		return seg
	}
	return ""
}

// AddToolPrompts adds tool-specific prompts by group name
func (b *PromptBuilder) AddToolPrompts(groups ...string) *PromptBuilder {
	for _, g := range groups {
		// Ollama: use OllamaSegTQLRules instead of tql_tools for compactness
		if b.ollama && g == "tql_tools" {
			b.parts = append(b.parts, OllamaSegTQLRules)
			continue
		}
		if prompt, ok := ToolPrompts[g]; ok {
			b.parts = append(b.parts, prompt)
		}
	}
	return b
}

// SetCatalog sets the document catalog text to append
func (b *PromptBuilder) SetCatalog(catalog string) *PromptBuilder {
	b.catalog = catalog
	return b
}

// Build produces the final system prompt string
func (b *PromptBuilder) Build() string {
	result := strings.Join(b.parts, "\n")
	if b.catalog != "" {
		result += "\n" + b.catalog
	}
	return result
}

// BuildLegacy returns the full monolithic prompt (for backward compatibility during migration)
func BuildLegacy() string {
	return NewBuilder().
		AddCore().
		AddWorkflow("QueryClassification", "AdvancedWorkflow", "BasicWorkflow", "HTMLReportWorkflow").
		AddToolPrompts("tql_tools", "dashboard_tools", "doc_tools", "report_tools").
		Build()
}
