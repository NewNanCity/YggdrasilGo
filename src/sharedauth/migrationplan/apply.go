package migrationplan

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"yggdrasil-api-go/src/sharedauth/migrations"
)

func (p Plan) Validate() error {
	if p.Version != Version || p.CreatedAt.IsZero() || p.Summary.Players < 0 || p.Summary.Mappings < 0 ||
		p.Summary.Active < 0 || p.Summary.Blocked < 0 || p.Summary.Reserved < 0 || p.Summary.InvalidMappings < 0 {
		return errors.New("invalid migration plan metadata")
	}
	if _, err := uuid.Parse(p.MigrationID); err != nil {
		return errors.New("invalid migration id")
	}
	if digest, err := hex.DecodeString(p.SourceSnapshotSHA256); err != nil || len(digest) != sha256Size {
		return errors.New("invalid source snapshot digest")
	}
	if p.Summary.Players != p.Summary.Active+p.Summary.Blocked ||
		len(p.Actions) != p.Summary.Active+p.Summary.Blocked+p.Summary.Reserved {
		return errors.New("migration summary does not match actions")
	}
	players := make(map[uint64]struct{}, p.Summary.Players)
	values := make(map[uuid.UUID]struct{}, p.Summary.Active+p.Summary.Reserved)
	mappings := make(map[uint64]struct{}, p.Summary.Active+p.Summary.Reserved)
	var maxPlayer uint64
	for _, action := range p.Actions {
		if action.Reason == "" {
			return errors.New("migration action reason is required")
		}
		switch action.State {
		case "active":
			if action.PlayerID == nil || action.UUID == "" {
				return errors.New("invalid active migration action")
			}
			switch action.Reason {
			case "unique_exact_mapping":
				if action.LegacyMappingID == nil {
					return errors.New("mapped active migration action requires legacy mapping")
				}
			case "generated_offline_v3":
				if action.LegacyMappingID != nil {
					return errors.New("generated active migration action must not reference legacy mapping")
				}
			default:
				return errors.New("invalid active migration action reason")
			}
		case "blocked":
			if action.PlayerID == nil || action.LegacyMappingID != nil || action.UUID != "" {
				return errors.New("invalid blocked migration action")
			}
		case "reserved":
			if action.PlayerID != nil || action.LegacyMappingID == nil || action.UUID == "" {
				return errors.New("invalid reserved migration action")
			}
		default:
			return fmt.Errorf("unsupported migration state %q", action.State)
		}
		if action.PlayerID != nil {
			if *action.PlayerID == 0 {
				return errors.New("zero player id in migration action")
			}
			if _, exists := players[*action.PlayerID]; exists {
				return errors.New("duplicate player id in migration plan")
			}
			players[*action.PlayerID] = struct{}{}
			maxPlayer = max(maxPlayer, *action.PlayerID)
		}
		if action.UUID != "" {
			value, err := parseLegacyUUID(action.UUID)
			if err != nil || action.UUID != fmt.Sprintf("%x", value[:]) {
				return errors.New("migration UUID is not canonical lowercase hex")
			}
			if _, exists := values[value]; exists {
				return errors.New("duplicate UUID in migration plan")
			}
			values[value] = struct{}{}
		}
		if action.LegacyMappingID != nil {
			if *action.LegacyMappingID == 0 {
				return errors.New("zero mapping id in migration action")
			}
			if _, exists := mappings[*action.LegacyMappingID]; exists {
				return errors.New("duplicate mapping id in migration plan")
			}
			mappings[*action.LegacyMappingID] = struct{}{}
		}
	}
	if maxPlayer != p.PlayerHighWatermark {
		return errors.New("migration watermark does not match planned players")
	}
	return nil
}

const sha256Size = 32

// Apply writes the verified plan as staged data. It never activates runtime traffic.
func Apply(ctx context.Context, db *sql.DB, plan Plan, maxRows int) error {
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
	if err := requireSnapshot(ctx, tx, plan, maxRows); err != nil {
		return err
	}
	phase, exists, err := readState(ctx, tx, plan, true)
	if err != nil {
		return err
	}
	if exists {
		if phase != "staged" {
			return fmt.Errorf("migration is already %s, refusing staged apply", phase)
		}
		if err := verifyIdentities(ctx, tx, plan, true); err != nil {
			return err
		}
		return tx.Commit()
	}
	// Security triggers may have created revocation anchors after schema upgrade.
	// They are authoritative retained state and must never be cleared for bootstrap.
	for _, table := range []string{"ygg_go_identities", "ygg_go_tokens", "ygg_go_join_sessions"} {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("refusing initial apply: %s is not empty", table)
		}
	}
	for _, action := range plan.Actions {
		var rawUUID any
		if action.UUID != "" {
			value, _ := parseLegacyUUID(action.UUID)
			rawUUID = value[:]
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO ygg_go_identities
			(player_id, uuid, state, legacy_mapping_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`,
			action.PlayerID, rawUUID, action.State, action.LegacyMappingID); err != nil {
			return fmt.Errorf("insert %s identity: %w", action.State, err)
		}
	}
	migrationID, _ := uuid.Parse(plan.MigrationID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO ygg_go_state
		(id, schema_version, phase, player_high_watermark, migration_id, activated_at)
		VALUES (1, ?, 'staged', ?, ?, NULL)`, plan.Version, plan.PlayerHighWatermark, migrationID[:]); err != nil {
		return err
	}
	if err := verifyIdentities(ctx, tx, plan, true); err != nil {
		return err
	}
	return tx.Commit()
}

// Verify checks source stability, state identity, and every staged/active row.
func Verify(ctx context.Context, db *sql.DB, plan Plan, maxRows int) (string, error) {
	if db == nil {
		return "", errors.New("database is required")
	}
	if err := plan.Validate(); err != nil {
		return "", err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := requireSnapshot(ctx, tx, plan, maxRows); err != nil {
		return "", err
	}
	phase, exists, err := readState(ctx, tx, plan, false)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", errors.New("migration state is missing")
	}
	if err := verifyIdentities(ctx, tx, plan, false); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return phase, nil
}

// Activate opens the runtime gate only after rechecking the frozen source and staged rows.
func Activate(ctx context.Context, db *sql.DB, plan Plan, maxRows int) error {
	// Runtime intentionally lacks TRIGGER metadata privileges, so activation is
	// the last operator-owned point that can bind hook verification to the gate.
	if err := migrations.VerifyHooks(ctx, db); err != nil {
		return err
	}
	return setPhase(ctx, db, plan, maxRows, "staged", "active")
}

// Deactivate closes the runtime gate without depending on a now-evolving source snapshot.
// Reactivation after this emergency action requires a newly reviewed reconciliation plan.
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
	phase, exists, err := readState(ctx, tx, plan, true)
	if err != nil {
		return err
	}
	if !exists || phase != "active" {
		return fmt.Errorf("expected migration phase active, got %s", phase)
	}
	result, err := tx.ExecContext(ctx, "UPDATE ygg_go_state SET phase='staged', activated_at=NULL WHERE id=1 AND phase='active'")
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("migration phase changed concurrently")
	}
	return tx.Commit()
}

func setPhase(ctx context.Context, db *sql.DB, plan Plan, maxRows int, from, to string) error {
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
	if err := requireSnapshot(ctx, tx, plan, maxRows); err != nil {
		return err
	}
	phase, exists, err := readState(ctx, tx, plan, true)
	if err != nil {
		return err
	}
	if !exists || phase != from {
		return fmt.Errorf("expected migration phase %s, got %s", from, phase)
	}
	if err := verifyIdentities(ctx, tx, plan, true); err != nil {
		return err
	}
	activatedAt := "NULL"
	if to == "active" {
		activatedAt = "UTC_TIMESTAMP(6)"
	}
	result, err := tx.ExecContext(ctx, "UPDATE ygg_go_state SET phase=?, activated_at="+activatedAt+" WHERE id=1 AND phase=?", to, from)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("migration phase changed concurrently")
	}
	return tx.Commit()
}

func requireSnapshot(ctx context.Context, tx *sql.Tx, plan Plan, maxRows int) error {
	snapshot, err := readSnapshot(ctx, tx, maxRows, true)
	if err != nil {
		return err
	}
	digest, err := SnapshotDigest(snapshot)
	if err != nil {
		return err
	}
	if digest != plan.SourceSnapshotSHA256 {
		return errors.New("source snapshot changed; regenerate and review the migration plan")
	}
	migrationID, err := uuid.Parse(plan.MigrationID)
	if err != nil {
		return errors.New("invalid migration id")
	}
	expected, err := Build(snapshot, migrationID, plan.CreatedAt)
	if err != nil {
		return err
	}
	expectedDigest, err := expected.Digest()
	if err != nil {
		return err
	}
	actualDigest, err := plan.Digest()
	if err != nil {
		return err
	}
	if actualDigest != expectedDigest {
		return errors.New("migration actions do not match the source snapshot")
	}
	return nil
}

func readState(ctx context.Context, tx *sql.Tx, plan Plan, lock bool) (string, bool, error) {
	var version int
	var phase string
	var watermark uint64
	var raw []byte
	query := `SELECT schema_version, phase, player_high_watermark, migration_id
		FROM ygg_go_state WHERE id=1`
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRowContext(ctx, query).Scan(&version, &phase, &watermark, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	migrationID, err := uuid.FromBytes(raw)
	if err != nil || version != plan.Version || watermark != plan.PlayerHighWatermark || migrationID.String() != plan.MigrationID {
		return "", false, errors.New("database migration state does not match the reviewed plan")
	}
	return phase, true, nil
}

func verifyIdentities(ctx context.Context, tx *sql.Tx, plan Plan, lock bool) error {
	query := `SELECT player_id, uuid, state, legacy_mapping_id
		FROM ygg_go_identities ORDER BY identity_id`
	if lock {
		query += " FOR SHARE"
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	actual := make([]Action, 0, len(plan.Actions))
	for rows.Next() {
		var playerID, mappingID sql.Null[uint64]
		var raw []byte
		var state string
		if err := rows.Scan(&playerID, &raw, &state, &mappingID); err != nil {
			return err
		}
		action := Action{State: state}
		if playerID.Valid {
			value := playerID.V
			action.PlayerID = &value
		}
		if len(raw) != 0 {
			value, err := uuid.FromBytes(raw)
			if err != nil {
				return errors.New("database contains an invalid identity UUID")
			}
			action.UUID = fmt.Sprintf("%x", value[:])
		}
		if mappingID.Valid {
			value := mappingID.V
			action.LegacyMappingID = &value
		}
		actual = append(actual, action)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	expected := append([]Action(nil), plan.Actions...)
	for i := range expected {
		expected[i].Reason = ""
	}
	sortActions(actual)
	sortActions(expected)
	if len(actual) != len(expected) {
		return fmt.Errorf("identity row count %d does not match plan %d", len(actual), len(expected))
	}
	for i := range expected {
		if !sameAction(actual[i], expected[i]) {
			return fmt.Errorf("identity row %d does not match the reviewed plan", i)
		}
	}
	return nil
}

func sameAction(a, b Action) bool {
	return a.State == b.State && a.UUID == b.UUID && equalOptional(a.PlayerID, b.PlayerID) && equalOptional(a.LegacyMappingID, b.LegacyMappingID)
}

func equalOptional(a, b *uint64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
