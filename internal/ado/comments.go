package ado

import (
	"fmt"
	"log"
	"os"
)

// PostComment creates an inline comment thread on the specified PR.
// filePath is relative to the repo root (e.g. "src/main.go").
// line is the line number in the new (right-hand) file; pass 0 for a PR-level comment.
func (c *Client) PostComment(prID int, filePath string, line int, content string) error {
	if os.Getenv("DRY_RUN") == "1" {
		log.Printf("[DRY RUN] would post to PR #%d  %s:%d\n%s\n", prID, filePath, line, content)
		return nil
	}

	thread := threadRequest{
		Comments: []commentRequest{
			{ParentCommentID: 0, Content: content, CommentType: 1},
		},
		Status: "active",
	}

	if filePath != "" && line > 0 {
		thread.ThreadContext = &threadContext{
			// ADO expects a leading slash on the file path.
			FilePath:       "/" + filePath,
			RightFileStart: &position{Line: line, Offset: 1},
			RightFileEnd:   &position{Line: line, Offset: 1},
		}
	}

	url := fmt.Sprintf("%s/pullRequests/%d/threads?api-version=7.1", c.repoBase(), prID)
	return c.post(url, thread, nil)
}

// --- request body types (unexported; only used for JSON serialisation) ---

type threadRequest struct {
	Comments      []commentRequest `json:"comments"`
	Status        string           `json:"status"`
	ThreadContext *threadContext   `json:"threadContext,omitempty"`
}

type commentRequest struct {
	ParentCommentID int    `json:"parentCommentId"`
	Content         string `json:"content"`
	CommentType     int    `json:"commentType"` // 1 = text
}

type threadContext struct {
	FilePath       string    `json:"filePath"`
	RightFileStart *position `json:"rightFileStart,omitempty"`
	RightFileEnd   *position `json:"rightFileEnd,omitempty"`
}

type position struct {
	Line   int `json:"line"`
	Offset int `json:"offset"`
}
