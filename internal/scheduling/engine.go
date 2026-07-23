// Package scheduling contains the provider-independent availability engine and
// the PostgreSQL repository which is the source of truth for appointments.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

const (
	// DefaultTenantID is the only tenant used by the Stage 1-3 application.  It
	// is still passed to every repository query to preserve the isolation seam.
	DefaultTenantID = "00000000-0000-4000-8000-000000000001"
	defaultTimezone = "Europe/Berlin"
)

var (
	ErrInvalidInput         = errors.New("invalid scheduling input")
	ErrNotFound             = errors.New("scheduling resource not found")
	ErrSlotUnavailable      = errors.New("slot is no longer available")
	ErrIdempotencyConflict  = errors.New("idempotency key was used with different arguments")
	ErrBookingStateConflict = errors.New("booking is not in a state that allows this operation")
	// ErrScheduleVersionConflict is returned when an optimistic-lock caller
	// (the operator console) supplies an expected schedule_version that no
	// longer matches the persisted booking, meaning it changed underneath them.
	ErrScheduleVersionConflict = errors.New("booking schedule version does not match the expected version")
)

// RuleKind distinguishes bookable hours from recurring breaks. Rules use the
// staff member's local wall clock.
type RuleKind string

const (
	RuleWorking RuleKind = "working"
	RuleBreak   RuleKind = "break"
)

// Service is the scheduling projection of a service catalog row.
type Service struct {
	ID           string
	Slug         string
	Name         string
	Description  string
	Duration     time.Duration
	BufferBefore time.Duration
	BufferAfter  time.Duration
	PriceMinor   int64
	Currency     string
}

// Staff is the scheduling projection of a staff catalog row.
type Staff struct {
	ID          string
	Slug        string
	DisplayName string
	Timezone    string
}

// AvailabilityRule recurs weekly. StartMinute and EndMinute are minutes after
// local midnight; EndMinute may be 1440. Overnight rules should be represented
// as two rules, which avoids ambiguity on daylight-saving boundaries.
type AvailabilityRule struct {
	Kind        RuleKind
	Weekday     time.Weekday
	StartMinute int
	EndMinute   int
	ValidFrom   *time.Time
	ValidUntil  *time.Time
}

// Interval is a half-open busy interval. Optional buffers are expanded before
// overlap checks, matching the database exclusion constraint.
type Interval struct {
	Start        time.Time
	End          time.Time
	BufferBefore time.Duration
	BufferAfter  time.Duration
}

// Slot is a genuinely available service window. All timestamps retain their
// IANA-zone offset and can be serialized as RFC 3339 values.
type Slot struct {
	ServiceID string    `json:"service_id"`
	StaffID   string    `json:"staff_id"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
}

// SearchInput supplies a complete availability snapshot for one staff member.
// To is exclusive. SlotInterval defaults to 15 minutes and Limit defaults to
// unlimited; repository callers normally set an application-level limit.
type SearchInput struct {
	Service      Service
	Staff        Staff
	From         time.Time
	To           time.Time
	Rules        []AvailabilityRule
	Busy         []Interval
	SlotInterval time.Duration
	Limit        int
}

// Engine computes slots without I/O, which keeps timezone and overlap behavior
// deterministic and straightforward to exercise in tests.
type Engine struct {
	location *time.Location
}

// NewEngine builds an engine with the fallback location used when Staff has no
// timezone. A nil location uses Europe/Berlin.
func NewEngine(location *time.Location) *Engine {
	if location == nil {
		location, _ = time.LoadLocation(defaultTimezone)
	}
	return &Engine{location: location}
}

// NewEngineForTimezone is the configuration-friendly constructor.
func NewEngineForTimezone(name string) (*Engine, error) {
	if name == "" {
		name = defaultTimezone
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: timezone %q: %w", ErrInvalidInput, name, err)
	}
	return NewEngine(location), nil
}

// FindSlots returns non-overlapping candidates in chronological order.
func (e *Engine) FindSlots(ctx context.Context, in SearchInput) ([]Slot, error) {
	location, step, err := e.validate(in)
	if err != nil {
		return nil, err
	}

	busy := make([]wallInterval, 0, len(in.Busy))
	for _, item := range in.Busy {
		if item.Start.IsZero() || item.End.IsZero() || !item.Start.Before(item.End) || item.BufferBefore < 0 || item.BufferAfter < 0 {
			return nil, fmt.Errorf("%w: invalid busy interval", ErrInvalidInput)
		}
		busy = append(busy, wallInterval{
			start: item.Start.Add(-item.BufferBefore),
			end:   item.End.Add(item.BufferAfter),
		})
	}

	localFrom := in.From.In(location)
	localLast := in.To.Add(-time.Nanosecond).In(location)
	day := time.Date(localFrom.Year(), localFrom.Month(), localFrom.Day(), 0, 0, 0, 0, location)
	lastDay := time.Date(localLast.Year(), localLast.Month(), localLast.Day(), 0, 0, 0, 0, location)

	var slots []Slot
	seen := make(map[int64]struct{})
	for !day.After(lastDay) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		working, breaks := intervalsForDate(day, location, in.Rules)
		working = mergeIntervals(working)
		breaks = mergeIntervals(breaks)

		for _, period := range working {
			for start := period.start; start.Before(period.end); start = start.Add(step) {
				end := start.Add(in.Service.Duration)
				if start.Before(in.From) || !end.After(start) || end.After(in.To) || end.After(period.end) {
					continue
				}

				occupied := wallInterval{
					start: start.Add(-in.Service.BufferBefore),
					end:   end.Add(in.Service.BufferAfter),
				}
				if overlapsAny(occupied, breaks) || overlapsAny(occupied, busy) {
					continue
				}
				key := start.UnixNano()
				if _, duplicate := seen[key]; duplicate {
					continue
				}
				seen[key] = struct{}{}
				slots = append(slots, Slot{
					ServiceID: in.Service.ID,
					StaffID:   in.Staff.ID,
					Start:     start,
					End:       end,
				})
				if in.Limit > 0 && len(slots) >= in.Limit {
					sortSlots(slots)
					return slots, nil
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}

	sortSlots(slots)
	return slots, nil
}

// IsAvailable reuses the exact slot grid used for offers. This is intended for
// the locked transaction recheck immediately before inserting a booking.
func (e *Engine) IsAvailable(ctx context.Context, in SearchInput, startsAt time.Time) (bool, error) {
	location, _, err := e.validate(in)
	if err != nil {
		return false, err
	}
	local := startsAt.In(location)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	dayEnd := dayStart.AddDate(0, 0, 1)
	in.From = dayStart
	in.To = dayEnd
	in.Limit = 0
	slots, err := e.FindSlots(ctx, in)
	if err != nil {
		return false, err
	}
	for _, slot := range slots {
		if slot.Start.Equal(startsAt) {
			return true, nil
		}
	}
	return false, nil
}

func (e *Engine) validate(in SearchInput) (*time.Location, time.Duration, error) {
	if in.Service.Duration <= 0 || in.Service.Duration > 24*time.Hour {
		return nil, 0, fmt.Errorf("%w: service duration must be in (0, 24h]", ErrInvalidInput)
	}
	if in.Service.BufferBefore < 0 || in.Service.BufferAfter < 0 ||
		in.Service.BufferBefore > 4*time.Hour || in.Service.BufferAfter > 4*time.Hour {
		return nil, 0, fmt.Errorf("%w: service buffer outside allowed range", ErrInvalidInput)
	}
	if in.From.IsZero() || in.To.IsZero() || !in.From.Before(in.To) {
		return nil, 0, fmt.Errorf("%w: date range must be positive", ErrInvalidInput)
	}
	if in.To.Sub(in.From) > 31*24*time.Hour+2*time.Hour {
		return nil, 0, fmt.Errorf("%w: date range exceeds 31 days", ErrInvalidInput)
	}
	if in.Limit < 0 || in.Limit > 1000 {
		return nil, 0, fmt.Errorf("%w: limit outside allowed range", ErrInvalidInput)
	}

	location := e.location
	if location == nil {
		location, _ = time.LoadLocation(defaultTimezone)
	}
	if in.Staff.Timezone != "" {
		var err error
		location, err = time.LoadLocation(in.Staff.Timezone)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: staff timezone %q: %w", ErrInvalidInput, in.Staff.Timezone, err)
		}
	}
	step := in.SlotInterval
	if step == 0 {
		step = 15 * time.Minute
	}
	if step < time.Minute || step > 24*time.Hour || step%time.Minute != 0 {
		return nil, 0, fmt.Errorf("%w: slot interval must be whole minutes", ErrInvalidInput)
	}
	for _, rule := range in.Rules {
		if (rule.Kind != RuleWorking && rule.Kind != RuleBreak) || rule.Weekday < time.Sunday || rule.Weekday > time.Saturday ||
			rule.StartMinute < 0 || rule.StartMinute >= 1440 || rule.EndMinute <= rule.StartMinute || rule.EndMinute > 1440 {
			return nil, 0, fmt.Errorf("%w: invalid availability rule", ErrInvalidInput)
		}
		if rule.ValidFrom != nil && rule.ValidUntil != nil && civilDay(*rule.ValidFrom) > civilDay(*rule.ValidUntil) {
			return nil, 0, fmt.Errorf("%w: rule validity range is reversed", ErrInvalidInput)
		}
	}
	return location, step, nil
}

type wallInterval struct {
	start time.Time
	end   time.Time
}

func intervalsForDate(day time.Time, location *time.Location, rules []AvailabilityRule) (working, breaks []wallInterval) {
	for _, rule := range rules {
		if rule.Weekday != day.Weekday() || !ruleAppliesOn(rule, day) {
			continue
		}
		start := localMinute(day, rule.StartMinute, location, false)
		end := localMinute(day, rule.EndMinute, location, true)
		// A DST jump can collapse a rule whose complete wall-clock range did not
		// exist. There is simply no availability to add in that case.
		if !start.Before(end) {
			continue
		}
		item := wallInterval{start: start, end: end}
		if rule.Kind == RuleWorking {
			working = append(working, item)
		} else {
			breaks = append(breaks, item)
		}
	}
	return working, breaks
}

func ruleAppliesOn(rule AvailabilityRule, day time.Time) bool {
	key := civilDay(day)
	if rule.ValidFrom != nil && key < civilDay(*rule.ValidFrom) {
		return false
	}
	if rule.ValidUntil != nil && key > civilDay(*rule.ValidUntil) {
		return false
	}
	return true
}

func civilDay(value time.Time) int {
	y, m, d := value.Date()
	return y*10000 + int(m)*100 + d
}

func localMinute(day time.Time, minute int, location *time.Location, preferLatest bool) time.Time {
	targetDay := day
	if minute == 1440 {
		targetDay = day.AddDate(0, 0, 1)
		minute = 0
	}
	for candidateMinute := minute; candidateMinute < 1440; candidateMinute++ {
		hour, min := candidateMinute/60, candidateMinute%60
		anchor := time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), hour, min, 0, 0, location)
		// Around a backward transition two distinct instants have the same wall
		// clock. Start boundaries choose the earliest and end boundaries choose
		// the latest, so a 02:00-03:00 rule covers both repeated 02:00 hours.
		var matches []time.Time
		for instant := anchor.Add(-3 * time.Hour); !instant.After(anchor.Add(3 * time.Hour)); instant = instant.Add(time.Minute) {
			local := instant.In(location)
			if local.Year() == targetDay.Year() && local.Month() == targetDay.Month() && local.Day() == targetDay.Day() &&
				local.Hour() == hour && local.Minute() == min && local.Second() == 0 {
				matches = append(matches, instant)
			}
		}
		if len(matches) == 0 {
			// The wall minute is inside a forward DST gap. Move to the first
			// real local minute after the gap instead of preserving the minute
			// component (02:30 becomes 03:00, not 03:30).
			continue
		}
		if preferLatest {
			return matches[len(matches)-1]
		}
		return matches[0]
	}
	return time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day()+1, 0, 0, 0, 0, location)
}

func mergeIntervals(items []wallInterval) []wallInterval {
	if len(items) < 2 {
		return items
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].start.Equal(items[j].start) {
			return items[i].end.Before(items[j].end)
		}
		return items[i].start.Before(items[j].start)
	})
	merged := []wallInterval{items[0]}
	for _, item := range items[1:] {
		last := &merged[len(merged)-1]
		if !item.start.After(last.end) {
			if item.end.After(last.end) {
				last.end = item.end
			}
			continue
		}
		merged = append(merged, item)
	}
	return merged
}

func overlapsAny(candidate wallInterval, items []wallInterval) bool {
	for _, item := range items {
		// Half-open intervals permit exact boundary adjacency.
		if candidate.start.Before(item.end) && item.start.Before(candidate.end) {
			return true
		}
	}
	return false
}

func sortSlots(slots []Slot) {
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].Start.Equal(slots[j].Start) {
			return slots[i].StaffID < slots[j].StaffID
		}
		return slots[i].Start.Before(slots[j].Start)
	})
}
