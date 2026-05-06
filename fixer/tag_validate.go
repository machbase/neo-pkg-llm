package fixer

import (
	"fmt"
	"strings"
)

// CaptureKnownTags parses list_table_tags result and stores tag names in the fixer context.
func CaptureKnownTags(result string, fctx *FixerContext) {
	fctx.KnownTags = nil
	for _, line := range strings.Split(strings.TrimSpace(result), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "NAME" {
			continue
		}
		// Format: "[TABLE] tag1, tag2, tag3"
		if strings.HasPrefix(line, "[") {
			if idx := strings.Index(line, "] "); idx >= 0 {
				for _, t := range strings.Split(line[idx+2:], ",") {
					if tag := strings.TrimSpace(t); tag != "" {
						fctx.KnownTags = append(fctx.KnownTags, tag)
					}
				}
			}
			continue
		}
		// Format: "tag"
		fctx.KnownTags = append(fctx.KnownTags, line)
	}
	if len(fctx.KnownTags) > 0 {
		fmt.Printf("  [guard] Known tags captured: %d tags\n", len(fctx.KnownTags))
	}
}
