package ics

import (
	"strings"
	"testing"
	"time"
)

const sample = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"X-WR-CALNAME:Work\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:1@x\r\n" +
	"SUMMARY:Standup\\, daily\r\n" +
	"LOCATION:Room 4\n second line\r\n" +
	"DTSTART:20260921T090000Z\r\n" +
	"DTEND:20260921T091500Z\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:2@x\r\n" +
	"SUMMARY:Release day\r\n" +
	"DTSTART;VALUE=DATE:20261001\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseSample(t *testing.T) {
	cal, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Name != "Work" {
		t.Fatalf("name = %q", cal.Name)
	}
	if len(cal.Events) != 2 {
		t.Fatalf("want 2 events, got %d", len(cal.Events))
	}
	e := cal.Events[0]
	if e.Summary != "Standup, daily" {
		t.Fatalf("summary = %q (escape bug)", e.Summary)
	}
	if !strings.Contains(e.Location, "second line") {
		t.Fatalf("folded location lost: %q", e.Location)
	}
	if e.Start.UTC().Hour() != 9 {
		t.Fatalf("start hour = %d", e.Start.UTC().Hour())
	}
	if !cal.Events[1].AllDay {
		t.Fatal("VALUE=DATE not detected as all-day")
	}
	if cal.Events[1].Start.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("all-day date = %s", cal.Events[1].Start)
	}
}

func TestRenderRoundTrip(t *testing.T) {
	cal, _ := Parse(sample)
	out := Render(cal)
	cal2, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if cal2.Name != cal.Name || len(cal2.Events) != len(cal.Events) {
		t.Fatal("round trip lost data")
	}
	for i := range cal.Events {
		if cal2.Events[i].Summary != cal.Events[i].Summary {
			t.Fatalf("event %d summary mismatch", i)
		}
		if !cal.Events[i].Start.Equal(cal2.Events[i].Start) {
			t.Fatalf("event %d start mismatch", i)
		}
	}
}

func TestFoldLongLines(t *testing.T) {
	long := strings.Repeat("x", 200)
	cal := &Calendar{Events: []Event{{UID: "a", Summary: long, Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}}
	out := Render(cal)
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > 75 {
			t.Fatalf("unfolded line too long: %d octets", len(line))
		}
	}
	cal2, _ := Parse(out)
	if cal2.Events[0].Summary != long {
		t.Fatal("fold/unfold lost content")
	}
}

func TestExpandWeeklyByDay(t *testing.T) {
	// 2026-09-21 is a Monday
	start := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	e := Event{
		UID:        "1",
		Start:      start,
		End:        start.Add(15 * time.Minute),
		Recurrence: "FREQ=WEEKLY;BYDAY=MO,WE,FR",
	}
	ws := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	we := ws.AddDate(0, 0, 8) // through Tuesday next week
	got := Expand(e, ws, we)
	if len(got) != 4 { // Mon, Wed, Fri of week 1 + Mon of week 2
		t.Fatalf("got %d occurrences (%v)", len(got), got)
	}
	weekdays := map[string]bool{}
	for _, g := range got {
		weekdays[g.Weekday().String()] = true
		if g.Weekday() == time.Tuesday || g.Weekday() == time.Thursday || g.Weekday() == time.Saturday || g.Weekday() == time.Sunday {
			t.Fatalf("unexpected weekday %s", g.Weekday())
		}
	}
	_ = weekdays
}

func TestExpandCountAndUntil(t *testing.T) {
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	e := Event{UID: "c", Start: start, Recurrence: "FREQ=DAILY;COUNT=3"}
	got := Expand(e, start.Add(-time.Hour), start.AddDate(0, 0, 30))
	if len(got) != 3 {
		t.Fatalf("COUNT=3 gave %d", len(got))
	}

	e2 := Event{UID: "u", Start: start, Recurrence: "FREQ=DAILY;UNTIL=20260104T000000Z"}
	got2 := Expand(e2, start.Add(-time.Hour), start.AddDate(0, 0, 30))
	if len(got2) != 3 { // Jan 1, 2, 3 (UNTIL is exclusive by instant)
		t.Fatalf("UNTIL gave %d: %v", len(got2), got2)
	}
}

func TestExpandNonRecurring(t *testing.T) {
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	e := Event{UID: "s", Start: start}
	if got := Expand(e, start.Add(time.Hour), start.AddDate(0, 0, 1)); len(got) != 0 {
		t.Fatal("outside window should yield nothing")
	}
	if got := Expand(e, start, start.Add(time.Hour)); len(got) != 1 {
		t.Fatal("inside window should yield itself")
	}
}

func TestExpandInterval(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC) // Monday
	e := Event{
		UID:        "i",
		Start:      start,
		Recurrence: "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO",
	}
	ws := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	we := ws.AddDate(0, 0, 21)
	got := Expand(e, ws, we)
	if len(got) != 2 { // Jan 5 and Jan 19 (every 2nd week)
		t.Fatalf("interval=2 gave %d occurrences: %v", len(got), got)
	}
}
