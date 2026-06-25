package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Finding is a single issue returned by the AI reviewer.
type Finding struct {
	Line     int    `json:"line"`
	Severity string `json:"severity"` // blocker | major | minor | info
	Message  string `json:"message"`
}

type reviewResponse struct {
	Findings []Finding `json:"findings"`
}

// Reviewer calls the OpenAI chat completions API to review code diffs.
type Reviewer struct {
	apiKey  string
	model   string
	rules   []string
	baseURL string
	client  *http.Client
}

// NewReviewer constructs a Reviewer. Set OPENAI_BASE_URL to use a different
// endpoint than the OpenAI default (for example, an Azure OpenAI deployment).
func NewReviewer(apiKey, model string, rules []string) *Reviewer {
	baseURL := "https://api.openai.com/v1"
	if u := os.Getenv("OPENAI_BASE_URL"); u != "" {
		baseURL = strings.TrimRight(u, "/")
	}
	return &Reviewer{
		apiKey:  apiKey,
		model:   model,
		rules:   rules,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

// ReviewFile sends a single file's annotated diff to the model and returns findings.
func (r *Reviewer) ReviewFile(filePath, diff string) ([]Finding, error) {
	userMessage := fmt.Sprintf("File: %s\n\n```diff\n%s\n```", filePath, diff)

	payload := map[string]any{
		"model": r.model,
		"messages": []map[string]string{
			{"role": "system", "content": r.systemPrompt()},
			{"role": "user", "content": userMessage},
		},
		// json_object mode guarantees a valid JSON response.
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.1,
		"max_tokens":      2048,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, r.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI %d: %s", resp.StatusCode, body)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse OpenAI response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI returned no choices")
	}

	var result reviewResponse
	if err := json.Unmarshal([]byte(apiResp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("parse review JSON from model output: %w", err)
	}
	return result.Findings, nil
}

func (r *Reviewer) systemPrompt() string {
	var b strings.Builder
	b.WriteString(`You are an expert code reviewer integrated into a CI/CD pipeline.

Analyze the provided file diff and return a JSON object with a "findings" array.
Each finding must have exactly these fields:
  "line":     integer, the new-file line number where the issue appears
              (must correspond to a line prefixed with "+" in the diff)
  "severity": one of: "blocker", "major", "minor", "info"
  "message":  a concise, actionable description of the issue

Severity definitions:
  blocker: security vulnerabilities, data loss, crashes, incorrect critical logic
  major:   significant bugs, missing error handling, serious performance problems
  minor:   code style, naming conventions, small inefficiencies
  info:    suggestions, documentation, best practices

Rules:
- Only report findings on ADDED lines (lines prefixed with "+" in the diff).
- Do not flag unchanged context lines.
- If there are no issues, return {"findings":[]}.

`)

	if len(r.rules) > 0 {
		b.WriteString("Project-specific rules to enforce:\n")
		for i, rule := range r.rules {
			fmt.Fprintf(&b, "%d. %s\n", i+1, rule)
		}
	}
	return b.String()
}
