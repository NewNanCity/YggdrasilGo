// Package resolutionplan applies an explicitly reviewed, reversible resolution
// of retained blocked identities to their reserved UUID identities.
package resolutionplan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const Version = 1

// Approval identifies rows selected by an operator after reviewing the
// read-only evidence. Names and UUIDs are deliberately not accepted as row
// selectors; Build reads and records those values from the database.
type Approval struct {
	PlayerID           uint64   `json:"player_id"`
	BlockedIdentityID  uint64   `json:"blocked_identity_id"`
	ReservedIdentityID uint64   `json:"reserved_identity_id"`
	LegacyMappingIDs   []uint64 `json:"legacy_mapping_ids"`
}

// LoadApprovals reads the operator-owned selector file. It is intentionally a
// separate artifact from a plan so names and UUIDs are fetched from the live
// database during the dry run.
func LoadApprovals(path string) ([]Approval, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var approvals []Approval
	if err := decoder.Decode(&approvals); err != nil {
		return nil, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("approval file contains trailing JSON values")
	}
	if err := validateApprovals(approvals); err != nil {
		return nil, err
	}
	return approvals, nil
}

type Decision struct {
	PlayerID           uint64   `json:"player_id"`
	PlayerName         string   `json:"player_name"`
	BlockedIdentityID  uint64   `json:"blocked_identity_id"`
	ReservedIdentityID uint64   `json:"reserved_identity_id"`
	UUID               string   `json:"uuid"`
	LegacyMappingIDs   []uint64 `json:"legacy_mapping_ids"`
}

type Plan struct {
	Version      int        `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	SourceSHA256 string     `json:"source_sha256"`
	Decisions    []Decision `json:"decisions"`
}

func (p Plan) Validate() error {
	if p.Version != Version || p.CreatedAt.IsZero() || len(p.Decisions) == 0 || len(p.Decisions) > 1000 {
		return errors.New("invalid resolution plan metadata")
	}
	decoded, err := hex.DecodeString(p.SourceSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("invalid resolution source digest")
	}
	players := make(map[uint64]struct{}, len(p.Decisions))
	blocked := make(map[uint64]struct{}, len(p.Decisions))
	reserved := make(map[uint64]struct{}, len(p.Decisions))
	uuids := make(map[string]struct{}, len(p.Decisions))
	mappings := make(map[uint64]struct{})
	for i := range p.Decisions {
		d := p.Decisions[i]
		if d.PlayerID == 0 || d.PlayerName == "" || d.BlockedIdentityID == 0 || d.ReservedIdentityID == 0 || d.BlockedIdentityID == d.ReservedIdentityID {
			return fmt.Errorf("invalid resolution decision %d", i)
		}
		if _, err := parseCanonicalUUID(d.UUID); err != nil {
			return fmt.Errorf("invalid resolution UUID at decision %d", i)
		}
		if _, exists := uuids[d.UUID]; exists {
			return errors.New("duplicate UUID in resolution plan")
		}
		uuids[d.UUID] = struct{}{}
		if _, exists := players[d.PlayerID]; exists {
			return errors.New("duplicate player in resolution plan")
		}
		if _, exists := blocked[d.BlockedIdentityID]; exists {
			return errors.New("duplicate blocked identity in resolution plan")
		}
		if _, exists := reserved[d.ReservedIdentityID]; exists {
			return errors.New("duplicate reserved identity in resolution plan")
		}
		players[d.PlayerID] = struct{}{}
		blocked[d.BlockedIdentityID] = struct{}{}
		reserved[d.ReservedIdentityID] = struct{}{}
		if len(d.LegacyMappingIDs) == 0 {
			return fmt.Errorf("resolution decision %d has no legacy mappings", i)
		}
		seenMappings := make(map[uint64]struct{}, len(d.LegacyMappingIDs))
		for _, id := range d.LegacyMappingIDs {
			if id == 0 {
				return fmt.Errorf("zero legacy mapping in decision %d", i)
			}
			if _, exists := seenMappings[id]; exists {
				return fmt.Errorf("duplicate legacy mapping in decision %d", i)
			}
			if _, exists := mappings[id]; exists {
				return errors.New("legacy mapping appears in multiple resolution decisions")
			}
			seenMappings[id] = struct{}{}
			mappings[id] = struct{}{}
		}
	}
	actualSource, err := digestDecisions(p.Decisions)
	if err != nil {
		return err
	}
	if actualSource != p.SourceSHA256 {
		return errors.New("resolution source digest does not match decisions")
	}
	return nil
}

func parseCanonicalUUID(value string) (uuid.UUID, error) {
	if len(value) != 32 || strings.ToLower(value) != value {
		return uuid.Nil, errors.New("UUID must be lowercase 32-character hex")
	}
	parsed, err := uuid.Parse(value)
	if err != nil || fmt.Sprintf("%x", parsed[:]) != value {
		return uuid.Nil, errors.New("invalid UUID")
	}
	return parsed, nil
}

func (p Plan) Digest() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func Save(path string, p Plan) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if path == "" {
		return errors.New("resolution plan path is required")
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".resolution-plan-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return errors.New("refusing to overwrite an existing resolution plan")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func Load(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var p Plan
	if err := decoder.Decode(&p); err != nil {
		return Plan{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Plan{}, errors.New("resolution plan contains trailing JSON values")
	}
	if err := p.Validate(); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// Build reads the current rows for explicit approvals and returns a plan whose
// digest binds every observed name, UUID and mapping ID.
func Build(ctx context.Context, db *sql.DB, approvals []Approval, now time.Time) (Plan, error) {
	if db == nil || len(approvals) == 0 || len(approvals) > 1000 || now.IsZero() {
		return Plan{}, errors.New("database, approvals, and creation time are required")
	}
	copyApprovals := append([]Approval(nil), approvals...)
	sort.Slice(copyApprovals, func(i, j int) bool { return copyApprovals[i].PlayerID < copyApprovals[j].PlayerID })
	if err := validateApprovals(copyApprovals); err != nil {
		return Plan{}, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback()
	if err := requireV2State(ctx, tx, false, false); err != nil {
		return Plan{}, err
	}
	decisions := make([]Decision, 0, len(copyApprovals))
	for _, approval := range copyApprovals {
		decision, err := readDecision(ctx, tx, approval, false)
		if err != nil {
			return Plan{}, err
		}
		if err := ensureNoLiveReferences(ctx, tx, false, decision.BlockedIdentityID, decision.ReservedIdentityID); err != nil {
			return Plan{}, err
		}
		decisions = append(decisions, decision)
	}
	plan := Plan{Version: Version, CreatedAt: now.UTC(), Decisions: decisions}
	source, err := digestDecisions(decisions)
	if err != nil {
		return Plan{}, err
	}
	plan.SourceSHA256 = source
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	if err := tx.Commit(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func validateApprovals(approvals []Approval) error {
	players := make(map[uint64]struct{}, len(approvals))
	blocked := make(map[uint64]struct{}, len(approvals))
	reserved := make(map[uint64]struct{}, len(approvals))
	mappings := make(map[uint64]struct{})
	for i, approval := range approvals {
		if approval.PlayerID == 0 || approval.BlockedIdentityID == 0 || approval.ReservedIdentityID == 0 || approval.BlockedIdentityID == approval.ReservedIdentityID || len(approval.LegacyMappingIDs) == 0 {
			return fmt.Errorf("invalid resolution approval %d", i)
		}
		if _, exists := players[approval.PlayerID]; exists {
			return errors.New("duplicate approved player")
		}
		if _, exists := blocked[approval.BlockedIdentityID]; exists {
			return errors.New("duplicate approved blocked identity")
		}
		if _, exists := reserved[approval.ReservedIdentityID]; exists {
			return errors.New("duplicate approved reserved identity")
		}
		players[approval.PlayerID] = struct{}{}
		blocked[approval.BlockedIdentityID] = struct{}{}
		reserved[approval.ReservedIdentityID] = struct{}{}
		seen := make(map[uint64]struct{}, len(approval.LegacyMappingIDs))
		for _, id := range approval.LegacyMappingIDs {
			if id == 0 {
				return fmt.Errorf("zero approved legacy mapping at index %d", i)
			}
			if _, exists := seen[id]; exists {
				return fmt.Errorf("duplicate approved legacy mapping at index %d", i)
			}
			if _, exists := mappings[id]; exists {
				return errors.New("legacy mapping appears in multiple approvals")
			}
			seen[id] = struct{}{}
			mappings[id] = struct{}{}
		}
	}
	return nil
}

func digestDecisions(decisions []Decision) (string, error) {
	data, err := json.Marshal(decisions)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func requireV2State(ctx context.Context, db queryer, lock, requireStaged bool) error {
	query := "SELECT schema_version, phase FROM ygg_go_state WHERE id=1"
	if lock {
		query += " FOR UPDATE"
	}
	var version int
	var phase string
	if err := db.QueryRowContext(ctx, query).Scan(&version, &phase); err != nil {
		return fmt.Errorf("read resolution schema state: %w", err)
	}
	if version != 2 || (phase != "active" && phase != "staged") || (requireStaged && phase != "staged") {
		return fmt.Errorf("resolution requires schema v2 %s state, got v%d %s", map[bool]string{true: "staged", false: "active or staged"}[requireStaged], version, phase)
	}
	return nil
}

func readDecision(ctx context.Context, db queryer, approval Approval, lock bool) (Decision, error) {
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE"
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM players WHERE pid=?"+lockClause, approval.PlayerID).Scan(&name); err != nil {
		return Decision{}, fmt.Errorf("read approved player: %w", err)
	}
	playerRows, err := db.QueryContext(ctx, "SELECT pid FROM players WHERE name=? ORDER BY pid"+lockClause, name)
	if err != nil {
		return Decision{}, err
	}
	var equivalentPlayerIDs []uint64
	for playerRows.Next() {
		var id uint64
		if err := playerRows.Scan(&id); err != nil {
			playerRows.Close()
			return Decision{}, err
		}
		equivalentPlayerIDs = append(equivalentPlayerIDs, id)
	}
	if err := playerRows.Close(); err != nil {
		return Decision{}, err
	}
	if len(equivalentPlayerIDs) != 1 || equivalentPlayerIDs[0] != approval.PlayerID {
		return Decision{}, fmt.Errorf("approved player name has %d collation-equivalent rows", len(equivalentPlayerIDs))
	}

	var blockedPlayer sql.Null[uint64]
	var blockedUUID []byte
	var blockedState string
	var blockedMapping, blockedTarget sql.Null[uint64]
	if err := db.QueryRowContext(ctx, `SELECT player_id, uuid, state, legacy_mapping_id, resolved_into_identity_id
		FROM ygg_go_identities WHERE identity_id=?`+lockClause, approval.BlockedIdentityID).
		Scan(&blockedPlayer, &blockedUUID, &blockedState, &blockedMapping, &blockedTarget); err != nil {
		return Decision{}, fmt.Errorf("read approved blocked identity: %w", err)
	}
	if !blockedPlayer.Valid || blockedPlayer.V != approval.PlayerID || len(blockedUUID) != 0 || blockedState != "blocked" || blockedMapping.Valid || blockedTarget.Valid {
		return Decision{}, errors.New("approved blocked identity no longer matches the reviewed state")
	}

	var reservedPlayer sql.Null[uint64]
	var reservedUUID []byte
	var reservedState string
	var reservedMapping, reservedTarget sql.Null[uint64]
	if err := db.QueryRowContext(ctx, `SELECT player_id, uuid, state, legacy_mapping_id, resolved_into_identity_id
		FROM ygg_go_identities WHERE identity_id=?`+lockClause, approval.ReservedIdentityID).
		Scan(&reservedPlayer, &reservedUUID, &reservedState, &reservedMapping, &reservedTarget); err != nil {
		return Decision{}, fmt.Errorf("read approved reserved identity: %w", err)
	}
	if reservedPlayer.Valid || reservedState != "reserved" || len(reservedUUID) != 16 || reservedTarget.Valid {
		return Decision{}, errors.New("approved reserved identity no longer matches the reviewed state")
	}
	uuidValue, err := uuid.FromBytes(reservedUUID)
	if err != nil {
		return Decision{}, errors.New("approved reserved identity contains invalid UUID")
	}

	mappingIDs := append([]uint64(nil), approval.LegacyMappingIDs...)
	sort.Slice(mappingIDs, func(i, j int) bool { return mappingIDs[i] < mappingIDs[j] })
	rows, err := db.QueryContext(ctx, "SELECT id, uuid FROM uuid WHERE name=? ORDER BY id"+lockClause, name)
	if err != nil {
		return Decision{}, err
	}
	defer rows.Close()
	seen := make(map[uint64]struct{}, len(mappingIDs))
	for rows.Next() {
		var id uint64
		var mappingUUID string
		if err := rows.Scan(&id, &mappingUUID); err != nil {
			return Decision{}, err
		}
		if _, exists := seen[id]; exists {
			return Decision{}, errors.New("duplicate legacy mapping row")
		}
		seen[id] = struct{}{}
		parsed, err := parseLegacyUUID(mappingUUID)
		if err != nil || parsed != uuidValue {
			return Decision{}, errors.New("legacy mapping UUID does not match the reserved identity")
		}
	}
	if err := rows.Err(); err != nil {
		return Decision{}, err
	}
	if len(seen) != len(mappingIDs) {
		return Decision{}, errors.New("approved legacy mapping set no longer matches the player name")
	}
	for _, id := range mappingIDs {
		if _, exists := seen[id]; !exists {
			return Decision{}, errors.New("approved legacy mapping set no longer matches the player name")
		}
	}
	if reservedMapping.Valid {
		found := false
		for _, id := range mappingIDs {
			if reservedMapping.V == id {
				found = true
				break
			}
		}
		if !found {
			return Decision{}, errors.New("reserved identity mapping is absent from the approval")
		}
	}
	return Decision{PlayerID: approval.PlayerID, PlayerName: name, BlockedIdentityID: approval.BlockedIdentityID,
		ReservedIdentityID: approval.ReservedIdentityID, UUID: fmt.Sprintf("%x", uuidValue[:]), LegacyMappingIDs: mappingIDs}, nil
}

func parseLegacyUUID(value string) (uuid.UUID, error) {
	if len(value) != 32 || strings.ToLower(value) != value {
		return uuid.Nil, errors.New("legacy UUID must be lowercase 32-character hex")
	}
	parsed, err := uuid.Parse(value)
	if err != nil || fmt.Sprintf("%x", parsed[:]) != value {
		return uuid.Nil, errors.New("invalid legacy UUID")
	}
	return parsed, nil
}
