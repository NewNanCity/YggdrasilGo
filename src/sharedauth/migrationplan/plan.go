// Package migrationplan classifies legacy name mappings into an auditable pid identity plan.
package migrationplan

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"yggdrasil-api-go/src/sharedauth"
)

const Version = 1

type Player struct {
	ID      uint64 `json:"id"`
	OwnerID uint64 `json:"owner_id"`
	Name    string `json:"name"`
	NameKey string `json:"name_key"`
}

type Mapping struct {
	ID      uint64 `json:"id"`
	Name    string `json:"name"`
	NameKey string `json:"name_key"`
	UUID    string `json:"uuid"`
}

type Snapshot struct {
	Schema            string    `json:"schema"`
	PlayersCollation  string    `json:"players_collation"`
	MappingsCollation string    `json:"mappings_collation"`
	NamePadAttribute  string    `json:"name_pad_attribute"`
	NameCharacterSize uint64    `json:"name_character_size"`
	Players           []Player  `json:"players"`
	Mappings          []Mapping `json:"mappings"`
}

type Action struct {
	State           string  `json:"state"`
	PlayerID        *uint64 `json:"player_id,omitempty"`
	UUID            string  `json:"uuid,omitempty"`
	LegacyMappingID *uint64 `json:"legacy_mapping_id,omitempty"`
	Reason          string  `json:"reason"`
}

type Summary struct {
	Players         int `json:"players"`
	Mappings        int `json:"mappings"`
	Active          int `json:"active"`
	Blocked         int `json:"blocked"`
	Reserved        int `json:"reserved"`
	InvalidMappings int `json:"invalid_mappings"`
}

type Plan struct {
	Version              int       `json:"version"`
	MigrationID          string    `json:"migration_id"`
	CreatedAt            time.Time `json:"created_at"`
	SourceSnapshotSHA256 string    `json:"source_snapshot_sha256"`
	PlayerHighWatermark  uint64    `json:"player_high_watermark"`
	Summary              Summary   `json:"summary"`
	Actions              []Action  `json:"actions"`
}

func Build(snapshot Snapshot, migrationID uuid.UUID, createdAt time.Time) (Plan, error) {
	if migrationID == uuid.Nil || snapshot.Schema == "" || snapshot.PlayersCollation == "" ||
		snapshot.PlayersCollation != snapshot.MappingsCollation ||
		(snapshot.NamePadAttribute != "PAD SPACE" && snapshot.NamePadAttribute != "NO PAD") ||
		snapshot.NameCharacterSize == 0 || snapshot.NameCharacterSize > 1024 {
		return Plan{}, errors.New("migration id, schema, and matching name collations are required")
	}
	if err := validateSnapshot(snapshot); err != nil {
		return Plan{}, err
	}
	snapshotHash, err := SnapshotDigest(snapshot)
	if err != nil {
		return Plan{}, err
	}

	playersByName := make(map[string][]Player)
	mappingsByName := make(map[string][]Mapping)
	namesByKey := make(map[string]map[string]struct{})
	parsed := make(map[uint64]uuid.UUID)
	uuidMappings := make(map[uuid.UUID][]Mapping)
	invalid := make(map[uint64]struct{})
	var watermark uint64
	for _, player := range snapshot.Players {
		playersByName[player.Name] = append(playersByName[player.Name], player)
		addName(namesByKey, player.NameKey, player.Name)
		watermark = max(watermark, player.ID)
	}
	for _, mapping := range snapshot.Mappings {
		mappingsByName[mapping.Name] = append(mappingsByName[mapping.Name], mapping)
		addName(namesByKey, mapping.NameKey, mapping.Name)
		value, err := parseLegacyUUID(mapping.UUID)
		if err != nil {
			invalid[mapping.ID] = struct{}{}
			continue
		}
		parsed[mapping.ID] = value
		uuidMappings[value] = append(uuidMappings[value], mapping)
	}

	actions := make([]Action, 0, len(snapshot.Players)+len(uuidMappings))
	activeUUIDs := make(map[uuid.UUID]struct{})
	generatedUUIDs := make(map[uuid.UUID]int)
	for _, player := range snapshot.Players {
		if playerReason(player, playersByName[player.Name], mappingsByName[player.Name], namesByKey, invalid, parsed, uuidMappings) == "missing_legacy_mapping" {
			generatedUUIDs[offlinePlayerUUID(player.Name)]++
		}
	}
	for _, player := range snapshot.Players {
		mappingRows := mappingsByName[player.Name]
		reason := playerReason(player, playersByName[player.Name], mappingRows, namesByKey, invalid, parsed, uuidMappings)
		if reason == "missing_legacy_mapping" {
			value := offlinePlayerUUID(player.Name)
			pid := player.ID
			if generatedUUIDs[value] != 1 || len(uuidMappings[value]) != 0 {
				actions = append(actions, Action{State: "blocked", PlayerID: &pid, Reason: "generated_v3_uuid_conflict"})
				continue
			}
			actions = append(actions, Action{State: "active", PlayerID: &pid, UUID: sharedauth.FormatUUID(value), Reason: "generated_offline_v3"})
			activeUUIDs[value] = struct{}{}
			continue
		}
		if reason != "active" {
			pid := player.ID
			actions = append(actions, Action{State: "blocked", PlayerID: &pid, Reason: reason})
			continue
		}
		mapping := mappingRows[0]
		value := parsed[mapping.ID]
		pid, mappingID := player.ID, mapping.ID
		actions = append(actions, Action{State: "active", PlayerID: &pid, UUID: sharedauth.FormatUUID(value), LegacyMappingID: &mappingID, Reason: "unique_exact_mapping"})
		activeUUIDs[value] = struct{}{}
	}

	values := make([]uuid.UUID, 0, len(uuidMappings))
	for value := range uuidMappings {
		if _, active := activeUUIDs[value]; !active {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].String() < values[j].String() })
	for _, value := range values {
		rows := uuidMappings[value]
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
		mappingID := rows[0].ID
		actions = append(actions, Action{State: "reserved", UUID: sharedauth.FormatUUID(value), LegacyMappingID: &mappingID, Reason: reservationReason(rows, playersByName)})
	}
	sortActions(actions)

	summary := Summary{Players: len(snapshot.Players), Mappings: len(snapshot.Mappings), InvalidMappings: len(invalid)}
	for _, action := range actions {
		switch action.State {
		case "active":
			summary.Active++
		case "blocked":
			summary.Blocked++
		case "reserved":
			summary.Reserved++
		}
	}
	return Plan{
		Version: Version, MigrationID: migrationID.String(), CreatedAt: createdAt.UTC(),
		SourceSnapshotSHA256: snapshotHash, PlayerHighWatermark: watermark,
		Summary: summary, Actions: actions,
	}, nil
}

func offlinePlayerUUID(name string) uuid.UUID {
	value := md5.Sum([]byte("OfflinePlayer:" + name))
	value[6] = value[6]&0x0f | 0x30
	value[8] = value[8]&0x3f | 0x80
	return uuid.UUID(value)
}

func SnapshotDigest(snapshot Snapshot) (string, error) {
	copy := snapshot
	copy.Players = append([]Player(nil), snapshot.Players...)
	copy.Mappings = append([]Mapping(nil), snapshot.Mappings...)
	sort.Slice(copy.Players, func(i, j int) bool { return copy.Players[i].ID < copy.Players[j].ID })
	sort.Slice(copy.Mappings, func(i, j int) bool { return copy.Mappings[i].ID < copy.Mappings[j].ID })
	data, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func (p Plan) Digest() (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateSnapshot(snapshot Snapshot) error {
	playerIDs := make(map[uint64]struct{}, len(snapshot.Players))
	for _, player := range snapshot.Players {
		if player.ID == 0 || player.OwnerID == 0 || player.Name == "" || player.NameKey == "" {
			return errors.New("invalid player row in source snapshot")
		}
		if _, exists := playerIDs[player.ID]; exists {
			return fmt.Errorf("duplicate player id %d", player.ID)
		}
		playerIDs[player.ID] = struct{}{}
	}
	mappingIDs := make(map[uint64]struct{}, len(snapshot.Mappings))
	for _, mapping := range snapshot.Mappings {
		if mapping.ID == 0 || mapping.Name == "" || mapping.NameKey == "" || mapping.UUID == "" {
			return errors.New("invalid mapping row in source snapshot")
		}
		if _, exists := mappingIDs[mapping.ID]; exists {
			return fmt.Errorf("duplicate mapping id %d", mapping.ID)
		}
		mappingIDs[mapping.ID] = struct{}{}
	}
	return nil
}

func addName(groups map[string]map[string]struct{}, key, name string) {
	if groups[key] == nil {
		groups[key] = make(map[string]struct{})
	}
	groups[key][name] = struct{}{}
}

func parseLegacyUUID(raw string) (uuid.UUID, error) {
	if len(raw) != 32 && len(raw) != 36 {
		return uuid.Nil, errors.New("unsupported UUID representation")
	}
	value, err := uuid.Parse(raw)
	if err != nil || value.Variant() != uuid.RFC4122 {
		return uuid.Nil, errors.New("invalid RFC 4122 UUID")
	}
	return value, nil
}

func playerReason(player Player, sameName []Player, mappings []Mapping, namesByKey map[string]map[string]struct{}, invalid map[uint64]struct{}, parsed map[uint64]uuid.UUID, byUUID map[uuid.UUID][]Mapping) string {
	if len(sameName) != 1 {
		return "duplicate_current_name"
	}
	if len(namesByKey[player.NameKey]) != 1 {
		return "collation_equivalent_name"
	}
	if len(mappings) == 0 {
		return "missing_legacy_mapping"
	}
	if len(mappings) != 1 {
		return "duplicate_legacy_name"
	}
	mapping := mappings[0]
	if _, bad := invalid[mapping.ID]; bad {
		return "invalid_legacy_uuid"
	}
	if len(byUUID[parsed[mapping.ID]]) != 1 {
		return "duplicate_legacy_uuid"
	}
	return "active"
}

func reservationReason(rows []Mapping, playersByName map[string][]Player) string {
	if len(rows) != 1 {
		return "conflicting_legacy_uuid"
	}
	if len(playersByName[rows[0].Name]) == 0 {
		return "orphan_legacy_mapping"
	}
	return "unassigned_conflict_uuid"
}

func sortActions(actions []Action) {
	sort.Slice(actions, func(i, j int) bool {
		order := map[string]int{"active": 0, "blocked": 1, "reserved": 2}
		if order[actions[i].State] != order[actions[j].State] {
			return order[actions[i].State] < order[actions[j].State]
		}
		if actions[i].PlayerID != nil && actions[j].PlayerID != nil {
			return *actions[i].PlayerID < *actions[j].PlayerID
		}
		return actions[i].UUID < actions[j].UUID
	})
}
