package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"wt/internal/testutil"
)

func TestInitCreatesScriptAndRegistersHook(t *testing.T) {
	repo := testutil.NewRepo(t)

	scriptPath, hookID, err := Init(repo)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	wantPath := filepath.Join(repo, ".wt-hooks", "post-create.sh")
	if scriptPath != wantPath {
		t.Errorf("scriptPath = %q, want %q", scriptPath, wantPath)
	}
	if hookID != "post-create.sh" {
		t.Errorf("hookID = %q, want %q", hookID, "post-create.sh")
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("script not written: %v", err)
	}
	if !strings.HasPrefix(string(data), "#!/bin/sh\n") {
		t.Error("script missing shebang")
	}
	if !strings.Contains(string(data), `-name node_modules -o -name .git`) {
		t.Error("script must prune node_modules when scanning for package.json")
	}

	// The template must handle every ecosystem it advertises.
	for _, marker := range []string{
		"package.json", "pnpm install --frozen-lockfile", // JS
		"pyproject.toml", "uv sync --locked", // Python
		"go.mod", "go mod download", // Go
		"Cargo.toml", "cargo fetch --locked", // Rust
		"Package.swift", "swift package resolve", // SPM
		"Podfile", "install --deployment", "bundle exec pod", // CocoaPods
		"build.gradle", "gradlew", // Gradle / Android
		"pubspec.yaml", "pub get", // Flutter / Dart
		"Gemfile", "bundle install", // Ruby
		"composer.json", "composer install", // PHP
		".csproj", "dotnet restore", // .NET
		"pom.xml", "mvn", // Maven
	} {
		if !strings.Contains(string(data), marker) {
			t.Errorf("template missing ecosystem marker %q", marker)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("script not executable: %v", info.Mode().Perm())
		}
	}

	hookMap, err := List(repo, "worktree.post_create")
	if err != nil {
		t.Fatal(err)
	}
	found := hookMap["worktree.post_create"]
	if len(found) != 1 {
		t.Fatalf("expected 1 hook, got %d: %+v", len(found), found)
	}
	h := found[0]
	if h.ID != "post-create.sh" || h.Command != "sh .wt-hooks/post-create.sh" || !h.Enabled {
		t.Errorf("hook registered wrong: %+v", h)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	repo := testutil.NewRepo(t)

	if _, _, err := Init(repo); err != nil {
		t.Fatalf("first Init: %v", err)
	}

	// User edits the script; a second init must not clobber it.
	scriptPath := filepath.Join(repo, ".wt-hooks", "post-create.sh")
	custom := "#!/bin/sh\necho customized\n"
	if err := os.WriteFile(scriptPath, []byte(custom), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Init(repo); err != nil {
		t.Fatalf("second Init: %v", err)
	}

	data, _ := os.ReadFile(scriptPath)
	if string(data) != custom {
		t.Error("second Init overwrote the user's script")
	}

	hookMap, _ := List(repo, "worktree.post_create")
	if n := len(hookMap["worktree.post_create"]); n != 1 {
		t.Errorf("second Init duplicated the hook: %d entries", n)
	}
}

func TestInitPreservesOtherHooks(t *testing.T) {
	repo := testutil.NewRepo(t)

	existing, err := Add(repo, "worktree.post_create", "npm install", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Init(repo); err != nil {
		t.Fatalf("Init: %v", err)
	}

	hookMap, _ := List(repo, "worktree.post_create")
	found := hookMap["worktree.post_create"]
	if len(found) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(found))
	}
	ids := map[string]bool{found[0].ID: true, found[1].ID: true}
	if !ids[existing] || !ids["post-create.sh"] {
		t.Errorf("missing hooks after Init: %+v", found)
	}
}
