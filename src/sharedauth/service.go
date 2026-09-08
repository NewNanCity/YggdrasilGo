// Package sharedauth owns the shared MySQL authentication transaction boundary.
package sharedauth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

var (
	ErrNotReady         = errors.New("shared authentication is not active")
	ErrNotFound         = errors.New("shared authentication record not found")
	ErrInvalid          = errors.New("invalid credentials or token")
	ErrIdentityConflict = errors.New("player identity requires review")
	ErrCommitUnknown    = errors.New("transaction commit outcome is unknown")
)

type Identity struct {
	ID       uint64
	PlayerID uint64
	UUID     uuid.UUID
	Name     string
	OwnerID  uint64
}

func FormatUUID(value uuid.UUID) string {
	return fmt.Sprintf("%x", value[:])
}

type Service struct {
	db             *sql.DB
	timeout        time.Duration
	newUUID        func() (uuid.UUID, error)
	policy         AccountPolicy
	tokenTTL       time.Duration
	newAccessToken func() (string, error)
}

// AccountPolicy is implemented by the owning BlessingSkin adapter, never a handler.
// Methods must be pure: authoritative rows are already read inside this service.
type AccountPolicy interface {
	Allowed(permission int, verified bool) bool
	VerifyPassword(password, storedHash string) bool
}

// NewAuth enables token operations only with an explicit, version-verified account policy.
func NewAuth(db *sql.DB, timeout, tokenTTL time.Duration, policy AccountPolicy) (*Service, error) {
	if policy == nil || tokenTTL < time.Microsecond {
		return nil, errors.New("account policy and positive token lifetime are required")
	}
	s, err := New(db, timeout)
	if err != nil {
		return nil, err
	}
	s.policy, s.tokenTTL = policy, tokenTTL
	return s, nil
}

func New(db *sql.DB, timeout time.Duration) (*Service, error) {
	if db == nil || timeout <= 0 {
		return nil, errors.New("database and positive transaction timeout are required")
	}
	return &Service{db: db, timeout: timeout, newUUID: uuid.NewRandom, newAccessToken: newOpaqueToken}, nil
}

func (s *Service) EnsureIdentity(ctx context.Context, pid uint64) (Identity, error) {
	if pid == 0 {
		return Identity{}, ErrNotFound
	}
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, watermark uint64) (Identity, error) {
		player, err := lockPlayer(ctx, tx, pid)
		if err != nil {
			return Identity{}, err
		}
		return s.ensureIdentity(ctx, tx, player, watermark)
	})
}

func transact[T any](ctx context.Context, s *Service, fn func(context.Context, *sql.Tx, uint64) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	var readOnly bool
	if err := tx.QueryRowContext(ctx, "SELECT @@global.read_only OR @@global.super_read_only").Scan(&readOnly); err != nil {
		return zero, err
	}
	if readOnly {
		return zero, ErrNotReady
	}
	var version int
	var phase string
	var watermark uint64
	err = tx.QueryRowContext(ctx, `SELECT schema_version, phase, player_high_watermark
		FROM ygg_go_state WHERE id=1 FOR SHARE`).Scan(&version, &phase, &watermark)
	if errors.Is(err, sql.ErrNoRows) {
		return zero, ErrNotReady
	}
	if err != nil {
		return zero, err
	}
	if version != 1 || phase != "active" {
		return zero, ErrNotReady
	}
	result, err := fn(ctx, tx, watermark)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		return zero, fmt.Errorf("%w: %w", ErrCommitUnknown, err)
	}
	return result, nil
}

// Ready verifies the runtime-visible active primary schema without repairing anything.
// Trigger definitions are verified by migrations.VerifyHooks before activation.
func (s *Service) Ready(ctx context.Context) error {
	_, err := transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (struct{}, error) {
		var tables int
		err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE()
			 AND ENGINE='InnoDB' AND TABLE_NAME IN ('users','players','ygg_go_state','ygg_go_identities',
			 'ygg_go_auth_subjects','ygg_go_tokens','ygg_go_join_sessions')`).Scan(&tables)
		if err != nil {
			return struct{}{}, err
		}
		if tables != 7 {
			return struct{}{}, ErrNotReady
		}
		return struct{}{}, nil
	})
	return err
}

func lockPlayer(ctx context.Context, tx *sql.Tx, pid uint64) (Identity, error) {
	var player Identity
	err := tx.QueryRowContext(ctx, "SELECT pid, uid, name FROM players WHERE pid=? FOR SHARE", pid).
		Scan(&player.PlayerID, &player.OwnerID, &player.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, ErrNotFound
	}
	return player, err
}

func readIdentity(ctx context.Context, tx *sql.Tx, player Identity) (Identity, error) {
	var raw []byte
	var state string
	err := tx.QueryRowContext(ctx, "SELECT identity_id, uuid, state FROM ygg_go_identities WHERE player_id=?", player.PlayerID).
		Scan(&player.ID, &raw, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, ErrNotFound
	}
	if err != nil {
		return Identity{}, err
	}
	if state != "active" {
		return Identity{}, ErrIdentityConflict
	}
	player.UUID, err = uuid.FromBytes(raw)
	if err != nil {
		return Identity{}, ErrIdentityConflict
	}
	return player, nil
}

func (s *Service) ensureIdentity(ctx context.Context, tx *sql.Tx, player Identity, watermark uint64) (Identity, error) {
	existing, err := readIdentity(ctx, tx, player)
	if !errors.Is(err, ErrNotFound) {
		return existing, err
	}
	if player.PlayerID <= watermark {
		return Identity{}, ErrIdentityConflict
	}
	for range 3 {
		candidate, err := s.newUUID()
		if err != nil {
			return Identity{}, err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO ygg_go_identities
			(player_id, uuid, state, created_at, updated_at)
			VALUES (?, ?, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, player.PlayerID, candidate[:])
		if err == nil {
			id, err := result.LastInsertId()
			if err != nil {
				return Identity{}, err
			}
			player.ID, player.UUID = uint64(id), candidate
			return player, nil
		}
		if !isDuplicate(err) {
			return Identity{}, err
		}
		// A concurrent allocator may have committed this pid while INSERT waited.
		existing, lookupErr := readIdentity(ctx, tx, player)
		if !errors.Is(lookupErr, ErrNotFound) {
			return existing, lookupErr
		}
	}
	return Identity{}, ErrIdentityConflict
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
