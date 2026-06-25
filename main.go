package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nitpicker/internal/ado"
	"nitpicker/internal/ai"
	"nitpicker/internal/config"

	"github.com/bmatcuk/doublestar/v4"
)

// severityRank orders severities so a finding fails the build when its rank is
// at or above the configured failOn level.
var severityRank = map[string]int{
	"blocker": 4,
	"major":   3,
	"minor":   2,
	"info":    1,
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	dryRun := os.Getenv("DRY_RUN") == "1"
	mockAI := os.Getenv("MOCK_AI") == "1"

	if dryRun {
		log.Println("dry run: no comments will be posted to Azure DevOps")
	}

	// In dry-run mode the ADO variables aren't needed, so use placeholders.
	orgURL := envOr("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI", "https://dev.azure.com/dry-run")
	project := envOr("SYSTEM_TEAMPROJECT", "dry-run")
	repoID := envOr("BUILD_REPOSITORY_ID", "dry-run")
	accessToken := envOr("SYSTEM_ACCESSTOKEN", "dry-run")
	if !dryRun {
		orgURL = mustEnv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI")
		project = mustEnv("SYSTEM_TEAMPROJECT")
		repoID = mustEnv("BUILD_REPOSITORY_ID")
		accessToken = mustEnv("SYSTEM_ACCESSTOKEN")
	}

	openAIKey := os.Getenv("OPENAI_API_KEY")
	if openAIKey == "" && !mockAI {
		log.Fatal("OPENAI_API_KEY is not set (set MOCK_AI=1 to run without a real key)")
	}

	repoPath := envOr("BUILD_REPOSITORY_LOCALPATH", ".")
	targetBranch := envOr("SYSTEM_PULLREQUEST_TARGETBRANCHNAME", "main")

	// SYSTEM_PULLREQUEST_PULLREQUESTID is only set on PR builds.
	prIDStr := os.Getenv("SYSTEM_PULLREQUEST_PULLREQUESTID")
	if prIDStr == "" && dryRun {
		prIDStr = "0"
	}
	prID, err := strconv.Atoi(prIDStr)
	if err != nil || prID == 0 {
		if !dryRun {
			log.Println("not a pull request build, skipping review")
			return nil
		}
		prID = 0
	}

	configPath := envOr("CONFIG_PATH", ".codereview.yml")
	cfg, err := config.Load(filepath.Join(repoPath, configPath))
	switch {
	case err == nil:
		// loaded successfully
	case errors.Is(err, os.ErrNotExist):
		// No config file is a supported, zero-config mode: fall back to defaults.
		log.Printf("no %s found, using defaults", configPath)
		cfg = config.Default()
	default:
		// The file exists but could not be parsed. Failing hard here is
		// deliberate: silently using defaults can drop the user's failOn
		// setting and let a broken gate report success.
		return fmt.Errorf("invalid %s: %w", configPath, err)
	}

	// FAIL_ON overrides the config value (set from the task's failOnSeverity input).
	failOn := cfg.FailOn
	if v := os.Getenv("FAIL_ON"); v != "" {
		failOn = v
	}

	log.Printf("reviewing PR #%d (model: %s, failOn: %s)", prID, cfg.OpenAIModel, failOn)

	diffs, err := ado.GetDiff(repoPath, targetBranch, cfg.DiffContext)
	if err != nil {
		return fmt.Errorf("get diff: %w", err)
	}

	diffs = excludeFiles(diffs, cfg.ExcludePatterns)
	if len(diffs) > cfg.MaxFilesPerReview {
		log.Printf("capping review at %d of %d changed files", cfg.MaxFilesPerReview, len(diffs))
		diffs = diffs[:cfg.MaxFilesPerReview]
	}
	log.Printf("reviewing %d file(s)", len(diffs))

	adoClient := ado.NewClient(orgURL, project, repoID, accessToken)
	reviewer := ai.NewReviewer(openAIKey, cfg.OpenAIModel, cfg.Rules)

	failCount := 0
	for _, fd := range diffs {
		diffText := renderDiff(fd)
		if strings.TrimSpace(diffText) == "" {
			continue
		}

		log.Printf("  %s", fd.Path)
		var findings []ai.Finding
		if mockAI {
			findings = fakeFinding(fd)
		} else {
			findings, err = reviewer.ReviewFile(fd.Path, diffText)
			if err != nil {
				log.Printf("  review failed for %s: %v", fd.Path, err)
				continue
			}
		}

		for _, f := range findings {
			if failOn != "none" && severityRank[f.Severity] >= severityRank[failOn] {
				failCount++
			}

			body := fmt.Sprintf("%s **[%s]** %s", severityEmoji(f.Severity), strings.ToUpper(f.Severity), f.Message)
			if err := adoClient.PostComment(prID, fd.Path, f.Line, body); err != nil {
				log.Printf("  could not post comment on %s:%d: %v", fd.Path, f.Line, err)
			} else {
				log.Printf("  posted %s finding on %s:%d", f.Severity, fd.Path, f.Line)
			}
		}
	}

	if failCount > 0 {
		log.Printf("%d finding(s) at or above %q severity", failCount, failOn)
		os.Exit(1)
	}

	log.Println("review complete, no blocking findings")
	return nil
}

// excludeFiles drops files matching any of the given glob patterns.
func excludeFiles(diffs []ado.FileDiff, patterns []string) []ado.FileDiff {
	if len(patterns) == 0 {
		return diffs
	}
	var out []ado.FileDiff
	for _, fd := range diffs {
		excluded := false
		for _, pattern := range patterns {
			if matched, _ := doublestar.Match(pattern, fd.Path); matched {
				excluded = true
				break
			}
		}
		if !excluded {
			out = append(out, fd)
		}
	}
	return out
}

// renderDiff formats a file diff for the prompt. Added lines are prefixed with
// their new-file line number so the model can reference them precisely.
func renderDiff(fd ado.FileDiff) string {
	var b strings.Builder
	for _, chunk := range fd.Chunks {
		fmt.Fprintf(&b, "@@ starting at line %d @@\n", chunk.NewStart)
		for _, l := range chunk.Lines {
			switch l.Kind {
			case "add":
				fmt.Fprintf(&b, "+[%d] %s\n", l.LineNo, l.Content)
			case "del":
				fmt.Fprintf(&b, "-      %s\n", l.Content)
			default:
				fmt.Fprintf(&b, " [%d] %s\n", l.LineNo, l.Content)
			}
		}
	}
	return b.String()
}

func severityEmoji(s string) string {
	switch s {
	case "blocker":
		return "🚫"
	case "major":
		return "⚠️"
	case "minor":
		return "💡"
	default:
		return "ℹ️"
	}
}

// fakeFinding returns a synthetic finding on the first added line, used when
// MOCK_AI=1 to exercise the pipeline without calling OpenAI.
func fakeFinding(fd ado.FileDiff) []ai.Finding {
	for _, chunk := range fd.Chunks {
		for _, l := range chunk.Lines {
			if l.Kind == "add" {
				return []ai.Finding{{
					Line:     l.LineNo,
					Severity: "info",
					Message:  "Mock finding (MOCK_AI=1): no real review was performed.",
				}}
			}
		}
	}
	return nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %q is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
