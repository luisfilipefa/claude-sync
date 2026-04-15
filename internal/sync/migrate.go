package sync

import (
	"context"
	"fmt"
	"strings"
)

// MigrateResult describes the outcome of a MigrateProjectPaths operation.
type MigrateResult struct {
	Migrated         []MigratedPath
	AlreadyNormalized int // keys already using ~HOME~
	DifferentHome     int // keys whose home prefix doesn't match sourceHome
	Errors           []error
}

// MigratedPath represents a single remote key migration.
type MigratedPath struct {
	OldKey string
	NewKey string
}

// MigrateProjectPaths re-uploads remote project keys that use old-style
// absolute-path encoding (e.g. projects/-home-alice-code-foo/...) under the
// portable ~HOME~ encoding (e.g. projects/~HOME~-code-foo/...), then deletes
// the old keys.
//
// sourceHome is the home directory whose encoded prefix to look for in remote
// keys. Defaults to s.homeDir if empty. Use this to migrate keys pushed from
// another machine (e.g. pass "/home/alice" while running on macOS to migrate
// Linux-pushed keys).
//
// The raw encrypted bytes are moved as-is — no decrypt/re-encrypt needed.
//
// If dryRun is true, no changes are made to remote storage; the result still
// describes what would be migrated.
func (s *Syncer) MigrateProjectPaths(ctx context.Context, dryRun bool, sourceHome string) (*MigrateResult, error) {
	result := &MigrateResult{}

	if sourceHome == "" {
		sourceHome = s.homeDir
	}

	s.progress(ProgressEvent{Action: "scan", Path: "Scanning remote project keys..."})

	objs, err := s.storage.List(ctx, "projects/")
	if err != nil {
		return nil, fmt.Errorf("failed to list remote objects: %w", err)
	}

	// First pass: collect what needs migrating so we have an accurate total
	var toMigrate []MigratedPath
	for _, obj := range objs {
		if !strings.HasSuffix(obj.Key, ".age") {
			continue
		}
		keyWithoutAge := strings.TrimSuffix(obj.Key, ".age")
		if isNormalized(keyWithoutAge) {
			result.AlreadyNormalized++
			continue
		}
		normalized := encodeProjectPath(keyWithoutAge, sourceHome)
		if normalized == keyWithoutAge {
			result.DifferentHome++
			continue
		}
		toMigrate = append(toMigrate, MigratedPath{OldKey: obj.Key, NewKey: normalized + ".age"})
	}

	total := len(toMigrate)

	if total == 0 {
		s.progress(ProgressEvent{Action: "migrate", Complete: true, Total: 0})
		return result, nil
	}

	if dryRun {
		result.Migrated = toMigrate
		s.progress(ProgressEvent{Action: "migrate", Complete: true, Total: total})
		return result, nil
	}

	// Second pass: migrate
	for i, mp := range toMigrate {
		s.progress(ProgressEvent{
			Action:  "migrate",
			Path:    mp.OldKey,
			Current: i + 1,
			Total:   total,
		})

		// Move raw encrypted bytes — no decrypt/re-encrypt needed
		data, err := s.storage.Download(ctx, mp.OldKey)
		if err != nil {
			s.progress(ProgressEvent{Action: "migrate", Path: mp.OldKey, Error: err})
			result.Errors = append(result.Errors, fmt.Errorf("download %s: %w", mp.OldKey, err))
			continue
		}

		if err := s.storage.Upload(ctx, mp.NewKey, data); err != nil {
			s.progress(ProgressEvent{Action: "migrate", Path: mp.OldKey, Error: err})
			result.Errors = append(result.Errors, fmt.Errorf("upload %s: %w", mp.NewKey, err))
			continue
		}

		// Delete old key only after the new one is safely written
		if err := s.storage.Delete(ctx, mp.OldKey); err != nil {
			// Non-fatal — new key is safely uploaded; old key is now orphaned but harmless
			result.Errors = append(result.Errors, fmt.Errorf("delete old key %s: %w", mp.OldKey, err))
		}

		result.Migrated = append(result.Migrated, mp)
	}

	s.progress(ProgressEvent{Action: "migrate", Complete: true, Total: total})

	return result, nil
}

// isNormalized reports whether a project key already uses the ~HOME~ token.
func isNormalized(keyWithoutAge string) bool {
	const prefix = "projects/"
	if !strings.HasPrefix(keyWithoutAge, prefix) {
		return false
	}
	rest := keyWithoutAge[len(prefix):]
	idx := strings.Index(rest, "/")
	var dir string
	if idx == -1 {
		dir = rest
	} else {
		dir = rest[:idx]
	}
	return strings.HasPrefix(dir, homeToken)
}
