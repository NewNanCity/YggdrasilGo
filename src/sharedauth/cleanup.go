package sharedauth

import (
	"context"
	"database/sql"
)

const maxCleanupBatch = 10_000

// CleanupResult reports rows removed by one bounded cleanup transaction.
type CleanupResult struct {
	Tokens   int64
	Sessions int64
}

// CleanupExpired removes expired runtime state without requiring a cross-node lock.
// Concurrent instances may select the same range; InnoDB serializes the deletes and
// every caller remains bounded by batch rows per table.
func (s *Service) CleanupExpired(ctx context.Context, batch int) (CleanupResult, error) {
	if batch <= 0 || batch > maxCleanupBatch {
		return CleanupResult{}, ErrInvalid
	}
	return transact(ctx, s, func(ctx context.Context, tx *sql.Tx, _ uint64) (CleanupResult, error) {
		tokens, err := tx.ExecContext(ctx, `DELETE FROM ygg_go_tokens
			WHERE expires_at<=UTC_TIMESTAMP(6) ORDER BY expires_at, token_hash LIMIT ?`, batch)
		if err != nil {
			return CleanupResult{}, err
		}
		sessions, err := tx.ExecContext(ctx, `DELETE FROM ygg_go_join_sessions
			WHERE expires_at<=UTC_TIMESTAMP(6) ORDER BY expires_at, server_id LIMIT ?`, batch)
		if err != nil {
			return CleanupResult{}, err
		}
		sessionCount, err := sessions.RowsAffected()
		if err != nil {
			return CleanupResult{}, err
		}
		tokenCount, err := tokens.RowsAffected()
		if err != nil {
			return CleanupResult{}, err
		}
		return CleanupResult{Tokens: tokenCount, Sessions: sessionCount}, nil
	})
}
