package sharedauth

import (
	"errors"
	"testing"
	"time"

	"yggdrasil-api-go/internal/mysqltest"
)

func TestMySQLCleanupExpired(t *testing.T) {
	server := mysqltest.Start(t)

	t.Run("deletes_expired_rows_in_bounded_batches", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, `INSERT INTO ygg_go_join_sessions
			VALUES ('expired-a', REPEAT(UNHEX('00'), 32), UNHEX('c0000201'), UTC_TIMESTAMP(6)-INTERVAL 2 MINUTE, UTC_TIMESTAMP(6)-INTERVAL 1 MINUTE),
			       ('expired-b', REPEAT(UNHEX('01'), 32), UNHEX('c0000201'), UTC_TIMESTAMP(6)-INTERVAL 2 MINUTE, UTC_TIMESTAMP(6)-INTERVAL 1 MINUTE),
			       ('live', REPEAT(UNHEX('02'), 32), UNHEX('c0000201'), UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)+INTERVAL 1 MINUTE)`)
		mustExec(t, db, `UPDATE ygg_go_tokens
			SET created_at=UTC_TIMESTAMP(6)-INTERVAL 2 HOUR, expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 HOUR`)

		first, err := s.CleanupExpired(t.Context(), 1)
		if err != nil || first.Tokens != 1 || first.Sessions != 1 {
			t.Fatalf("first cleanup = %+v, err=%v", first, err)
		}
		second, err := s.CleanupExpired(t.Context(), 10)
		if err != nil || second.Tokens != 0 || second.Sessions != 1 {
			t.Fatalf("second cleanup = %+v, err=%v", second, err)
		}
		var expired, live int
		if err := db.QueryRow(`SELECT
			SUM(expires_at<=UTC_TIMESTAMP(6)), SUM(expires_at>UTC_TIMESTAMP(6))
			FROM ygg_go_join_sessions`).Scan(&expired, &live); err != nil || expired != 0 || live != 1 {
			t.Fatalf("remaining sessions expired=%d live=%d err=%v", expired, live, err)
		}
	})

	t.Run("requires_active_state_and_valid_batch", func(t *testing.T) {
		s, db := authFixture(t, server)
		if _, err := s.CleanupExpired(t.Context(), 0); !errors.Is(err, ErrInvalid) {
			t.Fatalf("zero batch accepted: %v", err)
		}
		mustExec(t, db, "UPDATE ygg_go_state SET phase='staged', activated_at=NULL")
		if _, err := s.CleanupExpired(t.Context(), 10); !errors.Is(err, ErrNotReady) {
			t.Fatalf("staged cleanup accepted: %v", err)
		}
	})

	t.Run("failure_rolls_back_both_tables", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, `INSERT INTO ygg_go_join_sessions
			VALUES ('expired', REPEAT(UNHEX('00'), 32), UNHEX('c0000201'), UTC_TIMESTAMP(6)-INTERVAL 2 MINUTE, UTC_TIMESTAMP(6)-INTERVAL 1 MINUTE)`)
		mustExec(t, db, `UPDATE ygg_go_tokens
			SET created_at=UTC_TIMESTAMP(6)-INTERVAL 2 HOUR, expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 HOUR`)
		mustExec(t, db, `CREATE TRIGGER synthetic_cleanup_failure BEFORE DELETE ON ygg_go_join_sessions
			FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='synthetic cleanup failure'`)

		if _, err := s.CleanupExpired(t.Context(), 10); err == nil {
			t.Fatal("cleanup failure was hidden")
		}
		var sessions, tokens int
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_join_sessions").Scan(&sessions); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_tokens").Scan(&tokens); err != nil {
			t.Fatal(err)
		}
		if sessions != 1 || tokens != 1 {
			t.Fatalf("partial cleanup committed: sessions=%d tokens=%d", sessions, tokens)
		}
	})

	t.Run("locks_tokens_before_sessions", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, `INSERT INTO ygg_go_join_sessions
			VALUES ('expired', REPEAT(UNHEX('00'), 32), UNHEX('c0000201'), UTC_TIMESTAMP(6)-INTERVAL 2 MINUTE, UTC_TIMESTAMP(6)-INTERVAL 1 MINUTE)`)
		mustExec(t, db, `UPDATE ygg_go_tokens
			SET created_at=UTC_TIMESTAMP(6)-INTERVAL 2 HOUR, expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 HOUR`)
		blocker, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback()
		var hash []byte
		if err := blocker.QueryRowContext(t.Context(), "SELECT token_hash FROM ygg_go_tokens FOR UPDATE").Scan(&hash); err != nil {
			t.Fatal(err)
		}
		finished := make(chan error, 1)
		go func() {
			_, cleanupErr := s.CleanupExpired(t.Context(), 10)
			finished <- cleanupErr
		}()
		awaitLockWait(t, db, "ygg_go_tokens")

		probe, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var serverID []byte
		if err := probe.QueryRowContext(t.Context(), "SELECT server_id FROM ygg_go_join_sessions WHERE server_id='expired' FOR UPDATE NOWAIT").Scan(&serverID); err != nil {
			_ = probe.Rollback()
			t.Fatal("cleanup locked a session before its token", err)
		}
		if err := probe.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err := blocker.Rollback(); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("cleanup did not finish after token lock release")
		}
	})
}
