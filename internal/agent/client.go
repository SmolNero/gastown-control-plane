package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SmolNero/gastown-control-plane/internal/model"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type Info struct {
	Version              string `json:"version"`
	SchemaVersion        int    `json:"schema_version"`
	AgentDownloadBaseURL string `json:"agent_download_base_url"`
}

func NewClient(baseURL, apiKey string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) SendEvents(ctx context.Context, events []model.Event) error {
	return c.postJSON(ctx, "/v1/events", events)
}

func (c *Client) SendSnapshot(ctx context.Context, snapshot model.Snapshot) error {
	return c.postJSON(ctx, "/v1/snapshots", snapshot)
}

func (c *Client) postJSON(ctx context.Context, path string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}

func (c *Client) FetchInfo(ctx context.Context) (Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/info", nil)
	if err != nil {
		return Info{}, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Info{}, fmt.Errorf("server returned %s", resp.Status)
	}
	var info Info
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return Info{}, err
	}
	return info, nil
}
