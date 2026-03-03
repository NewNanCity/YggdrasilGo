package config

import "testing"

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
