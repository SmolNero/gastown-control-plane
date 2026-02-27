package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SmolNero/gastown-control-plane/internal/agent"
	"github.com/SmolNero/gastown-control-plane/internal/model"
)

type agentConfig struct {
	APIURL           string
	APIKey           string
	Workspace        string
	SpoolDir         string
	PollInterval     time.Duration
	SnapshotInterval time.Duration
	AgentName        string
	Source           string
	CheckUpdates     bool
	AgentVersion     string
}

func main() {
	command := "run"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "run":
		run(os.Args[2:])
	case "emit":
		emit(os.Args[2:])
	case "snapshot":
		snapshotOnce(os.Args[2:])
	case "version":
		fmt.Printf("gt-agent %s\n", agentVersion())
	default:
		fmt.Println("Usage: gt-agent [run|emit|snapshot|version]")
		os.Exit(1)
	}
}

func run(args []string) {
	cfg := loadConfig(args)
	if cfg.APIKey == "" {
		log.Fatal("GTCP_API_KEY is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := agent.NewClient(cfg.APIURL, cfg.APIKey)
	if cfg.CheckUpdates {
		checkForUpdates(ctx, client, cfg)
	}
	if err := sendSnapshot(ctx, client, cfg); err != nil {
		log.Printf("snapshot failed: %v", err)
	}

	heartbeatTicker := time.NewTicker(cfg.PollInterval)
	snapshotTicker := time.NewTicker(cfg.SnapshotInterval)
	defer heartbeatTicker.Stop()
	defer snapshotTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			if err := sendHeartbeat(ctx, client, cfg); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
			if err := sendSpool(ctx, client, cfg); err != nil {
				log.Printf("spool send failed: %v", err)
			}
		case <-snapshotTicker.C:
			if err := sendSnapshot(ctx, client, cfg); err != nil {
				log.Printf("snapshot failed: %v", err)
			}
		}
	}
}

func emit(args []string) {
	fs := flag.NewFlagSet("emit", flag.ExitOnError)
	apiURL := fs.String("api-url", envOrDefault("GTCP_API_URL", "http://localhost:8080"), "api url")
	apiKey := fs.String("api-key", os.Getenv("GTCP_API_KEY"), "api key")
	eventType := fs.String("type", "", "event type")
	rig := fs.String("rig", "", "rig name")
	agentName := fs.String("agent", "", "agent name")
	convoy := fs.String("convoy", "", "convoy name")
	hook := fs.String("hook", "", "hook name")
	status := fs.String("status", "", "status")
	message := fs.String("message", "", "message")
	payload := fs.String("payload", "", "payload json")
	_ = fs.Parse(args)

	if *apiKey == "" || *eventType == "" {
		log.Fatal("--type and --api-key are required")
	}

	var payloadRaw json.RawMessage
	if *payload != "" {
		if !json.Valid([]byte(*payload)) {
			log.Fatal("--payload must be valid json")
		}
		payloadRaw = json.RawMessage(*payload)
	}

	client := agent.NewClient(*apiURL, *apiKey)
	event := model.Event{
		SchemaVersion: 1,
		Type:          *eventType,
		Rig:           *rig,
		Agent:         *agentName,
		Convoy:        *convoy,
		Hook:          *hook,
		Status:        *status,
		Message:       *message,
		Payload:       payloadRaw,
		OccurredAt:    time.Now().UTC(),
	}
	if err := client.SendEvents(context.Background(), []model.Event{event}); err != nil {
		log.Fatalf("emit failed: %v", err)
	}
	fmt.Println("event sent")
}

func snapshotOnce(args []string) {
	cfg := loadConfig(args)
	if cfg.APIKey == "" {
		log.Fatal("GTCP_API_KEY is required")
	}
	client := agent.NewClient(cfg.APIURL, cfg.APIKey)
	if err := sendSnapshot(context.Background(), client, cfg); err != nil {
		log.Fatalf("snapshot failed: %v", err)
	}
	fmt.Println("snapshot sent")
}

func sendHeartbeat(ctx context.Context, client *agent.Client, cfg agentConfig) error {
	event := model.Event{
		SchemaVersion: 1,
		Type:          "heartbeat",
		Source:        cfg.Source,
		Agent:         cfg.AgentName,
		Status:        "ok",
		OccurredAt:    time.Now().UTC(),
	}
	return client.SendEvents(ctx, []model.Event{event})
}

func sendSnapshot(ctx context.Context, client *agent.Client, cfg agentConfig) error {
	rigs, agents, hooks, err := agent.ScanWorkspace(cfg.Workspace)
	if err != nil {
		return err
	}
	snapshot := model.Snapshot{
		SchemaVersion: 1,
		Source:        cfg.Source,
		Workspace:     cfg.Workspace,
		Host:          cfg.AgentName,
		CollectedAt:   time.Now().UTC(),
		Rigs:          rigs,
		Agents:        agents,
		Hooks:         hooks,
	}
	return client.SendSnapshot(ctx, snapshot)
}

func sendSpool(ctx context.Context, client *agent.Client, cfg agentConfig) error {
	events, processed, err := agent.ReadSpoolEvents(cfg.SpoolDir)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	if err := client.SendEvents(ctx, events); err != nil {
		return err
	}
	for _, path := range processed {
		_ = agent.MarkProcessed(path)
	}
	return nil
}

func loadConfig(args []string) agentConfig {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	apiURL := fs.String("api-url", envOrDefault("GTCP_API_URL", "http://localhost:8080"), "api url")
	apiKey := fs.String("api-key", os.Getenv("GTCP_API_KEY"), "api key")
	workspace := fs.String("workspace", envOrDefault("GTCP_WORKSPACE", mustCwd()), "workspace path")
	spoolDir := fs.String("spool-dir", "", "spool directory")
	pollInterval := fs.Duration("poll-interval", envDuration("GTCP_POLL_INTERVAL", 15*time.Second), "poll interval")
	snapshotInterval := fs.Duration("snapshot-interval", envDuration("GTCP_SNAPSHOT_INTERVAL", 60*time.Second), "snapshot interval")
	agentName := fs.String("agent-name", envOrDefault("GTCP_AGENT_NAME", hostname()), "agent name")
	source := fs.String("source", envOrDefault("GTCP_SOURCE", "gt-agent"), "event source")
	checkUpdates := fs.Bool("check-updates", envBool("GTCP_CHECK_UPDATES", true), "check for agent updates")
	agentVersion := fs.String("agent-version", envOrDefault("GTCP_AGENT_VERSION", "dev"), "agent version")
	_ = fs.Parse(args)

	resolvedSpool := *spoolDir
	if resolvedSpool == "" {
		resolvedSpool = filepath.Join(*workspace, ".gtcp", "spool")
	}

	return agentConfig{
		APIURL:           *apiURL,
		APIKey:           *apiKey,
		Workspace:        *workspace,
		SpoolDir:         resolvedSpool,
		PollInterval:     *pollInterval,
		SnapshotInterval: *snapshotInterval,
		AgentName:        *agentName,
		Source:           *source,
		CheckUpdates:     *checkUpdates,
		AgentVersion:     *agentVersion,
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil {
		return "agent"
	}
	return host
}

func agentVersion() string {
	if value := os.Getenv("GTCP_AGENT_VERSION"); value != "" {
		return value
	}
	return "dev"
}

func checkForUpdates(ctx context.Context, client *agent.Client, cfg agentConfig) {
	info, err := client.FetchInfo(ctx)
	if err != nil {
		return
	}
	if info.AgentDownloadBaseURL == "" || info.Version == "" {
		return
	}
	if cfg.AgentVersion == "" || cfg.AgentVersion == "dev" {
		return
	}
	if info.Version != cfg.AgentVersion {
		url := fmt.Sprintf("%s/%s/gt-agent-%s-%s", strings.TrimRight(info.AgentDownloadBaseURL, "/"), info.Version, runtime.GOOS, runtime.GOARCH)
		log.Printf("agent update available: %s", url)
	}
}
