package util

import (
	"path"
	"strings"
)

// ToLocalPath converts a global path to a local path with baseDir prefix
// Global: "document.md" + baseDir: "shared/" -> Local: "shared/document.md"
func ToLocalPath(globalPath string, baseDir string) string {
	// Join using POSIX-style paths (always forward slashes)
	joined := path.Join(baseDir, globalPath)

	// Handle special case where result is "."
	if joined == "." {
		return ""
	}

	// Handle paths starting with underscore (special in Obsidian)
	if strings.HasPrefix(joined, "_") {
		return "/" + joined
	}

	return joined
}

// ToGlobalPath converts a local path to a global path by removing baseDir prefix
// Local: "shared/document.md" + baseDir: "shared/" -> Global: "document.md"
func ToGlobalPath(localPath string, baseDir string) string {
	// Handle underscore prefix
	p := localPath
	if strings.HasPrefix(p, "_") {
		p = p[1:]
	}

	// Remove baseDir prefix if present
	if strings.HasPrefix(p, baseDir) {
		p = p[len(baseDir):]
	}

	return p
}

// IsPlainText determines if a file should be treated as plain text based on extension
func IsPlainText(filepath string) bool {
	// Common plain text extensions
	textExtensions := []string{
		".md", ".txt", ".json", ".xml", ".yaml", ".yml",
		".html", ".htm", ".css", ".js", ".ts", ".jsx", ".tsx",
		".go", ".py", ".rb", ".java", ".c", ".cpp", ".h",
		".sh", ".bash", ".sql", ".csv", ".log",
	}

	for _, ext := range textExtensions {
		if strings.HasSuffix(strings.ToLower(filepath), ext) {
			return true
		}
	}

	return false
}
