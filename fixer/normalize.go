package fixer

import (
	"fmt"
	"strings"
)

// aliasMap maps tool name → { aliasKey → canonicalKey }.
var aliasMap = map[string]map[string]string{
	"save_tql_file": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename", "tql_path": "filename",
		"script": "tql_content", "content": "tql_content", "code": "tql_content", "tql_script": "tql_content", "tql": "tql_content",
	},
	"execute_tql_script": {
		"script": "tql_content", "content": "tql_content", "code": "tql_content",
	},
	"validate_chart_tql": {
		"script": "tql_script", "tql_content": "tql_script", "content": "tql_script",
	},
	"execute_sql_query": {
		"sql": "sql_query", "query": "sql_query",
	},
	"list_table_tags": {
		"table": "table_name", "name": "table_name", "table_id": "table_name",
	},
	"get_full_document_content": {
		"file_path": "file_identifier", "doc_name": "file_identifier", "path": "file_identifier", "document_path": "file_identifier", "doc_path": "file_identifier",
	},
	"get_document_sections": {
		"file_path": "file_identifier", "doc_name": "file_identifier", "path": "file_identifier", "document_path": "file_identifier", "doc_path": "file_identifier",
	},
	"extract_code_blocks": {
		"file_path": "file_identifier", "doc_name": "file_identifier", "path": "file_identifier", "document_path": "file_identifier", "doc_path": "file_identifier",
	},
	"create_folder": {
		"name": "folder_name", "path": "folder_name",
	},
	"delete_file": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
	},
	"create_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"create_dashboard_with_charts": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"add_chart_to_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
		"title": "chart_title", "type": "chart_type", "tql": "tql_path",
	},
	"remove_chart_from_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
		"chart_id": "panel_id", "id": "panel_id",
		"chart_title": "panel_title", "title": "panel_title", "chart_name": "panel_title",
	},
	"update_chart_in_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"chart_id": "panel_id", "id": "panel_id",
		"chart_title": "panel_title", "title": "panel_title", "chart_name": "panel_title",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"get_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"preview_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
	"delete_dashboard": {
		"path": "filename", "file_path": "filename", "name": "filename", "file_name": "filename",
		"dashboard_id": "filename", "dashboard": "filename", "dashboard_name": "filename", "dashboard_filename": "filename", "dashboard_file": "filename",
	},
}

// canonicalKeys defines the expected parameter names per tool.
var canonicalKeys = map[string][]string{
	"save_tql_file":                {"filename", "tql_content"},
	"execute_tql_script":           {"tql_content"},
	"validate_chart_tql":           {"tql_script"},
	"execute_sql_query":            {"sql_query"},
	"list_table_tags":              {"table_name"},
	"get_full_document_content":    {"file_identifier"},
	"get_document_sections":        {"file_identifier"},
	"extract_code_blocks":          {"file_identifier"},
	"create_folder":                {"folder_name"},
	"delete_file":                  {"filename"},
	"create_dashboard":             {"filename"},
	"create_dashboard_with_charts": {"filename"},
	"add_chart_to_dashboard":       {"filename"},
	"remove_chart_from_dashboard":  {"filename"},
	"update_chart_in_dashboard":    {"filename"},
	"get_dashboard":                {"filename"},
	"preview_dashboard":            {"filename"},
	"delete_dashboard":             {"filename"},
	"update_connection":            {},
}

// knownParams are standard parameter names that should not be remapped.
var knownParams = map[string]bool{
	"format": true, "timeformat": true, "timezone": true,
	"timeout_seconds": true, "limit": true,
	"section_filter": true, "language": true,
	"host": true, "port": true, "user": true,
	"time_start": true, "time_end": true, "refresh": true,
	"charts": true, "chart_title": true, "chart_type": true,
	"table": true, "tag": true, "column": true, "color": true,
	"tql_path": true, "user_name": true,
	"x": true, "y": true, "w": true, "h": true,
	"smooth": true, "area_style": true, "is_stack": true,
	"panel_id": true, "panel_title": true,
	"new_title": true, "new_chart_type": true, "new_table": true,
	"new_tag": true, "new_column": true, "new_color": true,
	"title": true, "parent": true,
	"auto_fix": true, "add_validation_script": true,
}

// NormalizeArgs renames alias keys to their canonical names in-place.
func NormalizeArgs(toolName string, args map[string]any) {
	// Pass 1: explicit alias mapping
	if mapping, ok := aliasMap[toolName]; ok {
		for alias, canonical := range mapping {
			if _, hasCanonical := args[canonical]; hasCanonical {
				continue
			}
			if v, hasAlias := args[alias]; hasAlias {
				args[canonical] = v
				delete(args, alias)
			}
		}
	}

	// Strip leading "/" from filename/folder_name
	for _, key := range []string{"filename", "folder_name", "tql_path"} {
		if v, ok := args[key].(string); ok && strings.HasPrefix(v, "/") {
			args[key] = strings.TrimLeft(v, "/")
			fmt.Printf("  [fix] Stripped leading '/' from %s: %q\n", key, args[key])
		}
	}

	// Pass 2: fuzzy fallback
	expected, ok := canonicalKeys[toolName]
	if !ok {
		return
	}
	for _, canonical := range expected {
		if _, present := args[canonical]; present {
			continue
		}
		for key, val := range args {
			if knownParams[key] {
				continue
			}
			if _, isStr := val.(string); !isStr {
				continue
			}
			if fuzzyKeyMatch(key, canonical) {
				args[canonical] = val
				delete(args, key)
				fmt.Printf("  [fix] Fuzzy alias: %q → %q\n", key, canonical)
				break
			}
		}
	}
}

// fuzzyKeyMatch returns true if key is likely an alias for canonical.
func fuzzyKeyMatch(key, canonical string) bool {
	k := strings.ToLower(strings.ReplaceAll(key, "_", ""))
	c := strings.ToLower(strings.ReplaceAll(canonical, "_", ""))
	if strings.Contains(k, c) || strings.Contains(c, k) {
		return true
	}
	parts := strings.FieldsFunc(canonical, func(r rune) bool { return r == '_' })
	for _, part := range parts {
		if strings.Contains(k, strings.ToLower(part)) {
			return true
		}
	}
	return false
}
