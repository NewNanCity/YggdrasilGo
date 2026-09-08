package sharedauth

import (
	"context"
	"database/sql"
	"errors"
	"sort"
)

// ResolveIdentityByUUID reads the current player name and owner for a stable UUID.
func (s *Service) ResolveIdentityByUUID(ctx context.Context, rawUUID string) (Identity, error) {
	value, err := parseUUID(rawUUID)
	if err != nil {
		return Identity{}, ErrNotFound
	}
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (Identity, error) {
		var pid uint64
		err := tx.QueryRowContext(ctx,
			"SELECT player_id FROM ygg_go_identities WHERE uuid=? AND state='active'", value[:]).Scan(&pid)
		if errors.Is(err, sql.ErrNoRows) {
			return Identity{}, ErrNotFound
		}
		if err != nil {
			return Identity{}, err
		}
		player, err := lockPlayer(ctx, tx, pid)
		if err != nil {
			return Identity{}, err
		}
		return readIdentity(ctx, tx, player)
	})
}

// ResolveIdentityByName resolves a unique current player and explicitly provisions
// an identity only when that player is newer than the migration watermark.
func (s *Service) ResolveIdentityByName(ctx context.Context, name string) (Identity, error) {
	if name == "" {
		return Identity{}, ErrNotFound
	}
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, watermark uint64) (Identity, error) {
		return s.resolveIdentityByName(ctx, tx, name, watermark)
	})
}

// ResolveIdentitiesByNames performs the public batch lookup in one transaction.
// Missing and quarantined names are omitted, matching the Yggdrasil profile API.
func (s *Service) ResolveIdentitiesByNames(ctx context.Context, names []string) ([]Identity, error) {
	if len(names) > 10 {
		return nil, ErrInvalid
	}
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, watermark uint64) ([]Identity, error) {
		type candidate struct {
			name string
			pid  uint64
			uid  uint64
		}
		candidates := make([]candidate, 0, len(names))
		seen := make(map[uint64]struct{}, len(names))
		for _, name := range names {
			pid, uid, err := uniquePlayerName(ctx, tx, name)
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrIdentityConflict) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if _, exists := seen[pid]; exists {
				continue
			}
			seen[pid] = struct{}{}
			candidates = append(candidates, candidate{name: name, pid: pid, uid: uid})
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].pid < candidates[j].pid })
		resolved := make(map[uint64]Identity, len(candidates))
		for _, candidate := range candidates {
			player, err := lockPlayer(ctx, tx, candidate.pid)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			currentPID, currentUID, err := uniquePlayerName(ctx, tx, candidate.name)
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrIdentityConflict) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if currentPID != player.PlayerID || currentUID != player.OwnerID || candidate.uid != player.OwnerID {
				continue
			}
			identity, err := s.ensureIdentity(ctx, tx, player, watermark)
			if errors.Is(err, ErrIdentityConflict) {
				continue
			}
			if err != nil {
				return nil, err
			}
			resolved[candidate.pid] = identity
		}
		identities := make([]Identity, 0, len(resolved))
		for _, name := range names {
			pid, _, err := uniquePlayerName(ctx, tx, name)
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrIdentityConflict) {
				continue
			}
			if err != nil {
				return nil, err
			}
			identity, exists := resolved[pid]
			if !exists {
				continue
			}
			identities = append(identities, identity)
			delete(resolved, pid)
		}
		return identities, nil
	})
}

func (s *Service) resolveIdentityByName(ctx context.Context, tx *sql.Tx, name string, watermark uint64) (Identity, error) {
	pid, uid, err := uniquePlayerName(ctx, tx, name)
	if err != nil {
		return Identity{}, err
	}
	player, err := lockPlayer(ctx, tx, pid)
	if err != nil {
		return Identity{}, err
	}
	// Re-resolve after acquiring the player lock so a concurrent rename cannot
	// return an identity for a name that is no longer current.
	currentPID, currentUID, err := uniquePlayerName(ctx, tx, name)
	if err != nil {
		return Identity{}, err
	}
	if currentPID != player.PlayerID || currentUID != player.OwnerID || uid != player.OwnerID {
		return Identity{}, ErrIdentityConflict
	}
	return s.ensureIdentity(ctx, tx, player, watermark)
}
