package sync

import "strings"

// homeToken is the portable placeholder stored in remote keys in place of the
// machine-specific encoded home directory.
//
// TODO(tech-debt): This is Approach 2 (home-dir normalization). It covers ~95%
// of cases but breaks when usernames differ across machines or when projects
// live outside $HOME. Replace with Approach 3 (content fingerprinting via git
// remote URL / UUID sidecar) for full cross-machine portability.
const homeToken = "~HOME~"

// encodeHome replaces the encoded home prefix with homeToken in an
// already-encoded path segment (slashes already replaced with dashes).
// Guards against partial-prefix false matches (e.g. /Users/a vs /Users/alice).
func encodeHome(absPath, homeDir string) string {
	if homeDir == "" {
		return absPath
	}
	enc := strings.ReplaceAll(strings.TrimRight(homeDir, "/"), "/", "-")
	if !strings.HasPrefix(absPath, enc) {
		return absPath
	}
	rest := absPath[len(enc):]
	// Ensure the match is at a segment boundary: either exact or followed by "-"
	if rest != "" && !strings.HasPrefix(rest, "-") {
		return absPath
	}
	return homeToken + rest
}

// decodeHome replaces homeToken with the encoded home prefix.
func decodeHome(absPath, homeDir string) string {
	if homeDir == "" || !strings.HasPrefix(absPath, homeToken) {
		return absPath
	}
	enc := strings.ReplaceAll(strings.TrimRight(homeDir, "/"), "/", "-")
	return enc + absPath[len(homeToken):]
}

// encodeProjectPath normalizes a local relative path for remote storage.
// Only transforms paths under projects/ whose dir segment starts with the
// encoded home prefix. All other paths are returned unchanged.
//
// Example (macOS):
//
//	"projects/-Users-luis-code-foo/sessions/abc.jsonl" → "projects/~HOME~-code-foo/sessions/abc.jsonl"
func encodeProjectPath(relativePath, homeDir string) string {
	const prefix = "projects/"
	if !strings.HasPrefix(relativePath, prefix) {
		return relativePath
	}
	rest := relativePath[len(prefix):]
	idx := strings.Index(rest, "/")
	var dir, tail string
	if idx == -1 {
		dir, tail = rest, ""
	} else {
		dir, tail = rest[:idx], rest[idx:]
	}
	return prefix + encodeHome(dir, homeDir) + tail
}

// decodeProjectPath converts a normalized remote key back to a local relative
// path. Old-style keys (without ~HOME~) pass through unchanged for backward
// compatibility.
//
// Example (Linux):
//
//	"projects/~HOME~-code-foo/sessions/abc.jsonl" → "projects/-home-luis-code-foo/sessions/abc.jsonl"
func decodeProjectPath(remoteKey, homeDir string) string {
	const prefix = "projects/"
	if !strings.HasPrefix(remoteKey, prefix) {
		return remoteKey
	}
	rest := remoteKey[len(prefix):]
	idx := strings.Index(rest, "/")
	var dir, tail string
	if idx == -1 {
		dir, tail = rest, ""
	} else {
		dir, tail = rest[:idx], rest[idx:]
	}
	return prefix + decodeHome(dir, homeDir) + tail
}
