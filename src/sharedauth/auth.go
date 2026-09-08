package sharedauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TokenResult struct {
	AccessToken string
	ClientToken string
	OwnerID     uint64
	Identity    *Identity
}

func (s *Service) Validate(ctx context.Context, accessToken, clientToken string) (TokenResult, error) {
	hash := sha256.Sum256([]byte(accessToken))
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (TokenResult, error) {
		token, err := s.authorize(ctx, tx, hash, clientToken)
		return token.result(), err
	})
}

func (s *Service) Refresh(ctx context.Context, accessToken, clientToken, selectedUUID string) (TokenResult, error) {
	hash := sha256.Sum256([]byte(accessToken))
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (TokenResult, error) {
		token, err := s.authorize(ctx, tx, hash, clientToken)
		if err != nil {
			return TokenResult{}, err
		}
		if selectedUUID != "" {
			selected, err := parseUUID(selectedUUID)
			if err != nil || (token.identity != nil && token.identity.UUID != selected) {
				return TokenResult{}, ErrInvalid
			}
			if token.identity == nil {
				identity, err := identityByUUID(ctx, tx, selected)
				if err != nil {
					return TokenResult{}, invalidIfMissing(err)
				}
				if identity.OwnerID != token.owner {
					return TokenResult{}, ErrInvalid
				}
				token.identity = &identity
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM ygg_go_tokens WHERE token_hash=?", hash[:]); err != nil {
			return TokenResult{}, err
		}
		return s.issueToken(ctx, tx, token, &hash)
	})
}

func (s *Service) issueToken(ctx context.Context, tx *sql.Tx, token authorizedToken, previousHash *[sha256.Size]byte) (TokenResult, error) {
	var identityID any
	if token.identity != nil {
		identityID = token.identity.ID
	}
	for range 3 {
		raw, err := s.newAccessToken()
		if err != nil {
			return TokenResult{}, err
		}
		nextHash := sha256.Sum256([]byte(raw))
		if previousHash != nil && nextHash == *previousHash {
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ygg_go_tokens
			(token_hash, user_id, generation, identity_id, client_token, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP(6), TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6)))`,
			nextHash[:], token.owner, token.generation, identityID, []byte(token.client), s.tokenTTL.Microseconds())
		if isDuplicate(err) {
			continue
		}
		if err != nil {
			return TokenResult{}, err
		}
		result := token.result()
		result.AccessToken = raw
		return result, nil
	}
	return TokenResult{}, errors.New("unique access token allocation exhausted")
}

func (s *Service) Invalidate(ctx context.Context, accessToken, clientToken string) error {
	hash := sha256.Sum256([]byte(accessToken))
	_, err := transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (struct{}, error) {
		var owner uint64
		err := tx.QueryRowContext(ctx, "SELECT user_id FROM ygg_go_tokens WHERE token_hash=?", hash[:]).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			return struct{}{}, nil
		}
		if err != nil {
			return struct{}{}, err
		}
		// Revocation does not require an enabled account; even deleted accounts can be invalidated.
		if _, err := lockUser(ctx, tx, owner); err != nil && !errors.Is(err, ErrNotFound) {
			return struct{}{}, err
		}
		if _, err := lockGeneration(ctx, tx, owner); err != nil {
			return struct{}{}, err
		}
		var storedClient []byte
		err = tx.QueryRowContext(ctx, "SELECT client_token FROM ygg_go_tokens WHERE token_hash=? FOR UPDATE", hash[:]).Scan(&storedClient)
		if errors.Is(err, sql.ErrNoRows) {
			return struct{}{}, nil
		}
		if err != nil {
			return struct{}{}, err
		}
		if !matchesClient(storedClient, clientToken) {
			return struct{}{}, ErrInvalid
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM ygg_go_tokens WHERE token_hash=?", hash[:])
		return struct{}{}, err
	})
	return err
}

func (s *Service) Signout(ctx context.Context, username, password string) error {
	if s.policy == nil || username == "" || password == "" {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	user, err := s.credentialSnapshot(ctx, username)
	if err != nil {
		return invalidIfMissing(err)
	}
	if !s.policy.Allowed(user.permission, user.verified) || !s.policy.VerifyPassword(password, user.password) {
		return ErrInvalid
	}
	_, err = transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (struct{}, error) {
		current, err := lockUser(ctx, tx, user.id)
		if err != nil {
			return struct{}{}, invalidIfMissing(err)
		}
		if current.password != user.password || !s.policy.Allowed(current.permission, current.verified) {
			return struct{}{}, ErrInvalid
		}
		// Resolve the login identifier again so rename/transfer cannot change its owner during verification.
		if err := checkLoginOwner(ctx, tx, username, user.id); err != nil {
			return struct{}{}, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ygg_go_auth_subjects (user_id, generation, updated_at)
			VALUES (?, 1, UTC_TIMESTAMP(6))
			ON DUPLICATE KEY UPDATE generation=generation+1, updated_at=UTC_TIMESTAMP(6)`, user.id)
		return struct{}{}, err
	})
	return err
}

func (s *Service) Join(ctx context.Context, accessToken, selectedUUID, serverID string, ip netip.Addr) error {
	if serverID == "" || len(serverID) > 255 || !ip.IsValid() || ip.Zone() != "" {
		return ErrInvalid
	}
	selected, err := parseUUID(selectedUUID)
	if err != nil {
		return ErrInvalid
	}
	hash := sha256.Sum256([]byte(accessToken))
	_, err = transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (struct{}, error) {
		token, err := s.authorize(ctx, tx, hash, "")
		if err != nil {
			return struct{}{}, err
		}
		if token.identity == nil || token.identity.UUID != selected {
			return struct{}{}, ErrInvalid
		}
		ipBytes := ip.Unmap().AsSlice()
		// Conditional delete never overwrites another unexpired binding.
		if _, err := tx.ExecContext(ctx, "DELETE FROM ygg_go_join_sessions WHERE server_id=? AND expires_at<=UTC_TIMESTAMP(6)", []byte(serverID)); err != nil {
			return struct{}{}, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ygg_go_join_sessions
			VALUES (?, ?, ?, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6) + INTERVAL 30 SECOND)`, []byte(serverID), hash[:], ipBytes)
		if !isDuplicate(err) {
			return struct{}{}, err
		}
		var previousHash, previousIP []byte
		err = tx.QueryRowContext(ctx, "SELECT token_hash, client_ip FROM ygg_go_join_sessions WHERE server_id=? FOR UPDATE", []byte(serverID)).Scan(&previousHash, &previousIP)
		if err != nil {
			return struct{}{}, err
		}
		if subtle.ConstantTimeCompare(previousHash, hash[:]) != 1 || subtle.ConstantTimeCompare(previousIP, ipBytes) != 1 {
			return struct{}{}, ErrInvalid
		}
		return struct{}{}, nil
	})
	return err
}

func (s *Service) HasJoined(ctx context.Context, name, serverID string, ip netip.Addr) (Identity, error) {
	if serverID == "" || len(serverID) > 255 || name == "" || ip.Zone() != "" {
		return Identity{}, ErrInvalid
	}
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (Identity, error) {
		var initial []byte
		err := tx.QueryRowContext(ctx, "SELECT token_hash FROM ygg_go_join_sessions WHERE server_id=?", []byte(serverID)).Scan(&initial)
		if errors.Is(err, sql.ErrNoRows) {
			return Identity{}, ErrInvalid
		}
		if err != nil {
			return Identity{}, err
		}
		if len(initial) != sha256.Size {
			return Identity{}, ErrInvalid
		}
		token, err := s.authorize(ctx, tx, [32]byte(initial), "")
		if err != nil {
			return Identity{}, err
		}
		if token.identity == nil {
			return Identity{}, ErrInvalid
		}
		pid, _, err := uniquePlayerName(ctx, tx, name)
		if err != nil {
			return Identity{}, invalidIfMissing(err)
		}
		if pid != token.identity.PlayerID {
			return Identity{}, ErrInvalid
		}
		var current, storedIP []byte
		var expires time.Time
		err = tx.QueryRowContext(ctx, "SELECT token_hash, client_ip, expires_at FROM ygg_go_join_sessions WHERE server_id=? FOR SHARE", []byte(serverID)).Scan(&current, &storedIP, &expires)
		if err != nil {
			return Identity{}, invalidIfMissing(err)
		}
		var now time.Time
		if err := tx.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP(6)").Scan(&now); err != nil {
			return Identity{}, err
		}
		if subtle.ConstantTimeCompare(current, initial) != 1 || !now.Before(expires) || !now.Before(token.expires) ||
			(ip.IsValid() && subtle.ConstantTimeCompare(storedIP, ip.Unmap().AsSlice()) != 1) {
			return Identity{}, ErrInvalid
		}
		return *token.identity, nil
	})
}

type userRow struct {
	id         uint64
	password   string
	permission int
	verified   bool
}

type authorizedToken struct {
	owner      uint64
	generation uint64
	client     string
	identity   *Identity
	expires    time.Time
}

func (token authorizedToken) result() TokenResult {
	return TokenResult{ClientToken: token.client, OwnerID: token.owner, Identity: token.identity}
}

func lockUser(ctx context.Context, tx *sql.Tx, uid uint64) (userRow, error) {
	var user userRow
	err := tx.QueryRowContext(ctx, "SELECT uid, password, permission, verified FROM users WHERE uid=? FOR SHARE", uid).
		Scan(&user.id, &user.password, &user.permission, &user.verified)
	if errors.Is(err, sql.ErrNoRows) {
		return userRow{}, ErrNotFound
	}
	return user, err
}

func lockGeneration(ctx context.Context, tx *sql.Tx, uid uint64) (uint64, error) {
	var generation uint64
	err := tx.QueryRowContext(ctx, "SELECT generation FROM ygg_go_auth_subjects WHERE user_id=? FOR UPDATE", uid).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrInvalid
	}
	return generation, err
}

func (s *Service) authorize(ctx context.Context, tx *sql.Tx, hash [32]byte, client string) (authorizedToken, error) {
	if s.policy == nil {
		return authorizedToken{}, ErrNotReady
	}
	var owner uint64
	if err := tx.QueryRowContext(ctx, "SELECT user_id FROM ygg_go_tokens WHERE token_hash=?", hash[:]).Scan(&owner); err != nil {
		return authorizedToken{}, invalidIfMissing(err)
	}
	user, err := lockUser(ctx, tx, owner)
	if err != nil {
		return authorizedToken{}, invalidIfMissing(err)
	}
	if !s.policy.Allowed(user.permission, user.verified) {
		return authorizedToken{}, ErrInvalid
	}
	generation, err := lockGeneration(ctx, tx, owner)
	if err != nil {
		return authorizedToken{}, err
	}
	var token authorizedToken
	var identityID sql.Null[uint64]
	err = tx.QueryRowContext(ctx, `SELECT user_id, generation, client_token, identity_id, expires_at
		FROM ygg_go_tokens WHERE token_hash=? FOR UPDATE`, hash[:]).
		Scan(&token.owner, &token.generation, &token.client, &identityID, &token.expires)
	if err != nil {
		return authorizedToken{}, invalidIfMissing(err)
	}
	if token.owner != owner || token.generation != generation || !matchesClient([]byte(token.client), client) {
		return authorizedToken{}, ErrInvalid
	}
	if identityID.Valid {
		var pid uint64
		if err := tx.QueryRowContext(ctx, "SELECT player_id FROM ygg_go_identities WHERE identity_id=?", identityID.V).Scan(&pid); err != nil {
			return authorizedToken{}, invalidIfMissing(err)
		}
		player, err := lockPlayer(ctx, tx, pid)
		if err != nil {
			return authorizedToken{}, invalidIfMissing(err)
		}
		identity, err := readIdentity(ctx, tx, player)
		if err != nil {
			return authorizedToken{}, invalidIfMissing(err)
		}
		if identity.OwnerID != owner || identity.ID != identityID.V {
			return authorizedToken{}, ErrInvalid
		}
		token.identity = &identity
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP(6)").Scan(&now); err != nil {
		return authorizedToken{}, err
	}
	if !now.Before(token.expires) {
		return authorizedToken{}, ErrInvalid
	}
	return token, nil
}

func matchesClient(stored []byte, supplied string) bool {
	return supplied == "" || subtle.ConstantTimeCompare(stored, []byte(supplied)) == 1
}

func invalidIfMissing(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
		return ErrInvalid
	}
	return err
}

func newOpaqueToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func parseUUID(value string) (uuid.UUID, error) {
	if len(value) != 32 && len(value) != 36 {
		return uuid.Nil, ErrInvalid
	}
	return uuid.Parse(value)
}

func identityByUUID(ctx context.Context, tx *sql.Tx, value uuid.UUID) (Identity, error) {
	var pid uint64
	if err := tx.QueryRowContext(ctx, "SELECT player_id FROM ygg_go_identities WHERE uuid=? AND state='active'", value[:]).Scan(&pid); err != nil {
		return Identity{}, invalidIfMissing(err)
	}
	player, err := lockPlayer(ctx, tx, pid)
	if err != nil {
		return Identity{}, err
	}
	return readIdentity(ctx, tx, player)
}

type rowQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func uniquePlayerName(ctx context.Context, db rowQuerier, name string) (uint64, uint64, error) {
	rows, err := db.QueryContext(ctx, "SELECT pid, uid FROM players WHERE name=? LIMIT 2", name)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	var pid, uid uint64
	count := 0
	for rows.Next() {
		if err := rows.Scan(&pid, &uid); err != nil {
			return 0, 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if count == 0 {
		return 0, 0, ErrNotFound
	}
	if count != 1 {
		return 0, 0, ErrIdentityConflict
	}
	return pid, uid, nil
}

func uniqueEmail(ctx context.Context, db rowQuerier, email string) (uint64, error) {
	rows, err := db.QueryContext(ctx, "SELECT uid FROM users WHERE email=? LIMIT 2", email)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var uid uint64
	count := 0
	for rows.Next() {
		if err := rows.Scan(&uid); err != nil {
			return 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, ErrInvalid
	}
	return uid, nil
}

func loginOwner(ctx context.Context, db rowQuerier, username string) (uint64, error) {
	if strings.Contains(username, "@") {
		return uniqueEmail(ctx, db, username)
	}
	_, uid, err := uniquePlayerName(ctx, db, username)
	return uid, err
}

func checkLoginOwner(ctx context.Context, tx *sql.Tx, username string, uid uint64) error {
	current, err := loginOwner(ctx, tx, username)
	if err != nil {
		return invalidIfMissing(err)
	}
	if current != uid {
		return ErrInvalid
	}
	return nil
}

func (s *Service) credentialSnapshot(ctx context.Context, username string) (userRow, error) {
	uid, err := loginOwner(ctx, s.db, username)
	if err != nil {
		return userRow{}, err
	}
	var user userRow
	err = s.db.QueryRowContext(ctx, "SELECT uid, password, permission, verified FROM users WHERE uid=?", uid).
		Scan(&user.id, &user.password, &user.permission, &user.verified)
	return user, err
}
