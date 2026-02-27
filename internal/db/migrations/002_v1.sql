alter table api_keys
    add column if not exists revoked_at timestamptz;

alter table events
    add column if not exists schema_version smallint not null default 1;

alter table snapshots
    add column if not exists schema_version smallint not null default 1;

create table if not exists audit_logs (
    id bigserial primary key,
    workspace_id uuid,
    action text not null,
    actor text,
    metadata jsonb,
    created_at timestamptz not null default now()
);

create index if not exists idx_events_occurred_at on events (occurred_at desc);
create index if not exists idx_api_keys_workspace on api_keys (workspace_id);
