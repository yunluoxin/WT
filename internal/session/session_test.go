package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wt/internal/testutil"
)

func TestSaveLoadPreservesCreatedAt(t *testing.T) {
	testutil.SetHome(t)
	m := Metadata{Branch: "feat-x", AITool: "claude", WorktreePath: "/tmp/wt-feat-x"}
	if err := Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	first, err := LoadMetadata("feat-x")
	if err != nil {
		t.Fatalf("LoadMetadata: %v", err)
	}
	if first.Branch != "feat-x" || first.WorktreePath != "/tmp/wt-feat-x" {
		t.Errorf("metadata wrong: %+v", first)
	}
	if first.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}

	time.Sleep(10 * time.Millisecond)
	if err := Save(Metadata{Branch: "feat-x", AITool: "codex", WorktreePath: "/tmp/wt-feat-x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	second, _ := LoadMetadata("feat-x")
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed: %v → %v", first.CreatedAt, second.CreatedAt)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) && !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Errorf("UpdatedAt not refreshed")
	}
	if second.AITool != "codex" {
		t.Errorf("AITool not updated: %q", second.AITool)
	}
}

func TestContextRoundTrip(t *testing.T) {
	testutil.SetHome(t)
	if got := LoadContext("b1"); got != "" {
		t.Errorf("expected empty context, got %q", got)
	}
	if err := SaveContext("b1", "resolve conflicts"); err != nil {
		t.Fatal(err)
	}
	if got := LoadContext("b1"); got != "resolve conflicts" {
		t.Errorf("LoadContext = %q", got)
	}
}

func TestDelete(t *testing.T) {
	testutil.SetHome(t)
	Save(Metadata{Branch: "del-me", WorktreePath: "/tmp/x"})
	if err := Delete("del-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMetadata("del-me"); err == nil {
		t.Error("metadata still present after delete")
	}
}

func TestList(t *testing.T) {
	testutil.SetHome(t)
	Save(Metadata{Branch: "a", WorktreePath: "/a"})
	Save(Metadata{Branch: "b", WorktreePath: "/b"})
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(list))
	}
}

func TestExistsViaHistory(t *testing.T) {
	home := testutil.SetHome(t)
	wtPath := "/tmp/wt-smoke/myrepo-feat"
	Save(Metadata{Branch: "feat", WorktreePath: wtPath})

	// No history file yet.
	if Exists("feat") {
		t.Error("Exists true without history.jsonl")
	}

	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0o755)
	entry := map[string]any{"project": wtPath, "sessionId": "abc"}
	data, _ := json.Marshal(entry)
	os.WriteFile(filepath.Join(claudeDir, "history.jsonl"), append(data, '\n'), 0o644)
	if !Exists("feat") {
		t.Error("Exists false with matching history entry")
	}

	// Non-matching project.
	os.WriteFile(filepath.Join(claudeDir, "history.jsonl"),
		[]byte(`{"project": "/other/place"}`+"\n"), 0o644)
	if Exists("feat") {
		t.Error("Exists true for non-matching project")
	}
}

func TestClaudeNativeSessionExists(t *testing.T) {
	home := testutil.SetHome(t)
	wtPath := "/tmp/wt-smoke/myrepo-feat-1"
	encoded := EncodeClaudeProjectPath(wtPath)

	projects := filepath.Join(home, ".claude", "projects", encoded)
	if ClaudeNativeSessionExists(wtPath) {
		t.Error("false positive without projects dir")
	}
	os.MkdirAll(projects, 0o755)
	if ClaudeNativeSessionExists(wtPath) {
		t.Error("false positive without jsonl files")
	}
	os.WriteFile(filepath.Join(projects, "session.jsonl"), []byte("{}"), 0o644)
	if !ClaudeNativeSessionExists(wtPath) {
		t.Error("false negative with jsonl present")
	}
}

func TestClaudeNativeSessionLongPathPrefix(t *testing.T) {
	home := testutil.SetHome(t)
	// Build a path whose encoding exceeds 200 chars.
	long := "/tmp/" + string(make([]byte, 0))
	for i := 0; i < 30; i++ {
		long += "verylongsegmentname/"
	}
	long += "repo-feat"
	encoded := EncodeClaudeProjectPath(long)
	if len(encoded) <= ClaudeSessionPrefixLength {
		t.Skip("encoded path too short for prefix test")
	}
	// Claude truncates: directory name is the 200-char prefix plus maybe more;
	// we simulate a dir starting with the 200-char prefix.
	truncated := encoded[:ClaudeSessionPrefixLength] + "-extra"
	projects := filepath.Join(home, ".claude", "projects", truncated)
	os.MkdirAll(projects, 0o755)
	os.WriteFile(filepath.Join(projects, "s.jsonl"), []byte("{}"), 0o644)
	if !ClaudeNativeSessionExists(long) {
		t.Error("prefix match failed for long encoded path")
	}
}

func TestSanitizeBranchDirs(t *testing.T) {
	testutil.SetHome(t)
	Save(Metadata{Branch: "feat/auth-fix", WorktreePath: "/x"})
	dir := Dir("feat/auth-fix")
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("session dir not created: %v", err)
	}
	want := filepath.Join(filepath.Dir(dir), "feat-auth-fix")
	if dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}
}

func TestEncodeClaudeProjectPath(t *testing.T) {
	got := EncodeClaudeProjectPath("/Users/east/repo-feat")
	want := "-Users-east-repo-feat"
	if got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
	fmt.Println()
}
