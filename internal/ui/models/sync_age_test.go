package models

import (
	"testing"
	"time"
)

func TestSyncAgeSuffix(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, " · just now"},
		{90 * time.Second, " · 1m ago"},
		{59 * time.Minute, " · 59m ago"},
		{3 * time.Hour, " · 3h ago"},
		{50 * time.Hour, " · 2d ago"},
	} {
		if got := syncAgeSuffix(now, now.Add(-c.ago)); got != c.want {
			t.Errorf("%v: got %q, want %q", c.ago, got, c.want)
		}
	}
	if syncAgeSuffix(now, time.Time{}) != "" {
		t.Error("never synced should render nothing")
	}
}
