package orchestrator

import "testing"

func TestEstimateAICost(t *testing.T) {
	// gpt-4o: in $2.50/M, out $10/M -> 1000 in + 500 out = 0.0025 + 0.005 = 0.0075
	if got := estimateAICost("gpt-4o-2024-11-20", 1000, 500); got < 0.00749 || got > 0.00751 {
		t.Fatalf("gpt-4o cost = %v, want ~0.0075", got)
	}
	// gpt-4o-mini must match before gpt-4o (more specific first)
	if got := estimateAICost("gpt-4o-mini", 1_000_000, 0); got < 0.149 || got > 0.151 {
		t.Fatalf("gpt-4o-mini in cost = %v, want ~0.15", got)
	}
	// claude opus
	if got := estimateAICost("claude-opus-4-8", 1_000_000, 0); got < 14.9 || got > 15.1 {
		t.Fatalf("claude-opus in cost = %v, want ~15", got)
	}
	// mock is free
	if got := estimateAICost("mock-gpt", 1_000_000, 1_000_000); got != 0 {
		t.Fatalf("mock cost = %v, want 0", got)
	}
	// unknown -> default $1/$3 per M
	if got := estimateAICost("some-unknown-model", 1_000_000, 1_000_000); got < 3.99 || got > 4.01 {
		t.Fatalf("default cost = %v, want ~4", got)
	}
}

// TestPricingFamilyFallback locks in the F5 fix: versioned ids that contain
// neither "claude-haiku" nor "claude-3-haiku" must fall back to the haiku family
// price ($0.80/$4.00 per M), NOT the conservative $1/$3 default.
func TestPricingFamilyFallback(t *testing.T) {
	// claude-3-5-haiku-20241022 contains "haiku" but not the specific rows.
	if got := estimateAICost("claude-3-5-haiku-20241022", 1_000_000, 0); got < 0.79 || got > 0.81 {
		t.Fatalf("claude-3-5-haiku in cost = %v, want ~0.80 (haiku family), not default $1", got)
	}
	if got := estimateAICost("claude-3-5-haiku-20241022", 0, 1_000_000); got < 3.99 || got > 4.01 {
		t.Fatalf("claude-3-5-haiku out cost = %v, want ~4.00 (haiku family)", got)
	}
	// A specific row still wins over the family fallback (claude-3-haiku = $0.25/$1.25).
	if got := estimateAICost("claude-3-haiku-20240307", 1_000_000, 0); got < 0.24 || got > 0.26 {
		t.Fatalf("claude-3-haiku must keep its specific $0.25 price, got %v", got)
	}
	// A NON-Claude model whose id merely contains a family keyword must NOT borrow
	// Claude pricing - it falls through to the conservative default ($1/$3 per M).
	for _, id := range []string{"mistral-sonnet", "aws-opus-mistral", "open-haiku-test"} {
		// 1M in + 1M out at default = $1 + $3 = $4
		if got := estimateAICost(id, 1_000_000, 1_000_000); got < 3.99 || got > 4.01 {
			t.Fatalf("%q must use the default price (~$4), not a Claude family price, got %v", id, got)
		}
	}
}

// TestEstimateAICostTotal covers the F9 blended-rate fix: a total-only call (no
// in/out split) is priced at the average of the model's input and output rates
// instead of $0.
func TestEstimateAICostTotal(t *testing.T) {
	// gpt-4o: ($2.50 + $10.00)/2 = $6.25 per M -> 1M total = $6.25
	if got := estimateAICostTotal("gpt-4o", 1_000_000); got < 6.24 || got > 6.26 {
		t.Fatalf("gpt-4o total-only cost = %v, want ~6.25 (blended), not 0", got)
	}
}
