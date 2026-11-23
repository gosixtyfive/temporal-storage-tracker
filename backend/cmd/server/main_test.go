package main

import (
	"testing"
	"time"
)

func TestParseStorageDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "", want: 7 * 24 * time.Hour},
		{in: "168h", want: 168 * time.Hour},
		{in: "7m", want: 7 * time.Minute},
		{in: "bogus", wantErr: true},
		{in: "0s", wantErr: true},
		{in: "-1m", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseStorageDuration(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseStorageDuration(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("parseStorageDuration(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
}
