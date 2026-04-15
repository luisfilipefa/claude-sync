package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tawanorg/claude-sync/internal/config"
)

// --- encodeHome ---

func TestEncodeHome(t *testing.T) {
	cases := []struct {
		name     string
		absPath  string
		homeDir  string
		expected string
	}{
		{"macOS prefix match", "-Users-alice-code-foo", "/Users/alice", "~HOME~-code-foo"},
		{"Linux prefix match", "-home-alice-code-foo", "/home/alice", "~HOME~-code-foo"},
		{"no match — different user", "-Users-bob-code-foo", "/Users/alice", "-Users-bob-code-foo"},
		{"empty homeDir passthrough", "-Users-alice-foo", "", "-Users-alice-foo"},
		{"exact match (project == home)", "-Users-alice", "/Users/alice", "~HOME~"},
		{"trailing slash in homeDir stripped", "-Users-alice-foo", "/Users/alice/", "~HOME~-foo"},
		{"already has token — no change", "~HOME~-foo", "/Users/alice", "~HOME~-foo"},
		{"partial prefix guard — /Users/a vs /Users/alice", "-Users-alice-foo", "/Users/a", "-Users-alice-foo"},
		{"root level path", "-opt-work-foo", "/Users/alice", "-opt-work-foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeHome(tc.absPath, tc.homeDir)
			if got != tc.expected {
				t.Errorf("encodeHome(%q, %q) = %q, want %q", tc.absPath, tc.homeDir, got, tc.expected)
			}
		})
	}
}

// --- decodeHome ---

func TestDecodeHome(t *testing.T) {
	cases := []struct {
		name     string
		absPath  string
		homeDir  string
		expected string
	}{
		{"token to macOS", "~HOME~-code-foo", "/Users/alice", "-Users-alice-code-foo"},
		{"token to Linux", "~HOME~-code-foo", "/home/alice", "-home-alice-code-foo"},
		{"no token passthrough", "-Users-alice-foo", "/Users/alice", "-Users-alice-foo"},
		{"empty homeDir passthrough", "~HOME~-code-foo", "", "~HOME~-code-foo"},
		{"exact token (no suffix)", "~HOME~", "/Users/alice", "-Users-alice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeHome(tc.absPath, tc.homeDir)
			if got != tc.expected {
				t.Errorf("decodeHome(%q, %q) = %q, want %q", tc.absPath, tc.homeDir, got, tc.expected)
			}
		})
	}
}

// --- encodeProjectPath ---

func TestEncodeProjectPath(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		homeDir  string
		expected string
	}{
		{
			"macOS project path",
			"projects/-Users-alice-code-foo/sessions/abc.jsonl",
			"/Users/alice",
			"projects/~HOME~-code-foo/sessions/abc.jsonl",
		},
		{
			"Linux project path",
			"projects/-home-alice-code-foo/f.jsonl",
			"/home/alice",
			"projects/~HOME~-code-foo/f.jsonl",
		},
		{
			"non-home project path unchanged",
			"projects/-opt-shared-code-foo/f.jsonl",
			"/Users/alice",
			"projects/-opt-shared-code-foo/f.jsonl",
		},
		{
			"CLAUDE.md unchanged",
			"CLAUDE.md",
			"/Users/alice",
			"CLAUDE.md",
		},
		{
			"settings.json unchanged",
			"settings.json",
			"/Users/alice",
			"settings.json",
		},
		{
			"agents path unchanged",
			"agents/helper.json",
			"/Users/alice",
			"agents/helper.json",
		},
		{
			"empty homeDir passthrough",
			"projects/-Users-alice-code-foo/f.jsonl",
			"",
			"projects/-Users-alice-code-foo/f.jsonl",
		},
		{
			"already-normalized key not double-encoded",
			"projects/~HOME~-code-foo/f.jsonl",
			"/Users/alice",
			"projects/~HOME~-code-foo/f.jsonl",
		},
		{
			"project dir with no subpath",
			"projects/-Users-alice-code-foo",
			"/Users/alice",
			"projects/~HOME~-code-foo",
		},
		{
			"partial prefix guard",
			"projects/-Users-alice-foo/f.jsonl",
			"/Users/a",
			"projects/-Users-alice-foo/f.jsonl",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeProjectPath(tc.input, tc.homeDir)
			if got != tc.expected {
				t.Errorf("encodeProjectPath(%q, %q) = %q, want %q", tc.input, tc.homeDir, got, tc.expected)
			}
		})
	}
}

// --- decodeProjectPath ---

func TestDecodeProjectPath(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		homeDir  string
		expected string
	}{
		{
			"token to macOS home",
			"projects/~HOME~-code-foo/sessions/abc.jsonl",
			"/Users/alice",
			"projects/-Users-alice-code-foo/sessions/abc.jsonl",
		},
		{
			"token to Linux home",
			"projects/~HOME~-code-foo/f.jsonl",
			"/home/alice",
			"projects/-home-alice-code-foo/f.jsonl",
		},
		{
			"old-style key (no token) passthrough — backward compat",
			"projects/-Users-alice-code-foo/f.jsonl",
			"/home/alice",
			"projects/-Users-alice-code-foo/f.jsonl",
		},
		{
			"non-projects key unchanged",
			"_external/mcp-servers.json",
			"/Users/alice",
			"_external/mcp-servers.json",
		},
		{
			"CLAUDE.md unchanged",
			"CLAUDE.md",
			"/Users/alice",
			"CLAUDE.md",
		},
		{
			"empty homeDir passthrough",
			"projects/~HOME~-code-foo/f.jsonl",
			"",
			"projects/~HOME~-code-foo/f.jsonl",
		},
		{
			"project dir with no subpath",
			"projects/~HOME~-code-foo",
			"/Users/alice",
			"projects/-Users-alice-code-foo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeProjectPath(tc.input, tc.homeDir)
			if got != tc.expected {
				t.Errorf("decodeProjectPath(%q, %q) = %q, want %q", tc.input, tc.homeDir, got, tc.expected)
			}
		})
	}
}

// --- Round-trip ---

func TestProjectPathRoundTrip_CrossOS(t *testing.T) {
	macHome := "/Users/alice"
	linuxHome := "/home/alice"

	localMacPath := "projects/-Users-alice-code-myproject/sessions/f.jsonl"

	remoteKey := encodeProjectPath(localMacPath, macHome)
	if remoteKey != "projects/~HOME~-code-myproject/sessions/f.jsonl" {
		t.Fatalf("unexpected remote key: %q", remoteKey)
	}

	localLinuxPath := decodeProjectPath(remoteKey, linuxHome)
	want := "projects/-home-alice-code-myproject/sessions/f.jsonl"
	if localLinuxPath != want {
		t.Errorf("got %q, want %q", localLinuxPath, want)
	}
}

func TestProjectPathRoundTrip_SameOS(t *testing.T) {
	home := "/Users/alice"
	local := "projects/-Users-alice-code-myproject/sessions/f.jsonl"

	remote := encodeProjectPath(local, home)
	recovered := decodeProjectPath(remote, home)

	if recovered != local {
		t.Errorf("round-trip mismatch: got %q, want %q", recovered, local)
	}
}

// --- Syncer integration tests ---

func TestSyncerProjectPathNormalization(t *testing.T) {
	syncer, store, claudeDir := testSyncer(t)
	syncer.WithHomeDir("/Users/testuser")
	ctx := context.Background()

	projectPath := "projects/-Users-testuser-code-myapp/sessions/abc.jsonl"
	createTestFile(t, claudeDir, projectPath, `{"event":"test"}`)

	if _, err := syncer.Push(ctx); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	objs, _ := store.List(ctx, "")
	want := "projects/~HOME~-code-myapp/sessions/abc.jsonl.age"
	found := false
	var keys []string
	for _, obj := range objs {
		keys = append(keys, obj.Key)
		if obj.Key == want {
			found = true
		}
	}
	if !found {
		t.Errorf("expected normalized remote key %q, got: %v", want, keys)
	}
}

func TestSyncerProjectPathDenormalization(t *testing.T) {
	// Syncer 1 (macOS) pushes; Syncer 2 (Linux) pulls from same storage.
	syncer1, store, claudeDir1 := testSyncer(t)
	syncer1.WithHomeDir("/Users/alice")
	ctx := context.Background()

	projectPath := "projects/-Users-alice-code-foo/sessions/abc.jsonl"
	createTestFile(t, claudeDir1, projectPath, `{"event":"mac"}`)

	if _, err := syncer1.Push(ctx); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Set up Linux syncer sharing storage + encryptor
	tmpDir2 := t.TempDir()
	claudeDir2 := filepath.Join(tmpDir2, ".claude")
	stateDir2 := filepath.Join(tmpDir2, ".claude-sync")
	if err := os.MkdirAll(claudeDir2, 0755); err != nil {
		t.Fatalf("mkdir claudeDir2: %v", err)
	}
	if err := os.MkdirAll(stateDir2, 0700); err != nil {
		t.Fatalf("mkdir stateDir2: %v", err)
	}
	state2, err := LoadStateFromDir(stateDir2)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	syncer2 := NewSyncerWith(&config.Config{}, store, syncer1.encryptor, state2, claudeDir2, true)
	syncer2.WithHomeDir("/home/alice")

	result, err := syncer2.Pull(ctx)
	if err != nil {
		t.Fatalf("Pull failed: %v", err)
	}
	if len(result.Downloaded) != 1 {
		t.Errorf("expected 1 download, got %d: %v", len(result.Downloaded), result.Downloaded)
	}

	linuxPath := filepath.Join(claudeDir2, "projects/-home-alice-code-foo/sessions/abc.jsonl")
	data, err := os.ReadFile(linuxPath)
	if err != nil {
		t.Fatalf("expected file at Linux path %q: %v", linuxPath, err)
	}
	if string(data) != `{"event":"mac"}` {
		t.Errorf("content mismatch: got %q", string(data))
	}
}

func TestSyncerProjectPath_NonHomePaths_Unchanged(t *testing.T) {
	// Projects outside home should sync with their literal path on both machines.
	syncer1, store, claudeDir1 := testSyncer(t)
	syncer1.WithHomeDir("/Users/alice")
	ctx := context.Background()

	// A project at /opt/shared/proj — encoded as -opt-shared-proj
	projectPath := "projects/-opt-shared-proj/sessions/f.jsonl"
	createTestFile(t, claudeDir1, projectPath, `{"event":"shared"}`)

	if _, err := syncer1.Push(ctx); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	objs, _ := store.List(ctx, "")
	want := "projects/-opt-shared-proj/sessions/f.jsonl.age"
	found := false
	for _, obj := range objs {
		if obj.Key == want {
			found = true
		}
	}
	if !found {
		t.Errorf("non-home project path should be stored as-is (%q not found)", want)
	}
}
