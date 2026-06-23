package ado

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FileDiff holds all diff chunks for a single changed file.
type FileDiff struct {
	Path   string
	Chunks []DiffChunk
}

// DiffChunk is a contiguous hunk of lines in the unified diff.
type DiffChunk struct {
	NewStart int
	Lines    []DiffLine
}

// DiffLine is one line within a chunk.
type DiffLine struct {
	LineNo  int    // line number in the NEW file (0 for deleted lines)
	Kind    string // "add" | "del" | "context"
	Content string
}

// GetDiff runs `git diff <merge-base>...HEAD` and returns per-file diffs.
// repoPath is the local checkout directory (BUILD_REPOSITORY_LOCALPATH).
// targetBranch is the PR target (SYSTEM_PULLREQUEST_TARGETBRANCHNAME), e.g. "main".
func GetDiff(repoPath, targetBranch string, contextLines int) ([]FileDiff, error) {
	base, err := mergeBase(repoPath, targetBranch)
	if err != nil {
		return nil, fmt.Errorf("find merge base with origin/%s: %w", targetBranch, err)
	}

	// Diff the merge base against HEAD.
	// --diff-filter=ACMRT: only Added, Copied, Modified, Renamed, Type-changed files
	//   (deleted files are skipped since there's nothing to comment on).
	// --no-color: keep escape codes out of the text sent to the model.
	// --unified=N: lines of unchanged context kept around each change. More context
	//   helps the model but costs more tokens, so it's configurable (diffContext).
	out, err := git(repoPath, "diff", base, "HEAD",
		fmt.Sprintf("--unified=%d", contextLines), "--no-color", "--diff-filter=ACMRT")
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	return parseUnifiedDiff(out), nil
}

func mergeBase(repoPath, targetBranch string) (string, error) {
	// Best-effort fetch; ignore errors (branch may already be present).
	//Önce bir fetch yapıyoruz ki merge-base doğru çalışsın. Eğer fetch başarısız olursa, zaten localde varsa merge-base yine çalışır.
	git(repoPath, "fetch", "--depth=50", "origin", targetBranch) //nolint:errcheck

	//targetBranch ile şuan bulunduğumuz branch(HEAD) arasındaki en son ortak commiti(merge-base) buluyoruz. sha bu en son ortak commitin hash'idir.
	sha, err := git(repoPath, "merge-base", "HEAD", "origin/"+targetBranch)
	if err != nil {
		// Shallow clone or no remote? Fall back to the local branch name.
		sha, err = git(repoPath, "merge-base", "HEAD", targetBranch)
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(sha), nil
}

func git(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

// parseUnifiedDiff parses `git diff` output into FileDiff structs.
// The line numbers embedded in DiffLine.LineNo correspond to the new (right-hand) file,
// which is what the ADO comment thread API expects.
func parseUnifiedDiff(raw string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var chunk *DiffChunk
	newLine := 0

	flushChunk := func() {
		if chunk != nil && cur != nil {
			cur.Chunks = append(cur.Chunks, *chunk)
			chunk = nil
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushChunk()
			if cur != nil {
				files = append(files, *cur)
			}
			cur = &FileDiff{}

		case strings.HasPrefix(line, "+++ b/") && cur != nil:
			cur.Path = strings.TrimPrefix(line, "+++ b/")

		case strings.HasPrefix(line, "@@ ") && cur != nil:
			flushChunk()
			newLine = parseHunkStart(line)
			chunk = &DiffChunk{NewStart: newLine}

		case chunk != nil && len(line) > 0:
			switch line[0] {
			case '+':
				chunk.Lines = append(chunk.Lines, DiffLine{LineNo: newLine, Kind: "add", Content: line[1:]})
				newLine++
			case '-':
				// Deleted lines have no line number in the new file.
				chunk.Lines = append(chunk.Lines, DiffLine{Kind: "del", Content: line[1:]})
			default:
				chunk.Lines = append(chunk.Lines, DiffLine{LineNo: newLine, Kind: "context", Content: line[1:]})
				newLine++
			}
		}
	}

	flushChunk()
	if cur != nil {
		files = append(files, *cur)
	}
	return files
}

// parseHunkStart extracts the new-file start line from a unified diff @@ header.
// Header format: @@ -<old_start>[,<old_count>] +<new_start>[,<new_count>] @@
func parseHunkStart(header string) int {
	i := strings.Index(header, " +")
	if i < 0 {
		return 1
	}
	part := header[i+2:]
	// Trim everything after the line count separator or the closing @@.
	if j := strings.IndexAny(part, ",@ "); j >= 0 {
		part = part[:j]
	}
	n, err := strconv.Atoi(part)
	if err != nil || n == 0 {
		return 1
	}
	return n
}
