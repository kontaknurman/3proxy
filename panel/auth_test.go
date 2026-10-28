package main

import (
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		origin, fetchSite string
		ok                bool
	}{
		{"", "", true},
		{"http://203.0.113.10:8088", "", true},
		{"http://203.0.113.10:8088", "same-origin", true},
		{"null", "", false},
		{"http://evil.example", "", false},
		{"http://203.0.113.10:9000", "", false},
		{"", "cross-site", false},
		{"", "same-site", false},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("POST", "http://203.0.113.10:8088/login", nil)
		if tt.origin != "" {
			r.Header.Set("Origin", tt.origin)
		}
		if tt.fetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", tt.fetchSite)
		}
		if got := sameOrigin(r); got != tt.ok {
			t.Errorf("Origin %q, Sec-Fetch-Site %q: sameOrigin() = %v, want %v", tt.origin, tt.fetchSite, got, tt.ok)
		}
	}
}
