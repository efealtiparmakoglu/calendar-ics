// Command calendar-ics creates, parses and checks iCalendar files.

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	ics "github.com/efealtiparmakoglu/calendar-ics/ics"
)

const version = "1.0.0"

const help = `calendar-ics — create, parse and check iCalendar files

USAGE:
  calendar-ics create -o out.ics -name "Plan"  \
      -e "2026-09-21 09:00|2026-09-21 10:00|Standup|Room 4|FREQ=WEEKLY;BYDAY=MO,WE,FR"
  calendar-ics parse out.ics                   # list events
  calendar-ics conflicts out.ics               # overlapping events (ignoring recurrences)

EVENT FORMAT: start|end|summary[|location[|rrule]]
  times: "2006-01-02 15:04" local, or "20260921T090000Z" UTC
  all-day: start=end=YYYY-MM-DD

Exit codes: 0 ok, 1 error.`

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, help)
		return 2
	}
	switch os.Args[1] {
	case "create":
		return cmdCreate(os.Args[2:])
	case "parse":
		return cmdParse(os.Args[2:])
	case "conflicts":
		return cmdConflicts(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Println(help)
		return 0
	case "-V", "--version", "version":
		fmt.Println("calendar-ics", version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", os.Args[1], help)
		return 2
	}
}

func parseWhen(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) == 10 {
		return time.Parse("2006-01-02", s)
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
		return t, nil
	}
	return time.Parse("20060102T150405Z", s)
}

func cmdCreate(args []string) int {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	name := fs.String("name", "Calendar", "calendar name")
	out := fs.String("o", "calendar.ics", "output file")
	var events []string
	fs.Func("e", "event: start|end|summary[|location[|rrule]]", func(v string) error {
		events = append(events, v)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cal := &ics.Calendar{Name: *name}
	for i, spec := range events {
		parts := strings.Split(spec, "|")
		if len(parts) < 3 {
			fmt.Fprintf(os.Stderr, "event %d: need start|end|summary\n", i+1)
			return 2
		}
		start, err := parseWhen(parts[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "event %d: bad start: %v\n", i+1, err)
			return 2
		}
		end, err := parseWhen(parts[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "event %d: bad end: %v\n", i+1, err)
			return 2
		}
		e := ics.Event{Summary: parts[2], Start: start, End: end}
		if len(parts) > 3 {
			e.Location = parts[3]
		}
		if len(parts) > 4 {
			e.Recurrence = parts[4]
		}
		cal.Events = append(cal.Events, e)
	}
	if err := os.WriteFile(*out, []byte(ics.Render(cal)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "calendar-ics:", err)
		return 1
	}
	fmt.Printf("wrote %s (%d events)\n", *out, len(cal.Events))
	return 0
}

func loadCal(args []string) (*ics.Calendar, error) {
	fs := flag.NewFlagSet("load", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	files := fs.Args()
	if len(files) != 1 {
		return nil, fmt.Errorf("need exactly one .ics file")
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		return nil, err
	}
	return ics.Parse(string(data))
}

func cmdParse(args []string) int {
	cal, err := loadCal(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "calendar-ics:", err)
		return 1
	}
	if cal.Name != "" {
		fmt.Println("calendar:", cal.Name)
	}
	for _, e := range cal.Events {
		when := e.Start.Format("2006-01-02 15:04")
		if e.AllDay {
			when = e.Start.Format("2006-01-02") + " (all day)"
		}
		line := fmt.Sprintf("- %s | %s", when, e.Summary)
		if e.Location != "" {
			line += " @ " + e.Location
		}
		if e.Recurrence != "" {
			line += " [" + e.Recurrence + "]"
		}
		fmt.Println(line)
	}
	return 0
}

func cmdConflicts(args []string) int {
	cal, err := loadCal(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "calendar-ics:", err)
		return 1
	}
	var spans []span
	for _, e := range cal.Events {
		for _, occ := range ics.Expand(e, e.Start.AddDate(-1, 0, 0), e.Start.AddDate(1, 0, 0)) {
			spans = append(spans, span{e.Summary, occ, occ.Add(e.End.Sub(e.Start))})
		}
	}
	sortSpans(spans)
	found := 0
	for i := 1; i < len(spans); i++ {
		if spans[i].start.Before(spans[i-1].end) {
			found++
			fmt.Printf("CONFLICT: %q overlaps %q (from %s)\n",
				spans[i].summary, spans[i-1].summary,
				spans[i].start.Format("2006-01-02 15:04"))
		}
	}
	if found == 0 {
		fmt.Println("no conflicts")
		return 0
	}
	fmt.Printf("%d conflict(s)\n", found)
	return 1
}

type span struct {
	summary string
	start   time.Time
	end     time.Time
}

func sortSpans(s []span) {
	// insertion sort keeps this tiny and dependency-free
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].start.Before(s[j-1].start); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
