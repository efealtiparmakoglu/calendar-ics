// Package ics implements a focused slice of RFC 5545 (iCalendar):
// VEVENT parsing/writing, line folding, UTC/local DATE-TIME values,
// DATE values, and the recurrence subset FREQ/INTERVAL/COUNT/UNTIL/BYDAY.
package ics

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Event is one VEVENT.
type Event struct {
	UID        string
	Summary    string
	Location   string
	Start      time.Time
	End        time.Time
	AllDay     bool
	Recurrence string // raw RRULE value, e.g. "FREQ=WEEKLY;BYDAY=MO,WE"
}

// Calendar is a VCALENDAR with its events.
type Calendar struct {
	Name   string
	Events []Event
}

// Parse reads an ICS payload and returns the calendar with all events.
// Line folding (RFC 5545 §3.1) is unfolded before parsing.
func Parse(data string) (*Calendar, error) {
	lines := unfold(data)
	cal := &Calendar{}
	var cur *Event
	for _, line := range lines {
		name, params, value := parseLine(line)
		switch name {
		case "BEGIN":
			if value == "VEVENT" {
				cur = &Event{}
			}
		case "END":
			if value == "VEVENT" {
				if cur != nil {
					cal.Events = append(cal.Events, *cur)
				}
				cur = nil
			}
		case "X-WR-CALNAME":
			cal.Name = unescapeText(value)
		default:
			if cur == nil {
				continue
			}
			applyProp(cur, name, params, value)
		}
	}
	return cal, nil
}

func applyProp(e *Event, name, params, value string) {
	switch name {
	case "UID":
		e.UID = value
	case "SUMMARY":
		e.Summary = unescapeText(value)
	case "LOCATION":
		e.Location = unescapeText(value)
	case "DTSTART":
		e.Start, e.AllDay = parseDTValue(params, value)
	case "DTEND":
		e.End, _ = parseDTValue(params, value)
	case "RRULE":
		e.Recurrence = value
	}
}

func parseDTValue(params, value string) (time.Time, bool) {
	allDay := strings.Contains(params, "VALUE=DATE")
	layouts := []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	}
	var t time.Time
	for _, l := range layouts {
		if tt, err := time.Parse(l, value); err == nil {
			t = tt
			if l == "20060102" {
				return t, true
			}
			return t, allDay || false
		}
	}
	return t, false
}

// unfold joins continuation lines (leading space/tab) with the previous line.
func unfold(data string) []string {
	raw := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var out []string
	for _, line := range raw {
		if line == "" {
			continue
		}
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(out) > 0 {
			out[len(out)-1] += line[1:]
			continue
		}
		out = append(out, line)
	}
	return out
}

// parseLine splits "NAME;PARAM=V;PARAM2=V2:value".
func parseLine(line string) (name, params, value string) {
	// the colon inside a param value would break a naive split; RFC 5545
	// quoted params are rare enough to treat naively here
	i := strings.Index(line, ":")
	if i < 0 {
		return strings.ToUpper(line), "", ""
	}
	head, value := line[:i], line[i+1:]
	parts := strings.Split(head, ";")
	name = strings.ToUpper(parts[0])
	params = strings.Join(parts[1:], ";")
	return name, params, value
}

func escapeText(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\n", "\\n")
	return r.Replace(s)
}

func unescapeText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n', 'N':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Render serializes the calendar, folding lines longer than 75 octets.
func Render(cal *Calendar) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//hashset//calendar-ics " + "1.0//EN\r\n")
	if cal.Name != "" {
		b.WriteString(fold("X-WR-CALNAME:" + escapeText(cal.Name)))
	}
	for _, e := range cal.Events {
		b.WriteString("BEGIN:VEVENT\r\n")
		uid := e.UID
		if uid == "" {
			uid = fmt.Sprintf("%d-%s", e.Start.Unix(), sanitizeUID(e.Summary))
		}
		b.WriteString(fold("UID:" + uid))
		b.WriteString(fold("SUMMARY:" + escapeText(e.Summary)))
		if e.Location != "" {
			b.WriteString(fold("LOCATION:" + escapeText(e.Location)))
		}
		if e.AllDay {
			b.WriteString(fold("DTSTART;VALUE=DATE:" + e.Start.Format("20060102")))
			end := e.End
			if end.Before(e.Start) || end.Equal(e.Start) {
				end = e.Start.AddDate(0, 0, 1)
			}
			b.WriteString(fold("DTEND;VALUE=DATE:" + end.Format("20060102")))
		} else {
			b.WriteString(fold("DTSTART:" + e.Start.UTC().Format("20060102T150405Z")))
			b.WriteString(fold("DTEND:" + e.End.UTC().Format("20060102T150405Z")))
		}
		if e.Recurrence != "" {
			b.WriteString(fold("RRULE:" + e.Recurrence))
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func sanitizeUID(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, strings.ToLower(s))
}

// fold breaks lines over 75 octets with a leading space on continuations.
func fold(line string) string {
	if len(line) <= 75 {
		return line + "\r\n"
	}
	var b strings.Builder
	first := true
	for {
		limit := 75
		if !first {
			limit = 74 // the continuation space counts toward the 75
		}
		if len(line) <= limit {
			break
		}
		cut := limit
		// avoid splitting a UTF-8 sequence
		for cut > 1 && !isContinuationStart(line[cut]) {
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		first = false
	}
	b.WriteString(line)
	b.WriteString("\r\n")
	return b.String()
}

func isContinuationStart(b byte) bool {
	return b&0xC0 != 0x80
}

// ----------------------------------------------------------------------------
// Recurrence expansion (FREQ, INTERVAL, COUNT, UNTIL, BYDAY for WEEKLY)

// Expand returns the event occurrence start times between windowStart and
// windowEnd (inclusive). Supports FREQ=DAILY/WEEKLY/MONTHLY/YEARLY with
// INTERVAL, COUNT, UNTIL and (for WEEKLY) BYDAY. Non-recurring events yield
// themselves when inside the window.
func Expand(e Event, windowStart, windowEnd time.Time) []time.Time {
	if e.Recurrence == "" {
		if !e.Start.Before(windowStart) && !e.Start.After(windowEnd) {
			return []time.Time{e.Start}
		}
		return nil
	}
	rule := parseRRule(e.Recurrence)
	freq := strings.ToUpper(rule["FREQ"])
	interval := 1
	if rule["INTERVAL"] != "" {
		fmt.Sscanf(rule["INTERVAL"], "%d", &interval)
		if interval < 1 {
			interval = 1
		}
	}
	count := -1
	if rule["COUNT"] != "" {
		fmt.Sscanf(rule["COUNT"], "%d", &count)
	}
	var until time.Time
	hasUntil := false
	if rule["UNTIL"] != "" {
		until, _ = time.Parse("20060102T150405Z", rule["UNTIL"])
		if until.IsZero() {
			until, _ = time.Parse("20060102", rule["UNTIL"])
		}
		hasUntil = true
	}
	byDays := parseByDay(rule["BYDAY"])

	var out []time.Time
	n := 0
	if freq == "WEEKLY" {
		// walk day by day; a day belongs to occurrence-week
		// floor(days between the week anchors / (7*interval))
		anchor := time.Date(e.Start.Year(), e.Start.Month(), e.Start.Day(), 0, 0, 0, 0, e.Start.Location())
		for cur := e.Start; !cur.After(windowEnd) && n < 1000; cur = cur.AddDate(0, 0, 1) {
			if hasUntil && !until.IsZero() && cur.After(until) {
				break
			}
			if count >= 0 && n >= count {
				break
			}
			wd := cur.Weekday()
			matched := len(byDays) == 0
			for _, d := range byDays {
				if wd == d {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			anchorCur := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, cur.Location())
			weeks := int(anchorCur.Sub(anchor).Hours() / (24 * 7))
			if weeks%interval != 0 {
				continue
			}
			if !cur.Before(windowStart) {
				out = append(out, cur)
			}
			n++
		}
		return out
	}

	cur := e.Start
	for n < 1000 {
		if cur.After(windowEnd) {
			break
		}
		if hasUntil && !until.IsZero() && cur.After(until) {
			break
		}
		if count >= 0 && n >= count {
			break
		}
		if !cur.Before(windowStart) && !cur.After(windowEnd) {
			out = append(out, cur)
		}
		n++
		switch freq {
		case "DAILY":
			cur = cur.AddDate(0, 0, interval)
		case "MONTHLY":
			cur = cur.AddDate(0, interval, 0)
		case "YEARLY":
			cur = cur.AddDate(interval, 0, 0)
		default:
			cur = cur.AddDate(0, 0, interval)
		}
	}
	return out
}

func parseRRule(value string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(value, ";") {
		if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
			out[strings.ToUpper(kv[0])] = kv[1]
		}
	}
	return out
}

func parseByDay(v string) []time.Weekday {
	names := map[string]time.Weekday{
		"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
		"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday,
	}
	var out []time.Weekday
	for _, part := range strings.Split(strings.ToUpper(v), ",") {
		if d, ok := names[strings.TrimSpace(part)]; ok {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
