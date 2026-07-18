package share

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"wt/internal/testutil"
)

func TestEntries(t *testing.T) {
	repo := testutil.NewRepo(t)
	content := `# comment
.env

config/local.json
  # indented comment
.env.local
`
	os.WriteFile(filepath.Join(repo, FileName), []byte(content), 0o644)
	entries := Entries(repo)
	want := []string{".env", "config/local.json", ".env.local"}
	if len(entries) != len(want) {
		t.Fatalf("Entries = %v, want %v", entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("Entries[%d] = %q, want %q", i, entries[i], want[i])
		}
	}
}

func TestEntriesMissingFile(t *testing.T) {
	repo := testutil.NewRepo(t)
	if entries := Entries(repo); entries != nil {
		t.Errorf("expected nil, got %v", entries)
	}
	if HasFile(repo) {
		t.Error("HasFile true without file")
	}
}

func TestCopyToWorktree(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := t.TempDir()

	testutil.WriteFile(t, repo, ".env", "SECRET=1\n")
	testutil.WriteFile(t, repo, "config/local.json", "{}\n")
	os.WriteFile(filepath.Join(repo, FileName), []byte(".env\nconfig/local.json\nmissing.txt\n"), 0o644)

	CopyToWorktree(repo, wt)

	data, err := os.ReadFile(filepath.Join(wt, ".env"))
	if err != nil || string(data) != "SECRET=1\n" {
		t.Errorf(".env not copied: %v %q", err, data)
	}
	if _, err := os.Stat(filepath.Join(wt, "config", "local.json")); err != nil {
		t.Errorf("config/local.json not copied: %v", err)
	}
	// Missing source skipped silently.
	if _, err := os.Stat(filepath.Join(wt, "missing.txt")); !os.IsNotExist(err) {
		t.Error("missing source should be skipped")
	}
	// Existing target not overwritten.
	testutil.WriteFile(t, wt, ".env", "EXISTING=1\n")
	CopyToWorktree(repo, wt)
	data, _ = os.ReadFile(filepath.Join(wt, ".env"))
	if string(data) != "EXISTING=1\n" {
		t.Error("existing target was overwritten")
	}
}

func TestCopyDirectoryWithSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	repo := testutil.NewRepo(t)
	wt := t.TempDir()

	testutil.WriteFile(t, repo, "shared/real.txt", "real\n")
	target := filepath.Join(repo, "shared", "real.txt")
	link := filepath.Join(repo, "shared", "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, FileName), []byte("shared\n"), 0o644)

	CopyToWorktree(repo, wt)

	if _, err := os.Stat(filepath.Join(wt, "shared", "real.txt")); err != nil {
		t.Errorf("real file not copied: %v", err)
	}
	got, err := os.Readlink(filepath.Join(wt, "shared", "link.txt"))
	if err != nil {
		t.Fatalf("symlink not preserved: %v", err)
	}
	if got != target {
		t.Errorf("symlink target = %q, want %q", got, target)
	}
}

func TestCreateTemplate(t *testing.T) {
	repo := testutil.NewRepo(t)
	testutil.WriteFile(t, repo, ".env", "x")
	if err := CreateTemplate(repo, DetectCommonFiles(repo)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !containsStr(string(data), "# .env") {
		t.Errorf("template missing commented .env suggestion:\n%s", data)
	}
	if DetectCommonFiles(repo)[0] != ".env" {
		t.Error("DetectCommonFiles missed .env")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
