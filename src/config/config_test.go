package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateTrustedProxies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.TrustedProxies = []string{"10.0.0.0/8", "192.0.2.1"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if !cfg.IsTrustedProxy("10.2.3.4:1234") || !cfg.IsTrustedProxy("192.0.2.1:443") || cfg.IsTrustedProxy("192.0.2.2:443") {
		t.Fatal("trusted proxy matching did not honor the configured CIDR and address")
	}

	cfg = DefaultConfig()
	cfg.Server.TrustedProxies = []string{"not-a-cidr"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid trusted proxy was accepted")
	}
}

func TestValidateSecurityDefaultsAndRequestLimit(t *testing.T) {
	t.Run("parses_existing_size_field", func(t *testing.T) {
		cfg := DefaultConfig()
		if err := cfg.Validate(); err != nil || cfg.Security.MaxRequestBytes != 1_000_000 {
			t.Fatalf("bytes=%d err=%v", cfg.Security.MaxRequestBytes, err)
		}
	})
	t.Run("omitted_fields_keep_backward_compatible_defaults", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Security = SecurityConfig{}
		if err := cfg.Validate(); err != nil || cfg.Security.MaxRequestBytes != 1_000_000 ||
			cfg.Security.ReadTimeout == 0 || cfg.Security.WriteTimeout == 0 || cfg.Security.IdleTimeout == 0 {
			t.Fatalf("security=%+v err=%v", cfg.Security, err)
		}
	})
	for _, value := range []string{"0", "-1MB", "1GB", "invalid"} {
		t.Run(value, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Security.MaxRequestSize = value
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid request limit accepted")
			}
		})
	}
}

func TestGetAPILocation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 8080
	cfg.Server.BaseURL = "/api/yggdrasil"

	got := cfg.GetAPILocation("https", "api.example.com")
	want := "https://api.example.com/api/yggdrasil/"
	if got != want {
		t.Fatalf("GetAPILocation() = %q, want %q", got, want)
	}
}

func TestValidateAPILocationNormalized(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.APILocation = "https://api.example.com"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}

	if cfg.Server.APILocation != "https://api.example.com/" {
		t.Fatalf("APILocation not normalized: %q", cfg.Server.APILocation)
	}
}

func TestValidateAuthMode(t *testing.T) {
	t.Run("omitted mode preserves legacy", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Auth.Mode = ""
		if err := cfg.Validate(); err != nil || cfg.Auth.Mode != "legacy" || cfg.Storage.SharedAuth {
			t.Fatalf("legacy default: mode=%q shared=%v err=%v", cfg.Auth.Mode, cfg.Storage.SharedAuth, err)
		}
	})
	t.Run("shared mysql derives storage behavior", func(t *testing.T) {
		cfg := newSharedMySQLConfig(t)
		cfg.Auth.JWTSecret = ""
		if err := cfg.Validate(); err != nil || !cfg.Storage.SharedAuth {
			t.Fatalf("shared mode not accepted: shared=%v err=%v", cfg.Storage.SharedAuth, err)
		}
		if !strings.Contains(cfg.Storage.BlessingSkinOptions.EffectiveDatabaseDSN, "tls=ygg-rds-") {
			t.Fatal("validated DSN did not use the registered verified TLS profile")
		}
	})
	t.Run("legacy still requires jwt secret", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Auth.JWTSecret = "short"
		if err := cfg.Validate(); err == nil {
			t.Fatal("legacy mode accepted a short JWT secret")
		}
	})
	t.Run("derived storage flag follows mode", func(t *testing.T) {
		cfg := newSharedMySQLConfig(t)
		if err := cfg.Validate(); err != nil || !cfg.Storage.SharedAuth {
			t.Fatal("shared mode did not set the derived flag", err)
		}
		cfg.Auth.Mode = "legacy"
		if err := cfg.Validate(); err != nil || cfg.Storage.SharedAuth {
			t.Fatal("legacy mode retained the shared derived flag", err)
		}
	})
	for _, mutate := range []struct {
		name string
		fn   func(*Config)
	}{
		{"unknown_mode", func(cfg *Config) { cfg.Auth.Mode = "unknown" }},
		{"wrong_storage", func(cfg *Config) { cfg.Storage.Type = "file" }},
		{"zero_limit", func(cfg *Config) { cfg.Auth.TokensLimit = 0 }},
		{"zero_expiration", func(cfg *Config) { cfg.Auth.TokenExpiration = 0 }},
		{"zero_timeout", func(cfg *Config) { cfg.Security.ReadTimeout = 0 }},
		{"missing_parse_time", func(cfg *Config) {
			cfg.Storage.BlessingSkinOptions.DatabaseDSN = "user:pass@tcp(rds.example:3306)/blessing_skin?timeout=5s&readTimeout=10s&writeTimeout=10s"
		}},
		{"missing_connect_timeout", func(cfg *Config) {
			cfg.Storage.BlessingSkinOptions.DatabaseDSN = "user:pass@tcp(rds.example:3306)/blessing_skin?parseTime=true&readTimeout=10s&writeTimeout=10s"
		}},
		{"insecure_tls_mode", func(cfg *Config) {
			cfg.Storage.BlessingSkinOptions.DatabaseDSN += "&tls=skip-verify"
		}},
		{"server_name_mismatch", func(cfg *Config) {
			cfg.Storage.BlessingSkinOptions.DatabaseTLS.ServerName = "other.example"
		}},
		{"missing_ca", func(cfg *Config) { cfg.Storage.BlessingSkinOptions.DatabaseTLS.CAPath = "" }},
		{"invalid_dsn", func(cfg *Config) { cfg.Storage.BlessingSkinOptions.DatabaseDSN = "not a dsn" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			cfg := newSharedMySQLConfig(t)
			mutate.fn(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid shared authentication configuration accepted")
			}
		})
	}
}

func newSharedMySQLConfig(t *testing.T) *Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
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
	cfg := DefaultConfig()
	cfg.Auth.Mode = "shared_mysql"
	cfg.Storage.Type = "blessing_skin"
	cfg.Storage.BlessingSkinOptions.DatabaseDSN = "user:pass@tcp(rds.example:3306)/blessing_skin?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s"
	cfg.Storage.BlessingSkinOptions.DatabaseTLS = MySQLTLSConfig{CAPath: caPath, ServerName: "rds.example"}
	return cfg
}
