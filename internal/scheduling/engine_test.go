package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFindSlotsWorkingHoursAndLimit(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.July, 20) // Monday.
	in := SearchInput{
		Service: Service{ID: "service", Duration: time.Hour},
		Staff:   Staff{ID: "staff", Timezone: "Europe/Berlin"},
		From:    day.Add(8 * time.Hour),
		To:      day.Add(13 * time.Hour),
		Rules: []AvailabilityRule{{
			Kind: RuleWorking, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 12 * 60,
		}},
		SlotInterval: 30 * time.Minute,
	}

	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"09:00", "09:30", "10:00", "10:30", "11:00"}
	assertLocalTimes(t, slots, want)

	in.Limit = 2
	slots, err = NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalTimes(t, slots, want[:2])
}

func TestFindSlotsBreaksAndBuffers(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.July, 20)
	in := SearchInput{
		Service: Service{
			ID: "service", Duration: 30 * time.Minute,
			BufferBefore: 5 * time.Minute, BufferAfter: 5 * time.Minute,
		},
		Staff: Staff{ID: "staff", Timezone: "Europe/Berlin"},
		From:  day.Add(9 * time.Hour),
		To:    day.Add(13 * time.Hour),
		Rules: []AvailabilityRule{
			{Kind: RuleWorking, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 13 * 60},
			{Kind: RuleBreak, Weekday: time.Monday, StartMinute: 10 * 60, EndMinute: 10*60 + 30},
		},
		Busy: []Interval{{
			Start: day.Add(11 * time.Hour), End: day.Add(11*time.Hour + 30*time.Minute),
			BufferBefore: 10 * time.Minute, BufferAfter: 10 * time.Minute,
		}},
		SlotInterval: 15 * time.Minute,
	}

	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// 09:15 would have an after-buffer through 09:50 and is valid. 09:30's
	// after-buffer reaches into the 10:00 break; 11:45's pre-buffer starts at
	// 11:40, exactly at the existing booking's buffered end and is valid.
	want := []string{"09:00", "09:15", "11:45", "12:00", "12:15", "12:30"}
	assertLocalTimes(t, slots, want)
}

func TestFindSlotsAdjacentIntervalsDoNotOverlap(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.July, 20)
	in := SearchInput{
		Service: Service{Duration: 30 * time.Minute},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    day.Add(9 * time.Hour),
		To:      day.Add(11 * time.Hour),
		Rules: []AvailabilityRule{{
			Kind: RuleWorking, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60,
		}},
		Busy:         []Interval{{Start: day.Add(9*time.Hour + 30*time.Minute), End: day.Add(10 * time.Hour)}},
		SlotInterval: 30 * time.Minute,
	}
	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalTimes(t, slots, []string{"09:00", "10:00", "10:30"})
}

func TestFindSlotsMergesOverlappingWorkingRulesWithoutDuplicates(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.July, 20)
	in := SearchInput{
		Service: Service{Duration: time.Hour},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    day.Add(9 * time.Hour),
		To:      day.Add(13 * time.Hour),
		Rules: []AvailabilityRule{
			{Kind: RuleWorking, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 12 * 60},
			{Kind: RuleWorking, Weekday: time.Monday, StartMinute: 11 * 60, EndMinute: 13 * 60},
		},
		SlotInterval: time.Hour,
	}
	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalTimes(t, slots, []string{"09:00", "10:00", "11:00", "12:00"})
}

func TestFindSlotsEuropeBerlinSpringDST(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.March, 29) // 02:00-02:59 does not exist.
	in := SearchInput{
		Service: Service{Duration: time.Hour},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    day,
		To:      day.AddDate(0, 0, 1),
		Rules: []AvailabilityRule{{
			Kind: RuleWorking, Weekday: time.Sunday, StartMinute: 60, EndMinute: 5 * 60,
		}},
		SlotInterval: time.Hour,
	}
	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalTimesWithZone(t, slots, []string{"01:00 CET", "03:00 CEST", "04:00 CEST"})
}

func TestFindSlotsSpringDSTMapsNonexistentBreakBoundaryForward(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.March, 29)
	in := SearchInput{
		Service: Service{Duration: 30 * time.Minute},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    day,
		To:      day.AddDate(0, 0, 1),
		Rules: []AvailabilityRule{
			{Kind: RuleWorking, Weekday: time.Sunday, StartMinute: 60, EndMinute: 4 * 60},
			{Kind: RuleBreak, Weekday: time.Sunday, StartMinute: 2*60 + 30, EndMinute: 3*60 + 30},
		},
		SlotInterval: 30 * time.Minute,
	}
	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalTimesWithZone(t, slots, []string{"01:00 CET", "01:30 CET", "03:30 CEST"})
}

func TestFindSlotsEuropeBerlinFallDSTIncludesBothRepeatedHours(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.October, 25) // 02:00 occurs twice.
	in := SearchInput{
		Service: Service{Duration: time.Hour},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    day,
		To:      day.AddDate(0, 0, 1),
		Rules: []AvailabilityRule{{
			Kind: RuleWorking, Weekday: time.Sunday, StartMinute: 60, EndMinute: 4 * 60,
		}},
		SlotInterval: time.Hour,
	}
	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalTimesWithZone(t, slots, []string{"01:00 CEST", "02:00 CEST", "02:00 CET", "03:00 CET"})
}

func TestFindSlotsEuropeBerlinFallDSTAmbiguousRuleStartUsesFirstOccurrence(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.October, 25)
	in := SearchInput{
		Service: Service{Duration: time.Hour},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    day,
		To:      day.AddDate(0, 0, 1),
		Rules: []AvailabilityRule{{
			Kind: RuleWorking, Weekday: time.Sunday, StartMinute: 2 * 60, EndMinute: 3 * 60,
		}},
		SlotInterval: time.Hour,
	}
	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalTimesWithZone(t, slots, []string{"02:00 CEST", "02:00 CET"})
}

func TestFindSlotsHonorsRuleValidityDates(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	monday := localDate(loc, 2026, time.July, 20)
	nextMonday := monday.AddDate(0, 0, 7)
	validUntil := monday
	in := SearchInput{
		Service: Service{Duration: time.Hour},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    monday,
		To:      nextMonday.AddDate(0, 0, 1),
		Rules: []AvailabilityRule{{
			Kind: RuleWorking, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60,
			ValidUntil: &validUntil,
		}},
		SlotInterval: time.Hour,
	}
	slots, err := NewEngine(loc).FindSlots(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || slots[0].Start.Day() != 20 {
		t.Fatalf("expected only July 20 slot, got %#v", slots)
	}
}

func TestIsAvailableUsesExactSlotGrid(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.July, 20)
	in := SearchInput{
		Service: Service{Duration: 30 * time.Minute},
		Staff:   Staff{Timezone: "Europe/Berlin"},
		From:    day,
		To:      day.AddDate(0, 0, 1),
		Rules: []AvailabilityRule{{
			Kind: RuleWorking, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 12 * 60,
		}},
		SlotInterval: 15 * time.Minute,
	}
	engine := NewEngine(loc)
	available, err := engine.IsAvailable(context.Background(), in, day.Add(9*time.Hour+15*time.Minute))
	if err != nil || !available {
		t.Fatalf("expected exact grid slot: available=%v err=%v", available, err)
	}
	available, err = engine.IsAvailable(context.Background(), in, day.Add(9*time.Hour+7*time.Minute))
	if err != nil || available {
		t.Fatalf("expected off-grid slot to be unavailable: available=%v err=%v", available, err)
	}
}

func TestFindSlotsValidationAndCancellation(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	day := localDate(loc, 2026, time.July, 20)
	_, err := NewEngine(loc).FindSlots(context.Background(), SearchInput{
		Service: Service{Duration: 0}, Staff: Staff{Timezone: "Europe/Berlin"}, From: day, To: day.Add(time.Hour),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewEngine(loc).FindSlots(ctx, SearchInput{
		Service: Service{Duration: time.Hour}, Staff: Staff{Timezone: "Europe/Berlin"}, From: day, To: day.Add(time.Hour),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func localDate(loc *time.Location, year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func assertLocalTimes(t *testing.T, slots []Slot, want []string) {
	t.Helper()
	if len(slots) != len(want) {
		t.Fatalf("slot count: got %d (%v), want %d (%v)", len(slots), formatSlots(slots, "15:04"), len(want), want)
	}
	for i := range want {
		if got := slots[i].Start.Format("15:04"); got != want[i] {
			t.Fatalf("slot %d: got %s, want %s", i, got, want[i])
		}
	}
}

func assertLocalTimesWithZone(t *testing.T, slots []Slot, want []string) {
	t.Helper()
	if len(slots) != len(want) {
		t.Fatalf("slot count: got %d (%v), want %d (%v)", len(slots), formatSlots(slots, "15:04 MST"), len(want), want)
	}
	for i := range want {
		if got := slots[i].Start.Format("15:04 MST"); got != want[i] {
			t.Fatalf("slot %d: got %s, want %s", i, got, want[i])
		}
	}
}

func formatSlots(slots []Slot, layout string) []string {
	formatted := make([]string, len(slots))
	for i, slot := range slots {
		formatted[i] = slot.Start.Format(layout)
	}
	return formatted
}
