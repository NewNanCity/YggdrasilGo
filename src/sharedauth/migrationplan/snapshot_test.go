package migrationplan

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"yggdrasil-api-go/internal/mysqltest"
)

func TestMySQLReadSnapshot(t *testing.T) {
	server := mysqltest.Start(t)
	db := server.Database(t)
	for _, statement := range []string{
		`CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY, uid BIGINT UNSIGNED NOT NULL,
			name VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE uuid (id BIGINT UNSIGNED PRIMARY KEY, name VARCHAR(50) CHARACTER SET utf8mb4
			COLLATE utf8mb4_unicode_ci NOT NULL, uuid VARCHAR(36) NOT NULL) ENGINE=InnoDB`,
		`INSERT INTO players VALUES (10,1,'Alpha'),(11,1,'ALPHA')`,
		`INSERT INTO uuid VALUES (1,'Alpha','a826612caebb3b2380ae77d4712a373a')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := ReadSnapshot(t.Context(), db, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Players) != 2 || len(snapshot.Mappings) != 1 ||
		snapshot.Players[0].NameKey != snapshot.Players[1].NameKey {
		t.Fatalf("snapshot did not preserve collation equivalence: %+v", snapshot)
	}
	if _, err := ReadSnapshot(t.Context(), db, 1); err == nil {
		t.Fatal("row limit was ignored")
	}
}

func TestMySQLSnapshotTreatsPadSpaceNamesAsEquivalent(t *testing.T) {
	server := mysqltest.Start(t)
	db := server.Database(t)
	for _, statement := range []string{
		`CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY, uid BIGINT UNSIGNED NOT NULL,
			name VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE uuid (id BIGINT UNSIGNED PRIMARY KEY, name VARCHAR(50) CHARACTER SET utf8mb4
			COLLATE utf8mb4_unicode_ci NOT NULL, uuid VARCHAR(36) NOT NULL) ENGINE=InnoDB`,
		`INSERT INTO players VALUES (10,1,'Alpha')`,
		`INSERT INTO uuid VALUES (1,'Alpha','a826612caebb3b2380ae77d4712a373a'),
			(2,'Alpha ','00000000000040008000000000000001')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	var equal bool
	if err := db.QueryRow("SELECT 'Alpha' COLLATE utf8mb4_unicode_ci = 'Alpha '").Scan(&equal); err != nil || !equal {
		t.Fatalf("fixture is not PAD SPACE equivalent: equal=%v err=%v", equal, err)
	}
	snapshot, err := ReadSnapshot(t.Context(), db, 10)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Build(snapshot, uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary.Active != 0 || plan.Summary.Blocked != 1 || plan.Actions[0].Reason != "collation_equivalent_name" {
		t.Fatalf("PAD SPACE alias was not isolated: %+v", plan.Actions)
	}
}

func TestMySQLSnapshotMatchesNBSPCollationEquality(t *testing.T) {
	server := mysqltest.Start(t)
	db := server.Database(t)
	for _, statement := range []string{
		`CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY, uid BIGINT UNSIGNED NOT NULL,
			name VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE uuid (id BIGINT UNSIGNED PRIMARY KEY, name VARCHAR(50) CHARACTER SET utf8mb4
			COLLATE utf8mb4_unicode_ci NOT NULL, uuid VARCHAR(36) NOT NULL) ENGINE=InnoDB`,
		`INSERT INTO players VALUES (10,1,'Alpha')`,
		`INSERT INTO uuid VALUES (1,'Alpha','a826612caebb3b2380ae77d4712a373a'),
			(2,CONCAT('Alpha', CONVERT(0xC2A0 USING utf8mb4)),'00000000000040008000000000000001')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	var equal bool
	if err := db.QueryRow(`SELECT 'Alpha' COLLATE utf8mb4_unicode_ci =
		CONCAT('Alpha', CONVERT(0xC2A0 USING utf8mb4))`).Scan(&equal); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadSnapshot(t.Context(), db, 10)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Build(snapshot, uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	blocked := plan.Summary.Blocked == 1 && plan.Summary.Active == 0
	if blocked != equal {
		t.Fatalf("classification differs from SQL equality: equal=%v actions=%+v", equal, plan.Actions)
	}
}
