package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	// Match: ### 1-1. <title>\n<any lines>\n```tql\n<code>\n```
	// Use \r?\n to handle both Unix (LF) and Windows (CRLF) line endings.
	templateRE = regexp.MustCompile("(?s)###\\s*(\\d+-\\d+)\\.[^\\n]*\\r?\\n.*?```tql\\r?\\n(.*?)\\r?\\n```")
	templates  = map[string]string{}
	tmplOnce   sync.Once
	tmplPath   string
)

func init() {
	const tmplFile = "tql-analysis-templates.md"
	// Try to locate tql-analysis-templates.md
	candidates := []string{
		filepath.Join("neo", "tql", tmplFile),
		filepath.Join("..", "Agentic_Loop", "neo", "tql", tmplFile),
	}
	exe, err := os.Executable()
	if err == nil {
		candidates = append([]string{
			filepath.Join(filepath.Dir(exe), "neo", "tql", tmplFile),
		}, candidates...)
	}
	// Also try CWD-based paths
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "neo", "tql", tmplFile))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			tmplPath, _ = filepath.Abs(c)
			fmt.Printf("[Templates] Found at: %s\n", tmplPath)
			break
		}
	}
}

// LoadTemplates parses TQL templates from the markdown file.
func LoadTemplates() map[string]string {
	tmplOnce.Do(func() {
		if tmplPath == "" {
			fmt.Println("[Templates] WARNING: template file not found")
			return
		}
		data, err := os.ReadFile(tmplPath)
		if err != nil {
			fmt.Printf("[Templates] Failed to read: %v\n", err)
			return
		}
		// Normalize CRLF to LF before matching, so captured code has clean line endings.
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		matches := templateRE.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			templates[m[1]] = strings.TrimSpace(m[2])
		}
		if len(templates) > 0 {
			ids := make([]string, 0, len(templates))
			for id := range templates {
				ids = append(ids, id)
			}
			fmt.Printf("[Templates] Loaded %d templates: %v\n", len(templates), ids)
		} else {
			fmt.Println("[Templates] WARNING: no templates loaded")
		}
	})
	return templates
}

// ExpandTemplate substitutes placeholders in a TQL template.
func ExpandTemplate(templateID string, params map[string]string) (string, error) {
	tmpl := LoadTemplates()
	code, ok := tmpl[templateID]
	if !ok {
		ids := make([]string, 0, len(tmpl))
		for id := range tmpl {
			ids = append(ids, id)
		}
		return "", fmt.Errorf("template '%s' not found. available: %v", templateID, ids)
	}

	// Strip surrounding quotes from TAG values (Gemini sends TAG:'value' with quotes)
	for _, tagKey := range []string{"TAG", "TAG1", "TAG2"} {
		if v, ok := params[tagKey]; ok {
			params[tagKey] = strings.Trim(v, "'\"")
		}
	}

	for key, val := range params {
		code = strings.ReplaceAll(code, "{"+key+"}", val)
	}

	// Warn about unsubstituted placeholders
	unmatched := regexp.MustCompile(`\{(TABLE|TAG|TAG1|TAG2|UNIT)\}`).FindAllString(code, -1)
	if len(unmatched) > 0 {
		fmt.Printf("[Templates] WARNING: unsubstituted placeholders: %v\n", unmatched)
	}

	return code, nil
}
