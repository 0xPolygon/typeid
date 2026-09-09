package typeid

import "testing"

// A frozen clock is the only way to reach the branch that gives up on a
// millisecond and borrows the next one, since real time would otherwise move on
// long before 32,768 values were spent.
func TestInt64Sequence_BorrowsWhenAMillisecondFills(t *testing.T) {
	const frozen = 1_700_000_000_000

	restore := nowMilli
	nowMilli = func() int64 { return frozen }
	t.Cleanup(func() { nowMilli = restore })

	var seq int64Sequence

	// Well past the 32,768 values one millisecond holds, so the sequence has to
	// borrow at least twice.
	const n = 100_000
	seen := make(map[int64]int, n)
	spilled := 0
	for i := range n {
		v, err := seq.next()
		if err != nil {
			t.Fatal(err)
		}
		if first, dup := seen[v]; dup {
			t.Fatalf("mint %d repeated the value from mint %d: %d", i, first, v)
		}
		seen[v] = i
		if v>>randomBits != frozen {
			spilled++
		}
	}

	if spilled == 0 {
		t.Fatal("no value spilled past the frozen millisecond, so the borrow branch never ran")
	}
	if seq.ms <= frozen {
		t.Fatalf("sequence stayed on millisecond %d after minting %d values", seq.ms, n)
	}
}

// A clock that steps backwards must not walk back into values already spent.
func TestInt64Sequence_HoldsItsMillisecondWhenTheClockStepsBack(t *testing.T) {
	const start = 1_700_000_000_000

	now := int64(start)
	restore := nowMilli
	nowMilli = func() int64 { return now }
	t.Cleanup(func() { nowMilli = restore })

	var seq int64Sequence
	before, err := seq.next()
	if err != nil {
		t.Fatal(err)
	}

	now = start - 60_000
	after, err := seq.next()
	if err != nil {
		t.Fatal(err)
	}

	if after>>randomBits != start {
		t.Errorf("value moved to millisecond %d after the clock stepped back; want %d",
			after>>randomBits, start)
	}
	if len(seq.dirty) != 2 {
		t.Errorf("issued-value set holds %d entries; a reset would have dropped the earlier one",
			len(seq.dirty))
	}
	if before == after {
		t.Fatalf("both mints returned %d", before)
	}
}
