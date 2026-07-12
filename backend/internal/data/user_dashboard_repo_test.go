package data

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
)

// seedUser inserts an iris_users row and returns its id.
func seedUser(t *testing.T, db *DB, email string) string {
	t.Helper()
	var id string
	if err := db.Pool.QueryRow(context.Background(),
		`INSERT INTO iris_users (email, display_name, status, mfa_required)
		 VALUES ($1, $1, 'active', false) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestUserDashboardRepoCRUDAndOwnership(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `TRUNCATE user_dashboards, iris_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	repo := NewUserDashboardRepo(db)

	alice := seedUser(t, db, "alice@example.com")
	bob := seedUser(t, db, "bob@example.com")

	// Create for alice.
	created, err := repo.Create(ctx, &biz.UserDashboard{
		UserID: alice, Name: "Ops", Widgets: json.RawMessage(`[{"id":"w1"}]`),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.CreatedAt.IsZero() {
		t.Fatalf("expected populated id/timestamps, got %+v", created)
	}

	// Bob cannot see or fetch alice's dashboard.
	if _, err := repo.Get(ctx, bob, created.ID); err == nil {
		t.Fatal("expected NotFound fetching another user's dashboard")
	}
	list, err := repo.List(ctx, bob)
	if err != nil {
		t.Fatalf("list bob: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected bob to see no dashboards, got %d", len(list))
	}

	// Update by alice.
	updated, err := repo.Update(ctx, &biz.UserDashboard{
		ID: created.ID, UserID: alice, Name: "Ops v2", Widgets: json.RawMessage(`[]`),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Ops v2" {
		t.Fatalf("expected renamed dashboard, got %q", updated.Name)
	}

	// Bob cannot update alice's dashboard.
	if _, err := repo.Update(ctx, &biz.UserDashboard{ID: created.ID, UserID: bob, Name: "hax", Widgets: json.RawMessage(`[]`)}); err == nil {
		t.Fatal("expected NotFound updating another user's dashboard")
	}

	// Bob cannot delete alice's dashboard.
	if err := repo.Delete(ctx, bob, created.ID); err == nil {
		t.Fatal("expected NotFound deleting another user's dashboard")
	}
}

func TestUserDashboardRepoSetDefaultSingleDefault(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `TRUNCATE user_dashboards, iris_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	repo := NewUserDashboardRepo(db)
	alice := seedUser(t, db, "alice@example.com")

	a, err := repo.Create(ctx, &biz.UserDashboard{UserID: alice, Name: "A", Widgets: json.RawMessage(`[]`)})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := repo.Create(ctx, &biz.UserDashboard{UserID: alice, Name: "B", Widgets: json.RawMessage(`[]`)})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	if err := repo.SetDefault(ctx, alice, a.ID); err != nil {
		t.Fatalf("set default A: %v", err)
	}
	// Switching default must clear the prior one (partial-unique index would
	// otherwise reject two defaults).
	if err := repo.SetDefault(ctx, alice, b.ID); err != nil {
		t.Fatalf("set default B: %v", err)
	}

	list, err := repo.List(ctx, alice)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defaults := 0
	var defaultID string
	for _, d := range list {
		if d.IsDefault {
			defaults++
			defaultID = d.ID
		}
	}
	if defaults != 1 || defaultID != b.ID {
		t.Fatalf("expected B to be sole default, got %d defaults (id %s)", defaults, defaultID)
	}
}
