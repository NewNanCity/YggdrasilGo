package migrationplan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSaveLoadAndNoOverwrite(t *testing.T) {
	pid, mappingID := uint64(1), uint64(2)
	plan := Plan{
		Version: Version, MigrationID: uuid.New().String(), CreatedAt: time.Now().UTC(),
		SourceSnapshotSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		PlayerHighWatermark:  1, Summary: Summary{Players: 1, Mappings: 1, Active: 1},
		Actions: []Action{{State: "active", PlayerID: &pid, UUID: "a826612caebb3b2380ae77d4712a373a", LegacyMappingID: &mappingID, Reason: "unique_exact_mapping"}},
	}
	path := filepath.Join(t.TempDir(), "private", "plan.json")
	if err := Save(path, plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := plan.Digest()
	got, _ := loaded.Digest()
	if got != want {
		t.Fatal("saved plan changed")
	}
	if err := Save(path, plan); err == nil {
		t.Fatal("existing plan was overwritten")
	}
	invalid := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(invalid, []byte(`{"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(invalid); err == nil {
		t.Fatal("unknown plan fields were accepted")
	}
}
