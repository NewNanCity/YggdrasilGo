package migrationplan

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type snapshotQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ReadSnapshot reads a consistent, read-only legacy identity snapshot.
func ReadSnapshot(ctx context.Context, db *sql.DB, maxRows int) (Snapshot, error) {
	if db == nil || maxRows <= 0 {
		return Snapshot{}, errors.New("database and positive row limit are required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := readSnapshot(ctx, tx, maxRows, false)
	if err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("finish source snapshot: %w", err)
	}
	return snapshot, nil
}

func readSnapshot(ctx context.Context, db snapshotQuerier, maxRows int, lock bool) (Snapshot, error) {
	var snapshot Snapshot
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&snapshot.Schema); err != nil {
		return Snapshot{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT TABLE_NAME, COLLATION_NAME, CHARACTER_MAXIMUM_LENGTH
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND COLUMN_NAME='name' AND TABLE_NAME IN ('players','uuid')
		ORDER BY TABLE_NAME`)
	if err != nil {
		return Snapshot{}, err
	}
	for rows.Next() {
		var table, collation string
		var characterSize uint64
		if err := rows.Scan(&table, &collation, &characterSize); err != nil {
			rows.Close()
			return Snapshot{}, err
		}
		switch table {
		case "players":
			snapshot.PlayersCollation = collation
		case "uuid":
			snapshot.MappingsCollation = collation
		}
		snapshot.NameCharacterSize = max(snapshot.NameCharacterSize, characterSize)
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, err
	}
	if snapshot.PlayersCollation == "" || snapshot.MappingsCollation == "" || snapshot.PlayersCollation != snapshot.MappingsCollation {
		return Snapshot{}, errors.New("players.name and uuid.name must exist with the same collation")
	}
	if err := db.QueryRowContext(ctx, `SELECT PAD_ATTRIBUTE FROM information_schema.COLLATIONS
		WHERE COLLATION_NAME=?`, snapshot.PlayersCollation).Scan(&snapshot.NamePadAttribute); err != nil {
		return Snapshot{}, err
	}
	if (snapshot.NamePadAttribute != "PAD SPACE" && snapshot.NamePadAttribute != "NO PAD") ||
		snapshot.NameCharacterSize == 0 || snapshot.NameCharacterSize > 1024 {
		return Snapshot{}, errors.New("unsupported name collation padding behavior")
	}
	counts := make(map[string]int, 2)
	for _, table := range []string{"players", "uuid"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return Snapshot{}, err
		}
		if count > maxRows {
			return Snapshot{}, fmt.Errorf("%s row count %d exceeds limit %d", table, count, maxRows)
		}
		counts[table] = count
	}

	lockClause := ""
	if lock {
		lockClause = " FOR SHARE"
	}
	weightExpression := "HEX(WEIGHT_STRING(name))"
	if snapshot.NamePadAttribute == "PAD SPACE" {
		weightExpression = fmt.Sprintf("HEX(WEIGHT_STRING(name AS CHAR(%d)))", snapshot.NameCharacterSize)
	}
	rows, err = db.QueryContext(ctx, `SELECT pid, uid, name, `+weightExpression+`
		FROM players ORDER BY pid`+lockClause)
	if err != nil {
		return Snapshot{}, err
	}
	for rows.Next() {
		var player Player
		if err := rows.Scan(&player.ID, &player.OwnerID, &player.Name, &player.NameKey); err != nil {
			rows.Close()
			return Snapshot{}, err
		}
		snapshot.Players = append(snapshot.Players, player)
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, err
	}
	rows, err = db.QueryContext(ctx, `SELECT id, name, `+weightExpression+`, uuid
		FROM uuid ORDER BY id`+lockClause)
	if err != nil {
		return Snapshot{}, err
	}
	for rows.Next() {
		var mapping Mapping
		if err := rows.Scan(&mapping.ID, &mapping.Name, &mapping.NameKey, &mapping.UUID); err != nil {
			rows.Close()
			return Snapshot{}, err
		}
		snapshot.Mappings = append(snapshot.Mappings, mapping)
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, err
	}
	if len(snapshot.Players) != counts["players"] || len(snapshot.Mappings) != counts["uuid"] {
		return Snapshot{}, errors.New("source table row counts changed during snapshot")
	}
	return snapshot, nil
}
