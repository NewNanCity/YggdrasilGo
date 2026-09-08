package migrationplan

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"yggdrasil-api-go/internal/mysqltest"
	"yggdrasil-api-go/src/sharedauth"
	"yggdrasil-api-go/src/sharedauth/migrations"
)

func TestMySQLApplyVerifyAndPhaseGate(t *testing.T) {
	server := mysqltest.Start(t)
	fixture := func(t *testing.T) (*Plan, *sql.DB) {
		t.Helper()
		db := server.Database(t)
		for _, statement := range []string{
			`CREATE TABLE users (uid BIGINT UNSIGNED PRIMARY KEY, password VARCHAR(255) NOT NULL,
				permission INT NOT NULL) ENGINE=InnoDB`,
			`CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY, uid BIGINT UNSIGNED NOT NULL,
				name VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL) ENGINE=InnoDB`,
			`CREATE TABLE uuid (id BIGINT UNSIGNED PRIMARY KEY, name VARCHAR(50) CHARACTER SET utf8mb4
				COLLATE utf8mb4_unicode_ci NOT NULL, uuid VARCHAR(36) NOT NULL) ENGINE=InnoDB`,
			`INSERT INTO users VALUES (1,'hash',0)`,
			`INSERT INTO players VALUES (10,1,'Alpha'),(11,1,'Missing')`,
			`INSERT INTO uuid VALUES (1,'Alpha','a826612caebb3b2380ae77d4712a373a'),
				(2,'Orphan','00000000000040008000000000000001')`,
		} {
			if _, err := db.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
		snapshot, err := ReadSnapshot(t.Context(), db, 100)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := Build(snapshot, uuid.New(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := migrations.Upgrade(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		return &plan, db
	}

	t.Run("staged_apply_is_idempotent_and_gate_is_reversible", func(t *testing.T) {
		plan, db := fixture(t)
		if err := Apply(t.Context(), db, *plan, 100); err != nil {
			t.Fatal(err)
		}
		if err := Apply(t.Context(), db, *plan, 100); err != nil {
			t.Fatal("same staged plan was not idempotent", err)
		}
		if phase, err := Verify(t.Context(), db, *plan, 100); err != nil || phase != "staged" {
			t.Fatalf("verify staged phase=%q err=%v", phase, err)
		}
		service, err := sharedauth.New(db, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Ready(t.Context()); !errors.Is(err, sharedauth.ErrNotReady) {
			t.Fatal("staged schema opened runtime gate", err)
		}
		if err := Activate(t.Context(), db, *plan, 100); err != nil {
			t.Fatal(err)
		}
		if err := service.Ready(t.Context()); err != nil {
			t.Fatal("active schema remained closed", err)
		}
		if err := Deactivate(t.Context(), db, *plan); err != nil {
			t.Fatal(err)
		}
		if phase, err := Verify(t.Context(), db, *plan, 100); err != nil || phase != "staged" {
			t.Fatalf("verify deactivated phase=%q err=%v", phase, err)
		}
		var active, blocked, reserved int
		if err := db.QueryRow(`SELECT SUM(state='active'), SUM(state='blocked'), SUM(state='reserved')
			FROM ygg_go_identities`).Scan(&active, &blocked, &reserved); err != nil || active != 2 || blocked != 0 || reserved != 1 {
			t.Fatalf("identity counts active=%d blocked=%d reserved=%d err=%v", active, blocked, reserved, err)
		}
	})

	t.Run("source_drift_refuses_apply_without_partial_rows", func(t *testing.T) {
		plan, db := fixture(t)
		if _, err := db.ExecContext(t.Context(), "UPDATE players SET name='Renamed' WHERE pid=10"); err != nil {
			t.Fatal(err)
		}
		if err := Apply(t.Context(), db, *plan, 100); err == nil {
			t.Fatal("source drift was accepted")
		}
		var identities, states int
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_identities").Scan(&identities); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_state").Scan(&states); err != nil || identities != 0 || states != 0 {
			t.Fatalf("failed apply left identities=%d states=%d err=%v", identities, states, err)
		}
	})

	t.Run("tampered_generated_uuid_refuses_apply", func(t *testing.T) {
		plan, db := fixture(t)
		for index := range plan.Actions {
			if plan.Actions[index].Reason == "generated_offline_v3" {
				plan.Actions[index].UUID = "00000000000040008000000000000099"
			}
		}
		if err := Apply(t.Context(), db, *plan, 100); err == nil {
			t.Fatal("generated UUID not derived from the source snapshot was accepted")
		}
		var identities int
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_identities").Scan(&identities); err != nil || identities != 0 {
			t.Fatalf("tampered apply left identities=%d err=%v", identities, err)
		}
	})

	t.Run("pre_apply_revocation_anchor_is_preserved", func(t *testing.T) {
		plan, db := fixture(t)
		if _, err := db.ExecContext(t.Context(), "UPDATE users SET password='changed' WHERE uid=1"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "UPDATE users SET permission=1 WHERE uid=1"); err != nil {
			t.Fatal(err)
		}
		if err := Apply(t.Context(), db, *plan, 100); err != nil {
			t.Fatal("legitimate pre-apply revocation anchor blocked migration", err)
		}
		var generation uint64
		if err := db.QueryRow("SELECT generation FROM ygg_go_auth_subjects WHERE user_id=1").Scan(&generation); err != nil || generation != 2 {
			t.Fatalf("revocation anchor generation=%d err=%v", generation, err)
		}
	})

	t.Run("tampered_staged_rows_block_verify_and_activation", func(t *testing.T) {
		plan, db := fixture(t)
		if err := Apply(t.Context(), db, *plan, 100); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "UPDATE ygg_go_identities SET state='retired' WHERE player_id=10"); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(t.Context(), db, *plan, 100); err == nil {
			t.Fatal("tampered identity passed verification")
		}
		if err := Activate(t.Context(), db, *plan, 100); err == nil {
			t.Fatal("tampered identity was activated")
		}
	})

	t.Run("missing_security_hook_blocks_activation", func(t *testing.T) {
		plan, db := fixture(t)
		if err := Apply(t.Context(), db, *plan, 100); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "DROP TRIGGER ygg_go_users_security_update"); err != nil {
			t.Fatal(err)
		}
		if err := Activate(t.Context(), db, *plan, 100); err == nil {
			t.Fatal("migration activated without the approved security hook")
		}
		var phase string
		if err := db.QueryRow("SELECT phase FROM ygg_go_state WHERE id=1").Scan(&phase); err != nil || phase != "staged" {
			t.Fatalf("failed activation changed phase=%q err=%v", phase, err)
		}
	})
}
