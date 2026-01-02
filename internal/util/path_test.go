package util

import (
	"testing"
)

func TestToLocalPath(t *testing.T) {
	tests := []struct {
		name       string
		globalPath string
		baseDir    string
		expected   string
	}{
		{
			name:       "simple",
			globalPath: "document.md",
			baseDir:    "shared/",
			expected:   "shared/document.md",
		},
		{
			name:       "nested",
			globalPath: "folder/document.md",
			baseDir:    "shared/",
			expected:   "shared/folder/document.md",
		},
		{
			name:       "empty baseDir",
			globalPath: "document.md",
			baseDir:    "",
			expected:   "document.md",
		},
		{
			name:       "empty path with baseDir",
			globalPath: "",
			baseDir:    "shared/",
			expected:   "shared",
		},
		{
			name:       "underscore prefix",
			globalPath: "_special.md",
			baseDir:    "",
			expected:   "/_special.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToLocalPath(tt.globalPath, tt.baseDir)
			if result != tt.expected {
				t.Errorf("ToLocalPath(%q, %q) = %q, want %q",
					tt.globalPath, tt.baseDir, result, tt.expected)
			}
		})
	}
}

func TestToGlobalPath(t *testing.T) {
	tests := []struct {
		name      string
		localPath string
		baseDir   string
		expected  string
	}{
		{
			name:      "simple",
			localPath: "shared/document.md",
			baseDir:   "shared/",
			expected:  "document.md",
		},
		{
			name:      "nested",
			localPath: "shared/folder/document.md",
			baseDir:   "shared/",
			expected:  "folder/document.md",
		},
		{
			name:      "no baseDir match",
			localPath: "other/document.md",
			baseDir:   "shared/",
			expected:  "other/document.md",
		},
		{
			name:      "underscore prefix",
			localPath: "_special.md",
			baseDir:   "",
			expected:  "special.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToGlobalPath(tt.localPath, tt.baseDir)
			if result != tt.expected {
				t.Errorf("ToGlobalPath(%q, %q) = %q, want %q",
					tt.localPath, tt.baseDir, result, tt.expected)
			}
		})
	}
}

func TestIsPlainText(t *testing.T) {
	tests := []struct {
		name     string
		filepath string
		expected bool
	}{
		{"markdown", "document.md", true},
		{"text", "readme.txt", true},
		{"json", "config.json", true},
		{"go", "main.go", true},
		{"binary", "image.png", false},
		{"binary", "archive.zip", false},
		{"no extension", "README", false},
		{"uppercase", "FILE.MD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPlainText(tt.filepath)
			if result != tt.expected {
				t.Errorf("IsPlainText(%q) = %v, want %v", tt.filepath, result, tt.expected)
			}
		})
	}
}
