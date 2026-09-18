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

const (
	initialSchemaVersion    = 1
	resolutionSchemaVersion = 2
)

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

// UpgradeResolutionSchema adds the audit state used by reviewed blocked identity
// resolutions. It intentionally leaves ygg_go_state at version 1 until the
// separately verified activation step succeeds.
func UpgradeResolutionSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("database is required")
	}
	version, phase, err := readSchemaState(ctx, db, false)
	if err != nil {
		return err
	}
	if version != initialSchemaVersion || (phase != "staged" && phase != "active") {
		return fmt.Errorf("resolution schema upgrade requires v1 staged/active state, got v%d %s", version, phase)
	}
	var columns int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ygg_go_identities'
		AND COLUMN_NAME='resolved_into_identity_id'`).Scan(&columns); err != nil {
		return err
	}
	if columns != 0 {
		return errors.New("resolution schema column already exists; inspect the migration state")
	}
	statement, err := statements.ReadFile("008_identity_resolution_v2.up.sql")
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, string(statement)); err != nil {
		return fmt.Errorf("resolution schema upgrade failed; inspect committed DDL state: %w", err)
	}
	for _, name := range []string{"009_identity_resolution_insert.sql", "010_identity_resolution_update.sql"} {
		statement, err := statements.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, string(statement)); err != nil {
			return fmt.Errorf("resolution schema step %s failed; inspect committed DDL state: %w", name, err)
		}
	}
	return VerifyResolutionSchema(ctx, db)
}

// VerifyResolutionSchema checks the structural objects required before the
// schema version can be activated. It performs no repair.
func VerifyResolutionSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("database is required")
	}
	var columns int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ygg_go_identities'
		AND COLUMN_NAME='resolved_into_identity_id' AND COLUMN_TYPE='bigint unsigned'
		AND IS_NULLABLE='YES'`).Scan(&columns); err != nil {
		return fmt.Errorf("inspect resolution target column: %w", err)
	} else if columns != 1 {
		return fmt.Errorf("resolution target column is not verifiable: count=%d", columns)
	}
	var indexes int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ygg_go_identities'
		AND INDEX_NAME='ix_identity_resolved_into' AND COLUMN_NAME='resolved_into_identity_id'`).Scan(&indexes); err != nil {
		return fmt.Errorf("inspect resolution target index: %w", err)
	} else if indexes != 1 {
		return fmt.Errorf("resolution target index is not verifiable: count=%d", indexes)
	}
	var foreignKeys int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS r
		JOIN information_schema.KEY_COLUMN_USAGE k
		  ON k.CONSTRAINT_SCHEMA=r.CONSTRAINT_SCHEMA AND k.CONSTRAINT_NAME=r.CONSTRAINT_NAME
		WHERE r.CONSTRAINT_SCHEMA=DATABASE() AND r.TABLE_NAME='ygg_go_identities'
		AND r.CONSTRAINT_NAME='fk_identity_resolved_into'
		AND r.REFERENCED_TABLE_NAME='ygg_go_identities'
		AND r.UPDATE_RULE='RESTRICT' AND r.DELETE_RULE='RESTRICT'
		AND k.COLUMN_NAME='resolved_into_identity_id' AND k.REFERENCED_COLUMN_NAME='identity_id'`).Scan(&foreignKeys); err != nil {
		return fmt.Errorf("inspect resolution target foreign key: %w", err)
	} else if foreignKeys != 1 {
		return fmt.Errorf("resolution target foreign key is not verifiable: count=%d", foreignKeys)
	}
	var clause string
	if err := db.QueryRowContext(ctx, `SELECT CHECK_CLAUSE FROM information_schema.CHECK_CONSTRAINTS
		WHERE CONSTRAINT_SCHEMA=DATABASE() AND CONSTRAINT_NAME='ck_identity_state'`).Scan(&clause); err != nil {
		return fmt.Errorf("resolution identity state constraint is not verifiable: %w", err)
	}
	normalized := strings.ToLower(clause)
	normalized = strings.NewReplacer("`", "", "_utf8mb4", "", "'", "", "\"", "", "\\", "", "(", "", ")", "", " ", "", "\t", "", "\r", "", "\n", "").Replace(normalized)
	expectedClause := "stateinactive,retiredandplayer_idisnotnullanduuidisnotnullandresolved_into_identity_idisnull" +
		"orstate=reservedandplayer_idisnullanduuidisnotnullandresolved_into_identity_idisnull" +
		"orstate=blockedandplayer_idisnotnullanduuidisnullandresolved_into_identity_idisnull" +
		"orstate=resolvedandplayer_idisnullanduuidisnullandlegacy_mapping_idisnullandresolved_into_identity_idisnotnull"
	if normalized != expectedClause {
		return errors.New("resolution identity state constraint differs from the reviewed definition")
	}
	for _, hook := range []struct{ file, name, timing, event string }{
		{"009_identity_resolution_insert.sql", "ygg_go_identity_resolution_insert", "AFTER", "INSERT"},
		{"010_identity_resolution_update.sql", "ygg_go_identity_resolution_update", "BEFORE", "UPDATE"},
	} {
		dll, err := statements.ReadFile(hook.file)
		if err != nil {
			return err
		}
		expectedBody, err := triggerBody(string(dll))
		if err != nil {
			return err
		}
		var body string
		if err := db.QueryRowContext(ctx, `SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS
			WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE='ygg_go_identities'
			AND ACTION_TIMING=? AND EVENT_MANIPULATION=?`, hook.name, hook.timing, hook.event).Scan(&body); err != nil {
			return fmt.Errorf("resolution hook %s is not verifiable: %w", hook.name, err)
		}
		if strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n")) != expectedBody {
			return fmt.Errorf("resolution hook %s differs from the reviewed definition", hook.name)
		}
	}
	return nil
}

// ActivateResolutionSchema changes only schema metadata after the DDL has been
// independently verified.
func ActivateResolutionSchema(ctx context.Context, db *sql.DB) error {
	if err := VerifyResolutionSchema(ctx, db); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	version, phase, err := readSchemaState(ctx, tx, true)
	if err != nil {
		return err
	}
	if version != initialSchemaVersion || (phase != "staged" && phase != "active") {
		return fmt.Errorf("resolution schema activation requires v1 staged/active state, got v%d %s", version, phase)
	}
	result, err := tx.ExecContext(ctx, `UPDATE ygg_go_state SET schema_version=?
		WHERE id=1 AND schema_version=?`, resolutionSchemaVersion, initialSchemaVersion)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("resolution schema version changed concurrently")
	}
	return tx.Commit()
}

// DeactivateResolutionSchema returns schema metadata to v1 only while no
// resolved audit rows remain. DDL is removed by a separate operation.
func DeactivateResolutionSchema(ctx context.Context, db *sql.DB) error {
	if err := VerifyResolutionSchema(ctx, db); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	version, phase, err := readSchemaState(ctx, tx, true)
	if err != nil {
		return err
	}
	if version != resolutionSchemaVersion || (phase != "staged" && phase != "active") {
		return fmt.Errorf("resolution schema deactivation requires v2 staged/active state, got v%d %s", version, phase)
	}
	var retained int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ygg_go_identities
		WHERE state='resolved' OR resolved_into_identity_id IS NOT NULL`).Scan(&retained); err != nil {
		return err
	}
	if retained != 0 {
		return fmt.Errorf("refusing resolution schema deactivation: %d resolved identities remain", retained)
	}
	result, err := tx.ExecContext(ctx, `UPDATE ygg_go_state SET schema_version=?
		WHERE id=1 AND schema_version=?`, initialSchemaVersion, resolutionSchemaVersion)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("resolution schema version changed concurrently")
	}
	return tx.Commit()
}

// DowngradeResolutionSchema removes only the unused v2 DDL after metadata has
// already returned to v1.
func DowngradeResolutionSchema(ctx context.Context, db *sql.DB) error {
	if err := VerifyResolutionSchema(ctx, db); err != nil {
		return err
	}
	version, phase, err := readSchemaState(ctx, db, false)
	if err != nil {
		return err
	}
	if version != initialSchemaVersion || (phase != "staged" && phase != "active") {
		return fmt.Errorf("resolution schema downgrade requires v1 staged/active state, got v%d %s", version, phase)
	}
	var retained int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ygg_go_identities
		WHERE state='resolved' OR resolved_into_identity_id IS NOT NULL`).Scan(&retained); err != nil {
		return err
	}
	if retained != 0 {
		return fmt.Errorf("refusing resolution schema downgrade: %d resolved identities remain", retained)
	}
	for _, name := range []string{"ygg_go_identity_resolution_update", "ygg_go_identity_resolution_insert"} {
		if _, err := db.ExecContext(ctx, "DROP TRIGGER "+name); err != nil {
			return fmt.Errorf("drop resolution hook %s: %w", name, err)
		}
	}
	statement, err := statements.ReadFile("008_identity_resolution_v2.down.sql")
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, string(statement)); err != nil {
		return fmt.Errorf("resolution schema downgrade failed; inspect committed DDL state: %w", err)
	}
	var columns int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ygg_go_identities'
		AND COLUMN_NAME='resolved_into_identity_id'`).Scan(&columns); err != nil {
		return fmt.Errorf("inspect downgraded resolution column: %w", err)
	} else if columns != 0 {
		return fmt.Errorf("resolution schema downgrade is incomplete: columns=%d", columns)
	}
	return nil
}

type schemaStateQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readSchemaState(ctx context.Context, db schemaStateQuerier, lock bool) (int, string, error) {
	query := "SELECT schema_version, phase FROM ygg_go_state WHERE id=1"
	if lock {
		query += " FOR UPDATE"
	}
	var version int
	var phase string
	if err := db.QueryRowContext(ctx, query).Scan(&version, &phase); err != nil {
		return 0, "", fmt.Errorf("read shared auth schema state: %w", err)
	}
	return version, phase, nil
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
