package ops

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"wt/internal/aitool"
	"wt/internal/config"
	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/hooks"
	"wt/internal/registry"
	"wt/internal/termenv"
)

// GHStatus is the tri-state result of a GitHub merged check.
type GHStatus int

const (
	GHUnknown   GHStatus = iota // gh unavailable or failed
	GHMerged                    // merged PR found
	GHNotMerged                 // no merged PR
)

// IsBranchMergedViaGH detects squash/rebase merges via `gh pr list`.
func IsBranchMergedViaGH(branch, base, repo string) GHStatus {
	if !git.HasCommand("gh") {
		return GHUnknown
	}
	cmd := exec.Command("gh", "pr", "list",
		"--head", branch, "--base", base,
		"--state", "merged", "--json", "number", "--jq", "length")
	cmd.Dir = repo
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return GHUnknown
	}
	count, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if err != nil {
		return GHUnknown
	}
	if count > 0 {
		return GHMerged
	}
	return GHNotMerged
}

// generatePRDescriptionWithAI creates a PR title/body from commit history.
// Returns ("","") when AI is not configured, fails, or output is unparseable.
func generatePRDescriptionWithAI(cfg map[string]any, feature, base, cwd string) (string, string) {
	aiCmd := aitool.EffectiveCommand(cfg)
	if len(aiCmd) == 0 || aiCmd[0] == "echo" {
		return "", ""
	}

	log, err := git.Output(cwd, "log", base+".."+feature,
		"--pretty=format:Commit: %h%nAuthor: %an%nDate: %ad%nMessage: %s%n%b%n---", "--date=short")
	if err != nil || log == "" {
		return "", ""
	}
	diffStats, _ := git.Output(cwd, "diff", "--stat", base+"..."+feature)

	prompt := fmt.Sprintf(`Analyze the following git commits and generate a pull request title and description.

Branch: %s -> %s

Commits:
%s

Diff Statistics:
%s

Please provide:
1. A concise PR title (one line, following conventional commit format if applicable)
2. A detailed PR description with:
   - Summary of changes (2-3 sentences)
   - Test plan (bullet points)

Format your response EXACTLY as:
TITLE: <your title here>
BODY:
<your body here>
`, feature, base, log, diffStats)

	termenv.Info("%s", termenv.Yellow("Generating PR description with AI..."))

	args := append(append([]string{}, aiCmd...), prompt)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		termenv.Warn("AI tool failed or timed out\n")
		return "", ""
	}

	output := stdout.String()
	var title, body string
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "TITLE:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "TITLE:"))
		} else if strings.HasPrefix(line, "BODY:") {
			body = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			break
		}
	}
	if title == "" || body == "" {
		termenv.Warn("Could not parse AI output\n")
		return "", ""
	}
	termenv.Success("AI generated PR description\n")
	termenv.Info("%s %s", termenv.Dim("Title:"), title)
	preview := body
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	termenv.Info("%s %s\n", termenv.Dim("Body preview:"), preview)
	return title, body
}

// PROptions parameterizes CreatePR.
type PROptions struct {
	Target     string
	NoPush     bool
	Title      string
	Body       string
	Draft      bool
	LookupMode LookupMode
	Global     bool
}

// CreatePR rebases, pushes, and opens a GitHub Pull Request via gh.
func CreatePR(opts PROptions) error {
	if !git.HasCommand("gh") {
		return wterrors.New(wterrors.ErrConfig,
			"GitHub CLI (gh) is required to create pull requests.\nInstall it from: https://cli.github.com/")
	}

	t, err := ResolveWorktreeTarget(opts.Target, opts.LookupMode, opts.Global)
	if err != nil {
		return err
	}
	cwd, feature := t.WorktreePath, t.Branch

	baseBranch, basePath, err := WorktreeMetadata(feature, t.WorktreeRepo)
	if err != nil {
		return err
	}
	repo := basePath

	// Safety: a PR from a branch into itself is meaningless (e.g. running
	// `wt pr` inside the main working tree on the base branch).
	if feature == baseBranch {
		return wterrors.New(wterrors.ErrProtectedWorktree,
			"cannot create a PR from branch '%s' into itself.\nHint: 'wt pr' is meant to be run inside a feature worktree created with 'wt new'.", feature)
	}

	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Creating Pull Request:")))
	termenv.Info("  Feature:     %s", termenv.Green(feature))
	termenv.Info("  Base:        %s", termenv.Green(baseBranch))
	termenv.Info("  Repo:        %s\n", termenv.Cyan(repo))

	hookCtx := hooks.Context{
		Branch: feature, BaseBranch: baseBranch,
		WorktreePath: cwd, RepoPath: repo, Operation: "pr",
	}
	if err := hooks.RunHooks(repo, "pr.pre", hookCtx, cwd); err != nil {
		return err
	}

	termenv.Info("%s", termenv.Yellow("Fetching updates from remote..."))
	fetchRes, _ := git.Git(repo, false, "fetch", "--all", "--prune")
	rebaseTarget := baseBranch
	if fetchRes.ExitCode == 0 {
		if res, _ := git.Git(cwd, false, "rev-parse", "--verify", "origin/"+baseBranch); res.ExitCode == 0 {
			rebaseTarget = "origin/" + baseBranch
		}
	}

	termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Rebasing %s onto %s...", feature, rebaseTarget)))
	if _, err := git.Git(cwd, true, "rebase", rebaseTarget); err != nil {
		conflicts := conflictedFiles(cwd)
		_, _ = git.Git(cwd, false, "rebase", "--abort")
		return rebaseError(cwd, rebaseTarget, conflicts, false)
	}
	termenv.Success("Rebase successful")
	fmt.Println()

	if !opts.NoPush {
		termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Pushing %s to origin...", feature)))
		if _, err := git.Git(cwd, true, "push", "-u", "origin", feature); err != nil {
			termenv.Warn("Push failed: %v\n", err)
			return err
		}
		termenv.Success("Pushed to origin")
		fmt.Println()
	}

	termenv.Info("%s", termenv.Yellow("Creating pull request..."))
	prArgs := []string{"pr", "create", "--base", baseBranch}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	title, body := opts.Title, opts.Body
	if title != "" {
		prArgs = append(prArgs, "--title", title)
		if body != "" {
			prArgs = append(prArgs, "--body", body)
		}
	} else {
		aiTitle, aiBody := generatePRDescriptionWithAI(cfg, feature, baseBranch, cwd)
		if aiTitle != "" && aiBody != "" {
			prArgs = append(prArgs, "--title", aiTitle, "--body", aiBody)
		} else {
			aiCmd := aitool.EffectiveCommand(cfg)
			if len(aiCmd) > 0 && aiCmd[0] != "echo" {
				return wterrors.New(wterrors.ErrConfig,
					"AI tool is configured but failed to generate PR description.\nPlease either:\n  1. Provide --title and --body explicitly\n  2. Fix your AI tool configuration\n  3. Use 'wt config use-preset no-op' to disable AI generation")
			}
			prArgs = append(prArgs, "--fill")
		}
	}
	if opts.Draft {
		prArgs = append(prArgs, "--draft")
	}

	cmd := exec.Command("gh", prArgs...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return wterrors.Wrap(wterrors.ErrMergeFailed, err,
			"failed to create pull request: %s", strings.TrimSpace(stderr.String()))
	}

	prURL := strings.TrimSpace(stdout.String())
	termenv.Success("Pull request created!")
	fmt.Println()
	termenv.Info("%s %s\n", termenv.Bold("PR URL:"), prURL)
	termenv.Info("%s\n", termenv.Dim("Note: Worktree is still active. Use 'wt delete' to remove it after PR is merged."))

	hookCtx.PRURL = prURL
	_ = hooks.RunHooks(repo, "pr.post", hookCtx, cwd)
	registry.UpdateLastSeen(repo)
	return nil
}
