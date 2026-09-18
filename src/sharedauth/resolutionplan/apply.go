package resolutionplan

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"yggdrasil-api-go/src/sharedauth/migrations"
)

// Apply atomically converts every reviewed pair while the runtime gate is
// closed. Identity rows are retained: the blocked row becomes an audit link
// and the reserved row becomes the player's active UUID identity.
func Apply(ctx context.Context, db *sql.DB, plan Plan) error {
	if db == nil {
		return errors.New("database is required")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireV2State(ctx, tx, true, true); err != nil {
		return err
	}
	if err := verifyPlanRows(ctx, tx, plan, true, false); err != nil {
		return err
	}
	for _, decision := range plan.Decisions {
		if err := ensureNoLiveReferences(ctx, tx, true, decision.BlockedIdentityID, decision.ReservedIdentityID); err != nil {
			return err
		}
	}
	for _, decision := range plan.Decisions {
		result, err := tx.ExecContext(ctx, `UPDATE ygg_go_identities
			SET state='resolved', player_id=NULL, uuid=NULL, legacy_mapping_id=NULL,
				resolved_into_identity_id=?, updated_at=UTC_TIMESTAMP(6)
			WHERE identity_id=? AND state='blocked' AND player_id=?
				AND resolved_into_identity_id IS NULL`, decision.ReservedIdentityID, decision.BlockedIdentityID, decision.PlayerID)
		if err != nil {
			return fmt.Errorf("resolve blocked identity: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return errors.New("blocked identity changed concurrently")
		}
		result, err = tx.ExecContext(ctx, `UPDATE ygg_go_identities
			SET state='active', player_id=?, updated_at=UTC_TIMESTAMP(6)
			WHERE identity_id=? AND state='reserved' AND player_id IS NULL
				AND resolved_into_identity_id IS NULL`, decision.PlayerID, decision.ReservedIdentityID)
		if err != nil {
			return fmt.Errorf("activate reserved identity: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return errors.New("reserved identity changed concurrently")
		}
	}
	if err := verifyPlanRows(ctx, tx, plan, true, true); err != nil {
		return err
	}
	return tx.Commit()
}

// Verify checks that all planned links and active bindings are present. It
// accepts either a staged or active schema state so it can be used before and
// after the runtime gate is reopened.
func Verify(ctx context.Context, db *sql.DB, plan Plan) error {
	if db == nil {
		return errors.New("database is required")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireV2State(ctx, tx, false, false); err != nil {
		return err
	}
	if err := verifyPlanRows(ctx, tx, plan, false, true); err != nil {
		return err
	}
	return tx.Commit()
}

// Activate opens the runtime gate after the resolved identity shape and all
// schema/security hooks have been verified by the operator principal.
func Activate(ctx context.Context, db *sql.DB, plan Plan) error {
	if db == nil {
		return errors.New("database is required")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	if err := migrations.VerifyHooks(ctx, db); err != nil {
		return err
	}
	if err := migrations.VerifyResolutionSchema(ctx, db); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireV2State(ctx, tx, true, true); err != nil {
		return err
	}
	if err := verifyPlanRows(ctx, tx, plan, true, true); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE ygg_go_state SET phase='active', activated_at=UTC_TIMESTAMP(6)
		WHERE id=1 AND schema_version=2 AND phase='staged'`)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("resolution activation state changed concurrently")
	}
	return tx.Commit()
}

// Deactivate closes the runtime gate using the reviewed resolution plan. It
// intentionally does not require identity verification so an emergency gate
// close remains possible when runtime data is suspect.
func Deactivate(ctx context.Context, db *sql.DB, plan Plan) error {
	if db == nil {
		return errors.New("database is required")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireV2State(ctx, tx, true, false); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE ygg_go_state SET phase='staged', activated_at=NULL
		WHERE id=1 AND schema_version=2 AND phase='active'`)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("expected active resolution schema gate")
	}
	return tx.Commit()
}

// Rollback reverses an applied plan only while the runtime gate is closed and
// no token or join-session references point at either identity.
func Rollback(ctx context.Context, db *sql.DB, plan Plan) error {
	if db == nil {
		return errors.New("database is required")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireV2State(ctx, tx, true, true); err != nil {
		return err
	}
	if err := verifyPlanRows(ctx, tx, plan, true, true); err != nil {
		return err
	}
	for _, decision := range plan.Decisions {
		if err := ensureNoLiveReferences(ctx, tx, true, decision.BlockedIdentityID, decision.ReservedIdentityID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE ygg_go_identities
			SET state='reserved', player_id=NULL, updated_at=UTC_TIMESTAMP(6)
			WHERE identity_id=? AND state='active' AND player_id=?`, decision.ReservedIdentityID, decision.PlayerID)
		if err != nil {
			return fmt.Errorf("rollback active identity: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return errors.New("active identity changed concurrently")
		}
		result, err = tx.ExecContext(ctx, `UPDATE ygg_go_identities
			SET state='blocked', player_id=?, resolved_into_identity_id=NULL, updated_at=UTC_TIMESTAMP(6)
			WHERE identity_id=? AND state='resolved' AND player_id IS NULL
				AND uuid IS NULL AND legacy_mapping_id IS NULL
				AND resolved_into_identity_id=?`, decision.PlayerID, decision.BlockedIdentityID, decision.ReservedIdentityID)
		if err != nil {
			return fmt.Errorf("rollback resolved identity: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return errors.New("resolved identity changed concurrently")
		}
	}
	if err := verifyPlanRows(ctx, tx, plan, true, false); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureNoLiveReferences(ctx context.Context, tx *sql.Tx, lock bool, ids ...uint64) error {
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE"
	}
	for _, id := range ids {
		tokenRows, err := tx.QueryContext(ctx, "SELECT token_hash FROM ygg_go_tokens WHERE identity_id=? ORDER BY token_hash"+lockClause, id)
		if err != nil {
			return err
		}
		var tokenCount int
		for tokenRows.Next() {
			var tokenHash []byte
			if err := tokenRows.Scan(&tokenHash); err != nil {
				tokenRows.Close()
				return err
			}
			tokenCount++
		}
		if err := tokenRows.Close(); err != nil {
			return err
		}
		if tokenCount != 0 {
			return fmt.Errorf("reviewed identity has %d live token references", tokenCount)
		}
		sessionRows, err := tx.QueryContext(ctx, `SELECT s.server_id FROM ygg_go_join_sessions s
			JOIN ygg_go_tokens t ON t.token_hash=s.token_hash WHERE t.identity_id=? ORDER BY s.server_id`+lockClause, id)
		if err != nil {
			return err
		}
		var sessionCount int
		for sessionRows.Next() {
			var serverID []byte
			if err := sessionRows.Scan(&serverID); err != nil {
				sessionRows.Close()
				return err
			}
			sessionCount++
		}
		if err := sessionRows.Close(); err != nil {
			return err
		}
		if sessionCount != 0 {
			return fmt.Errorf("reviewed identity has %d live join-session references", sessionCount)
		}
	}
	return nil
}

func verifyPlanRows(ctx context.Context, tx *sql.Tx, plan Plan, lock, resolved bool) error {
	decisions := append([]Decision(nil), plan.Decisions...)
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].PlayerID < decisions[j].PlayerID })
	for _, decision := range decisions {
		if resolved {
			var state string
			var playerID sql.Null[uint64]
			var resolvedUUID []byte
			var resolvedMapping sql.Null[uint64]
			var target sql.Null[uint64]
			if err := tx.QueryRowContext(ctx, "SELECT state, player_id, uuid, legacy_mapping_id, resolved_into_identity_id FROM ygg_go_identities WHERE identity_id=?"+forUpdate(lock), decision.BlockedIdentityID).
				Scan(&state, &playerID, &resolvedUUID, &resolvedMapping, &target); err != nil {
				return err
			}
			if state != "resolved" || playerID.Valid || len(resolvedUUID) != 0 || resolvedMapping.Valid || !target.Valid || target.V != decision.ReservedIdentityID {
				return errors.New("resolved identity does not match the reviewed plan")
			}
			var activeState string
			var activePlayer uint64
			var raw []byte
			var activeTarget sql.Null[uint64]
			if err := tx.QueryRowContext(ctx, "SELECT state, player_id, uuid FROM ygg_go_identities WHERE identity_id=?"+forUpdate(lock), decision.ReservedIdentityID).
				Scan(&activeState, &activePlayer, &raw); err != nil {
				return err
			}
			if err := tx.QueryRowContext(ctx, "SELECT resolved_into_identity_id FROM ygg_go_identities WHERE identity_id=?"+forUpdate(lock), decision.ReservedIdentityID).Scan(&activeTarget); err != nil {
				return err
			}
			if activeState != "active" || activePlayer != decision.PlayerID || len(raw) != 16 || fmt.Sprintf("%x", raw) != decision.UUID || activeTarget.Valid {
				return errors.New("active identity does not match the reviewed plan")
			}
			continue
		}
		approval := Approval{PlayerID: decision.PlayerID, BlockedIdentityID: decision.BlockedIdentityID,
			ReservedIdentityID: decision.ReservedIdentityID, LegacyMappingIDs: decision.LegacyMappingIDs}
		actual, err := readDecision(ctx, tx, approval, lock)
		if err != nil {
			return err
		}
		if actual.PlayerName != decision.PlayerName || actual.UUID != decision.UUID || !sameIDs(actual.LegacyMappingIDs, decision.LegacyMappingIDs) {
			return errors.New("resolution source rows changed since the plan was created")
		}
	}
	return nil
}

func sameIDs(a, b []uint64) bool {
	left, right := append([]uint64(nil), a...), append([]uint64(nil), b...)
	sort.Slice(left, func(i, j int) bool { return left[i] < left[j] })
	sort.Slice(right, func(i, j int) bool { return right[i] < right[j] })
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func forUpdate(lock bool) string {
	if lock {
		return " FOR UPDATE"
	}
	return ""
}

// VerifyHooksForResolution keeps the operator gate consistent with the normal
// migration path without exposing trigger metadata to the runtime principal.
func VerifyHooksForResolution(ctx context.Context, db *sql.DB) error {
	return migrations.VerifyHooks(ctx, db)
}
