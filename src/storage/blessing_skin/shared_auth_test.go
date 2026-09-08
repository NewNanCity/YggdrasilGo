package blessing_skin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yggdrasil-api-go/internal/mysqltest"
	"yggdrasil-api-go/src/sharedauth"
)

func TestMySQLSharedAuthAdapter(t *testing.T) {
	server := mysqltest.Start(t)
	sqlDB := server.Database(t)
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY, uid BIGINT UNSIGNED NOT NULL,
			name VARCHAR(50) NOT NULL, tid_skin BIGINT NOT NULL, tid_cape BIGINT NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE textures (tid BIGINT UNSIGNED PRIMARY KEY, hash VARCHAR(64) NOT NULL,
			type VARCHAR(10) NOT NULL) ENGINE=InnoDB`,
		`INSERT INTO players VALUES (10,1,'Renamed',20,21)`,
		`INSERT INTO textures VALUES (20,'skin-hash','alex'),(21,'cape-hash','cape')`,
	} {
		if _, err := sqlDB.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	store := &Storage{
		db:            db,
		config:        &Config{PwdMethod: "BCRYPT", TextureBaseURLOverride: true},
		textureConfig: &TextureConfig{BaseURL: "https://textures.example.invalid/base"},
	}

	if sharedDB, err := store.SharedAuthDB(); err != nil || sharedDB != sqlDB {
		t.Fatalf("shared database pool differs: db=%p err=%v", sharedDB, err)
	}
	identity := sharedauth.Identity{
		PlayerID: 10, OwnerID: 1, UUID: uuid.MustParse("a826612c-aebb-3b23-80ae-77d4712a373a"), Name: "OldName",
	}
	profile, err := store.GetSharedProfile(t.Context(), identity)
	if err != nil || profile.ID != "a826612caebb3b2380ae77d4712a373a" || profile.Name != "Renamed" || len(profile.Properties) != 1 {
		t.Fatalf("shared profile=%+v err=%v", profile, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(profile.Properties[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	var texturePayload map[string]any
	if err := json.Unmarshal(decoded, &texturePayload); err != nil || !strings.Contains(string(decoded), "https://textures.example.invalid/base/skin-hash") ||
		!strings.Contains(string(decoded), "https://textures.example.invalid/base/cape-hash") {
		t.Fatalf("texture payload=%s err=%v", decoded, err)
	}
	if _, err := store.GetSharedProfile(t.Context(), sharedauth.Identity{PlayerID: 10, OwnerID: 2, UUID: identity.UUID}); err == nil {
		t.Fatal("profile ownership mismatch was accepted")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.GetSharedProfile(cancelled, identity); !errors.Is(err, context.Canceled) {
		t.Fatalf("profile query ignored cancellation: %v", err)
	}

	policy, err := store.SharedAuthPolicy(true)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.VerifyPassword("secret", string(hash)) || policy.VerifyPassword("wrong", string(hash)) ||
		policy.Allowed(-1, true) || policy.Allowed(0, false) || !policy.Allowed(0, true) {
		t.Fatal("shared account policy changed BlessingSkin semantics")
	}
	store.config.PwdMethod = "unknown"
	if _, err := store.SharedAuthPolicy(false); err == nil {
		t.Fatal("unknown password method silently fell back")
	}
}
