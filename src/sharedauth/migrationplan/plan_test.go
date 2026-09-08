package migrationplan

import (
	"crypto/md5"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildClassifiesLegacyIdentitySnapshot(t *testing.T) {
	snapshot := Snapshot{
		Schema: "synthetic", PlayersCollation: "utf8mb4_unicode_ci", MappingsCollation: "utf8mb4_unicode_ci",
		NamePadAttribute: "PAD SPACE", NameCharacterSize: 50,
		Players: []Player{
			{ID: 10, OwnerID: 1, Name: "Alpha", NameKey: "a"},
			{ID: 11, OwnerID: 1, Name: "NoMap", NameKey: "n"},
			{ID: 12, OwnerID: 2, Name: "Case", NameKey: "c"},
			{ID: 13, OwnerID: 3, Name: "CASE", NameKey: "c"},
			{ID: 14, OwnerID: 3, Name: "Bad", NameKey: "b"},
			{ID: 15, OwnerID: 3, Name: "DupUUID", NameKey: "d"},
		},
		Mappings: []Mapping{
			{ID: 1, Name: "Alpha", NameKey: "a", UUID: "a826612caebb3b2380ae77d4712a373a"},
			{ID: 2, Name: "Orphan", NameKey: "o", UUID: "00000000000040008000000000000001"},
			{ID: 3, Name: "Case", NameKey: "c", UUID: "00000000000040008000000000000002"},
			{ID: 4, Name: "Bad", NameKey: "b", UUID: "not-a-uuid"},
			{ID: 5, Name: "DupUUID", NameKey: "d", UUID: "00000000000040008000000000000003"},
			{ID: 6, Name: "OtherOrphan", NameKey: "x", UUID: "00000000000040008000000000000003"},
		},
	}
	plan, err := Build(snapshot, uuid.MustParse("00000000-0000-4000-8000-000000000099"), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlayerHighWatermark != 15 || plan.Summary != (Summary{Players: 6, Mappings: 6, Active: 2, Blocked: 4, Reserved: 3, InvalidMappings: 1}) {
		t.Fatalf("unexpected summary: %+v watermark=%d", plan.Summary, plan.PlayerHighWatermark)
	}
	if plan.Actions[0].State != "active" || *plan.Actions[0].PlayerID != 10 || plan.Actions[0].UUID != "a826612caebb3b2380ae77d4712a373a" {
		t.Fatalf("unique mapping was not preserved: %+v", plan.Actions[0])
	}
	if plan.Actions[1].State != "active" || *plan.Actions[1].PlayerID != 11 || plan.Actions[1].UUID != expectedOfflinePlayerUUID("NoMap") ||
		plan.Actions[1].LegacyMappingID != nil || plan.Actions[1].Reason != "generated_offline_v3" {
		t.Fatalf("missing mapping did not receive deterministic v3 identity: %+v", plan.Actions[1])
	}
	reasons := make(map[uint64]string)
	for _, action := range plan.Actions {
		if action.State == "blocked" {
			reasons[*action.PlayerID] = action.Reason
		}
	}
	if reasons[12] != "collation_equivalent_name" || reasons[13] != "collation_equivalent_name" ||
		reasons[14] != "invalid_legacy_uuid" || reasons[15] != "duplicate_legacy_uuid" {
		t.Fatalf("blocked reasons: %+v", reasons)
	}
	firstDigest, _ := plan.Digest()
	second, err := Build(snapshot, uuid.MustParse(plan.MigrationID), plan.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, _ := second.Digest()
	if firstDigest != secondDigest {
		t.Fatal("same snapshot produced a nondeterministic plan")
	}
}

func TestBuildBlocksGeneratedV3CollisionWithLegacyUUID(t *testing.T) {
	snapshot := Snapshot{
		Schema: "synthetic", PlayersCollation: "utf8mb4_unicode_ci", MappingsCollation: "utf8mb4_unicode_ci",
		NamePadAttribute: "PAD SPACE", NameCharacterSize: 50,
		Players:  []Player{{ID: 1, OwnerID: 1, Name: "NoMap", NameKey: "n"}},
		Mappings: []Mapping{{ID: 1, Name: "Orphan", NameKey: "o", UUID: expectedOfflinePlayerUUID("NoMap")}},
	}
	plan, err := Build(snapshot, uuid.New(), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary.Active != 0 || plan.Summary.Blocked != 1 || plan.Summary.Reserved != 1 {
		t.Fatalf("unexpected collision summary: %+v", plan.Summary)
	}
	if plan.Actions[0].State != "blocked" || plan.Actions[0].Reason != "generated_v3_uuid_conflict" {
		t.Fatalf("generated collision was not blocked: %+v", plan.Actions[0])
	}
}

func TestPlanValidationBindsGeneratedIdentityReasonToMissingMapping(t *testing.T) {
	snapshot := Snapshot{
		Schema: "synthetic", PlayersCollation: "utf8mb4_unicode_ci", MappingsCollation: "utf8mb4_unicode_ci",
		NamePadAttribute: "PAD SPACE", NameCharacterSize: 50,
		Players: []Player{{ID: 1, OwnerID: 1, Name: "NoMap", NameKey: "n"}},
	}
	plan, err := Build(snapshot, uuid.New(), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	plan.Actions[0].Reason = "unique_exact_mapping"
	if err := plan.Validate(); err == nil {
		t.Fatal("generated identity with a mapped reason was accepted")
	}
}

func expectedOfflinePlayerUUID(name string) string {
	value := md5.Sum([]byte("OfflinePlayer:" + name))
	value[6] = value[6]&0x0f | 0x30
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x", value)
}

func TestBuildRejectsAmbiguousSnapshotMetadata(t *testing.T) {
	snapshot := Snapshot{Schema: "synthetic", PlayersCollation: "utf8mb4_unicode_ci", MappingsCollation: "utf8mb4_bin", NamePadAttribute: "PAD SPACE", NameCharacterSize: 50}
	if _, err := Build(snapshot, uuid.New(), time.Now()); err == nil {
		t.Fatal("mismatched collations were accepted")
	}
}
