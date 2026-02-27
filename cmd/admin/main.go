package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/SmolNero/gastown-control-plane/internal/config"
	"github.com/SmolNero/gastown-control-plane/internal/db"
	"github.com/SmolNero/gastown-control-plane/internal/store"
	"github.com/SmolNero/gastown-control-plane/internal/util"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	ctx := context.Background()
	cfg := config.FromEnv()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connection failed: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)

	switch os.Args[1] {
	case "migrate":
		if err := db.Migrate(ctx, pool); err != nil {
			log.Fatalf("migrations failed: %v", err)
		}
		fmt.Println("migrations applied")
	case "create-org":
		fs := flag.NewFlagSet("create-org", flag.ExitOnError)
		name := fs.String("name", "", "organization name")
		_ = fs.Parse(os.Args[2:])
		if *name == "" {
			log.Fatal("--name is required")
		}
		id, err := st.CreateOrganization(ctx, *name)
		if err != nil {
			log.Fatalf("create org failed: %v", err)
		}
		fmt.Printf("org_id=%s\n", id)
	case "create-workspace":
		fs := flag.NewFlagSet("create-workspace", flag.ExitOnError)
		orgID := fs.String("org-id", "", "organization id")
		name := fs.String("name", "", "workspace name")
		_ = fs.Parse(os.Args[2:])
		if *orgID == "" || *name == "" {
			log.Fatal("--org-id and --name are required")
		}
		parsed, err := uuid.Parse(*orgID)
		if err != nil {
			log.Fatalf("invalid org id: %v", err)
		}
		id, err := st.CreateWorkspace(ctx, parsed, *name)
		if err != nil {
			log.Fatalf("create workspace failed: %v", err)
		}
		fmt.Printf("workspace_id=%s\n", id)
	case "create-api-key":
		fs := flag.NewFlagSet("create-api-key", flag.ExitOnError)
		workspaceID := fs.String("workspace-id", "", "workspace id")
		name := fs.String("name", "default", "api key name")
		_ = fs.Parse(os.Args[2:])
		if *workspaceID == "" {
			log.Fatal("--workspace-id is required")
		}
		parsed, err := uuid.Parse(*workspaceID)
		if err != nil {
			log.Fatalf("invalid workspace id: %v", err)
		}
		token, err := util.RandomToken(32)
		if err != nil {
			log.Fatalf("token generation failed: %v", err)
		}
		keyID, err := st.CreateAPIKey(ctx, parsed, *name, token)
		if err != nil {
			log.Fatalf("create api key failed: %v", err)
		}
		_ = st.AddAuditLog(ctx, parsed, "api_key_created", "admin", nil)
		fmt.Printf("api_key_id=%s\napi_key=%s\n", keyID, token)
	case "create-all":
		fs := flag.NewFlagSet("create-all", flag.ExitOnError)
		orgName := fs.String("org", "", "organization name")
		workspaceName := fs.String("workspace", "", "workspace name")
		keyName := fs.String("key-name", "default", "api key name")
		_ = fs.Parse(os.Args[2:])
		if *orgName == "" || *workspaceName == "" {
			log.Fatal("--org and --workspace are required")
		}
		orgID, err := st.CreateOrganization(ctx, *orgName)
		if err != nil {
			log.Fatalf("create org failed: %v", err)
		}
		workspaceID, err := st.CreateWorkspace(ctx, orgID, *workspaceName)
		if err != nil {
			log.Fatalf("create workspace failed: %v", err)
		}
		token, err := util.RandomToken(32)
		if err != nil {
			log.Fatalf("token generation failed: %v", err)
		}
		keyID, err := st.CreateAPIKey(ctx, workspaceID, *keyName, token)
		if err != nil {
			log.Fatalf("create api key failed: %v", err)
		}
		_ = st.AddAuditLog(ctx, workspaceID, "api_key_created", "admin", nil)
		fmt.Printf("org_id=%s\nworkspace_id=%s\napi_key_id=%s\napi_key=%s\n", orgID, workspaceID, keyID, token)
	case "list-api-keys":
		fs := flag.NewFlagSet("list-api-keys", flag.ExitOnError)
		workspaceID := fs.String("workspace-id", "", "workspace id")
		_ = fs.Parse(os.Args[2:])
		if *workspaceID == "" {
			log.Fatal("--workspace-id is required")
		}
		parsed, err := uuid.Parse(*workspaceID)
		if err != nil {
			log.Fatalf("invalid workspace id: %v", err)
		}
		keys, err := st.ListAPIKeys(ctx, parsed)
		if err != nil {
			log.Fatalf("list api keys failed: %v", err)
		}
		for _, key := range keys {
			revoked := ""
			if !key.RevokedAt.IsZero() {
				revoked = key.RevokedAt.Format(time.RFC3339)
			}
			lastUsed := ""
			if !key.LastUsedAt.IsZero() {
				lastUsed = key.LastUsedAt.Format(time.RFC3339)
			}
			fmt.Printf("id=%s name=%s created_at=%s last_used_at=%s revoked_at=%s\n", key.ID, key.Name, key.CreatedAt.Format(time.RFC3339), lastUsed, revoked)
		}
	case "revoke-api-key":
		fs := flag.NewFlagSet("revoke-api-key", flag.ExitOnError)
		keyID := fs.String("key-id", "", "api key id")
		workspaceID := fs.String("workspace-id", "", "workspace id (optional, for audit)")
		_ = fs.Parse(os.Args[2:])
		if *keyID == "" {
			log.Fatal("--key-id is required")
		}
		parsedKey, err := uuid.Parse(*keyID)
		if err != nil {
			log.Fatalf("invalid key id: %v", err)
		}
		if err := st.RevokeAPIKey(ctx, parsedKey); err != nil {
			log.Fatalf("revoke api key failed: %v", err)
		}
		if *workspaceID != "" {
			parsedWorkspace, err := uuid.Parse(*workspaceID)
			if err == nil {
				_ = st.AddAuditLog(ctx, parsedWorkspace, "api_key_revoked", "admin", nil)
			}
		}
		fmt.Println("api key revoked")
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: admin <command> [options]")
	fmt.Println("Commands:")
	fmt.Println("  migrate")
	fmt.Println("  create-org --name <name>")
	fmt.Println("  create-workspace --org-id <uuid> --name <name>")
	fmt.Println("  create-api-key --workspace-id <uuid> [--name <name>]")
	fmt.Println("  list-api-keys --workspace-id <uuid>")
	fmt.Println("  revoke-api-key --key-id <uuid> [--workspace-id <uuid>]")
	fmt.Println("  create-all --org <name> --workspace <name> [--key-name <name>]")
}
