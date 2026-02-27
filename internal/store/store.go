package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SmolNero/gastown-control-plane/internal/model"
	"github.com/SmolNero/gastown-control-plane/internal/util"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

type EventFilter struct {
	Rig    string
	Agent  string
	Convoy string
	Hook   string
	Type   string
	Status string
	Since  time.Time
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) WorkspaceIDFromAPIKey(ctx context.Context, token string) (uuid.UUID, error) {
	hash := util.TokenHash(token)
	var workspaceID uuid.UUID
	err := s.pool.QueryRow(ctx, `select workspace_id from api_keys where token_hash = $1 and revoked_at is null`, hash).Scan(&workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, err
	}
	_, _ = s.pool.Exec(ctx, `update api_keys set last_used_at = now() where token_hash = $1`, hash)
	return workspaceID, nil
}

func (s *Store) InsertEvent(ctx context.Context, workspaceID uuid.UUID, event model.Event) error {
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
	}
	_, err := s.pool.Exec(ctx, `insert into events (
		workspace_id, schema_version, type, source, rig_name, agent_name, convoy_name, hook_name, status, message, payload, occurred_at
	) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12)`,
		workspaceID,
		event.SchemaVersion,
		event.Type,
		nullString(event.Source),
		nullString(event.Rig),
		nullString(event.Agent),
		nullString(event.Convoy),
		nullString(event.Hook),
		nullString(event.Status),
		nullString(event.Message),
		util.JSONBValue(event.Payload),
		event.OccurredAt,
	)
	if err != nil {
		return err
	}
	if event.Rig != "" {
		_ = s.UpsertRig(ctx, workspaceID, model.Rig{Name: event.Rig})
	}
	if event.Agent != "" {
		_ = s.UpsertAgent(ctx, workspaceID, model.Agent{Name: event.Agent, Rig: event.Rig, Status: event.Status, LastSeenAt: event.OccurredAt})
	}
	if event.Convoy != "" {
		_ = s.UpsertConvoy(ctx, workspaceID, model.Convoy{Name: event.Convoy, Rig: event.Rig, Status: event.Status})
	}
	if event.Hook != "" {
		_ = s.UpsertHook(ctx, workspaceID, model.Hook{Name: event.Hook, Rig: event.Rig, Status: event.Status})
	}
	return nil
}

func (s *Store) InsertSnapshot(ctx context.Context, workspaceID uuid.UUID, snapshot model.Snapshot) error {
	if snapshot.SchemaVersion == 0 {
		snapshot.SchemaVersion = 1
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `insert into snapshots (id, workspace_id, source, collected_at, schema_version, payload) values ($1,$2,$3,$4,$5,$6::jsonb)`,
		uuid.New(), workspaceID, nullString(snapshot.Source), snapshot.CollectedAt, snapshot.SchemaVersion, util.JSONBBytes(raw))
	return err
}

func (s *Store) UpsertRig(ctx context.Context, workspaceID uuid.UUID, rig model.Rig) error {
	if rig.Name == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `insert into rigs (id, workspace_id, name, path, last_seen_at)
        values ($1,$2,$3,$4,now())
        on conflict (workspace_id, name)
        do update set path = excluded.path, last_seen_at = now(), updated_at = now()`,
		uuid.New(), workspaceID, rig.Name, nullString(rig.Path))
	return err
}

func (s *Store) UpsertAgent(ctx context.Context, workspaceID uuid.UUID, agent model.Agent) error {
	if agent.Name == "" {
		return nil
	}
	lastSeen := agent.LastSeenAt
	if lastSeen.IsZero() {
		lastSeen = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `insert into agents (id, workspace_id, name, rig_name, role, status, last_seen_at, metadata)
        values ($1,$2,$3,$4,$5,$6,$7,$8::jsonb)
        on conflict (workspace_id, name, rig_name)
        do update set role = excluded.role, status = excluded.status, last_seen_at = excluded.last_seen_at, metadata = excluded.metadata, updated_at = now()`,
		uuid.New(), workspaceID, agent.Name, nullString(agent.Rig), nullString(agent.Role), nullString(agent.Status), lastSeen, util.JSONBValue(agent.Metadata))
	return err
}

func (s *Store) UpsertConvoy(ctx context.Context, workspaceID uuid.UUID, convoy model.Convoy) error {
	if convoy.Name == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `insert into convoys (id, workspace_id, name, rig_name, status)
        values ($1,$2,$3,$4,$5)
        on conflict (workspace_id, name, rig_name)
        do update set status = excluded.status, updated_at = now()`,
		uuid.New(), workspaceID, convoy.Name, nullString(convoy.Rig), nullString(convoy.Status))
	return err
}

func (s *Store) UpsertHook(ctx context.Context, workspaceID uuid.UUID, hook model.Hook) error {
	if hook.Name == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `insert into hooks (id, workspace_id, name, rig_name, status)
        values ($1,$2,$3,$4,$5)
        on conflict (workspace_id, name, rig_name)
        do update set status = excluded.status, updated_at = now()`,
		uuid.New(), workspaceID, hook.Name, nullString(hook.Rig), nullString(hook.Status))
	return err
}

func (s *Store) ListRigs(ctx context.Context, workspaceID uuid.UUID) ([]model.Rig, error) {
	rows, err := s.pool.Query(ctx, `select name, path from rigs where workspace_id = $1 order by name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rigs []model.Rig
	for rows.Next() {
		var rig model.Rig
		var path *string
		if err := rows.Scan(&rig.Name, &path); err != nil {
			return nil, err
		}
		if path != nil {
			rig.Path = *path
		}
		rigs = append(rigs, rig)
	}
	return rigs, rows.Err()
}

func (s *Store) ListAgents(ctx context.Context, workspaceID uuid.UUID) ([]model.Agent, error) {
	rows, err := s.pool.Query(ctx, `select name, rig_name, role, status, last_seen_at, metadata from agents where workspace_id = $1 order by name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []model.Agent
	for rows.Next() {
		var agent model.Agent
		var (
			rig      *string
			role     *string
			status   *string
			lastSeen *time.Time
			metadata []byte
		)
		if err := rows.Scan(&agent.Name, &rig, &role, &status, &lastSeen, &metadata); err != nil {
			return nil, err
		}
		if rig != nil {
			agent.Rig = *rig
		}
		if role != nil {
			agent.Role = *role
		}
		if status != nil {
			agent.Status = *status
		}
		if lastSeen != nil {
			agent.LastSeenAt = *lastSeen
		}
		agent.Metadata = metadata
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

func (s *Store) ListConvoys(ctx context.Context, workspaceID uuid.UUID) ([]model.Convoy, error) {
	rows, err := s.pool.Query(ctx, `select name, rig_name, status from convoys where workspace_id = $1 order by name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convoys []model.Convoy
	for rows.Next() {
		var convoy model.Convoy
		var rig *string
		var status *string
		if err := rows.Scan(&convoy.Name, &rig, &status); err != nil {
			return nil, err
		}
		if rig != nil {
			convoy.Rig = *rig
		}
		if status != nil {
			convoy.Status = *status
		}
		convoys = append(convoys, convoy)
	}
	return convoys, rows.Err()
}

func (s *Store) ListHooks(ctx context.Context, workspaceID uuid.UUID) ([]model.Hook, error) {
	rows, err := s.pool.Query(ctx, `select name, rig_name, status from hooks where workspace_id = $1 order by name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hooks []model.Hook
	for rows.Next() {
		var hook model.Hook
		var rig *string
		var status *string
		if err := rows.Scan(&hook.Name, &rig, &status); err != nil {
			return nil, err
		}
		if rig != nil {
			hook.Rig = *rig
		}
		if status != nil {
			hook.Status = *status
		}
		hooks = append(hooks, hook)
	}
	return hooks, rows.Err()
}

func (s *Store) ListEvents(ctx context.Context, workspaceID uuid.UUID, filter EventFilter, limit int) ([]model.Event, error) {
	query := `select type, source, rig_name, agent_name, convoy_name, hook_name, status, message, payload, occurred_at, schema_version
		from events where workspace_id = $1`
	args := []interface{}{workspaceID}
	index := 2
	if filter.Rig != "" {
		query += " and rig_name = $" + strconv.Itoa(index)
		args = append(args, filter.Rig)
		index++
	}
	if filter.Agent != "" {
		query += " and agent_name = $" + strconv.Itoa(index)
		args = append(args, filter.Agent)
		index++
	}
	if filter.Convoy != "" {
		query += " and convoy_name = $" + strconv.Itoa(index)
		args = append(args, filter.Convoy)
		index++
	}
	if filter.Hook != "" {
		query += " and hook_name = $" + strconv.Itoa(index)
		args = append(args, filter.Hook)
		index++
	}
	if filter.Type != "" {
		query += " and type = $" + strconv.Itoa(index)
		args = append(args, filter.Type)
		index++
	}
	if filter.Status != "" {
		query += " and status = $" + strconv.Itoa(index)
		args = append(args, filter.Status)
		index++
	}
	if !filter.Since.IsZero() {
		query += " and occurred_at >= $" + strconv.Itoa(index)
		args = append(args, filter.Since)
		index++
	}
	query += " order by created_at desc limit $" + strconv.Itoa(index)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []model.Event
	for rows.Next() {
		var event model.Event
		var (
			source   *string
			rig      *string
			agent    *string
			convoy   *string
			hook     *string
			status   *string
			message  *string
			payload  []byte
			occurred *time.Time
		)
		if err := rows.Scan(&event.Type, &source, &rig, &agent, &convoy, &hook, &status, &message, &payload, &occurred, &event.SchemaVersion); err != nil {
			return nil, err
		}
		if source != nil {
			event.Source = *source
		}
		if rig != nil {
			event.Rig = *rig
		}
		if agent != nil {
			event.Agent = *agent
		}
		if convoy != nil {
			event.Convoy = *convoy
		}
		if hook != nil {
			event.Hook = *hook
		}
		if status != nil {
			event.Status = *status
		}
		if message != nil {
			event.Message = *message
		}
		if occurred != nil {
			event.OccurredAt = *occurred
		}
		event.Payload = payload
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) ListAlerts(ctx context.Context, workspaceID uuid.UUID, staleAfter time.Duration) ([]model.Alert, error) {
	threshold := time.Now().UTC().Add(-staleAfter)
	rows, err := s.pool.Query(ctx, `select name, rig_name, last_seen_at from agents where workspace_id = $1 and last_seen_at < $2 order by last_seen_at asc`, workspaceID, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []model.Alert
	for rows.Next() {
		var name, rig string
		var lastSeen time.Time
		if err := rows.Scan(&name, &rig, &lastSeen); err != nil {
			return nil, err
		}
		alerts = append(alerts, model.Alert{
			Type:       "stale_agent",
			Severity:   "warning",
			Message:    "Agent has not reported recently",
			Agent:      name,
			Rig:        rig,
			LastSeenAt: lastSeen,
		})
	}
	return alerts, rows.Err()
}

func (s *Store) CreateOrganization(ctx context.Context, name string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx, `insert into organizations (id, name) values ($1,$2)`, id, name)
	return id, err
}

func (s *Store) CreateWorkspace(ctx context.Context, orgID uuid.UUID, name string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx, `insert into workspaces (id, org_id, name) values ($1,$2,$3)`, id, orgID, name)
	return id, err
}

func (s *Store) CreateAPIKey(ctx context.Context, workspaceID uuid.UUID, name, token string) (uuid.UUID, error) {
	keyID := uuid.New()
	hash := util.TokenHash(token)
	_, err := s.pool.Exec(ctx, `insert into api_keys (id, workspace_id, name, token_hash) values ($1,$2,$3,$4)`, keyID, workspaceID, name, hash)
	return keyID, err
}

func (s *Store) ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]model.APIKey, error) {
	rows, err := s.pool.Query(ctx, `select id, name, created_at, last_used_at, revoked_at from api_keys where workspace_id = $1 order by created_at desc`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var (
			id         uuid.UUID
			name       string
			createdAt  time.Time
			lastUsedAt *time.Time
			revokedAt  *time.Time
		)
		if err := rows.Scan(&id, &name, &createdAt, &lastUsedAt, &revokedAt); err != nil {
			return nil, err
		}
		key := model.APIKey{
			ID:        id.String(),
			Name:      name,
			CreatedAt: createdAt,
		}
		if lastUsedAt != nil {
			key.LastUsedAt = *lastUsedAt
		}
		if revokedAt != nil {
			key.RevokedAt = *revokedAt
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, keyID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `update api_keys set revoked_at = now() where id = $1 and revoked_at is null`, keyID)
	return err
}

func (s *Store) AddAuditLog(ctx context.Context, workspaceID uuid.UUID, action, actor string, metadata json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `insert into audit_logs (workspace_id, action, actor, metadata) values ($1,$2,$3,$4::jsonb)`,
		workspaceID, action, nullString(actor), util.JSONBValue(metadata))
	return err
}

func (s *Store) PruneEvents(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.pool.Exec(ctx, `delete from events where created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (s *Store) PruneSnapshots(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.pool.Exec(ctx, `delete from snapshots where created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func nullString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
