package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDSNAndWriteConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	data := migrationConfig(t, true)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadDSN(path, nil)
	if err != nil || cfg.DBName != "skin" || !cfg.ParseTime {
		t.Fatalf("load DSN: cfg=%+v err=%v", cfg, err)
	}
	if err := requireDatabaseConfirmation("skin", "wrong"); err == nil {
		t.Fatal("wrong database confirmation was accepted")
	}
	if err := requireDatabaseConfirmation("skin", "skin"); err != nil {
		t.Fatal(err)
	}
	data = migrationConfig(t, false)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDSN(path, nil); err == nil {
		t.Fatal("DSN without parseTime was accepted")
	}
	stdin := bytes.NewBuffer(migrationConfig(t, true))
	if cfg, err := loadDSN("-", stdin); err != nil || cfg.DBName != "skin" {
		t.Fatalf("load DSN from stdin: cfg=%+v err=%v", cfg, err)
	}
}

func migrationConfig(t *testing.T, parseTime bool) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	parseOption := ""
	if parseTime {
		parseOption = "&parseTime=true"
	}
	return []byte(fmt.Sprintf("storage:\n  type: blessing_skin\n  blessingskin_options:\n    database_dsn: 'user:pass@tcp(rds.example:3306)/skin?timeout=5s&readTimeout=10s&writeTimeout=10s%s'\n    database_tls:\n      ca_path: '%s'\n      server_name: 'rds.example'\n", parseOption, caPath))
}

func TestRunRejectsUnknownCommandBeforeDatabaseUse(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("missing command was accepted")
	}
	if err := run([]string{"unknown", "-config", "missing"}); err == nil {
		t.Fatal("unknown command reached configuration handling")
	}
}

func TestPrivatePlanPath(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "plan.json")
	if _, err := privatePlanPath(outside); err == nil {
		t.Fatal("plan outside private directory was accepted")
	}
	inside := filepath.Join(".local", "shared-auth", "plan.json")
	if got, err := privatePlanPath(inside); err != nil || !filepath.IsAbs(got) {
		t.Fatalf("private plan path=%q err=%v", got, err)
	}
}
