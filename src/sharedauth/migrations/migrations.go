// Package migrations contains explicit, offline-only schema operations.
// Runtime constructors must never call Upgrade or Downgrade.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
)

//go:embed *.sql
var statements embed.FS

var tables = []string{
	"ygg_go_identities", "ygg_go_auth_subjects", "ygg_go_tokens",
	"ygg_go_join_sessions", "ygg_go_state",
}

// Upgrade requires an explicitly approved, exclusive maintenance window.
// MySQL DDL commits separately; partial failure must be inspected, not retried blindly.
func Upgrade(ctx context.Context, db *sql.DB) error {
	var version, engine string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return err
	}
	var major, minor, patch int
	if _, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch); err != nil ||
		major != 8 || (minor == 0 && patch < 22) {
		return errors.New("shared auth requires MySQL 8.0.22 or later in the 8.x series")
	}
	if err := db.QueryRowContext(ctx, `SELECT ENGINE FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users'`).Scan(&engine); err != nil {
		return fmt.Errorf("inspect users engine: %w", err)
	}
	if engine != "InnoDB" {
		return errors.New("users must use InnoDB before installing security triggers")
	}
	for _, name := range tables {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, name).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("target table %s already exists; inspect the migration state", name)
		}
	}
	var triggers int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TRIGGERS
		WHERE TRIGGER_SCHEMA = DATABASE() AND EVENT_OBJECT_TABLE = 'users'`).Scan(&triggers); err != nil {
		return err
	}
	if triggers != 0 {
		return errors.New("existing users triggers require review before installation")
	}
	files := []string{"001_identities.sql", "002_subjects.sql", "003_tokens.sql",
		"004_sessions.sql", "005_state.sql", "006_user_update.sql", "007_user_delete.sql"}
	for _, name := range files {
		statement, err := statements.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, string(statement)); err != nil {
			return fmt.Errorf("schema step %s failed; earlier DDL may be committed: %w", name, err)
		}
	}
	return VerifyHooks(ctx, db)
}

// VerifyHooks is an operator check: MySQL hides these definitions from principals
// without TRIGGER privilege. It must succeed before marking migration state active.
func VerifyHooks(ctx context.Context, db *sql.DB) error {
	hooks := []struct{ file, name, event string }{
		{"006_user_update.sql", "ygg_go_users_security_update", "UPDATE"},
		{"007_user_delete.sql", "ygg_go_users_security_delete", "DELETE"},
	}
	for _, hook := range hooks {
		ddl, err := statements.ReadFile(hook.file)
		if err != nil {
			return err
		}
		expectedBody, err := triggerBody(string(ddl))
		if err != nil {
			return err
		}
		var body string
		err = db.QueryRowContext(ctx, `SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS
			WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE='users'
			AND ACTION_TIMING='AFTER' AND EVENT_MANIPULATION=?`, hook.name, hook.event).Scan(&body)
		if err != nil {
			return fmt.Errorf("security hook %s not verifiable: %w", hook.name, err)
		}
		if strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n")) != expectedBody {
			return fmt.Errorf("security hook %s differs from the reviewed definition", hook.name)
		}
	}
	return nil
}

func triggerBody(ddl string) (string, error) {
	// Each embedded file is one fixed CREATE TRIGGER with this exact header.
	_, body, found := strings.Cut(strings.ReplaceAll(ddl, "\r\n", "\n"), "FOR EACH ROW\n")
	if !found {
		return "", errors.New("invalid embedded security trigger header")
	}
	return strings.TrimSpace(body), nil
}

// Downgrade only removes an unused schema. Callers must first stop all writers.
// Any row, including staged migration data or a revocation anchor, prevents deletion.
func Downgrade(ctx context.Context, db *sql.DB) error {
	for _, name := range tables {
		var exists bool
		// Names come exclusively from the fixed allowlist above.
		if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+name+" LIMIT 1)").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("refusing downgrade: %s contains retained state", name)
		}
	}
	ddl := []string{
		"DROP TRIGGER ygg_go_users_security_update",
		"DROP TRIGGER ygg_go_users_security_delete",
		"DROP TABLE ygg_go_join_sessions", "DROP TABLE ygg_go_tokens",
		"DROP TABLE ygg_go_state", "DROP TABLE ygg_go_auth_subjects", "DROP TABLE ygg_go_identities",
	}
	for _, statement := range ddl {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("downgrade failed; inspect partial DDL state: %w", err)
		}
	}
	return nil
}
