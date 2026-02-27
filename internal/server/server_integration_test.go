package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"

	"github.com/SmolNero/gastown-control-plane/internal/db"
	"github.com/SmolNero/gastown-control-plane/internal/model"
	"github.com/SmolNero/gastown-control-plane/internal/store"
	"github.com/SmolNero/gastown-control-plane/internal/util"
)

func TestServerIngestAndQuery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("embedded postgres not supported on windows in this test")
	}

	database := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(54329).
			Database("gtcp").
			Username("gtcp").
			Password("gtcp"),
	)
	if err := database.Start(); err != nil {
		t.Fatalf("start embedded postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Stop()
	})

	ctx := context.Background()
	pool, err := db.Open(ctx, "postgres://gtcp:gtcp@localhost:54329/gtcp?sslmode=disable")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	st := store.New(pool)
	orgID, err := st.CreateOrganization(ctx, "Test")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	workspaceID, err := st.CreateWorkspace(ctx, orgID, "Dev")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	apiToken, err := util.RandomToken(16)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := st.CreateAPIKey(ctx, workspaceID, "test", apiToken); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	server := httptest.NewServer(New(st, Config{SchemaVersion: 1, Version: "test"}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	event := model.Event{
		SchemaVersion: 1,
		Type:          "heartbeat",
		Rig:           "alpha",
		Agent:         "mayor",
		Status:        "ok",
		OccurredAt:    time.Now().UTC(),
	}
	if err := postJSON(client, server.URL+"/v1/events", apiToken, event); err != nil {
		t.Fatalf("post event: %v", err)
	}

	snapshot := model.Snapshot{
		SchemaVersion: 1,
		Source:        "gt-agent",
		Workspace:     "/tmp/gt",
		Host:          "host",
		CollectedAt:   time.Now().UTC(),
		Rigs:          []model.Rig{{Name: "alpha", Path: "/tmp/gt/alpha"}},
		Agents:        []model.Agent{{Name: "mayor", Rig: "alpha", Status: "ok"}},
		Hooks:         []model.Hook{{Name: "hook-1", Rig: "alpha", Status: "idle"}},
	}
	if err := postJSON(client, server.URL+"/v1/snapshots", apiToken, snapshot); err != nil {
		t.Fatalf("post snapshot: %v", err)
	}

	rigs := make([]model.Rig, 0)
	if err := getJSON(client, server.URL+"/v1/rigs", apiToken, &rigs); err != nil {
		t.Fatalf("get rigs: %v", err)
	}
	if len(rigs) == 0 {
		t.Fatalf("expected rigs")
	}

	agents := make([]model.Agent, 0)
	if err := getJSON(client, server.URL+"/v1/agents", apiToken, &agents); err != nil {
		t.Fatalf("get agents: %v", err)
	}
	if len(agents) == 0 {
		t.Fatalf("expected agents")
	}

	info := struct {
		Version       string `json:"version"`
		SchemaVersion int    `json:"schema_version"`
	}{}
	if err := getJSON(client, server.URL+"/v1/info", "", &info); err != nil {
		t.Fatalf("get info: %v", err)
	}
	if info.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version: %d", info.SchemaVersion)
	}
}

func postJSON(client *http.Client, url, token string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func getJSON(client *http.Client, url, token string, target interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
