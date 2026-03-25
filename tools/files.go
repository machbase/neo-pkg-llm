package tools

import (
	"fmt"
	"strings"
)

func (r *Registry) registerFileTools() {
	r.register(&Tool{
		Name:        "create_folder",
		Description: "Create a folder in Machbase Neo file system.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"folder_name": {Type: "string", Description: "Folder name to create"},
				"parent":      {Type: "string", Description: "Parent path", Default: ""},
			},
			Required: []string{"folder_name"},
		},
		Fn: func(args map[string]any) (string, error) {
			name := argStr(args, "folder_name", "")
			if name == "" {
				return "", fmt.Errorf("folder_name is required")
			}
			parent := argStr(args, "parent", "")
			path := name
			if parent != "" {
				path = parent + "/" + name
			}
			err := r.client.CreateFolder(path)
			if err != nil {
				return "", fmt.Errorf("create_folder failed: %w", err)
			}
			return fmt.Sprintf("Folder created: %s", path), nil
		},
	})

	r.register(&Tool{
		Name:        "list_files",
		Description: "List files and folders in Machbase Neo file system.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"path": {Type: "string", Description: "Directory path", Default: "/"},
			},
		},
		Fn: func(args map[string]any) (string, error) {
			dirPath := argStr(args, "path", "/")
			entries, err := r.client.ListDir(dirPath)
			if err != nil {
				return "", fmt.Errorf("list_files failed: %w", err)
			}
			var result strings.Builder
			result.WriteString(fmt.Sprintf("Files in %s:\n", dirPath))
			for _, e := range entries {
				result.WriteString(fmt.Sprintf("  [%s] %s\n", e["type"], e["name"]))
			}
			return result.String(), nil
		},
	})

	r.register(&Tool{
		Name:        "delete_file",
		Description: "Delete a file or empty folder from Machbase Neo file system.",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ToolProperty{
				"filename": {Type: "string", Description: "File path to delete"},
			},
			Required: []string{"filename"},
		},
		Fn: func(args map[string]any) (string, error) {
			filename := argStr(args, "filename", "")
			if filename == "" {
				return "", fmt.Errorf("filename is required")
			}
			if err := r.client.DeleteFile(filename); err != nil {
				return "", fmt.Errorf("delete_file failed: %w", err)
			}
			return fmt.Sprintf("Deleted: %s", filename), nil
		},
	})
}
