package context

import "fmt"

// FormatCatalog formats the document catalog for injection into the system prompt.
func FormatCatalog(catalogText string) string {
	if catalogText == "" {
		return ""
	}
	return fmt.Sprintf("\n## 문서 카탈로그 (경로 | 한국어 제목 | 키워드)\n%s", catalogText)
}
