package sharedauth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"yggdrasil-api-go/internal/mysqltest"
)

func TestMySQLAuthenticateConcurrency(t *testing.T) {
	server := mysqltest.Start(t)
	const old = "synthetic-old-token"
	t.Run("four_instances_share_quota_and_create_one_subject", func(t *testing.T) {
		_, db := fixture(t, server)
		var instances [4]*Service
		for i := range instances {
			instance, err := NewAuth(db, 5*time.Second, 72*time.Hour, fixturePolicy{})
			if err != nil {
				t.Fatal(err)
			}
			instances[i] = instance
		}
		var outcomes [8]authenticateOutcome
		var group sync.WaitGroup
		start := make(chan struct{})
		for i := range outcomes {
			group.Go(func() {
				<-start
				result, err := instances[i%len(instances)].Authenticate(t.Context(), "Alpha", "synthetic-hash", "client", 2)
				outcomes[i] = authenticateOutcome{result: result, err: err}
			})
		}
		close(start)
		group.Wait()
		valid := 0
		for _, outcome := range outcomes {
			if outcome.err != nil || outcome.result.Token.AccessToken == "" {
				t.Fatal("concurrent login failed", outcome.err)
			}
			_, err := instances[0].Validate(t.Context(), outcome.result.Token.AccessToken, "client")
			if err == nil {
				valid++
			} else if !errors.Is(err, ErrInvalid) {
				t.Fatal(err)
			}
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_tokens WHERE user_id=1 AND generation=1").Scan(&count); err != nil || count != 2 || valid != 2 {
			t.Fatalf("shared quota: records=%d valid=%d err=%v", count, valid, err)
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_auth_subjects WHERE user_id=1 AND generation=1").Scan(&count); err != nil || count != 1 {
			t.Fatal("first concurrent login changed or duplicated generation", err)
		}
	})
	t.Run("login_evicts_old_token_before_waiting_refresh", func(t *testing.T) {
		s, db := authFixture(t, server)
		other, err := NewAuth(db, 5*time.Second, 72*time.Hour, fixturePolicy{})
		if err != nil {
			t.Fatal(err)
		}
		entered, release := pauseTokenIssue(t, s)
		login := startAuthenticate(t.Context(), s, "Alpha")
		awaitCheckpoint(t, entered)
		refresh := startRefresh(t, other, old)
		awaitLockWait(t, db, "ygg_go_auth_subjects")
		release()
		loggedIn, refreshed := <-login, <-refresh
		if loggedIn.err != nil || !errors.Is(refreshed.err, ErrInvalid) || refreshed.token.AccessToken != "" {
			t.Fatalf("login=%v waiting refresh=%v", loggedIn.err, refreshed.err)
		}
		if _, err := other.Validate(t.Context(), loggedIn.result.Token.AccessToken, "client"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("waiting_login_evicts_the_just_refreshed_token_at_limit_one", func(t *testing.T) {
		s, db := authFixture(t, server)
		other, err := NewAuth(db, 5*time.Second, 72*time.Hour, fixturePolicy{})
		if err != nil {
			t.Fatal(err)
		}
		entered, release := pauseTokenIssue(t, s)
		refresh := startRefresh(t, s, old)
		awaitCheckpoint(t, entered)
		login := startAuthenticate(t.Context(), other, "Alpha")
		awaitLockWait(t, db, "ygg_go_auth_subjects")
		release()
		refreshed, loggedIn := <-refresh, <-login
		if refreshed.err != nil || loggedIn.err != nil {
			t.Fatalf("refresh=%v waiting login=%v", refreshed.err, loggedIn.err)
		}
		if _, err := s.Validate(t.Context(), refreshed.token.AccessToken, "client"); !errors.Is(err, ErrInvalid) {
			t.Fatal("waiting login failed to enforce quota after refresh", err)
		}
		if _, err := s.Validate(t.Context(), loggedIn.result.Token.AccessToken, "client"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("password_update_waits_for_login_then_revokes_new_token", func(t *testing.T) {
		s, db := authFixture(t, server)
		entered, release := pauseTokenIssue(t, s)
		login := startAuthenticate(t.Context(), s, "Alpha")
		awaitCheckpoint(t, entered)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		changed := make(chan error, 1)
		go func() {
			_, err := db.ExecContext(ctx, "UPDATE users SET password='changed' WHERE uid=1")
			changed <- err
		}()
		awaitLockWait(t, db, "users")
		release()
		outcome := <-login
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if err := <-changed; err != nil {
			t.Fatal(err)
		}
		if _, err := s.Validate(t.Context(), outcome.result.Token.AccessToken, "client"); !errors.Is(err, ErrInvalid) {
			t.Fatal("token survived the password update committed after login", err)
		}
	})
	t.Run("signout_waits_for_login_then_revokes_new_token", func(t *testing.T) {
		s, db := authFixture(t, server)
		entered, release := pauseTokenIssue(t, s)
		login := startAuthenticate(t.Context(), s, "Alpha")
		awaitCheckpoint(t, entered)
		signedOut := make(chan error, 1)
		go func() { signedOut <- s.Signout(t.Context(), "Alpha", "synthetic-hash") }()
		awaitLockWait(t, db, "ygg_go_auth_subjects")
		release()
		outcome := <-login
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if err := <-signedOut; err != nil {
			t.Fatal(err)
		}
		if _, err := s.Validate(t.Context(), outcome.result.Token.AccessToken, "client"); !errors.Is(err, ErrInvalid) {
			t.Fatal("token survived the signout committed after login", err)
		}
	})
	t.Run("cancelling_subject_lock_wait_does_not_evict", func(t *testing.T) {
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
		login := startAuthenticate(ctx, s, "Alpha")
		awaitLockWait(t, db, "ygg_go_auth_subjects")
		cancel()
		select {
		case outcome := <-login:
			if !errors.Is(outcome.err, context.Canceled) || outcome.result.Token.AccessToken != "" {
				t.Fatal("cancelled login published state", outcome.err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("login did not respond to cancellation while its subject was locked")
		}
		if err := blocker.Rollback(); err != nil {
			t.Fatal(err)
		}
		assertOriginalLoginState(t, s, db)
	})
	for _, change := range []struct {
		name, username, query string
		failure               error
	}{
		{"password", "Alpha", "UPDATE users SET password='changed' WHERE uid=1", ErrInvalid},
		{"permission", "Alpha", "UPDATE users SET permission=-1 WHERE uid=1", ErrInvalid},
		{"account_deletion", "Alpha", "DELETE FROM users WHERE uid=1", ErrInvalid},
		{"player_transfer", "Alpha", "UPDATE players SET uid=2 WHERE pid=10", ErrInvalid},
		{"duplicate_name", "Alpha", "INSERT INTO players VALUES (12, 2, 'ALPHA')", ErrIdentityConflict},
		{"duplicate_email", "user@example.invalid", "UPDATE users SET email='USER@example.invalid' WHERE uid=2", ErrInvalid},
	} {
		t.Run("rechecks_"+change.name+"_after_password_verification", func(t *testing.T) {
			s, db := authFixture(t, server)
			mustExec(t, db, "INSERT INTO users VALUES (2, 'other@example.invalid', 'other-hash', 0, 1)")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			s.policy = checkpointPolicy{ctx: ctx, entered: entered, release: release}
			login := startAuthenticate(ctx, s, change.username)
			awaitCheckpoint(t, entered)
			mustExec(t, db, change.query)
			unblock()
			outcome := <-login
			if !errors.Is(outcome.err, change.failure) || outcome.result.Token.AccessToken != "" || outcome.result.AvailableProfiles != nil {
				t.Fatal("changed authorization snapshot published state", outcome.err)
			}
			assertTokenRow(t, db, old, true)
			var tokens, allocated int
			if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_tokens").Scan(&tokens); err != nil || tokens != 1 {
				t.Fatal("stale credential verification retained a token", err)
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_identities WHERE player_id=11").Scan(&allocated); err != nil || allocated != 0 {
				t.Fatal("stale credential verification retained an identity", err)
			}
		})
	}
}

type authenticateOutcome struct {
	result AuthenticateResult
	err    error
}

func startAuthenticate(ctx context.Context, s *Service, username string) <-chan authenticateOutcome {
	result := make(chan authenticateOutcome, 1)
	go func() {
		value, err := s.Authenticate(ctx, username, "synthetic-hash", "client", 1)
		result <- authenticateOutcome{result: value, err: err}
	}()
	return result
}

func pauseTokenIssue(t *testing.T, s *Service) (<-chan struct{}, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	entered, release := make(chan struct{}), make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() { cancel(); unblock() })
	original := s.newAccessToken
	s.newAccessToken = func() (string, error) {
		enterOnce.Do(func() { close(entered) })
		select {
		case <-release:
			return original()
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return entered, unblock
}

func awaitCheckpoint(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("operation did not reach its controlled checkpoint")
	}
}

// A bounded test-only verification checkpoint; not a production account policy.
type checkpointPolicy struct {
	ctx     context.Context
	entered chan<- struct{}
	release <-chan struct{}
}

func (checkpointPolicy) Allowed(permission int, verified bool) bool {
	return (fixturePolicy{}).Allowed(permission, verified)
}

func (p checkpointPolicy) VerifyPassword(password, hash string) bool {
	close(p.entered)
	select {
	case <-p.release:
		return (fixturePolicy{}).VerifyPassword(password, hash)
	case <-p.ctx.Done():
		return false
	}
}
