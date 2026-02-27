create table if not exists organizations (
    id uuid primary key,
    name text not null,
    created_at timestamptz not null default now()
);

create table if not exists workspaces (
    id uuid primary key,
    org_id uuid not null references organizations(id) on delete cascade,
    name text not null,
    created_at timestamptz not null default now(),
    unique (org_id, name)
);

create table if not exists api_keys (
    id uuid primary key,
    workspace_id uuid not null references workspaces(id) on delete cascade,
    name text not null,
    token_hash text not null unique,
    created_at timestamptz not null default now(),
    last_used_at timestamptz
);

create table if not exists rigs (
    id uuid primary key,
    workspace_id uuid not null references workspaces(id) on delete cascade,
    name text not null,
    path text,
    last_seen_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (workspace_id, name)
);

create table if not exists agents (
    id uuid primary key,
    workspace_id uuid not null references workspaces(id) on delete cascade,
    name text not null,
    rig_name text,
    role text,
    status text,
    last_seen_at timestamptz,
    metadata jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (workspace_id, name, rig_name)
);

create table if not exists convoys (
    id uuid primary key,
    workspace_id uuid not null references workspaces(id) on delete cascade,
    name text not null,
    rig_name text,
    status text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (workspace_id, name, rig_name)
);

create table if not exists hooks (
    id uuid primary key,
    workspace_id uuid not null references workspaces(id) on delete cascade,
    name text not null,
    rig_name text,
    status text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (workspace_id, name, rig_name)
);

create table if not exists events (
    id bigserial primary key,
    workspace_id uuid not null references workspaces(id) on delete cascade,
    type text not null,
    source text,
    rig_name text,
    agent_name text,
    convoy_name text,
    hook_name text,
    status text,
    message text,
    payload jsonb,
    occurred_at timestamptz,
    created_at timestamptz not null default now()
);

create index if not exists idx_events_workspace_created_at on events (workspace_id, created_at desc);

create table if not exists snapshots (
    id uuid primary key,
    workspace_id uuid not null references workspaces(id) on delete cascade,
    source text,
    collected_at timestamptz,
    payload jsonb,
    created_at timestamptz not null default now()
);
