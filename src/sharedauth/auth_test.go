package sharedauth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"yggdrasil-api-go/internal/mysqltest"
)

// This is a synthetic fixture policy, not evidence about the installed BlessingSkin version.
type fixturePolicy struct{}

func (fixturePolicy) Allowed(permission int, verified bool) bool { return permission != -1 }
func (fixturePolicy) VerifyPassword(password, hash string) bool  { return password == hash }

func authFixture(t *testing.T, server *mysqltest.Server) (*Service, *sql.DB) {
	t.Helper()
	_, db := fixture(t, server)
	s, err := NewAuth(db, 5*time.Second, 72*time.Hour, fixturePolicy{})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, "INSERT INTO ygg_go_auth_subjects VALUES (1, 1, UTC_TIMESTAMP(6))")
	hash := sha256.Sum256([]byte("synthetic-old-token"))
	mustExec(t, db, `INSERT INTO ygg_go_tokens
		SELECT ?, 1, 1, identity_id, ?, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6) + INTERVAL 1 HOUR
		FROM ygg_go_identities WHERE player_id=10`, hash[:], "client")
	return s, db
}

func TestMySQLAuth(t *testing.T) {
	server := mysqltest.Start(t)
	ip := netip.MustParseAddr("192.0.2.1")
	const old = "synthetic-old-token"
	const profile = "a826612caebb3b2380ae77d4712a373a"
	t.Run("active_token_and_repeated_session_validation", func(t *testing.T) {
		s, _ := authFixture(t, server)
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatalf("valid token rejected: %v", err)
		}
		if err := s.Join(t.Context(), old, profile, "server", ip); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			identity, err := s.HasJoined(t.Context(), "Alpha", "server", ip)
			if err != nil || identity.PlayerID != 10 {
				t.Fatalf("repeated verification failed: %v", err)
			}
		}
	})
	t.Run("concurrent_refresh_has_one_winner", func(t *testing.T) {
		s, db := authFixture(t, server)
		other, err := NewAuth(db, 5*time.Second, 72*time.Hour, fixturePolicy{})
		if err != nil {
			t.Fatal(err)
		}
		var group sync.WaitGroup
		var results [2]TokenResult
		var failures [2]error
		start := make(chan struct{})
		for i, instance := range []*Service{s, other} {
			group.Go(func() {
				<-start
				results[i], failures[i] = instance.Refresh(t.Context(), old, "client", "")
			})
		}
		close(start)
		group.Wait()
		winners := 0
		for i, failure := range failures {
			if failure == nil {
				winners++
				if len(results[i].AccessToken) != 43 || strings.Contains(results[i].AccessToken, ".") {
					t.Fatal("refresh did not issue an opaque token")
				}
			} else if !errors.Is(failure, ErrInvalid) {
				t.Fatal(failure)
			}
		}
		if winners != 1 {
			t.Fatalf("refresh winners = %d, errors=%v", winners, failures)
		}
	})
	t.Run("password_aba_revokes_token_and_existing_session", func(t *testing.T) {
		s, db := authFixture(t, server)
		if err := s.Join(t.Context(), old, profile, "server", ip); err != nil {
			t.Fatal(err)
		}
		mustExec(t, db, "UPDATE users SET password='other' WHERE uid=1")
		mustExec(t, db, "UPDATE users SET password='synthetic-hash' WHERE uid=1")
		if _, err := s.Validate(t.Context(), old, ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("old token survived password ABA: %v", err)
		}
		if _, err := s.HasJoined(t.Context(), "Alpha", "server", ip); !errors.Is(err, ErrInvalid) {
			t.Fatalf("old session survived password ABA: %v", err)
		}
	})
	t.Run("failed_refresh_preserves_old_token", func(t *testing.T) {
		s, _ := authFixture(t, server)
		if _, err := s.Refresh(t.Context(), old, "wrong-client", ""); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := s.Refresh(t.Context(), old, "client", "ffffffffffffffffffffffffffffffff"); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal("unsuccessful refresh lost the old token", err)
		}
	})
	t.Run("invalidate_binding_and_signout", func(t *testing.T) {
		s, _ := authFixture(t, server)
		if err := s.Invalidate(t.Context(), old, "wrong"); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal(err)
		}
		if err := s.Signout(t.Context(), "user@example.invalid", "wrong"); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if err := s.Signout(t.Context(), "user@example.invalid", "synthetic-hash"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Validate(t.Context(), old, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if err := s.Invalidate(t.Context(), "missing", ""); err != nil {
			t.Fatal("invalidate missing token was not idempotent", err)
		}
	})
	t.Run("refresh_insert_failure_rolls_back_deletion", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, `CREATE TRIGGER synthetic_token_insert_failure BEFORE INSERT ON ygg_go_tokens
			FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='synthetic insert failure'`)
		if got, err := s.Refresh(t.Context(), old, "client", ""); err == nil || got.AccessToken != "" {
			t.Fatal("insert failure returned a token")
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal("insert failure committed deletion", err)
		}
	})
	t.Run("revocation_write_failure_is_not_success", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, `CREATE TRIGGER synthetic_token_delete_failure BEFORE DELETE ON ygg_go_tokens
			FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='synthetic delete failure'`)
		if err := s.Invalidate(t.Context(), old, "client"); err == nil || errors.Is(err, ErrInvalid) {
			t.Fatal("storage failure was hidden", err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("session_binding_expiry_and_name_checks", func(t *testing.T) {
		s, db := authFixture(t, server)
		if err := s.Join(t.Context(), old, profile, "server", ip); err != nil {
			t.Fatal(err)
		}
		var original, after time.Time
		if err := db.QueryRow("SELECT expires_at FROM ygg_go_join_sessions").Scan(&original); err != nil {
			t.Fatal(err)
		}
		if err := s.Join(t.Context(), old, profile, "server", netip.MustParseAddr("::ffff:192.0.2.1")); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT expires_at FROM ygg_go_join_sessions").Scan(&after); err != nil || !after.Equal(original) {
			t.Fatal("retry extended session lifetime", err)
		}
		if err := s.Join(t.Context(), old, profile, "server", netip.MustParseAddr("192.0.2.2")); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := s.HasJoined(t.Context(), "Wrong", "server", ip); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := s.HasJoined(t.Context(), "aLpHa", "server", ip); err != nil {
			t.Fatal("site collation compatibility lost", err)
		}
		mustExec(t, db, "UPDATE ygg_go_join_sessions SET created_at=UTC_TIMESTAMP(6)-INTERVAL 31 SECOND, expires_at=UTC_TIMESTAMP(6)")
		if _, err := s.HasJoined(t.Context(), "Alpha", "server", ip); !errors.Is(err, ErrInvalid) {
			t.Fatal("expired session accepted", err)
		}
	})
	t.Run("current_owner_ban_deletion_and_duplicate_names", func(t *testing.T) {
		s, db := authFixture(t, server)
		if err := s.Join(t.Context(), old, profile, "server", ip); err != nil {
			t.Fatal(err)
		}
		mustExec(t, db, "INSERT INTO players VALUES (12, 1, 'ALPHA')")
		if _, err := s.HasJoined(t.Context(), "Alpha", "server", ip); !errors.Is(err, ErrIdentityConflict) {
			t.Fatal("ambiguous name accepted", err)
		}
		mustExec(t, db, "UPDATE players SET uid=2 WHERE pid=10")
		if _, err := s.Validate(t.Context(), old, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("old owner retained token authorization", err)
		}
		mustExec(t, db, "UPDATE players SET uid=1 WHERE pid=10")
		mustExec(t, db, "UPDATE users SET permission=-1 WHERE uid=1")
		mustExec(t, db, "UPDATE users SET permission=0 WHERE uid=1")
		if _, err := s.Validate(t.Context(), old, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("unban resurrected token", err)
		}
		mustExec(t, db, "DELETE FROM users WHERE uid=1")
		if _, err := s.Validate(t.Context(), old, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("deleted user accepted", err)
		}
	})
	t.Run("unselected_refresh_can_bind_once_and_client_token_is_binary", func(t *testing.T) {
		s, db := authFixture(t, server)
		client := strings.Repeat("AbC", 200)
		mustExec(t, db, "UPDATE ygg_go_tokens SET identity_id=NULL, client_token=?", []byte(client))
		if err := s.Join(t.Context(), old, profile, "server", ip); !errors.Is(err, ErrInvalid) {
			t.Fatal("unselected token joined", err)
		}
		if _, err := s.Refresh(t.Context(), old, strings.ToLower(client), profile); !errors.Is(err, ErrInvalid) {
			t.Fatal("client binding was case insensitive", err)
		}
		result, err := s.Refresh(t.Context(), old, client, profile)
		if err != nil || result.ClientToken != client || result.Identity == nil || result.Identity.PlayerID != 10 {
			t.Fatal("profile binding or long client token failed", err)
		}
	})
	t.Run("refresh_and_signout_never_resurrect", func(t *testing.T) {
		for round := range 8 {
			s, _ := authFixture(t, server)
			var group sync.WaitGroup
			start := make(chan struct{})
			var result TokenResult
			var refreshErr, signoutErr error
			group.Go(func() {
				<-start
				result, refreshErr = s.Refresh(t.Context(), old, "client", "")
			})
			group.Go(func() {
				<-start
				signoutErr = s.Signout(t.Context(), "user@example.invalid", "synthetic-hash")
			})
			close(start)
			group.Wait()
			if signoutErr != nil || (refreshErr != nil && !errors.Is(refreshErr, ErrInvalid)) {
				t.Fatalf("round %d: refresh=%v signout=%v", round, refreshErr, signoutErr)
			}
			for _, candidate := range []string{old, result.AccessToken} {
				if _, err := s.Validate(t.Context(), candidate, ""); !errors.Is(err, ErrInvalid) {
					t.Fatal("token survived committed signout", err)
				}
			}
		}
	})
	t.Run("cancelled_refresh_preserves_old_state", func(t *testing.T) {
		s, _ := authFixture(t, server)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if result, err := s.Refresh(ctx, old, "", ""); !errors.Is(err, context.Canceled) || result.AccessToken != "" {
			t.Fatal("cancelled refresh published state", err)
		}
		if _, err := s.Validate(t.Context(), old, ""); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("password_update_commits_while_refresh_waits_for_user", func(t *testing.T) {
		s, db := authFixture(t, server)
		writer, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Rollback()
		if _, err := writer.ExecContext(t.Context(), "UPDATE users SET password='changed' WHERE uid=1"); err != nil {
			t.Fatal(err)
		}
		result := startRefresh(t, s, old)
		awaitLockWait(t, db, "users")
		if err := writer.Commit(); err != nil {
			t.Fatal(err)
		}
		got := <-result
		if !errors.Is(got.err, ErrInvalid) || got.token.AccessToken != "" {
			t.Fatal("refresh waiting behind password change published a token", got.err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); !errors.Is(err, ErrInvalid) {
			t.Fatal("old token survived committed password change", err)
		}
	})
	t.Run("password_update_waits_for_refresh_then_revokes_its_result", func(t *testing.T) {
		s, db := authFixture(t, server)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		allocated, release := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		unblock := func() { releaseOnce.Do(func() { close(release) }) }
		defer unblock()
		s.newAccessToken = func() (string, error) {
			close(allocated)
			select {
			case <-release:
				return "synthetic-refreshed-token", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		result := startRefresh(t, s, old)
		select {
		case <-allocated:
		case <-ctx.Done():
			t.Fatal("refresh did not reach the token allocation checkpoint")
		}
		written := make(chan error, 1)
		go func() {
			_, err := db.ExecContext(ctx, "UPDATE users SET password='changed' WHERE uid=1")
			written <- err
		}()
		awaitLockWait(t, db, "users")
		unblock()
		got := <-result
		if got.err != nil || got.token.AccessToken == "" {
			t.Fatal("refresh holding the user lock did not commit", got.err)
		}
		if err := <-written; err != nil {
			t.Fatal("password update failed after refresh committed", err)
		}
		for _, candidate := range []string{old, got.token.AccessToken} {
			if _, err := s.Validate(t.Context(), candidate, "client"); !errors.Is(err, ErrInvalid) {
				t.Fatal("token survived the waiting password update", err)
			}
		}
	})
	t.Run("cancel_during_subject_lock_wait_preserves_old_token", func(t *testing.T) {
		s, db := authFixture(t, server)
		blocker, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback()
		var generation uint64
		if err := blocker.QueryRowContext(t.Context(), "SELECT generation FROM ygg_go_auth_subjects WHERE user_id=1 FOR UPDATE").Scan(&generation); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan refreshOutcome, 1)
		go func() {
			token, err := s.Refresh(ctx, old, "client", "")
			result <- refreshOutcome{token: token, err: err}
		}()
		awaitLockWait(t, db, "ygg_go_auth_subjects")
		cancel()
		select {
		case got := <-result:
			if !errors.Is(got.err, context.Canceled) || got.token.AccessToken != "" {
				t.Fatal("cancelled lock waiter published a token", got.err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("lock waiter did not respond to cancellation")
		}
		if err := blocker.Rollback(); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal("cancellation during lock wait changed old token state", err)
		}
	})
	t.Run("read_only_database_must_not_authorize", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, "SET GLOBAL super_read_only=ON")
		defer mustExec(t, db, "SET GLOBAL read_only=OFF")
		defer mustExec(t, db, "SET GLOBAL super_read_only=OFF")
		if _, err := s.Validate(t.Context(), old, "client"); !errors.Is(err, ErrNotReady) {
			t.Fatalf("read-only source used as authorization authority: %v", err)
		}
	})
	t.Run("restricted_runtime_can_authorize_without_business_writes", func(t *testing.T) {
		_, db := authFixture(t, server)
		restricted := server.Restricted(t, db)
		s, err := NewAuth(restricted, 5*time.Second, 72*time.Hour, fixturePolicy{})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Ready(t.Context()); err != nil {
			t.Fatal("restricted readiness failed", err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal("restricted validation failed", err)
		}
		if _, err := s.EnsureIdentity(t.Context(), 11); err != nil {
			t.Fatal("restricted identity allocation failed", err)
		}
		if err := s.Join(t.Context(), old, profile, "server", ip); err != nil {
			t.Fatal(err)
		}
		if _, err := s.HasJoined(t.Context(), "Alpha", "server", ip); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Refresh(t.Context(), old, "client", ""); err != nil {
			t.Fatal(err)
		}
		if err := s.Signout(t.Context(), "user@example.invalid", "synthetic-hash"); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"UPDATE users SET permission=1 WHERE uid=1",
			"DELETE FROM ygg_go_identities", "DELETE FROM ygg_go_auth_subjects",
			"UPDATE ygg_go_state SET phase='staged', activated_at=NULL",
			"UPDATE ygg_go_identities SET player_id=99 WHERE player_id=10",
			"CREATE TABLE unexpected_table (id INT)",
		} {
			if _, err := restricted.ExecContext(t.Context(), forbidden); err == nil {
				t.Fatal("runtime principal gained an unapproved write privilege")
			}
		}
	})
	t.Run("refresh_collision_budget_and_entropy_failure_roll_back", func(t *testing.T) {
		s, _ := authFixture(t, server)
		calls := 0
		s.newAccessToken = func() (string, error) { calls++; return old, nil }
		if result, err := s.Refresh(t.Context(), old, "client", ""); err == nil || result.AccessToken != "" || calls != 3 {
			t.Fatal("candidate retry budget was not enforced", err)
		}
		sentinel := errors.New("synthetic entropy failure")
		s.newAccessToken = func() (string, error) { return "", sentinel }
		if result, err := s.Refresh(t.Context(), old, "client", ""); !errors.Is(err, sentinel) || result.AccessToken != "" {
			t.Fatal("entropy failure was hidden", err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal("token allocation failure committed old token deletion", err)
		}
	})
	t.Run("expired_token_rejected_and_refresh_uses_configured_lifetime", func(t *testing.T) {
		s, db := authFixture(t, server)
		result, err := s.Refresh(t.Context(), old, "client", "")
		if err != nil {
			t.Fatal(err)
		}
		var lifetime int64
		if err := db.QueryRow("SELECT TIMESTAMPDIFF(SECOND, created_at, expires_at) FROM ygg_go_tokens").Scan(&lifetime); err != nil || lifetime != 72*3600 {
			t.Fatalf("lifetime=%d err=%v", lifetime, err)
		}
		mustExec(t, db, "UPDATE ygg_go_tokens SET created_at=UTC_TIMESTAMP(6)-INTERVAL 1 HOUR, expires_at=UTC_TIMESTAMP(6)")
		if _, err := s.Validate(t.Context(), result.AccessToken, ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("expired token accepted", err)
		}
		if _, err := s.Refresh(t.Context(), result.AccessToken, "", ""); !errors.Is(err, ErrInvalid) {
			t.Fatal("expired token refreshed", err)
		}
	})
}

type refreshOutcome struct {
	token TokenResult
	err   error
}

func startRefresh(t *testing.T, s *Service, token string) <-chan refreshOutcome {
	t.Helper()
	result := make(chan refreshOutcome, 1)
	go func() {
		value, err := s.Refresh(t.Context(), token, "client", "")
		result <- refreshOutcome{token: value, err: err}
	}()
	return result
}

// Observe a real InnoDB lock wait before releasing the controlling transaction.
func awaitLockWait(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits AS waits
			JOIN performance_schema.data_locks AS locks
			ON locks.ENGINE_LOCK_ID=waits.REQUESTING_ENGINE_LOCK_ID AND locks.ENGINE=waits.ENGINE
			WHERE locks.OBJECT_SCHEMA=DATABASE() AND locks.OBJECT_NAME=?`, table).Scan(&count)
		if err != nil {
			t.Fatal("inspect synthetic lock wait", err)
		}
		if count > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no controlled lock wait observed on %s", table)
		case <-ticker.C:
		}
	}
}
