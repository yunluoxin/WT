package git

import (
	"fmt"
	"strings"
)

// Metadata key templates stored in local git config.
const (
	KeyWorktreeBase    = "branch.%s.worktreeBase"
	KeyBasePath        = "worktree.%s.basePath"
	KeyIntendedBranch  = "worktree.%s.intendedBranch"
	KeyCwsharePrompted = "cwshare.prompted"
)

// GetConfig reads a local git config value; returns "" when unset.
func GetConfig(repo, key string) string {
	res, err := Git(repo, false, "config", "--local", "--get", key)
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

// SetConfig writes a local git config value.
func SetConfig(repo, key, value string) error {
	_, err := Git(repo, true, "config", "--local", key, value)
	return err
}

// UnsetConfig removes a local git config key (all values); missing keys are fine.
func UnsetConfig(repo, key string) {
	_, _ = Git(repo, false, "config", "--local", "--unset-all", key)
}

// ConfigRegexp returns key/value pairs matching the given regexp.
func ConfigRegexp(repo, pattern string) map[string]string {
	res, err := Git(repo, false, "config", "--local", "--get-regexp", pattern)
	out := map[string]string{}
	if err != nil || res.ExitCode != 0 {
		return out
	}
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		if line == "" {
			continue
		}
		k, v, _ := strings.Cut(line, " ")
		out[k] = v
	}
	return out
}

// MetadataKey formats a metadata key template with the branch name.
func MetadataKey(tmpl, branch string) string {
	return fmt.Sprintf(tmpl, branch)
}

// FindWorktreeByIntendedBranch locates a worktree by its intended branch
// (survives branch switches inside the worktree). Three strategies:
// 1. direct branch match; 2. intendedBranch metadata scan + path convention;
// 3. path naming convention <repo>-<sanitized-branch>.
func FindWorktreeByIntendedBranch(repo, branch string, sanitize func(string) string) (Worktree, bool, error) {
	if wt, found, err := FindWorktreeByBranch(repo, branch); err != nil || found {
		return wt, found, err
	}

	wts, err := ParseWorktrees(repo)
	if err != nil {
		return Worktree{}, false, err
	}

	// Strategy 2: scan worktree.*.intendedBranch metadata.
	meta := ConfigRegexp(repo, `^worktree\..*\.intendedBranch`)
	for key, intended := range meta {
		if intended != branch {
			continue
		}
		// key = worktree.<branch>.intendedBranch
		name := strings.TrimSuffix(strings.TrimPrefix(key, "worktree."), ".intendedBranch")
		expectedSuffix := "-" + sanitize(name)
		for _, wt := range wts {
			base := filepathBase(wt.Path)
			if strings.HasSuffix(base, expectedSuffix) || base == name {
				return wt, true, nil
			}
		}
	}

	// Strategy 3: path convention fallback.
	expected := filepathBase(repo) + "-" + sanitize(branch)
	for _, wt := range wts {
		if filepathBase(wt.Path) == expected {
			return wt, true, nil
		}
	}
	return Worktree{}, false, nil
}

func filepathBase(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
