// Package window works out the time range a standup covers.
package window

import "time"

type Mode string

const (
	Today     Mode = "today"
	Yesterday Mode = "yesterday"
)

// Window is the half-open range [Since, Until) in local time, with the headings the standup uses for it.
type Window struct {
	Mode        Mode
	Since       time.Time
	Until       time.Time
	DoneHeading string
	NextHeading string
}

// For returns the window for mode as seen at now. Yesterday is the previous working day,
// so on a Monday it covers Friday to Sunday.
func For(mode Mode, now time.Time) Window {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if mode == Today {
		return Window{Mode: mode, Since: midnight, Until: midnight.AddDate(0, 0, 1), DoneHeading: "Today", NextHeading: "Next"}
	}
	back := 1
	switch now.Weekday() {
	case time.Monday:
		back = 3
	case time.Sunday:
		back = 2
	}
	return Window{Mode: mode, Since: midnight.AddDate(0, 0, -back), Until: midnight, DoneHeading: "Yesterday", NextHeading: "Today"}
}

func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.Since) && t.Before(w.Until)
}
