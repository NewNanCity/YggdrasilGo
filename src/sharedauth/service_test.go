package sharedauth

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"yggdrasil-api-go/internal/mysqltest"
	"yggdrasil-api-go/src/sharedauth/migrations"
)

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, server *mysqltest.Server) (*Service, *sql.DB) {
	t.Helper()
	db := server.Database(t)
	mustExec(t, db, `CREATE TABLE users (uid BIGINT UNSIGNED PRIMARY KEY,
		email VARCHAR(100) NOT NULL, password VARCHAR(255) NOT NULL,
		permission INT NOT NULL, verified BOOLEAN NOT NULL) ENGINE=InnoDB`)
	mustExec(t, db, `CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY,
		uid BIGINT NOT NULL, name VARCHAR(50) NOT NULL) ENGINE=InnoDB`)
	if err := migrations.Upgrade(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	migrationID := uuid.New()
	mustExec(t, db, `INSERT INTO ygg_go_state VALUES (1, 1, 'active', 10, ?, UTC_TIMESTAMP(6))`, migrationID[:])
	mustExec(t, db, "INSERT INTO users VALUES (1, 'user@example.invalid', 'synthetic-hash', 0, 1)")
	mustExec(t, db, "INSERT INTO players VALUES (10, 1, 'Alpha'), (11, 1, 'NewPlayer'), (9, 1, 'Unmapped')")
	old := uuid.MustParse("a826612c-aebb-3b23-80ae-77d4712a373a")
	mustExec(t, db, `INSERT INTO ygg_go_identities (player_id, uuid, state, created_at, updated_at)
		VALUES (10, ?, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, old[:])
	s, err := New(db, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return s, db
}

func TestMySQLIdentity(t *testing.T) {
	server := mysqltest.Start(t)
	t.Run("profile_resolution_uses_pid_identity", func(t *testing.T) {
		s, db := fixture(t, server)
		byUUID, err := s.ResolveIdentityByUUID(t.Context(), "a826612caebb3b2380ae77d4712a373a")
		if err != nil || byUUID.PlayerID != 10 || byUUID.Name != "Alpha" {
			t.Fatalf("resolve uuid: profile=%+v err=%v", byUUID, err)
		}
		mustExec(t, db, "UPDATE players SET name='Renamed' WHERE pid=10")
		byName, err := s.ResolveIdentityByName(t.Context(), "Renamed")
		if err != nil || byName.UUID != byUUID.UUID || byName.PlayerID != 10 {
			t.Fatalf("resolve renamed profile: profile=%+v err=%v", byName, err)
		}
		fresh, err := s.ResolveIdentityByName(t.Context(), "NewPlayer")
		if err != nil || fresh.PlayerID != 11 || fresh.UUID.Version() != 4 {
			t.Fatalf("resolve new profile: profile=%+v err=%v", fresh, err)
		}
		if _, err := s.ResolveIdentityByName(t.Context(), "Unmapped"); !errors.Is(err, ErrIdentityConflict) {
			t.Fatalf("unmapped old profile: %v", err)
		}
		profiles, err := s.ResolveIdentitiesByNames(t.Context(), []string{"Renamed", "missing", "Unmapped", "NewPlayer", "Renamed"})
		if err != nil || len(profiles) != 2 || profiles[0].PlayerID != 10 || profiles[1].PlayerID != 11 {
			t.Fatalf("batch profiles=%+v err=%v", profiles, err)
		}
	})
	t.Run("batch_profile_resolution_has_stable_lock_order", func(t *testing.T) {
		s, db := fixture(t, server)
		mustExec(t, db, "INSERT INTO players VALUES (12,1,'OtherNew')")
		other, err := New(db, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		var group sync.WaitGroup
		failures := make([]error, 2)
		start := make(chan struct{})
		for i, request := range [][]string{{"NewPlayer", "OtherNew"}, {"OtherNew", "NewPlayer"}} {
			instance := s
			if i == 1 {
				instance = other
			}
			group.Go(func() {
				<-start
				profiles, resolveErr := instance.ResolveIdentitiesByNames(t.Context(), request)
				if resolveErr == nil && len(profiles) != 2 {
					resolveErr = errors.New("batch omitted a resolvable profile")
				}
				failures[i] = resolveErr
			})
		}
		close(start)
		group.Wait()
		if failures[0] != nil || failures[1] != nil {
			t.Fatalf("concurrent batches failed: %v", failures)
		}
	})
	t.Run("preserves_existing_uuid_after_rename", func(t *testing.T) {
		s, db := fixture(t, server)
		before, err := s.EnsureIdentity(t.Context(), 10)
		if err != nil || before.UUID.String() != "a826612c-aebb-3b23-80ae-77d4712a373a" {
			t.Fatalf("existing identity not preserved: %v", err)
		}
		mustExec(t, db, "UPDATE players SET name='Renamed' WHERE pid=10")
		after, err := s.EnsureIdentity(t.Context(), 10)
		if err != nil || after.UUID != before.UUID || after.Name != "Renamed" {
			t.Fatalf("rename changed identity: %v", err)
		}
	})
	t.Run("concurrent_new_player_converges", func(t *testing.T) {
		s, db := fixture(t, server)
		other, err := New(db, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		var group sync.WaitGroup
		var results [2]Identity
		var failures [2]error
		start := make(chan struct{})
		for i, instance := range []*Service{s, other} {
			group.Go(func() {
				<-start
				results[i], failures[i] = instance.EnsureIdentity(t.Context(), 11)
			})
		}
		close(start)
		group.Wait()
		if failures[0] != nil || failures[1] != nil || results[0].UUID != results[1].UUID || results[0].UUID.Version() != 4 {
			t.Fatalf("concurrent allocation failed: %v", failures)
		}
	})
	t.Run("unmapped_old_player_and_staged_gate_refused", func(t *testing.T) {
		s, db := fixture(t, server)
		if _, err := s.EnsureIdentity(t.Context(), 9); !errors.Is(err, ErrIdentityConflict) {
			t.Fatalf("unmapped legacy player: %v", err)
		}
		mustExec(t, db, "UPDATE ygg_go_state SET phase='staged', activated_at=NULL")
		if got, err := s.EnsureIdentity(t.Context(), 11); !errors.Is(err, ErrNotReady) || got != (Identity{}) {
			t.Fatalf("staged state published identity: %v", err)
		}
	})
	t.Run("collision_is_bounded_and_random_failure_never_publishes", func(t *testing.T) {
		s, db := fixture(t, server)
		calls := 0
		s.newUUID = func() (uuid.UUID, error) {
			calls++
			return uuid.MustParse("a826612c-aebb-3b23-80ae-77d4712a373a"), nil
		}
		if got, err := s.EnsureIdentity(t.Context(), 11); !errors.Is(err, ErrIdentityConflict) || got != (Identity{}) || calls != 3 {
			t.Fatalf("collision handling: calls=%d err=%v", calls, err)
		}
		sentinel := errors.New("synthetic entropy failure")
		s.newUUID = func() (uuid.UUID, error) { return uuid.Nil, sentinel }
		if got, err := s.EnsureIdentity(t.Context(), 11); !errors.Is(err, sentinel) || got != (Identity{}) {
			t.Fatalf("entropy failure published identity: %v", err)
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_identities WHERE player_id=11").Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed candidate retained: count=%d err=%v", count, err)
		}
	})
	t.Run("retired_and_reserved_identities_stay_unassignable", func(t *testing.T) {
		s, db := fixture(t, server)
		mustExec(t, db, "UPDATE ygg_go_identities SET state='retired' WHERE player_id=10")
		if _, err := s.EnsureIdentity(t.Context(), 10); !errors.Is(err, ErrIdentityConflict) {
			t.Fatal(err)
		}
		mustExec(t, db, "UPDATE players SET name='Renamed' WHERE pid=10")
		mustExec(t, db, "UPDATE players SET name='Alpha' WHERE pid=11")
		fresh, err := s.EnsureIdentity(t.Context(), 11)
		if err != nil || fresh.UUID.String() == "a826612c-aebb-3b23-80ae-77d4712a373a" {
			t.Fatalf("old name inherited retired identity: %v", err)
		}
	})
	t.Run("cancelled_context_does_not_allocate", func(t *testing.T) {
		s, _ := fixture(t, server)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if got, err := s.EnsureIdentity(ctx, 11); !errors.Is(err, context.Canceled) || got != (Identity{}) {
			t.Fatalf("cancelled request published identity: %v", err)
		}
	})
	t.Run("readiness_requires_active_schema_and_operator_checks_hooks", func(t *testing.T) {
		s, db := fixture(t, server)
		if err := s.Ready(t.Context()); err != nil {
			t.Fatal(err)
		}
		mustExec(t, db, "DROP TRIGGER ygg_go_users_security_update")
		if err := migrations.VerifyHooks(t.Context(), db); err == nil {
			t.Fatal("operator verification ignored missing security hook")
		}
		mustExec(t, db, "UPDATE ygg_go_state SET phase='staged', activated_at=NULL")
		if err := s.Ready(t.Context()); !errors.Is(err, ErrNotReady) {
			t.Fatal("runtime ignored closed migration gate", err)
		}
	})
}
