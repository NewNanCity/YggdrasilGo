package sharedauth

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type AuthenticateResult struct {
	Token             TokenResult
	AvailableProfiles []Identity
}

// Authenticate receives tokenLimit from trusted service configuration, never the client.
func (s *Service) Authenticate(ctx context.Context, username, password, clientToken string, tokenLimit int) (AuthenticateResult, error) {
	if s.policy == nil || tokenLimit <= 0 {
		return AuthenticateResult{}, ErrNotReady
	}
	if username == "" || password == "" {
		return AuthenticateResult{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	user, err := s.credentialSnapshot(ctx, username)
	if err != nil {
		return AuthenticateResult{}, invalidIfMissing(err)
	}
	if !s.policy.Allowed(user.permission, user.verified) || !s.policy.VerifyPassword(password, user.password) {
		return AuthenticateResult{}, ErrInvalid
	}
	if clientToken == "" {
		value, err := uuid.NewRandom()
		if err != nil {
			return AuthenticateResult{}, err
		}
		clientToken = hex.EncodeToString(value[:])
	}
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, watermark uint64) (AuthenticateResult, error) {
		current, err := lockUser(ctx, tx, user.id)
		if err != nil {
			return AuthenticateResult{}, invalidIfMissing(err)
		}
		if current.password != user.password || !s.policy.Allowed(current.permission, current.verified) {
			return AuthenticateResult{}, ErrInvalid
		}
		if err := checkLoginOwner(ctx, tx, username, user.id); err != nil {
			return AuthenticateResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO ygg_go_auth_subjects (user_id, generation, updated_at)
			VALUES (?, 1, UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE generation=generation`, user.id); err != nil {
			return AuthenticateResult{}, err
		}
		generation, err := lockGeneration(ctx, tx, user.id)
		if err != nil {
			return AuthenticateResult{}, err
		}
		victims, err := lockEvictionCandidates(ctx, tx, user.id, generation, tokenLimit)
		if err != nil {
			return AuthenticateResult{}, err
		}
		profiles, err := s.loginProfiles(ctx, tx, user.id, watermark)
		if err != nil {
			return AuthenticateResult{}, err
		}
		token := authorizedToken{owner: user.id, generation: generation, client: clientToken}
		if strings.Contains(username, "@") {
			if len(profiles) == 1 {
				token.identity = &profiles[0]
			}
		} else {
			pid, owner, err := uniquePlayerName(ctx, tx, username)
			if err != nil {
				return AuthenticateResult{}, invalidIfMissing(err)
			}
			if owner != user.id {
				return AuthenticateResult{}, ErrInvalid
			}
			for i := range profiles {
				if profiles[i].PlayerID == pid {
					token.identity = &profiles[i]
					break
				}
			}
			if token.identity == nil {
				return AuthenticateResult{}, ErrIdentityConflict
			}
		}
		issued, err := s.issueToken(ctx, tx, token, nil)
		if err != nil {
			return AuthenticateResult{}, err
		}
		// Keep old hashes present until INSERT succeeds, including collision retries.
		if len(victims) > 0 {
			args := make([]any, len(victims))
			for i := range victims {
				args[i] = victims[i]
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(victims)), ",")
			deleted, err := tx.ExecContext(ctx, "DELETE FROM ygg_go_tokens WHERE token_hash IN ("+placeholders+")", args...)
			if err != nil {
				return AuthenticateResult{}, err
			}
			count, err := deleted.RowsAffected()
			if err != nil {
				return AuthenticateResult{}, err
			}
			if count != int64(len(victims)) {
				return AuthenticateResult{}, errors.New("locked token eviction count changed")
			}
		}
		return AuthenticateResult{Token: issued, AvailableProfiles: profiles}, nil
	})
}

func lockEvictionCandidates(ctx context.Context, tx *sql.Tx, uid, generation uint64, limit int) ([][]byte, error) {
	rows, err := tx.QueryContext(ctx, `SELECT token_hash FROM ygg_go_tokens
		WHERE user_id=? AND generation=? AND expires_at>UTC_TIMESTAMP(6)
		ORDER BY created_at, token_hash FOR UPDATE`, uid, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hashes [][]byte
	for rows.Next() {
		var hash []byte
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(hashes) < limit {
		return nil, nil
	}
	return hashes[:len(hashes)-limit+1], nil
}

func (s *Service) loginProfiles(ctx context.Context, tx *sql.Tx, uid, watermark uint64) ([]Identity, error) {
	rows, err := tx.QueryContext(ctx, "SELECT pid, uid, name FROM players WHERE uid=? ORDER BY pid FOR SHARE", uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var players []Identity
	for rows.Next() {
		var player Identity
		if err := rows.Scan(&player.PlayerID, &player.OwnerID, &player.Name); err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	profiles := make([]Identity, 0, len(players))
	for _, player := range players {
		identity, err := readIdentity(ctx, tx, player)
		if errors.Is(err, ErrIdentityConflict) || (errors.Is(err, ErrNotFound) && player.PlayerID <= watermark) {
			continue
		}
		if errors.Is(err, ErrNotFound) {
			identity, err = s.ensureIdentity(ctx, tx, player, watermark)
		}
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, identity)
	}
	return profiles, nil
}
