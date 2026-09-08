package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"yggdrasil-api-go/internal/mysqltest"
	"yggdrasil-api-go/src/config"
	"yggdrasil-api-go/src/sharedauth"
	"yggdrasil-api-go/src/sharedauth/migrations"
	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/yggdrasil"
)

type handlerPolicy struct{}

func (handlerPolicy) Allowed(permission int, verified bool) bool { return permission != -1 }
func (handlerPolicy) VerifyPassword(password, hash string) bool  { return password == hash }

type handlerStorage struct {
	storage.Storage
	failProfile     bool
	profileProperty bool
	privateKey      string
	publicKey       string
}

func (s *handlerStorage) GetSharedProfile(_ context.Context, identity sharedauth.Identity) (*yggdrasil.Profile, error) {
	if s.failProfile {
		return nil, errors.New("synthetic profile failure")
	}
	profile := &yggdrasil.Profile{
		ID: sharedauth.FormatUUID(identity.UUID), Name: identity.Name,
		Properties: []yggdrasil.ProfileProperty{},
	}
	if s.profileProperty {
		profile.Properties = append(profile.Properties, yggdrasil.ProfileProperty{Name: "textures", Value: "synthetic"})
	}
	return profile, nil
}

func (s *handlerStorage) IsUploadSupported() bool { return false }

func (s *handlerStorage) GetStorageType() string { return "blessing_skin" }

func (s *handlerStorage) GetSignatureKeyPair() (string, string, error) {
	if s.privateKey == "" {
		return "", "", errors.New("synthetic signature failure")
	}
	return s.privateKey, s.publicKey, nil
}

func TestMySQLSharedHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := mysqltest.Start(t)
	service, db := handlerService(t, server)
	store := &handlerStorage{}
	cfg := config.DefaultConfig()
	cfg.Auth.TokensLimit = 1
	auth := NewSharedAuthHandler(store, nil, nil, service, cfg.Auth)
	session := NewSharedSessionHandler(store, nil, nil, cfg, service)

	t.Run("complete_auth_and_session_flow", func(t *testing.T) {
		authResponse := performJSON(t, auth.Authenticate, `{"username":"Alpha","password":"synthetic-hash","clientToken":"client","requestUser":true}`)
		if authResponse.Code != http.StatusOK {
			t.Fatalf("authenticate status=%d body=%s", authResponse.Code, authResponse.Body.String())
		}
		var login yggdrasil.AuthenticateResponse
		if err := json.Unmarshal(authResponse.Body.Bytes(), &login); err != nil {
			t.Fatal(err)
		}
		if login.AccessToken == "" || login.ClientToken != "client" || len(login.AvailableProfiles) != 1 || login.SelectedProfile == nil ||
			login.SelectedProfile.ID != "a826612caebb3b2380ae77d4712a373a" || login.User == nil || login.User.ID != "1" {
			t.Fatal("shared authenticate response lost canonical fields")
		}
		validate := performJSON(t, auth.Validate, `{"accessToken":"`+login.AccessToken+`","clientToken":"client"}`)
		if validate.Code != http.StatusNoContent {
			t.Fatalf("validate status=%d body=%s", validate.Code, validate.Body.String())
		}
		refresh := performJSON(t, auth.Refresh, `{"accessToken":"`+login.AccessToken+`","clientToken":"client","requestUser":true}`)
		if refresh.Code != http.StatusOK {
			t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
		}
		var refreshed yggdrasil.RefreshResponse
		if err := json.Unmarshal(refresh.Body.Bytes(), &refreshed); err != nil || refreshed.AccessToken == "" || refreshed.AccessToken == login.AccessToken || refreshed.SelectedProfile == nil {
			t.Fatal("refresh response invalid", err)
		}
		join := performJSONFrom(t, session.Join, `{"accessToken":"`+refreshed.AccessToken+`","selectedProfile":"a826612caebb3b2380ae77d4712a373a","serverId":"server"}`, "192.0.2.1:1234")
		if join.Code != http.StatusNoContent {
			t.Fatalf("join status=%d body=%s", join.Code, join.Body.String())
		}
		for range 2 {
			joined := performQuery(t, session.HasJoined, "/?username=Alpha&serverId=server&ip=192.0.2.1")
			if joined.Code != http.StatusOK {
				t.Fatalf("hasJoined status=%d body=%s", joined.Code, joined.Body.String())
			}
		}
		invalidate := performJSON(t, auth.Invalidate, `{"accessToken":"`+refreshed.AccessToken+`","clientToken":"client"}`)
		if invalidate.Code != http.StatusNoContent {
			t.Fatal("invalidate failed", invalidate.Body.String())
		}
		if joined := performQuery(t, session.HasJoined, "/?username=Alpha&serverId=server&ip=192.0.2.1"); joined.Code != http.StatusNoContent {
			t.Fatal("revoked token retained its join session")
		}
	})

	t.Run("error_contracts_and_profile_failure", func(t *testing.T) {
		wrong := performJSON(t, auth.Authenticate, `{"username":"Alpha","password":"wrong"}`)
		if wrong.Code != http.StatusForbidden || !bytes.Contains(wrong.Body.Bytes(), []byte("Invalid credentials")) {
			t.Fatal("credential error contract changed")
		}
		invalid := performJSON(t, auth.Validate, `{"accessToken":"missing"}`)
		if invalid.Code != http.StatusForbidden || !bytes.Contains(invalid.Body.Bytes(), []byte("Invalid token")) {
			t.Fatal("token error contract changed")
		}
		login := performJSON(t, auth.Authenticate, `{"username":"Alpha","password":"synthetic-hash","clientToken":"client"}`)
		var response yggdrasil.AuthenticateResponse
		if err := json.Unmarshal(login.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if got := performJSONFrom(t, session.Join, `{"accessToken":"`+response.AccessToken+`","selectedProfile":"a826612caebb3b2380ae77d4712a373a","serverId":"profile-failure"}`, "192.0.2.1:1234"); got.Code != http.StatusNoContent {
			t.Fatalf("join status=%d body=%s", got.Code, got.Body.String())
		}
		store.failProfile = true
		failed := performQuery(t, session.HasJoined, "/?username=Alpha&serverId=profile-failure&ip=192.0.2.1")
		store.failProfile = false
		if failed.Code != http.StatusInternalServerError {
			t.Fatalf("profile backend failure status=%d", failed.Code)
		}
		if retried := performQuery(t, session.HasJoined, "/?username=Alpha&serverId=profile-failure&ip=192.0.2.1"); retried.Code != http.StatusOK {
			t.Fatal("profile failure consumed the repeatable session")
		}
		mustHandlerExec(t, db, "UPDATE ygg_go_state SET phase='staged', activated_at=NULL")
		unready := performJSON(t, auth.Validate, `{"accessToken":"`+response.AccessToken+`"}`)
		if unready.Code != http.StatusServiceUnavailable {
			t.Fatalf("readiness error status=%d", unready.Code)
		}
	})
}

func TestMySQLSharedProfileHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := mysqltest.Start(t)
	service, db := handlerService(t, server)
	store := &handlerStorage{}
	profile := NewSharedProfileHandler(store, config.DefaultConfig(), service)

	byUUID := performQuery(t, profile.GetProfileByUUID, "/?unsigned=true", gin.Params{{Key: "uuid", Value: "a826612caebb3b2380ae77d4712a373a"}})
	if byUUID.Code != http.StatusOK || !bytes.Contains(byUUID.Body.Bytes(), []byte(`"name":"Alpha"`)) {
		t.Fatalf("profile by uuid status=%d body=%s", byUUID.Code, byUUID.Body.String())
	}
	mustHandlerExec(t, db, "UPDATE players SET name='Renamed' WHERE pid=10")
	byName := performQuery(t, profile.SearchSingleProfile, "/", gin.Params{{Key: "username", Value: "Renamed"}})
	if byName.Code != http.StatusOK || !bytes.Contains(byName.Body.Bytes(), []byte(`"id":"a826612caebb3b2380ae77d4712a373a"`)) {
		t.Fatalf("profile by renamed name status=%d body=%s", byName.Code, byName.Body.String())
	}
	batch := performJSON(t, profile.SearchMultipleProfiles, `["Renamed","missing","Renamed"]`)
	if batch.Code != http.StatusOK || bytes.Count(batch.Body.Bytes(), []byte(`"id"`)) != 1 {
		t.Fatalf("profile batch status=%d body=%s", batch.Code, batch.Body.String())
	}
	missing := performQuery(t, profile.SearchSingleProfile, "/", gin.Params{{Key: "username", Value: "missing"}})
	if missing.Code != http.StatusNoContent {
		t.Fatalf("missing profile status=%d body=%s", missing.Code, missing.Body.String())
	}
	store.failProfile = true
	failed := performQuery(t, profile.GetProfileByUUID, "/?unsigned=true", gin.Params{{Key: "uuid", Value: "a826612caebb3b2380ae77d4712a373a"}})
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("profile backend failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	store.failProfile = false
	store.profileProperty = true
	unsigned := performQuery(t, profile.GetProfileByUUID, "/?unsigned=true", gin.Params{{Key: "uuid", Value: "a826612caebb3b2380ae77d4712a373a"}})
	if unsigned.Code != http.StatusOK {
		t.Fatalf("unsigned profile status=%d body=%s", unsigned.Code, unsigned.Body.String())
	}
	signed := performQuery(t, profile.GetProfileByUUID, "/?unsigned=false", gin.Params{{Key: "uuid", Value: "a826612caebb3b2380ae77d4712a373a"}})
	if signed.Code != http.StatusInternalServerError {
		t.Fatalf("signature failure status=%d body=%s", signed.Code, signed.Body.String())
	}
	store.privateKey, store.publicKey = testHandlerKeyPair(t)
	signed = performQuery(t, profile.GetProfileByUUID, "/?unsigned=false", gin.Params{{Key: "uuid", Value: "a826612caebb3b2380ae77d4712a373a"}})
	if signed.Code != http.StatusOK || !bytes.Contains(signed.Body.Bytes(), []byte(`"signature":"`)) {
		t.Fatalf("signed profile status=%d body=%s", signed.Code, signed.Body.String())
	}
	texture := NewTextureHandler(store)
	upload := performWithParams(t, texture.UploadTexture, httptest.NewRequest(http.MethodPut, "/", nil),
		gin.Params{{Key: "uuid", Value: "a826612caebb3b2380ae77d4712a373a"}, {Key: "textureType", Value: "skin"}})
	if upload.Code != http.StatusNotImplemented {
		t.Fatalf("unsupported texture upload status=%d", upload.Code)
	}
	remove := performWithParams(t, texture.DeleteTexture, httptest.NewRequest(http.MethodDelete, "/", nil),
		gin.Params{{Key: "uuid", Value: "a826612caebb3b2380ae77d4712a373a"}, {Key: "textureType", Value: "skin"}})
	if remove.Code != http.StatusNotImplemented {
		t.Fatalf("unsupported texture deletion status=%d", remove.Code)
	}
}

func testHandlerKeyPair(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
}

func handlerService(t *testing.T, server *mysqltest.Server) (*sharedauth.Service, *sql.DB) {
	t.Helper()
	db := server.Database(t)
	mustHandlerExec(t, db, `CREATE TABLE users (uid BIGINT UNSIGNED PRIMARY KEY, email VARCHAR(100) NOT NULL,
		password VARCHAR(255) NOT NULL, permission INT NOT NULL, verified BOOLEAN NOT NULL) ENGINE=InnoDB`)
	mustHandlerExec(t, db, `CREATE TABLE players (pid BIGINT UNSIGNED PRIMARY KEY, uid BIGINT UNSIGNED NOT NULL,
		name VARCHAR(50) NOT NULL) ENGINE=InnoDB`)
	if err := migrations.Upgrade(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	migrationID := uuid.New()
	mustHandlerExec(t, db, "INSERT INTO ygg_go_state VALUES (1,1,'active',10,?,UTC_TIMESTAMP(6))", migrationID[:])
	mustHandlerExec(t, db, "INSERT INTO users VALUES (1,'user@example.invalid','synthetic-hash',0,1)")
	mustHandlerExec(t, db, "INSERT INTO players VALUES (10,1,'Alpha')")
	profile := uuid.MustParse("a826612c-aebb-3b23-80ae-77d4712a373a")
	mustHandlerExec(t, db, `INSERT INTO ygg_go_identities (player_id,uuid,state,created_at,updated_at)
		VALUES (10,?,'active',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, profile[:])
	service, err := sharedauth.NewAuth(db, 5*time.Second, 72*time.Hour, handlerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	return service, db
}

func mustHandlerExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func performJSON(t *testing.T, handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	return performJSONFrom(t, handler, body, "192.0.2.1:1234")
}

func performJSONFrom(t *testing.T, handler gin.HandlerFunc, body, remote string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remote
	return perform(t, handler, request)
}

func performQuery(t *testing.T, handler gin.HandlerFunc, target string, params ...gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	return performWithParams(t, handler, request, params...)
}

func perform(t *testing.T, handler gin.HandlerFunc, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	return performWithParams(t, handler, request)
}

func performWithParams(t *testing.T, handler gin.HandlerFunc, request *http.Request, params ...gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	if len(params) > 0 {
		context.Params = params[0]
	}
	handler(context)
	context.Writer.WriteHeaderNow()
	return recorder
}
