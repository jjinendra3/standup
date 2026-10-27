package window

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestFor(t *testing.T) {
	tests := []struct {
		name         string
		mode         Mode
		now          string
		since, until string
	}{
		{"today", Today, "2026-10-21 15:30", "2026-10-21 00:00", "2026-10-22 00:00"},
		{"yesterday midweek", Yesterday, "2026-10-21 09:00", "2026-10-20 00:00", "2026-10-21 00:00"},
		{"yesterday on monday covers the weekend", Yesterday, "2026-10-26 09:00", "2026-10-23 00:00", "2026-10-26 00:00"},
		{"yesterday on sunday starts friday", Yesterday, "2026-10-25 09:00", "2026-10-23 00:00", "2026-10-25 00:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := For(tt.mode, day(tt.now))
			if !w.Since.Equal(day(tt.since)) || !w.Until.Equal(day(tt.until)) {
				t.Errorf("got %v to %v, want %s to %s", w.Since, w.Until, tt.since, tt.until)
			}
		})
	}
}

func TestContainsIsHalfOpen(t *testing.T) {
	w := For(Today, day("2026-10-21 15:30"))
	if !w.Contains(w.Since) {
		t.Error("window should include its start")
	}
	if w.Contains(w.Until) {
		t.Error("window should exclude its end")
	}
}
