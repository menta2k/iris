// seed is a one-shot bootstrap CLI:
//
//   1. Opens the Postgres connection from a DSN.
//   2. Runs ent auto-migration (creates all tables + indexes).
//   3. Inserts the default admin/operator/viewer roles.
//   4. Inserts an "admin" user (configurable username/password) with the
//      "admin" role attached.
//
// Usage:
//
//	go run ./scripts/seed \
//	    -dsn "postgres://iris:iris@127.0.0.1:5432/iris?sslmode=disable" \
//	    -username admin -password admin
//
// Idempotent: re-running the same command updates the user's
// password_hash + display_name, so it doubles as a password-reset tool.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/migrate"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/role"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/user"
	appcrypto "github.com/menta2k/iris/backend/pkg/crypto"
)

func main() {
	dsn := flag.String("dsn", "postgres://iris:iris@127.0.0.1:5432/iris?sslmode=disable", "Postgres DSN")
	username := flag.String("username", "admin", "admin username")
	password := flag.String("password", "admin", "admin password (no policy check; bcrypt cost 12)")
	email := flag.String("email", "admin@local", "admin email")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Open via the pgx/v5 stdlib driver — registered as "pgx", not
	// "postgres". Hand the resulting *sql.DB to ent's dialect adapter.
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("seed: open postgres: %v", err)
	}
	defer db.Close()
	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	log.Println("seed: running ent auto-migration…")
	if err := client.Schema.Create(ctx,
		migrate.WithDropIndex(false),
		migrate.WithDropColumn(false),
	); err != nil {
		log.Fatalf("seed: migrate: %v", err)
	}

	log.Println("seed: ensuring default roles…")
	if err := upsertRole(ctx, client, "admin", "Administrator", "Full access", []string{"*:*"}); err != nil {
		log.Fatalf("seed: role admin: %v", err)
	}
	if err := upsertRole(ctx, client, "operator", "Operator", "Day-to-day operations",
		[]string{
			"kumo.policy:read", "kumo.policy:write",
			"kumo.queue:write", "kumo.suppression:write",
			"audit.log:read",
		}); err != nil {
		log.Fatalf("seed: role operator: %v", err)
	}
	if err := upsertRole(ctx, client, "viewer", "Viewer", "Read-only dashboards",
		[]string{"kumo.*:read", "audit.log:read"}); err != nil {
		log.Fatalf("seed: role viewer: %v", err)
	}

	hash, err := appcrypto.HashPassword(*password, appcrypto.MinBcryptCost)
	if err != nil {
		log.Fatalf("seed: hash password: %v", err)
	}

	adminRole, err := client.Role.Query().Where(role.CodeEQ("admin")).Only(ctx)
	if err != nil {
		log.Fatalf("seed: load admin role: %v", err)
	}

	existing, err := client.User.Query().Where(user.UsernameEQ(*username)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		_, err = client.User.Create().
			SetUsername(*username).
			SetEmail(*email).
			SetDisplayName("Default Admin").
			SetPasswordHash(hash).
			SetActive(true).
			AddRoles(adminRole).
			Save(ctx)
		if err != nil {
			log.Fatalf("seed: create user: %v", err)
		}
		log.Printf("seed: created user %q (id=new)", *username)
	case err != nil:
		log.Fatalf("seed: query user: %v", err)
	default:
		_, err = existing.Update().
			SetEmail(*email).
			SetDisplayName("Default Admin").
			SetPasswordHash(hash).
			SetActive(true).
			ClearRoles().
			AddRoles(adminRole).
			Save(ctx)
		if err != nil {
			log.Fatalf("seed: update user: %v", err)
		}
		log.Printf("seed: refreshed user %q (id=%d) password + roles", *username, existing.ID)
	}

	fmt.Println("seed: done")
}

func upsertRole(ctx context.Context, client *ent.Client, code, name, desc string, perms []string) error {
	existing, err := client.Role.Query().Where(role.CodeEQ(code)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		_, err = client.Role.Create().
			SetCode(code).SetName(name).SetDescription(desc).
			SetPermissions(perms).SetSystem(true).Save(ctx)
		return err
	case err != nil:
		return err
	default:
		_, err = existing.Update().
			SetName(name).SetDescription(desc).SetPermissions(perms).Save(ctx)
		return err
	}
}
