package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tawanorg/claude-sync/internal/config"
)

// seedOldStyleProjectKey uploads raw bytes under an old-style absolute-path
// remote key, simulating a file pushed before ~HOME~ normalization.
func seedOldStyleProjectKey(t *testing.T, store *mockStorage, key, content string) {
	t.Helper()
	if err := store.Upload(context.Background(), key, []byte(content)); err != nil {
		t.Fatalf("seedOldStyleProjectKey: upload %s: %v", key, err)
	}
}

// storageKeys returns all keys currently in mock storage.
func storageKeys(t *testing.T, store *mockStorage) map[string]bool {
	t.Helper()
	objs, _ := store.List(context.Background(), "")
	m := make(map[string]bool, len(objs))
	for _, obj := range objs {
		m[obj.Key] = true
	}
	return m
}

// --- MigrateProjectPaths ---

func TestMigrateProjectPaths_MigratesOldKeys(t *testing.T) {
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	seedOldStyleProjectKey(t, store, "projects/-Users-alice-code-foo/sessions/abc.jsonl.age", "data-abc")
	seedOldStyleProjectKey(t, store, "projects/-Users-alice-code-bar/f.jsonl.age", "data-bar")

	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Migrated) != 2 {
		t.Errorf("expected 2 migrated, got %d: %v", len(result.Migrated), result.Migrated)
	}

	keys := storageKeys(t, store)

	// Old keys must be gone
	if keys["projects/-Users-alice-code-foo/sessions/abc.jsonl.age"] {
		t.Error("old key should have been deleted")
	}
	if keys["projects/-Users-alice-code-bar/f.jsonl.age"] {
		t.Error("old key should have been deleted")
	}
	// New keys must exist
	if !keys["projects/~HOME~-code-foo/sessions/abc.jsonl.age"] {
		t.Error("new normalized key missing: projects/~HOME~-code-foo/sessions/abc.jsonl.age")
	}
	if !keys["projects/~HOME~-code-bar/f.jsonl.age"] {
		t.Error("new normalized key missing: projects/~HOME~-code-bar/f.jsonl.age")
	}

	// Content must be preserved
	data, _ := store.Download(ctx, "projects/~HOME~-code-foo/sessions/abc.jsonl.age")
	if string(data) != "data-abc" {
		t.Errorf("content mismatch: got %q, want %q", string(data), "data-abc")
	}
}

func TestMigrateProjectPaths_DryRunMakesNoChanges(t *testing.T) {
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	oldKey := "projects/-Users-alice-code-foo/f.jsonl.age"
	seedOldStyleProjectKey(t, store, oldKey, "content")

	result, err := syncer.MigrateProjectPaths(ctx, true, "")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	// Must report what would be migrated
	if len(result.Migrated) != 1 {
		t.Errorf("expected 1 in Migrated, got %d", len(result.Migrated))
	}
	if result.Migrated[0].OldKey != oldKey {
		t.Errorf("OldKey: got %q, want %q", result.Migrated[0].OldKey, oldKey)
	}
	if result.Migrated[0].NewKey != "projects/~HOME~-code-foo/f.jsonl.age" {
		t.Errorf("NewKey: got %q", result.Migrated[0].NewKey)
	}

	// Storage must be untouched
	objs, _ := store.List(ctx, "")
	if len(objs) != 1 || objs[0].Key != oldKey {
		t.Errorf("dry-run must not modify storage, got: %v", objs)
	}
}

func TestMigrateProjectPaths_SkipsAlreadyNormalized(t *testing.T) {
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	seedOldStyleProjectKey(t, store, "projects/~HOME~-code-foo/f.jsonl.age", "content")

	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Migrated) != 0 {
		t.Errorf("expected 0 migrated, got %d", len(result.Migrated))
	}
	if result.AlreadyNormalized != 1 {
		t.Errorf("expected 1 AlreadyNormalized, got %d", result.AlreadyNormalized)
	}

	// Key must be untouched
	keys := storageKeys(t, store)
	if !keys["projects/~HOME~-code-foo/f.jsonl.age"] {
		t.Error("normalized key should be untouched")
	}
}

func TestMigrateProjectPaths_SkipsNonHomeProjects(t *testing.T) {
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	// Project under /opt — different home prefix
	seedOldStyleProjectKey(t, store, "projects/-opt-shared-proj/f.jsonl.age", "content")

	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Migrated) != 0 {
		t.Errorf("expected 0 migrated, got %d", len(result.Migrated))
	}
	if result.DifferentHome != 1 {
		t.Errorf("expected 1 DifferentHome, got %d", result.DifferentHome)
	}
}

func TestMigrateProjectPaths_SkipsNonProjectKeys(t *testing.T) {
	// Non-projects/ keys should not appear at all since we list with prefix "projects/"
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	seedOldStyleProjectKey(t, store, "CLAUDE.md.age", "content")
	seedOldStyleProjectKey(t, store, "settings.json.age", "content")

	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Migrated) != 0 {
		t.Errorf("expected 0 migrated, got %d", len(result.Migrated))
	}
	if result.AlreadyNormalized != 0 || result.DifferentHome != 0 {
		t.Errorf("expected 0 skipped (non-project keys are not listed), got AlreadyNormalized=%d DifferentHome=%d",
			result.AlreadyNormalized, result.DifferentHome)
	}
}

func TestMigrateProjectPaths_EmptyBucket(t *testing.T) {
	syncer, _, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Migrated) != 0 || result.AlreadyNormalized != 0 || result.DifferentHome != 0 || len(result.Errors) != 0 {
		t.Errorf("expected empty result for empty bucket, got: %+v", result)
	}
}

func TestMigrateProjectPaths_EmptyHomeDir_IsNoop(t *testing.T) {
	syncer, store, _ := testSyncer(t)
	// homeDir is "" — normalization disabled
	ctx := context.Background()

	seedOldStyleProjectKey(t, store, "projects/-Users-alice-code-foo/f.jsonl.age", "content")

	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Migrated) != 0 {
		t.Errorf("empty homeDir should produce 0 migrations, got %d", len(result.Migrated))
	}
}

func TestMigrateProjectPaths_PreservesEncryptedBytes(t *testing.T) {
	// Push a real encrypted file with an old-style key (homeDir=""), migrate,
	// then pull on a "Linux" syncer and verify content is intact.
	syncer, store, claudeDir := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	// Push with homeDir="" so the remote key keeps the raw local path (old style)
	rawSyncer := NewSyncerWith(&config.Config{}, store, syncer.encryptor, syncer.state, claudeDir, true)
	createTestFile(t, claudeDir, "projects/-Users-alice-code-myapp/sessions/f.jsonl", `{"event":"old"}`)
	if _, err := rawSyncer.Push(ctx); err != nil {
		t.Fatalf("raw push failed: %v", err)
	}

	// Confirm old-style key exists
	objs, _ := store.List(ctx, "projects/")
	if len(objs) != 1 || objs[0].Key != "projects/-Users-alice-code-myapp/sessions/f.jsonl.age" {
		t.Fatalf("expected old-style key in storage, got: %v", objs)
	}

	// Migrate
	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Migrated) != 1 {
		t.Fatalf("expected 1 migrated, got %d: %v", len(result.Migrated), result.Migrated)
	}

	// Pull with a Linux syncer sharing the same storage + encryptor
	linuxTmpDir := t.TempDir()
	linuxClaudeDir := filepath.Join(linuxTmpDir, ".claude")
	linuxStateDir := filepath.Join(linuxTmpDir, ".claude-sync")
	if err := os.MkdirAll(linuxClaudeDir, 0755); err != nil {
		t.Fatalf("mkdir linuxClaudeDir: %v", err)
	}
	if err := os.MkdirAll(linuxStateDir, 0700); err != nil {
		t.Fatalf("mkdir linuxStateDir: %v", err)
	}
	linuxState, _ := LoadStateFromDir(linuxStateDir)
	linuxSyncer := NewSyncerWith(&config.Config{}, store, syncer.encryptor, linuxState, linuxClaudeDir, true)
	linuxSyncer.WithHomeDir("/home/alice")

	pullResult, err := linuxSyncer.Pull(ctx)
	if err != nil {
		t.Fatalf("Linux pull failed: %v", err)
	}
	if len(pullResult.Downloaded) != 1 {
		t.Fatalf("expected 1 downloaded, got %d: %v", len(pullResult.Downloaded), pullResult.Downloaded)
	}

	// File must land at the Linux-local path
	linuxPath := filepath.Join(linuxClaudeDir, "projects/-home-alice-code-myapp/sessions/f.jsonl")
	data, err := os.ReadFile(linuxPath)
	if err != nil {
		t.Fatalf("file not found at Linux path %q: %v", linuxPath, err)
	}
	if string(data) != `{"event":"old"}` {
		t.Errorf("content mismatch: got %q", string(data))
	}
}

func TestMigrateProjectPaths_MixedKeys(t *testing.T) {
	// Bucket has: 1 old-style (migratable), 1 already normalized, 1 non-home
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	seedOldStyleProjectKey(t, store, "projects/-Users-alice-code-foo/f.jsonl.age", "old")
	seedOldStyleProjectKey(t, store, "projects/~HOME~-code-bar/f.jsonl.age", "normalized")
	seedOldStyleProjectKey(t, store, "projects/-opt-shared-proj/f.jsonl.age", "non-home")

	result, err := syncer.MigrateProjectPaths(ctx, false, "")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Migrated) != 1 {
		t.Errorf("expected 1 migrated, got %d", len(result.Migrated))
	}
	if result.AlreadyNormalized != 1 {
		t.Errorf("expected 1 AlreadyNormalized, got %d", result.AlreadyNormalized)
	}
	if result.DifferentHome != 1 {
		t.Errorf("expected 1 DifferentHome, got %d", result.DifferentHome)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestMigrateProjectPaths_FromHome_MigratesLinuxKeysOnMac(t *testing.T) {
	// Simulates running migrate-paths on macOS with --from-home /home/alice
	// to migrate keys that were pushed from a Linux machine.
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice") // current machine = macOS
	ctx := context.Background()

	// Keys pushed from Linux (home = /home/alice)
	seedOldStyleProjectKey(t, store, "projects/-home-alice-code-foo/f.jsonl.age", "linux-data")
	seedOldStyleProjectKey(t, store, "projects/-home-alice-code-bar/sessions/abc.jsonl.age", "linux-sessions")

	result, err := syncer.MigrateProjectPaths(ctx, false, "/home/alice")
	if err != nil {
		t.Fatalf("MigrateProjectPaths failed: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Migrated) != 2 {
		t.Errorf("expected 2 migrated, got %d: %v", len(result.Migrated), result.Migrated)
	}

	keys := storageKeys(t, store)

	// Old Linux keys must be gone
	if keys["projects/-home-alice-code-foo/f.jsonl.age"] {
		t.Error("old Linux key should have been deleted")
	}
	// New canonical keys must exist
	if !keys["projects/~HOME~-code-foo/f.jsonl.age"] {
		t.Error("normalized key missing: projects/~HOME~-code-foo/f.jsonl.age")
	}
	if !keys["projects/~HOME~-code-bar/sessions/abc.jsonl.age"] {
		t.Error("normalized key missing: projects/~HOME~-code-bar/sessions/abc.jsonl.age")
	}
}

func TestMigrateProjectPaths_FromHome_DryRun(t *testing.T) {
	syncer, store, _ := testSyncer(t)
	syncer.WithHomeDir("/Users/alice")
	ctx := context.Background()

	oldKey := "projects/-home-alice-code-foo/f.jsonl.age"
	seedOldStyleProjectKey(t, store, oldKey, "content")

	result, err := syncer.MigrateProjectPaths(ctx, true, "/home/alice")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if len(result.Migrated) != 1 {
		t.Errorf("expected 1 in Migrated, got %d", len(result.Migrated))
	}
	if result.Migrated[0].OldKey != oldKey {
		t.Errorf("OldKey: got %q, want %q", result.Migrated[0].OldKey, oldKey)
	}
	if result.Migrated[0].NewKey != "projects/~HOME~-code-foo/f.jsonl.age" {
		t.Errorf("NewKey: got %q", result.Migrated[0].NewKey)
	}

	// Storage must be untouched
	objs, _ := store.List(ctx, "")
	if len(objs) != 1 || objs[0].Key != oldKey {
		t.Errorf("dry-run must not modify storage")
	}
}
