package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveInstanceName(t *testing.T) {
	tests := []struct {
		basename string
		expected string
	}{
		{".claude-alpha", "alpha"},
		{".claude-beta", "beta"},
		{".claude-work", "work"},
		{".claude", ""},
		{"claude", "claude"},
		{".other", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.basename, func(t *testing.T) {
			result := deriveInstanceName(tt.basename)
			if result != tt.expected {
				t.Errorf("deriveInstanceName(%q) = %q, want %q", tt.basename, result, tt.expected)
			}
		})
	}
}

func TestResolveInstanceFromEnvVar(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		envValue         string
		expectedInstance string
		expectedDir      string
	}{
		{
			envValue:         filepath.Join(home, ".claude-alpha"),
			expectedInstance: "alpha",
			expectedDir:      filepath.Join(home, ".claude-alpha"),
		},
		{
			envValue:         filepath.Join(home, ".claude-beta"),
			expectedInstance: "beta",
			expectedDir:      filepath.Join(home, ".claude-beta"),
		},
		{
			envValue:         filepath.Join(home, ".claude"),
			expectedInstance: "",
			expectedDir:      filepath.Join(home, ".claude"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			// Ensure flags are cleared
			instanceName = ""
			claudeDir = ""

			os.Setenv("CLAUDE_CONFIG_DIR", tt.envValue)
			defer os.Unsetenv("CLAUDE_CONFIG_DIR")

			inst, dir := resolveInstance()
			if inst != tt.expectedInstance {
				t.Errorf("instance = %q, want %q", inst, tt.expectedInstance)
			}
			if dir != tt.expectedDir {
				t.Errorf("dir = %q, want %q", dir, tt.expectedDir)
			}
		})
	}
}

func TestResolveInstanceFromFlag(t *testing.T) {
	home, _ := os.UserHomeDir()
	os.Unsetenv("CLAUDE_CONFIG_DIR")

	instanceName = "gamma"
	claudeDir = ""
	defer func() { instanceName = ""; claudeDir = "" }()

	inst, dir := resolveInstance()
	if inst != "gamma" {
		t.Errorf("instance = %q, want %q", inst, "gamma")
	}
	if dir != filepath.Join(home, ".claude-gamma") {
		t.Errorf("dir = %q, want %q", dir, filepath.Join(home, ".claude-gamma"))
	}
}

func TestResolveInstanceFromClaudeDirFlag(t *testing.T) {
	os.Unsetenv("CLAUDE_CONFIG_DIR")

	instanceName = ""
	claudeDir = "/tmp/.claude-delta"
	defer func() { instanceName = ""; claudeDir = "" }()

	inst, dir := resolveInstance()
	if inst != "delta" {
		t.Errorf("instance = %q, want %q", inst, "delta")
	}
	if dir != "/tmp/.claude-delta" {
		t.Errorf("dir = %q, want %q", dir, "/tmp/.claude-delta")
	}
}

func TestResolveInstanceDefault(t *testing.T) {
	os.Unsetenv("CLAUDE_CONFIG_DIR")

	instanceName = ""
	claudeDir = ""
	defer func() { instanceName = ""; claudeDir = "" }()

	inst, dir := resolveInstance()
	if inst != "" {
		t.Errorf("instance = %q, want empty string", inst)
	}
	if dir != "" {
		t.Errorf("dir = %q, want empty string", dir)
	}
}

func TestResetInstanceConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create instance config dir with files
	instanceDir := filepath.Join(tmpDir, ".claude-sync/epsilon")
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"config.yaml", "age-key.txt", "state.json"} {
		if err := os.WriteFile(filepath.Join(instanceDir, f), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Default config dir should remain unaffected
	defaultDir := filepath.Join(tmpDir, ".claude-sync")
	if err := os.WriteFile(filepath.Join(defaultDir, "config.yaml"), []byte("default"), 0600); err != nil {
		t.Fatal(err)
	}

	// Simulate reset for epsilon instance only
	if err := os.RemoveAll(instanceDir); err != nil {
		t.Fatal(err)
	}

	// Instance dir should be gone
	if _, err := os.Stat(instanceDir); !os.IsNotExist(err) {
		t.Error("instance config dir should be removed after reset")
	}

	// Default dir should still exist
	if _, err := os.Stat(filepath.Join(defaultDir, "config.yaml")); os.IsNotExist(err) {
		t.Error("default config should not be affected by instance reset")
	}
}
