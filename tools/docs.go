package tools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// docsBasePath points to the neo/ documentation directory.
// Resolved relative to the executable or current directory.
var docsBasePath string

func init() {
	// Try to find neo/ docs relative to executable
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "neo")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			docsBasePath = candidate
			return
		}
	}
	// Fallback: look relative to current directory
	if info, err := os.Stat("neo"); err == nil && info.IsDir() {
		docsBasePath, _ = filepath.Abs("neo")
		return
	}
	// Fallback: look in parent (Agentic_Loop/neo)
	if info, err := os.Stat("../Agentic_Loop/neo"); err == nil && info.IsDir() {
		docsBasePath, _ = filepath.Abs("../Agentic_Loop/neo")
		return
	}
	docsBasePath = "neo"
}

// catalogEntry holds Korean title and keywords for a document.
type catalogEntry struct {
	TitleKo  string
	Keywords string
}

var (
	docCatalog     map[string]catalogEntry
	docCatalogOnce sync.Once
)

// loadCatalog parses neo/catalog.md markdown table into a map.
func loadCatalog() map[string]catalogEntry {
	docCatalogOnce.Do(func() {
		docCatalog = make(map[string]catalogEntry)
		catalogPath := filepath.Join(docsBasePath, "catalog.md")
		f, err := os.Open(catalogPath)
		if err != nil {
			return
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			// Skip header and separator lines
			if lineNum <= 3 {
				continue
			}
			// Parse table row: | path | title_ko | keywords |
			parts := strings.Split(line, "|")
			if len(parts) < 4 {
				continue
			}
			path := strings.TrimSpace(parts[1])
			titleKo := strings.TrimSpace(parts[2])
			keywords := strings.TrimSpace(parts[3])
			if path == "" || path == "path" {
				continue
			}
			docCatalog[path] = catalogEntry{TitleKo: titleKo, Keywords: keywords}
		}
	})
	return docCatalog
}

func (r *Registry) registerDocTools() {
	r.register(&Tool{
		Name:        "list_available_documents",
		Internal:    true, // Called by Go code in initMessages(), not exposed to LLM
		Description: "Search documentation by keyword. Returns matching documents with exact file paths. Use the returned path as-is for get_full_document_content.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"query": {Type: "string", Description: "Search keyword (e.g. 'PIVOT', 'rollup', '백업')"},
			},
		},
		Fn: func(args map[string]any) (string, error) {
			query := strings.ToLower(argStr(args, "query", ""))
			catalog := loadCatalog()
			var docs []string
			err := filepath.Walk(docsBasePath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if !info.IsDir() && strings.HasSuffix(path, ".md") {
					rel, _ := filepath.Rel(docsBasePath, path)
					rel = strings.ReplaceAll(rel, "\\", "/")
					if rel == "catalog.md" {
						return nil
					}
					entry, hasCatalog := catalog[rel]
					line := rel
					if hasCatalog {
						line = fmt.Sprintf("%s | %s | %s", rel, entry.TitleKo, entry.Keywords)
					}
					// If no query, return all (internal use)
					if query == "" {
						docs = append(docs, line)
						return nil
					}
					// Search: match query against path, title, keywords
					searchTarget := strings.ToLower(rel)
					if hasCatalog {
						searchTarget = strings.ToLower(rel + " " + entry.TitleKo + " " + entry.Keywords)
					}
					if strings.Contains(searchTarget, query) {
						docs = append(docs, line)
					}
				}
				return nil
			})
			if err != nil {
				return "", fmt.Errorf("failed to list documents: %w", err)
			}
			if len(docs) == 0 {
				if query != "" {
					return fmt.Sprintf("No documents found for '%s'", query), nil
				}
				return "No documentation files found", nil
			}
			return strings.Join(docs, "\n"), nil
		},
	})

	r.register(&Tool{
		Name:        "get_full_document_content",
		Description: "Get complete content of a specific documentation file.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"file_identifier": {Type: "string", Description: "Relative path (e.g., 'sql/rollup.md')"},
			},
			Required: []string{"file_identifier"},
		},
		Fn: func(args map[string]any) (string, error) {
			fileID := argStrAny(args, "", "file_identifier", "file_path", "doc_name", "document_name", "path")
			if fileID == "" {
				return "", fmt.Errorf("file_identifier is required")
			}
			fullPath := filepath.Join(docsBasePath, filepath.FromSlash(fileID))
			data, err := os.ReadFile(fullPath)
			if err != nil {
				// File not found: search catalog for keyword suggestions
				catalog := loadCatalog()
				// Extract search term from the requested path
				base := strings.TrimSuffix(filepath.Base(fileID), ".md")
				terms := strings.FieldsFunc(base, func(r rune) bool {
					return r == '-' || r == '_' || r == '.'
				})
				var suggestions []string
				for path, entry := range catalog {
					kw := strings.ToLower(entry.Keywords)
					for _, term := range terms {
						t := strings.ToLower(term)
						if len(t) >= 3 && strings.Contains(kw, t) {
							suggestions = append(suggestions, fmt.Sprintf("- %s (%s)", path, entry.Keywords))
							break
						}
					}
				}
				if len(suggestions) > 0 {
					return "", fmt.Errorf("문서를 찾을 수 없습니다: %s\n카탈로그에서 찾은 관련 문서:\n%s\n위 경로로 다시 호출하세요.", fileID, strings.Join(suggestions, "\n"))
				}
				return "", fmt.Errorf("failed to read document: %w", err)
			}
			return string(data), nil
		},
	})

	r.register(&Tool{
		Name:        "get_document_sections",
		Description: "Get document content organized by sections.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"file_identifier": {Type: "string", Description: "File path"},
				"section_filter":  {Type: "string", Description: "Filter sections containing this text"},
			},
			Required: []string{"file_identifier"},
		},
		Fn: func(args map[string]any) (string, error) {
			fileID := argStrAny(args, "", "file_identifier", "file_path", "doc_name", "document_name", "path")
			if fileID == "" {
				return "", fmt.Errorf("file_identifier is required")
			}
			filter := strings.ToLower(argStr(args, "section_filter", ""))

			fullPath := filepath.Join(docsBasePath, filepath.FromSlash(fileID))
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return "", fmt.Errorf("failed to read document: %w", err)
			}

			content := string(data)
			lines := strings.Split(content, "\n")

			type section struct {
				title   string
				content strings.Builder
			}
			var sections []section
			current := section{title: "Introduction"}

			for _, line := range lines {
				if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
					if current.content.Len() > 0 || current.title != "Introduction" {
						sections = append(sections, current)
					}
					current = section{title: strings.TrimLeft(line, "# ")}
				}
				current.content.WriteString(line)
				current.content.WriteString("\n")
			}
			if current.content.Len() > 0 {
				sections = append(sections, current)
			}

			var result strings.Builder
			for _, s := range sections {
				if filter != "" && !strings.Contains(strings.ToLower(s.title), filter) &&
					!strings.Contains(strings.ToLower(s.content.String()), filter) {
					continue
				}
				result.WriteString(fmt.Sprintf("\n--- %s ---\n", s.title))
				result.WriteString(s.content.String())
			}
			if result.Len() == 0 {
				return "No matching sections found", nil
			}
			return result.String(), nil
		},
	})

	r.register(&Tool{
		Name:        "extract_code_blocks",
		Description: "Extract all code blocks from a documentation file.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"file_identifier": {Type: "string", Description: "File path"},
				"language":        {Type: "string", Description: "Filter by language"},
			},
			Required: []string{"file_identifier"},
		},
		Fn: func(args map[string]any) (string, error) {
			fileID := argStrAny(args, "", "file_identifier", "file_path", "doc_name", "document_name", "path")
			if fileID == "" {
				return "", fmt.Errorf("file_identifier is required")
			}
			lang := strings.ToLower(argStr(args, "language", ""))

			fullPath := filepath.Join(docsBasePath, filepath.FromSlash(fileID))
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return "", fmt.Errorf("failed to read document: %w", err)
			}

			re := regexp.MustCompile("```(\\w*)\\n([\\s\\S]*?)\\n```")
			matches := re.FindAllStringSubmatch(string(data), -1)

			var result strings.Builder
			blockNum := 0
			for _, m := range matches {
				blockLang := strings.ToLower(m[1])
				code := m[2]
				if lang != "" && blockLang != lang {
					continue
				}
				blockNum++
				result.WriteString(fmt.Sprintf("\n--- Code Block %d [%s] ---\n", blockNum, m[1]))
				result.WriteString(code)
				result.WriteString("\n")
			}
			if result.Len() == 0 {
				return "No code blocks found", nil
			}
			return result.String(), nil
		},
	})
}
