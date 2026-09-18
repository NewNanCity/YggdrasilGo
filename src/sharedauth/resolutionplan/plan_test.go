package resolutionplan

import (
	"path/filepath"
	"testing"
	"time"
)

func validPlan() Plan {
	plan := Plan{
		Version:   Version,
		CreatedAt: time.Unix(1, 0).UTC(),
		Decisions: []Decision{{
			PlayerID: 10, PlayerName: "Alpha", BlockedIdentityID: 20, ReservedIdentityID: 21,
			UUID: "a826612caebb3b2380ae77d4712a373a", LegacyMappingIDs: []uint64{30},
		}},
	}
	plan.SourceSHA256, _ = digestDecisions(plan.Decisions)
	return plan
}

func TestPlanValidationRejectsIdentitySelectorAmbiguity(t *testing.T) {
	plan := validPlan()
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	plan.Decisions[0].UUID = "A826612CAEBB3B2380AE77D4712A373A"
	if err := plan.Validate(); err == nil {
		t.Fatal("uppercase UUID accepted")
	}
	plan = validPlan()
	plan.Decisions = append(plan.Decisions, plan.Decisions[0])
	if err := plan.Validate(); err == nil {
		t.Fatal("duplicate identity decision accepted")
	}
}

func TestPlanSaveLoadAndDigestAreDeterministic(t *testing.T) {
	plan := validPlan()
	digest, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := Save(path, plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	loadedDigest, err := loaded.Digest()
	if err != nil || loadedDigest != digest {
		t.Fatalf("digest changed after round trip: %s %v", loadedDigest, err)
	}
	if err := Save(path, plan); err == nil {
		t.Fatal("existing plan was overwritten")
	}
}
