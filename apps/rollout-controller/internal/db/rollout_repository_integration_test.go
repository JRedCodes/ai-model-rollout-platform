//go:build integration

package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JRedCodes/rollout-controller/internal/db"
	"github.com/JRedCodes/rollout-controller/internal/dbtest"
	"github.com/JRedCodes/rollout-controller/internal/tenant"
)

func TestRolloutRepositoryCreateGetListRollouts(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := db.NewRolloutRepository(pool)
	ctx := context.Background()

	tenantRow, _, err := tenant.NewRepository(pool).Create(ctx, "integration-test-"+uuid.NewString())
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}

	created, err := repo.CreateRollout(ctx, db.CreateRolloutInput{
		TenantID:                tenantRow.ID,
		RolloutPhaseID:          "phase-1",
		StableModelVersionID:    "model-v1",
		CandidateModelVersionID: "model-v2",
		CandidatePercentage:     10,
		FeatureFlagKey:          "feature-flag:model-routing:" + tenantRow.ID,
	})
	if err != nil {
		t.Fatalf("CreateRollout: %v", err)
	}
	if created.Status != "RUNNING" {
		t.Fatalf("expected status RUNNING, got %s", created.Status)
	}

	t.Run("GetRollout returns the created rollout", func(t *testing.T) {
		got, err := repo.GetRollout(ctx, tenantRow.ID, created.ID)
		if err != nil {
			t.Fatalf("GetRollout: %v", err)
		}
		if got.ID != created.ID {
			t.Fatalf("expected ID %q, got %q", created.ID, got.ID)
		}
	})

	t.Run("GetRollout scoped to the wrong tenant is not found", func(t *testing.T) {
		otherTenant, _, err := tenant.NewRepository(pool).Create(ctx, "integration-test-"+uuid.NewString())
		if err != nil {
			t.Fatalf("create other tenant: %v", err)
		}
		_, err = repo.GetRollout(ctx, otherTenant.ID, created.ID)
		var notFound *db.RolloutNotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("expected *db.RolloutNotFoundError, got %v", err)
		}
	})

	t.Run("GetRollout for an unknown ID is not found", func(t *testing.T) {
		_, err := repo.GetRollout(ctx, tenantRow.ID, uuid.NewString())
		var notFound *db.RolloutNotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("expected *db.RolloutNotFoundError, got %v", err)
		}
	})

	t.Run("ListRollouts includes the created rollout", func(t *testing.T) {
		rollouts, err := repo.ListRollouts(ctx, tenantRow.ID)
		if err != nil {
			t.Fatalf("ListRollouts: %v", err)
		}
		if len(rollouts) != 1 || rollouts[0].ID != created.ID {
			t.Fatalf("expected exactly the created rollout, got %+v", rollouts)
		}
	})

	t.Run("CreateRollout while one is already active fails", func(t *testing.T) {
		_, err := repo.CreateRollout(ctx, db.CreateRolloutInput{
			TenantID:                tenantRow.ID,
			RolloutPhaseID:          "phase-2",
			StableModelVersionID:    "model-v1",
			CandidateModelVersionID: "model-v2",
			CandidatePercentage:     10,
			FeatureFlagKey:          "feature-flag:model-routing:" + tenantRow.ID,
		})
		if !errors.Is(err, db.ErrActiveRolloutExists) {
			t.Fatalf("expected db.ErrActiveRolloutExists, got %v", err)
		}
	})
}

func TestRolloutRepositoryUpdateStatusAndPercentage(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := db.NewRolloutRepository(pool)
	ctx := context.Background()

	tenantRow, _, err := tenant.NewRepository(pool).Create(ctx, "integration-test-"+uuid.NewString())
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
	created, err := repo.CreateRollout(ctx, db.CreateRolloutInput{
		TenantID: tenantRow.ID, RolloutPhaseID: "phase-1",
		StableModelVersionID: "model-v1", CandidateModelVersionID: "model-v2",
		CandidatePercentage: 10, FeatureFlagKey: "feature-flag:model-routing:" + tenantRow.ID,
	})
	if err != nil {
		t.Fatalf("CreateRollout: %v", err)
	}

	if err := repo.UpdatePercentage(ctx, created.ID, 25); err != nil {
		t.Fatalf("UpdatePercentage: %v", err)
	}
	if err := repo.UpdateStatus(ctx, created.ID, "HELD"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := repo.GetRollout(ctx, tenantRow.ID, created.ID)
	if err != nil {
		t.Fatalf("GetRollout: %v", err)
	}
	if got.CandidatePercentage != 25 {
		t.Fatalf("expected candidate percentage 25, got %d", got.CandidatePercentage)
	}
	if got.Status != "HELD" {
		t.Fatalf("expected status HELD, got %s", got.Status)
	}
}

func TestRolloutRepositoryDecisions(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := db.NewRolloutRepository(pool)
	ctx := context.Background()

	tenantRow, _, err := tenant.NewRepository(pool).Create(ctx, "integration-test-"+uuid.NewString())
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
	created, err := repo.CreateRollout(ctx, db.CreateRolloutInput{
		TenantID: tenantRow.ID, RolloutPhaseID: "phase-1",
		StableModelVersionID: "model-v1", CandidateModelVersionID: "model-v2",
		CandidatePercentage: 10, FeatureFlagKey: "feature-flag:model-routing:" + tenantRow.ID,
	})
	if err != nil {
		t.Fatalf("CreateRollout: %v", err)
	}

	if err := repo.InsertDecision(ctx, uuid.NewString(), created.ID, "ADVANCE", "healthy window", "controller"); err != nil {
		t.Fatalf("InsertDecision 1: %v", err)
	}
	if err := repo.InsertDecision(ctx, uuid.NewString(), created.ID, "HOLD", "error rate spike", "guard"); err != nil {
		t.Fatalf("InsertDecision 2: %v", err)
	}

	decisions, err := repo.ListDecisions(ctx, created.ID, 50)
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	// Newest first.
	if decisions[0].Action != "HOLD" || decisions[1].Action != "ADVANCE" {
		t.Fatalf("expected [HOLD, ADVANCE] newest first, got [%s, %s]", decisions[0].Action, decisions[1].Action)
	}

	t.Run("respects the limit", func(t *testing.T) {
		limited, err := repo.ListDecisions(ctx, created.ID, 1)
		if err != nil {
			t.Fatalf("ListDecisions: %v", err)
		}
		if len(limited) != 1 {
			t.Fatalf("expected 1 decision, got %d", len(limited))
		}
	})
}

func TestRolloutRepositoryActiveAndCompletedLookups(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := db.NewRolloutRepository(pool)
	ctx := context.Background()

	tenantRow, _, err := tenant.NewRepository(pool).Create(ctx, "integration-test-"+uuid.NewString())
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}

	t.Run("no active rollout yet", func(t *testing.T) {
		_, _, err := repo.LoadActiveForTenant(ctx, tenantRow.ID)
		if !errors.Is(err, db.ErrNoActiveRollout) {
			t.Fatalf("expected db.ErrNoActiveRollout, got %v", err)
		}
	})

	t.Run("no completed rollout yet", func(t *testing.T) {
		_, err := repo.LatestCompletedCandidate(ctx, tenantRow.ID)
		if !errors.Is(err, db.ErrNoCompletedRollout) {
			t.Fatalf("expected db.ErrNoCompletedRollout, got %v", err)
		}
	})

	created, err := repo.CreateRollout(ctx, db.CreateRolloutInput{
		TenantID: tenantRow.ID, RolloutPhaseID: "phase-1",
		StableModelVersionID: "model-v1", CandidateModelVersionID: "model-v2",
		CandidatePercentage: 10, FeatureFlagKey: "feature-flag:model-routing:" + tenantRow.ID,
	})
	if err != nil {
		t.Fatalf("CreateRollout: %v", err)
	}

	t.Run("LoadActiveForTenant finds the RUNNING rollout", func(t *testing.T) {
		cfg, _, err := repo.LoadActiveForTenant(ctx, tenantRow.ID)
		if err != nil {
			t.Fatalf("LoadActiveForTenant: %v", err)
		}
		if cfg.RolloutID != created.ID {
			t.Fatalf("expected rollout ID %q, got %q", created.ID, cfg.RolloutID)
		}
	})

	t.Run("ListActiveRollouts includes it", func(t *testing.T) {
		refs, err := repo.ListActiveRollouts(ctx)
		if err != nil {
			t.Fatalf("ListActiveRollouts: %v", err)
		}
		for _, ref := range refs {
			if ref.TenantID == tenantRow.ID && ref.RolloutID == created.ID {
				return
			}
		}
		t.Fatalf("expected to find tenant %q's active rollout in ListActiveRollouts", tenantRow.ID)
	})

	if err := repo.UpdateStatus(ctx, created.ID, "COMPLETED"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	t.Run("no longer active once completed", func(t *testing.T) {
		_, _, err := repo.LoadActiveForTenant(ctx, tenantRow.ID)
		if !errors.Is(err, db.ErrNoActiveRollout) {
			t.Fatalf("expected db.ErrNoActiveRollout after completion, got %v", err)
		}
	})

	t.Run("LatestCompletedCandidate returns its candidate", func(t *testing.T) {
		candidate, err := repo.LatestCompletedCandidate(ctx, tenantRow.ID)
		if err != nil {
			t.Fatalf("LatestCompletedCandidate: %v", err)
		}
		if candidate != "model-v2" {
			t.Fatalf("expected candidate model-v2, got %s", candidate)
		}
	})
}
