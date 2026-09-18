package resolutionplan

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"yggdrasil-api-go/internal/mysqltest"
	"yggdrasil-api-go/src/sharedauth/migrations"
)

func resolutionFixture(t *testing.T) (*sql.DB, Approval) {
	t.Helper()
	server := mysqltest.Start(t)
	db := server.Database(t)
	for _, statement := range []string{
		`CREATE TABLE users (uid BIGINT UNSIGNED PRIMARY KEY, password VARCHAR(255) NOT NULL, permission INT NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY, uid BIGINT UNSIGNED NOT NULL, name VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE uuid (id BIGINT UNSIGNED PRIMARY KEY, name VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL, uuid VARCHAR(36) NOT NULL) ENGINE=InnoDB`,
		`INSERT INTO users VALUES (1,'hash',0)`,
		`INSERT INTO players VALUES (10,1,'Alpha')`,
		`INSERT INTO uuid VALUES (30,'alpha','a826612caebb3b2380ae77d4712a373a')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrations.Upgrade(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	migrationID := make([]byte, 16)
	migrationID[0] = 4
	if _, err := db.ExecContext(t.Context(), `INSERT INTO ygg_go_state
		(id, schema_version, phase, player_high_watermark, migration_id, activated_at)
		VALUES (1, 1, 'active', 10, ?, UTC_TIMESTAMP(6))`, migrationID); err != nil {
		t.Fatal(err)
	}
	if err := migrations.UpgradeResolutionSchema(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := migrations.ActivateResolutionSchema(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO ygg_go_identities
		(identity_id, player_id, uuid, state, legacy_mapping_id, created_at, updated_at)
		VALUES (20, 10, NULL, 'blocked', NULL, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
		(21, NULL, UNHEX('a826612caebb3b2380ae77d4712a373a'), 'reserved', 30, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`); err != nil {
		t.Fatal(err)
	}
	return db, Approval{PlayerID: 10, BlockedIdentityID: 20, ReservedIdentityID: 21, LegacyMappingIDs: []uint64{30}}
}

func TestBuildApplyVerifyAndRollback(t *testing.T) {
	db, approval := resolutionFixture(t)
	plan, err := Build(t.Context(), db, []Approval{approval}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decisions[0].PlayerName != "Alpha" || plan.Decisions[0].UUID != "a826612caebb3b2380ae77d4712a373a" {
		t.Fatalf("unexpected decision: %+v", plan.Decisions[0])
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE ygg_go_state SET phase='staged', activated_at=NULL WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := Apply(t.Context(), db, plan); err != nil {
		t.Fatal(err)
	}
	if err := Verify(t.Context(), db, plan); err != nil {
		t.Fatal(err)
	}
	if err := Activate(t.Context(), db, plan); err != nil {
		t.Fatal(err)
	}
	if err := Deactivate(t.Context(), db, plan); err != nil {
		t.Fatal(err)
	}
	var active, resolved, blocked, reserved int
	if err := db.QueryRowContext(t.Context(), `SELECT
		SUM(state='active'), SUM(state='resolved'), SUM(state='blocked'), SUM(state='reserved')
		FROM ygg_go_identities`).Scan(&active, &resolved, &blocked, &reserved); err != nil {
		t.Fatal(err)
	}
	if active != 1 || resolved != 1 || blocked != 0 || reserved != 0 {
		t.Fatalf("unexpected post-apply counts active=%d resolved=%d blocked=%d reserved=%d", active, resolved, blocked, reserved)
	}
	if err := Rollback(t.Context(), db, plan); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRowContext(t.Context(), "SELECT state FROM ygg_go_identities WHERE identity_id=20").Scan(&state); err != nil || state != "blocked" {
		t.Fatalf("rollback blocked state=%q err=%v", state, err)
	}
}

func TestApplyRejectsSourceDriftAndLiveReferences(t *testing.T) {
	db, approval := resolutionFixture(t)
	plan, err := Build(t.Context(), db, []Approval{approval}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE players SET name='Changed' WHERE pid=10"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE ygg_go_state SET phase='staged', activated_at=NULL WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := Apply(t.Context(), db, plan); err == nil {
		t.Fatal("source drift was accepted")
	}
	var identities int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM ygg_go_identities WHERE state='resolved'").Scan(&identities); err != nil || identities != 0 {
		t.Fatalf("source drift left resolved rows=%d err=%v", identities, err)
	}

	db, approval = resolutionFixture(t)
	plan, err = Build(t.Context(), db, []Approval{approval}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO ygg_go_auth_subjects (user_id, generation, updated_at) VALUES (1,1,UTC_TIMESTAMP(6))"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO ygg_go_tokens
		(token_hash, user_id, generation, identity_id, client_token, created_at, expires_at)
		VALUES (UNHEX(REPEAT('aa',32)),1,1,21,'x',UTC_TIMESTAMP(6),DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 1 HOUR))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE ygg_go_state SET phase='staged', activated_at=NULL WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	err = Apply(t.Context(), db, plan)
	if err == nil || !strings.Contains(err.Error(), "live token references") {
		t.Fatalf("live reference was not rejected: %v", err)
	}

	db, approval = resolutionFixture(t)
	plan, err = Build(t.Context(), db, []Approval{approval}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO uuid
		(id, name, uuid) VALUES (31, 'ALPHA', 'a826612caebb3b2380ae77d4712a373a')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE ygg_go_state SET phase='staged', activated_at=NULL WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := Apply(t.Context(), db, plan); err == nil || !strings.Contains(err.Error(), "mapping set") {
		t.Fatalf("additional collation-equivalent mapping was not rejected: %v", err)
	}
}
