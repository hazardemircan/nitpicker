package ado

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a thin Azure DevOps REST API client.
type Client struct {
	orgURL     string
	project    string
	repoID     string
	token      string
	httpClient *http.Client
}

// NewClient constructs a Client.
// orgURL is SYSTEM_TEAMFOUNDATIONCOLLECTIONURI (e.g. https://dev.azure.com/myorg/).
func NewClient(orgURL, project, repoID, token string) *Client {
	return &Client{
		orgURL:     strings.TrimRight(orgURL, "/"),
		project:    project,
		repoID:     repoID,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(url string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c.auth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ADO GET %s → %d: %s", url, resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

func (c *Client) post(url string, reqBody any, out any) error {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ADO POST %s → %d: %s", url, resp.StatusCode, body)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// auth sets the Basic auth header using the Personal Access Token format ADO expects.
// An empty username with the PAT as the password is the standard approach.
func (c *Client) auth(req *http.Request) {
	encoded := base64.StdEncoding.EncodeToString([]byte(":" + c.token))
	req.Header.Set("Authorization", "Basic "+encoded)
}

func (c *Client) repoBase() string {
	return fmt.Sprintf("%s/%s/_apis/git/repositories/%s", c.orgURL, c.project, c.repoID)
}
