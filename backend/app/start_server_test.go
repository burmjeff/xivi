package app

import (
	"context"
	"testing"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestDefaultAdministratorInitialization(t *testing.T) {
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE template (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	db.MustExec(`CREATE TABLE app_user (
		id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT NOT NULL, role TEXT NOT NULL, must_change_password BOOLEAN NOT NULL,
		initial_password BOOLEAN NOT NULL DEFAULT FALSE,
		auth_version INTEGER NOT NULL DEFAULT 1, disabled_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	db.MustExec(`CREATE TABLE user_lineup (
		user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
		lineup_id INTEGER NOT NULL REFERENCES template(id) ON DELETE CASCADE,
		PRIMARY KEY(user_id, lineup_id))`)
	db.MustExec(`CREATE TABLE user_mfa (user_id INTEGER PRIMARY KEY REFERENCES app_user(id))`)
	db.MustExec(`CREATE TABLE security_audit_event (
		id INTEGER PRIMARY KEY, actor_user_id INTEGER, actor_username TEXT NOT NULL DEFAULT '',
		target_user_id INTEGER, target_username TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL, client_ip TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`)

	original := database.Db
	database.Db = &database.Queries{SecurityQueries: queries.NewSecurityQueries(db)}
	t.Cleanup(func() {
		database.Db = original
		security.SetInitialSetupRequired(false)
	})

	created, err := initializeDefaultAdministrator(context.Background())
	if err != nil || !created {
		t.Fatalf("initialization returned created=%v err=%v", created, err)
	}
	if !security.InitialSetupRequired() {
		t.Fatal("initialization did not publish the required password-change state")
	}
	user, err := database.Db.GetUserByUsername(context.Background(), initialAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	passwordOK, err := security.VerifyPassword(initialAdminPassword, user.PasswordHash)
	if err != nil || !passwordOK || user.Role != "admin" || !user.MustChangePassword || !user.InitialPassword {
		t.Fatalf("unexpected default administrator: password=%v role=%q mustChange=%v initial=%v err=%v", passwordOK, user.Role, user.MustChangePassword, user.InitialPassword, err)
	}
	created, err = initializeDefaultAdministrator(context.Background())
	if err != nil || created {
		t.Fatalf("repeat initialization returned created=%v err=%v", created, err)
	}
	var auditTargets []string
	if err := db.Select(&auditTargets, `SELECT target_username FROM security_audit_event WHERE action = 'initial_admin_create'`); err != nil || len(auditTargets) != 1 {
		t.Fatalf("expected one bootstrap audit event, targets=%v err=%v", auditTargets, err)
	}
	if auditTargets[0] != initialAdminUsername {
		t.Fatalf("bootstrap audit did not identify the created administrator: %q", auditTargets[0])
	}
}
