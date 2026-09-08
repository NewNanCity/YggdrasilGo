package sharedauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"yggdrasil-api-go/internal/mysqltest"
)

func TestMySQLAuthenticate(t *testing.T) {
	server := mysqltest.Start(t)
	const old = "synthetic-old-token"
	t.Run("full_account_evicts_oldest_only_after_successful_login", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, "UPDATE ygg_go_tokens SET created_at=UTC_TIMESTAMP(6)-INTERVAL 1 HOUR")
		ip := netip.MustParseAddr("192.0.2.1")
		if err := s.Join(t.Context(), old, "a826612caebb3b2380ae77d4712a373a", "server", ip); err != nil {
			t.Fatal(err)
		}
		result, err := s.Authenticate(t.Context(), "Alpha", "synthetic-hash", "new-client", 1)
		if err != nil || result.Token.AccessToken == "" {
			t.Fatal("valid login at quota was not issued a token", err)
		}
		if result.Token.ClientToken != "new-client" || result.Token.Identity == nil || result.Token.Identity.PlayerID != 10 ||
			result.Token.Identity.UUID.String() != "a826612c-aebb-3b23-80ae-77d4712a373a" {
			t.Fatal("login changed the binding or existing UUID")
		}
		if _, err := s.Validate(t.Context(), result.Token.AccessToken, "new-client"); err != nil {
			t.Fatal("new token was not usable", err)
		}
		if _, err := s.Validate(t.Context(), old, "client"); !errors.Is(err, ErrInvalid) {
			t.Fatal("evicted token remained usable", err)
		}
		if _, err := s.HasJoined(t.Context(), "Alpha", "server", ip); !errors.Is(err, ErrInvalid) {
			t.Fatal("evicted token's session remained usable", err)
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_tokens WHERE user_id=1").Scan(&count); err != nil || count != 1 {
			t.Fatalf("token count=%d err=%v", count, err)
		}
	})
	t.Run("below_quota_preserves_active_tokens_and_uses_configured_lifetime", func(t *testing.T) {
		s, db := authFixture(t, server)
		client := strings.Repeat("AbC\u4e2d", 200)
		result, err := s.Authenticate(t.Context(), "aLpHa", "synthetic-hash", client, 2)
		if err != nil || result.Token.Identity == nil || result.Token.Identity.Name != "Alpha" || result.Token.ClientToken != client {
			t.Fatal("current name or binary client binding lost", err)
		}
		if len(result.Token.AccessToken) != 43 || strings.Contains(result.Token.AccessToken, ".") {
			t.Fatal("login did not issue an opaque token")
		}
		if _, err := s.Validate(t.Context(), old, "client"); err != nil {
			t.Fatal("login below quota evicted a valid token", err)
		}
		hash := sha256.Sum256([]byte(result.Token.AccessToken))
		var lifetime int64
		if err := db.QueryRow("SELECT TIMESTAMPDIFF(SECOND, created_at, expires_at) FROM ygg_go_tokens WHERE token_hash=?", hash[:]).Scan(&lifetime); err != nil || lifetime != 72*3600 {
			t.Fatalf("login lifetime=%d err=%v", lifetime, err)
		}
		if _, err := s.Validate(t.Context(), result.Token.AccessToken, strings.ToLower(client)); !errors.Is(err, ErrInvalid) {
			t.Fatal("client binding became case insensitive", err)
		}
	})
	t.Run("eviction_uses_issue_time_not_expiry", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, "UPDATE ygg_go_tokens SET created_at=UTC_TIMESTAMP(6)-INTERVAL 2 HOUR, expires_at=UTC_TIMESTAMP(6)+INTERVAL 10 HOUR")
		seedLoginToken(t, db, "synthetic-newer-token", 1, 1, -3600, 1800)
		if _, err := s.Authenticate(t.Context(), "Alpha", "synthetic-hash", "client", 2); err != nil {
			t.Fatal(err)
		}
		assertTokenRow(t, db, old, false)
		assertTokenRow(t, db, "synthetic-newer-token", true)
	})
	t.Run("only_current_generation_unexpired_own_tokens_count", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, "UPDATE ygg_go_auth_subjects SET generation=2 WHERE user_id=1")
		mustExec(t, db, "UPDATE ygg_go_tokens SET generation=2")
		seedLoginToken(t, db, "synthetic-revoked-token", 1, 1, -3600, 3600)
		seedLoginToken(t, db, "synthetic-expired-token", 1, 2, -3600, -60)
		mustExec(t, db, "INSERT INTO users VALUES (2, 'second@example.invalid', 'other-hash', 0, 1)")
		mustExec(t, db, "INSERT INTO ygg_go_auth_subjects VALUES (2, 1, UTC_TIMESTAMP(6))")
		seedLoginToken(t, db, "synthetic-other-owner", 2, 1, -3600, 3600)
		result, err := s.Authenticate(t.Context(), "Alpha", "synthetic-hash", "client", 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{old, "synthetic-revoked-token", "synthetic-expired-token", "synthetic-other-owner"} {
			assertTokenRow(t, db, raw, true)
		}
		hash := sha256.Sum256([]byte(result.Token.AccessToken))
		var generation uint64
		if err := db.QueryRow("SELECT generation FROM ygg_go_tokens WHERE token_hash=?", hash[:]).Scan(&generation); err != nil || generation != 2 {
			t.Fatal("login did not issue in the current generation", err)
		}
	})
	t.Run("equal_issue_times_have_stable_binary_tie_break", func(t *testing.T) {
		s, db := authFixture(t, server)
		const second = "synthetic-second-token"
		seedLoginToken(t, db, second, 1, 1, -3600, 3600)
		mustExec(t, db, "UPDATE ygg_go_tokens SET created_at='2000-01-01 00:00:00'")
		if _, err := s.Authenticate(t.Context(), "Alpha", "synthetic-hash", "client", 2); err != nil {
			t.Fatal(err)
		}
		firstHash, secondHash := sha256.Sum256([]byte(old)), sha256.Sum256([]byte(second))
		keepFirst := bytes.Compare(firstHash[:], secondHash[:]) > 0
		assertTokenRow(t, db, old, keepFirst)
		assertTokenRow(t, db, second, !keepFirst)
	})
	t.Run("reduced_limit_evicts_only_the_required_old_tokens", func(t *testing.T) {
		s, db := authFixture(t, server)
		seedLoginToken(t, db, "synthetic-second-token", 1, 1, -3600, 3600)
		seedLoginToken(t, db, "synthetic-third-token", 1, 1, -1800, 3600)
		result, err := s.Authenticate(t.Context(), "Alpha", "synthetic-hash", "client", 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{old, "synthetic-second-token", "synthetic-third-token"} {
			assertTokenRow(t, db, raw, false)
		}
		assertTokenRow(t, db, result.Token.AccessToken, true)
	})
	t.Run("email_login_lists_confirmed_profiles_without_guessing_legacy_uuid", func(t *testing.T) {
		s, db := authFixture(t, server)
		result, err := s.Authenticate(t.Context(), "user@example.invalid", "synthetic-hash", "", 2)
		if err != nil || result.Token.Identity != nil || len(result.AvailableProfiles) != 2 {
			t.Fatal("multi-profile login was not unselected", err)
		}
		client, err := uuid.Parse(result.Token.ClientToken)
		if err != nil || client.Version() != 4 || len(result.Token.ClientToken) != 32 {
			t.Fatal("missing client token did not get a compatible random value", err)
		}
		if result.AvailableProfiles[0].PlayerID != 10 || result.AvailableProfiles[0].UUID.Version() != 3 ||
			result.AvailableProfiles[1].PlayerID != 11 || result.AvailableProfiles[1].UUID.Version() != 4 {
			t.Fatal("profile identity or pid ordering changed")
		}
		var unmapped int
		if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_identities WHERE player_id=9").Scan(&unmapped); err != nil || unmapped != 0 {
			t.Fatal("legacy player was assigned a guessed identity", err)
		}
		selected, err := s.Refresh(t.Context(), result.Token.AccessToken, result.Token.ClientToken, result.AvailableProfiles[1].UUID.String())
		if err != nil || selected.Identity == nil || selected.Identity.PlayerID != 11 {
			t.Fatal("unselected login could not bind a confirmed profile", err)
		}
	})
	t.Run("email_with_one_available_profile_selects_it", func(t *testing.T) {
		s, db := authFixture(t, server)
		mustExec(t, db, "DELETE FROM players WHERE pid=11")
		result, err := s.Authenticate(t.Context(), "user@example.invalid", "synthetic-hash", "client", 2)
		if err != nil || result.Token.Identity == nil || result.Token.Identity.PlayerID != 10 || len(result.AvailableProfiles) != 1 {
			t.Fatal("single confirmed profile was not selected", err)
		}
	})
	t.Run("account_without_players_gets_unselected_token_and_first_subject", func(t *testing.T) {
		_, db := fixture(t, server)
		s, err := NewAuth(db, 5*time.Second, 72*time.Hour, fixturePolicy{})
		if err != nil {
			t.Fatal(err)
		}
		mustExec(t, db, "DELETE FROM players WHERE uid=1")
		result, err := s.Authenticate(t.Context(), "user@example.invalid", "synthetic-hash", "client", 1)
		if err != nil || result.Token.Identity != nil || result.AvailableProfiles == nil || len(result.AvailableProfiles) != 0 {
			t.Fatal("account without players could not get an unselected token", err)
		}
		var generation uint64
		if err := db.QueryRow("SELECT generation FROM ygg_go_auth_subjects WHERE user_id=1").Scan(&generation); err != nil || generation != 1 {
			t.Fatal("first login did not create its generation anchor", err)
		}
		if _, err := s.Validate(t.Context(), result.Token.AccessToken, "client"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unavailable_selected_profile_rolls_back_new_identities_and_eviction", func(t *testing.T) {
		s, db := authFixture(t, server)
		result, err := s.Authenticate(t.Context(), "Unmapped", "synthetic-hash", "client", 1)
		if !errors.Is(err, ErrIdentityConflict) || result.Token.AccessToken != "" || result.AvailableProfiles != nil {
			t.Fatal("unavailable selected profile published state", err)
		}
		assertOriginalLoginState(t, s, db)
	})
	for _, credentials := range []struct{ name, username, password string }{
		{"wrong_password", "Alpha", "wrong"},
		{"missing_user", "missing@example.invalid", "synthetic-hash"},
		{"empty_user", "", "synthetic-hash"},
		{"empty_password", "Alpha", ""},
	} {
		t.Run(credentials.name, func(t *testing.T) {
			s, db := authFixture(t, server)
			result, err := s.Authenticate(t.Context(), credentials.username, credentials.password, "client", 1)
			if !errors.Is(err, ErrInvalid) || result.Token.AccessToken != "" {
				t.Fatal("invalid credentials returned a token", err)
			}
			assertOriginalLoginState(t, s, db)
		})
	}
	t.Run("non_positive_limit_and_cancelled_context_do_not_evict", func(t *testing.T) {
		s, db := authFixture(t, server)
		for _, limit := range []int{0, -1} {
			if _, err := s.Authenticate(t.Context(), "Alpha", "synthetic-hash", "client", limit); !errors.Is(err, ErrNotReady) {
				t.Fatal("invalid configuration silently disabled quota", err)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := s.Authenticate(ctx, "Alpha", "synthetic-hash", "client", 1); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		assertOriginalLoginState(t, s, db)
	})
	for _, event := range []string{"insert_failure", "delete_failure", "access_entropy_failure", "uuid_entropy_failure", "repeated_old_token", "cancel_before_insert"} {
		t.Run(event+"_preserves_previous_state", func(t *testing.T) {
			s, db := authFixture(t, server)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			sentinel := errors.New("synthetic entropy failure")
			switch event {
			case "insert_failure":
				mustExec(t, db, `CREATE TRIGGER synthetic_login_insert_failure BEFORE INSERT ON ygg_go_tokens
					FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='synthetic insert failure'`)
			case "delete_failure":
				mustExec(t, db, `CREATE TRIGGER synthetic_login_delete_failure BEFORE DELETE ON ygg_go_tokens
					FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='synthetic delete failure'`)
			case "access_entropy_failure":
				s.newAccessToken = func() (string, error) { return "", sentinel }
			case "uuid_entropy_failure":
				s.newUUID = func() (uuid.UUID, error) { return uuid.Nil, sentinel }
			case "repeated_old_token":
				s.newAccessToken = func() (string, error) { calls++; return old, nil }
			case "cancel_before_insert":
				s.newAccessToken = func() (string, error) { cancel(); return "synthetic-cancelled-token", nil }
			}
			result, err := s.Authenticate(ctx, "Alpha", "synthetic-hash", "client", 1)
			if err == nil || errors.Is(err, ErrInvalid) || result.Token.AccessToken != "" || result.AvailableProfiles != nil {
				t.Fatal("backend failure was hidden or published state", err)
			}
			if event == "repeated_old_token" && calls != 3 {
				t.Fatalf("candidate retry count=%d", calls)
			}
			if strings.Contains(event, "entropy") && !errors.Is(err, sentinel) {
				t.Fatal("entropy error identity was lost", err)
			}
			assertOriginalLoginState(t, s, db)
		})
	}
	t.Run("restricted_runtime_can_log_in_and_evict", func(t *testing.T) {
		_, db := authFixture(t, server)
		s, err := NewAuth(server.Restricted(t, db), 5*time.Second, 72*time.Hour, fixturePolicy{})
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.Authenticate(t.Context(), "Alpha", "synthetic-hash", "client", 1)
		if err != nil {
			t.Fatal("least-privilege login failed", err)
		}
		if _, err := s.Validate(t.Context(), result.Token.AccessToken, "client"); err != nil {
			t.Fatal(err)
		}
		assertTokenRow(t, db, old, false)
	})
}

func seedLoginToken(t *testing.T, db *sql.DB, raw string, uid, generation uint64, issuedOffset, expiresOffset int) {
	t.Helper()
	hash := sha256.Sum256([]byte(raw))
	mustExec(t, db, `INSERT INTO ygg_go_tokens VALUES (?, ?, ?, NULL, 'client',
		TIMESTAMPADD(SECOND, ?, UTC_TIMESTAMP(6)), TIMESTAMPADD(SECOND, ?, UTC_TIMESTAMP(6)))`,
		hash[:], uid, generation, issuedOffset, expiresOffset)
}

func assertTokenRow(t *testing.T, db *sql.DB, raw string, want bool) {
	t.Helper()
	hash := sha256.Sum256([]byte(raw))
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_tokens WHERE token_hash=?", hash[:]).Scan(&count); err != nil || (count == 1) != want {
		t.Fatalf("token presence=%v want=%v err=%v", count == 1, want, err)
	}
}

func assertOriginalLoginState(t *testing.T, s *Service, db *sql.DB) {
	t.Helper()
	if _, err := s.Validate(t.Context(), "synthetic-old-token", "client"); err != nil {
		t.Fatal("unsuccessful login changed old authorization", err)
	}
	var tokens, allocated int
	if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_tokens").Scan(&tokens); err != nil || tokens != 1 {
		t.Fatal("unsuccessful login retained a new token", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM ygg_go_identities WHERE player_id=11").Scan(&allocated); err != nil || allocated != 0 {
		t.Fatal("unsuccessful login retained a new UUID", err)
	}
}
