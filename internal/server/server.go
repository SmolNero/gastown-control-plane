package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/SmolNero/gastown-control-plane/internal/model"
	"github.com/SmolNero/gastown-control-plane/internal/store"
)

type Config struct {
	MaxEventBytes        int64
	MaxSnapshotBytes     int64
	RateLimitPerMinute   int
	SchemaVersion        int
	Version              string
	AgentDownloadBaseURL string
}

type Server struct {
	store   *store.Store
	mux     *http.ServeMux
	config  Config
	limiter *rateLimiter
}

func New(store *store.Store, cfg Config) *Server {
	if cfg.MaxEventBytes == 0 {
		cfg.MaxEventBytes = 1 << 20
	}
	if cfg.MaxSnapshotBytes == 0 {
		cfg.MaxSnapshotBytes = 4 << 20
	}
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = 1
	}
	s := &Server{store: store, mux: http.NewServeMux(), config: cfg}
	if cfg.RateLimitPerMinute > 0 {
		s.limiter = newRateLimiter(cfg.RateLimitPerMinute)
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.Handle("/v1/events", s.withAuth(http.HandlerFunc(s.handleEvents)))
	s.mux.Handle("/v1/snapshots", s.withAuth(http.HandlerFunc(s.handleSnapshots)))
	s.mux.Handle("/v1/rigs", s.withAuth(http.HandlerFunc(s.handleRigs)))
	s.mux.Handle("/v1/agents", s.withAuth(http.HandlerFunc(s.handleAgents)))
	s.mux.Handle("/v1/convoys", s.withAuth(http.HandlerFunc(s.handleConvoys)))
	s.mux.Handle("/v1/hooks", s.withAuth(http.HandlerFunc(s.handleHooks)))
	s.mux.Handle("/v1/activity", s.withAuth(http.HandlerFunc(s.handleActivity)))
	s.mux.Handle("/v1/alerts", s.withAuth(http.HandlerFunc(s.handleAlerts)))
	s.mux.HandleFunc("/v1/info", s.handleInfo)

	fileServer := http.FileServer(http.FS(WebFS()))
	s.mux.Handle("/", fileServer)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxEventBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	payload = bytesTrimSpace(payload)
	if len(payload) == 0 {
		writeError(w, http.StatusBadRequest, "empty body")
		return
	}

	var events []model.Event
	if payload[0] == '[' {
		if err := json.Unmarshal(payload, &events); err != nil {
			writeError(w, http.StatusBadRequest, "invalid event payload")
			return
		}
	} else {
		var event model.Event
		if err := json.Unmarshal(payload, &event); err != nil {
			writeError(w, http.StatusBadRequest, "invalid event payload")
			return
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		writeError(w, http.StatusBadRequest, "no events provided")
		return
	}

	now := time.Now().UTC()
	for i, event := range events {
		if event.Type == "" {
			writeError(w, http.StatusBadRequest, "event missing type at index "+strconv.Itoa(i))
			return
		}
		if event.SchemaVersion == 0 {
			event.SchemaVersion = s.config.SchemaVersion
		}
		if event.SchemaVersion != s.config.SchemaVersion {
			writeError(w, http.StatusBadRequest, "unsupported schema version at index "+strconv.Itoa(i))
			return
		}
		if event.OccurredAt.IsZero() {
			event.OccurredAt = now
		}
		if err := s.store.InsertEvent(r.Context(), workspaceID, event); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store event")
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxSnapshotBytes)
	var snapshot model.Snapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid snapshot payload")
		return
	}
	if snapshot.SchemaVersion == 0 {
		snapshot.SchemaVersion = s.config.SchemaVersion
	}
	if snapshot.SchemaVersion != s.config.SchemaVersion {
		writeError(w, http.StatusBadRequest, "unsupported schema version")
		return
	}
	if snapshot.CollectedAt.IsZero() {
		snapshot.CollectedAt = time.Now().UTC()
	}
	if err := s.store.InsertSnapshot(r.Context(), workspaceID, snapshot); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store snapshot")
		return
	}
	for _, rig := range snapshot.Rigs {
		if err := s.store.UpsertRig(r.Context(), workspaceID, rig); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store rig")
			return
		}
	}
	for _, agent := range snapshot.Agents {
		if err := s.store.UpsertAgent(r.Context(), workspaceID, agent); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store agent")
			return
		}
	}
	for _, convoy := range snapshot.Convoys {
		if err := s.store.UpsertConvoy(r.Context(), workspaceID, convoy); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store convoy")
			return
		}
	}
	for _, hook := range snapshot.Hooks {
		if err := s.store.UpsertHook(r.Context(), workspaceID, hook); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store hook")
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) handleRigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())
	rigs, err := s.store.ListRigs(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list rigs")
		return
	}
	writeJSON(w, http.StatusOK, rigs)
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())
	agents, err := s.store.ListAgents(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleConvoys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())
	convoys, err := s.store.ListConvoys(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list convoys")
		return
	}
	writeJSON(w, http.StatusOK, convoys)
}

func (s *Server) handleHooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())
	hooks, err := s.store.ListHooks(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list hooks")
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())
	filter := store.EventFilter{
		Rig:    r.URL.Query().Get("rig"),
		Agent:  r.URL.Query().Get("agent"),
		Convoy: r.URL.Query().Get("convoy"),
		Hook:   r.URL.Query().Get("hook"),
		Type:   r.URL.Query().Get("type"),
		Status: r.URL.Query().Get("status"),
	}
	if sinceValue := r.URL.Query().Get("since"); sinceValue != "" {
		if parsed, err := time.Parse(time.RFC3339, sinceValue); err == nil {
			filter.Since = parsed
		}
	}
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := parseLimit(value, 1000); err == nil {
			limit = parsed
		}
	}
	events, err := s.store.ListEvents(r.Context(), workspaceID, filter, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	workspaceID := workspaceIDFromContext(r.Context())
	staleWindow := 5 * time.Minute
	if value := r.URL.Query().Get("stale_minutes"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			staleWindow = time.Duration(parsed) * time.Minute
		}
	}
	alerts, err := s.store.ListAlerts(r.Context(), workspaceID, staleWindow)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

type contextKey string

const workspaceKey contextKey = "workspaceID"

func workspaceIDFromContext(ctx context.Context) uuid.UUID {
	value := ctx.Value(workspaceKey)
	if id, ok := value.(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := readToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing api key")
			return
		}
		workspaceID, err := s.store.WorkspaceIDFromAPIKey(r.Context(), token)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusForbidden, "invalid api key")
				return
			}
			writeError(w, http.StatusInternalServerError, "auth failed")
			return
		}
		if s.limiter != nil {
			if !s.limiter.allow(workspaceID) {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
		}
		ctx := context.WithValue(r.Context(), workspaceKey, workspaceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func readToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); header != "" {
		if strings.HasPrefix(strings.ToLower(header), "bearer ") {
			return strings.TrimSpace(header[7:])
		}
		return strings.TrimSpace(header)
	}
	if header := r.Header.Get("X-API-Key"); header != "" {
		return strings.TrimSpace(header)
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseLimit(value string, max int) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, errors.New("invalid limit")
	}
	if parsed > max {
		parsed = max
	}
	return parsed, nil
}

func bytesTrimSpace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self'; connect-src 'self'")
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":                 s.config.Version,
		"schema_version":          s.config.SchemaVersion,
		"agent_download_base_url": s.config.AgentDownloadBaseURL,
	})
}

type rateLimiter struct {
	mu      sync.Mutex
	rate    int
	buckets map[uuid.UUID]*rateBucket
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(ratePerMinute int) *rateLimiter {
	return &rateLimiter{
		rate:    ratePerMinute,
		buckets: make(map[uuid.UUID]*rateBucket),
	}
}

func (r *rateLimiter) allow(id uuid.UUID) bool {
	if id == uuid.Nil {
		return true
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.buckets[id]
	if !ok {
		bucket = &rateBucket{tokens: float64(r.rate), last: now}
		r.buckets[id] = bucket
	}
	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		refill := elapsed * (float64(r.rate) / 60.0)
		bucket.tokens += refill
		if bucket.tokens > float64(r.rate) {
			bucket.tokens = float64(r.rate)
		}
		bucket.last = now
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens -= 1
	return true
}
