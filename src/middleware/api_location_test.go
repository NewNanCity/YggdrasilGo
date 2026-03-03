package middleware

import "testing"

func TestResolveRequestScheme(t *testing.T) {
	tests := []struct {
		name           string
		forwardedProto string
		isTLS          bool
		want           string
	}{
		{name: "forwarded proto single", forwardedProto: "https", isTLS: false, want: "https"},
		{name: "forwarded proto chain", forwardedProto: "https,http", isTLS: false, want: "https"},
		{name: "forwarded proto chain with spaces", forwardedProto: " https , http ", isTLS: false, want: "https"},
		{name: "fallback tls", forwardedProto: "", isTLS: true, want: "https"},
		{name: "fallback http", forwardedProto: "", isTLS: false, want: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRequestScheme(tt.forwardedProto, tt.isTLS)
			if got != tt.want {
				t.Fatalf("ResolveRequestScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}
