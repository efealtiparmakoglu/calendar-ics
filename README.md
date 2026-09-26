# calendar-ics

**iCalendar (RFC 5545) writer/parser in Go — recurrence expansion, conflict detection, folded lines. Standard library only.**

[![CI](https://github.com/efealtiparmakoglu/calendar-ics/actions/workflows/ci.yml/badge.svg)](https://github.com/efealtiparmakoglu/calendar-ics/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8.svg)](https://go.dev)

```
$ calendar-ics parse demo.ics
calendar: Work
- 2026-09-21 07:00 | Plan @ Room 4 [FREQ=WEEKLY;BYDAY=MO]
```

Generates VCALENDAR files from a simple event spec, parses existing ones
(folded lines, escaped text, all-day DATE values) and expands recurring
events into concrete occurrences.

## Why calendar-ics

- **RFC 5545 line discipline.** Output lines fold at 75 octets with
  continuation spaces (UTF-8 safe); parsing unfolds them. Text escaping
  for `\;` `\,` `\n` round trips.
- **Recurrence subset done right.** FREQ=DAILY/WEEKLY/MONTHLY/YEARLY
  with INTERVAL, COUNT, UNTIL and BYDAY for weekly rules — expanded to
  concrete occurrences inside a query window.
- **Conflict detection.** Expansion + interval sweep lists overlapping
  events (e.g. two meetings colliding on a recurring slot).
- **All-day events.** `VALUE=DATE` read and written.
- **Standard library only.** `time`, `flag`, `sort`.

## Usage

```sh
calendar-ics create -o plan.ics -name Work \
  -e "2026-09-21 09:00|2026-09-21 10:00|Standup|Room 4|FREQ=WEEKLY;BYDAY=MO,WE,FR"
calendar-ics parse plan.ics
calendar-ics conflicts plan.ics
```

Event format: `start|end|summary[|location[|rrule]]`. Times are local
`2006-01-02 15:04` or UTC `20060921T090000Z`; all-day events use
`YYYY-MM-DD` for both ends.

## Known limitations

- Recurrence supports the FREQ/INTERVAL/COUNT/UNTIL/BYDAY subset
  (BYMONTHDAY, BYSETPOS and exception dates are not implemented).
- No VALARM / timezone-definition (VTIMEZONE) handling; UTC and local
  DATE-TIME only.

## Development

```sh
go test ./...
gofmt -l . && go vet ./...
```

Tests cover parsing the sample calendar (escapes, folding, all-day,
UTC), a render round trip, the 75-octet fold limit, weekly BYDAY
expansion, COUNT/UNTIL bounds, interval=2 weekly expansion and
non-recurring windows.

## License

MIT — see [LICENSE](LICENSE).
