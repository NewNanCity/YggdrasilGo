package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"yggdrasil-api-go/internal/mysqltest"
)

func TestTriggerBodyLineEndings(t *testing.T) {
	ddl := "CREATE TRIGGER sample AFTER UPDATE ON users\nFOR EACH ROW\nBEGIN\nEND\n"
	for _, input := range []string{ddl, strings.ReplaceAll(ddl, "\n", "\r\n")} {
		body, err := triggerBody(input)
		if err != nil || body != "BEGIN\nEND" {
			t.Fatalf("line ending changed the reviewed trigger body: %v", err)
		}
	}
	if _, err := triggerBody("invalid"); err == nil {
		t.Fatal("invalid embedded statement accepted")
	}
}

func TestMySQLSchema(t *testing.T) {
	server := mysqltest.Start(t)
	fixture := func(t *testing.T) *sql.DB {
		t.Helper()
		db := server.Database(t)
		_, err := db.Exec(`CREATE TABLE users (
			uid BIGINT UNSIGNED PRIMARY KEY, password VARCHAR(255) NOT NULL,
			permission INT NOT NULL, nickname VARCHAR(50) NOT NULL DEFAULT ''
		) ENGINE=InnoDB`)
		if err != nil {
			t.Fatal(err)
		}
		if err := Upgrade(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		return db
	}
	execSQL := func(t *testing.T, db *sql.DB, query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("upgrade_and_empty_downgrade", func(t *testing.T) {
		db := fixture(t)
		if err := Upgrade(t.Context(), db); err == nil {
			t.Fatal("repeated upgrade must not adopt existing schema")
		}
		if err := Downgrade(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
			t.Fatal("downgrade changed the BlessingSkin table", err)
		}
	})
	t.Run("identity_shape_and_uniqueness", func(t *testing.T) {
		db := fixture(t)
		insert := `INSERT INTO ygg_go_identities
			(player_id, uuid, state, created_at, updated_at) VALUES (?, ?, ?, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`
		uuid := make([]byte, 16)
		uuid[0] = 1
		execSQL(t, db, insert, 1, uuid, "active")
		for _, args := range [][]any{{2, uuid, "active"}, {1, nil, "blocked"}, {3, nil, "active"}, {nil, nil, "reserved"}, {0, nil, "blocked"}} {
			if _, err := db.Exec(insert, args...); err == nil {
				t.Fatalf("accepted invalid identity shape %v", args)
			}
		}
		if err := Downgrade(t.Context(), db); err == nil {
			t.Fatal("downgrade must retain identities")
		}
	})
	t.Run("password_bytes_aba_permission_and_delete", func(t *testing.T) {
		db := fixture(t)
		execSQL(t, db, "INSERT INTO users (uid, password, permission) VALUES (1, 'HashA', 0)")
		execSQL(t, db, "INSERT INTO ygg_go_auth_subjects VALUES (1, 1, UTC_TIMESTAMP(6))")
		for _, query := range []string{
			"UPDATE users SET nickname = 'Synthetic' WHERE uid = 1",
			"UPDATE users SET password = 'hasha' WHERE uid = 1",
			"UPDATE users SET password = 'HashA' WHERE uid = 1",
			"UPDATE users SET permission = 1 WHERE uid = 1",
			"DELETE FROM users WHERE uid = 1",
		} {
			execSQL(t, db, query)
		}
		var generation uint64
		if err := db.QueryRow("SELECT generation FROM ygg_go_auth_subjects WHERE user_id=1").Scan(&generation); err != nil || generation != 5 {
			t.Fatalf("generation = %d, err = %v; want 5", generation, err)
		}
	})
	t.Run("rollback_restores_password_and_generation", func(t *testing.T) {
		db := fixture(t)
		execSQL(t, db, "INSERT INTO users (uid, password, permission) VALUES (1, 'before', 0)")
		execSQL(t, db, "INSERT INTO ygg_go_auth_subjects VALUES (1, 8, UTC_TIMESTAMP(6))")
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec("UPDATE users SET password='after' WHERE uid=1"); err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		var hash string
		var generation uint64
		if err := db.QueryRow(`SELECT u.password, s.generation FROM users u
			JOIN ygg_go_auth_subjects s ON s.user_id=u.uid WHERE u.uid=1`).Scan(&hash, &generation); err != nil || hash != "before" || generation != 8 {
			t.Fatalf("rollback mismatch: generation=%d, err=%v", generation, err)
		}
	})
	t.Run("trigger_failure_aborts_password_change", func(t *testing.T) {
		db := fixture(t)
		execSQL(t, db, "INSERT INTO users (uid, password, permission) VALUES (1, 'before', 0)")
		execSQL(t, db, "INSERT INTO ygg_go_auth_subjects VALUES (1, 18446744073709551615, UTC_TIMESTAMP(6))")
		if _, err := db.Exec("UPDATE users SET password='after' WHERE uid=1"); err == nil {
			t.Fatal("epoch overflow must fail the entire password update")
		}
		var hash string
		if err := db.QueryRow("SELECT password FROM users WHERE uid=1").Scan(&hash); err != nil || hash != "before" {
			t.Fatal("trigger failure left a changed password", err)
		}
	})
	t.Run("operator_detects_changed_hook_definition", func(t *testing.T) {
		db := fixture(t)
		execSQL(t, db, "DROP TRIGGER ygg_go_users_security_update")
		execSQL(t, db, `CREATE TRIGGER ygg_go_users_security_update AFTER UPDATE ON users
			FOR EACH ROW BEGIN END`)
		if err := VerifyHooks(t.Context(), db); err == nil {
			t.Fatal("same trigger name with an empty body passed verification")
		}
	})
	t.Run("non_transactional_users_rejected_before_ddl", func(t *testing.T) {
		db := server.Database(t)
		execSQL(t, db, "CREATE TABLE users (uid INT PRIMARY KEY, password VARCHAR(255), permission INT) ENGINE=MyISAM")
		if err := Upgrade(t.Context(), db); err == nil {
			t.Fatal("installed atomic hooks on a nontransactional table")
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES
			WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ygg_go_identities'`).Scan(&count); err != nil || count != 0 {
			t.Fatal("preflight failure left schema changes", err)
		}
	})
	t.Run("existing_trigger_requires_review_before_ddl", func(t *testing.T) {
		db := server.Database(t)
		execSQL(t, db, "CREATE TABLE users (uid INT PRIMARY KEY, password VARCHAR(255), permission INT) ENGINE=InnoDB")
		execSQL(t, db, "CREATE TRIGGER synthetic_existing_hook AFTER UPDATE ON users FOR EACH ROW BEGIN END")
		if err := Upgrade(t.Context(), db); err == nil {
			t.Fatal("installed hooks without reviewing existing trigger order")
		}
	})
}
