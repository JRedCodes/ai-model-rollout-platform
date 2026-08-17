//go:build integration

package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JRedCodes/rollout-controller/internal/dbtest"
)

func uniqueName(t *testing.T) string {
	t.Helper()
	return "integration-test-" + t.Name() + "-" + uuid.NewString()
}

func TestRepositoryCreateAndGetIDByAPIKey(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	name := uniqueName(t)
	created, plaintextKey, err := repo.Create(ctx, name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name != name {
		t.Fatalf("expected name %q, got %q", name, created.Name)
	}
	if plaintextKey == "" {
		t.Fatal("expected a non-empty plaintext key")
	}

	gotID, err := repo.GetIDByAPIKey(ctx, plaintextKey)
	if err != nil {
		t.Fatalf("GetIDByAPIKey: %v", err)
	}
	if gotID != created.ID {
		t.Fatalf("expected tenant ID %q, got %q", created.ID, gotID)
	}
}

func TestRepositoryGetIDByAPIKeyInvalidKey(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := NewRepository(pool)

	_, err := repo.GetIDByAPIKey(context.Background(), "tk_"+uuid.NewString())
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestRepositoryCreateSeedsDefaultModelConfigs(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	created, _, err := repo.Create(ctx, uniqueName(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM model_configurations WHERE tenant_id = $1`,
		created.ID,
	).Scan(&count); err != nil {
		t.Fatalf("query model_configurations: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 seeded model configs (model-v1, model-v2), got %d", count)
	}
}

func TestRepositoryRegenerateAPIKey(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	created, oldKey, err := repo.Create(ctx, uniqueName(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newKey, err := repo.RegenerateAPIKey(ctx, created.ID)
	if err != nil {
		t.Fatalf("RegenerateAPIKey: %v", err)
	}
	if newKey == oldKey {
		t.Fatal("expected a different key after regeneration")
	}

	if _, err := repo.GetIDByAPIKey(ctx, oldKey); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected the old key to stop working immediately, got err: %v", err)
	}

	gotID, err := repo.GetIDByAPIKey(ctx, newKey)
	if err != nil {
		t.Fatalf("GetIDByAPIKey with new key: %v", err)
	}
	if gotID != created.ID {
		t.Fatalf("expected tenant ID %q, got %q", created.ID, gotID)
	}
}

func TestRepositoryListAuthIncludesNewTenant(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	created, _, err := repo.Create(ctx, uniqueName(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	entries, err := repo.ListAuth(ctx)
	if err != nil {
		t.Fatalf("ListAuth: %v", err)
	}

	for _, e := range entries {
		if e.TenantID == created.ID {
			return
		}
	}
	t.Fatalf("expected tenant %q to appear in ListAuth", created.ID)
}
