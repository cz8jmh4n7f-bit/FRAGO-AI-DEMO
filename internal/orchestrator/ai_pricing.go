package orchestrator

import "strings"

// AI model pricing for REAL-TIME cost at the gateway. Public list prices in USD
// per 1,000,000 tokens (input, output). These are ESTIMATES for governance/budget
// enforcement - they drift as providers change pricing, and authoritative spend
// for LiteLLM-minted keys comes from the LiteLLM spend report. Match is
// longest-prefix / contains, most-specific first; unknown models use the default.
type modelPrice struct {
	match   string  // lowercase substring matched against the model id
	inPerM  float64 // USD per 1M input tokens
	outPerM float64 // USD per 1M output tokens
}

// Ordered most-specific first so "gpt-4o-mini" matches before "gpt-4o", etc.
var aiModelPrices = []modelPrice{
	{"gpt-4o-mini", 0.15, 0.60},
	{"gpt-4o", 2.50, 10.00},
	{"gpt-4-turbo", 10.00, 30.00},
	{"gpt-4.1-mini", 0.40, 1.60},
	{"gpt-4.1", 2.00, 8.00},
	{"gpt-4", 30.00, 60.00},
	{"gpt-3.5", 0.50, 1.50},
	{"o1-mini", 1.10, 4.40},
	{"o1", 15.00, 60.00},
	{"o3-mini", 1.10, 4.40},
	{"o3", 10.00, 40.00},
	{"claude-opus", 15.00, 75.00},
	{"claude-sonnet", 3.00, 15.00},
	{"claude-haiku", 0.80, 4.00},
	{"claude-3-opus", 15.00, 75.00},
	{"claude-3-5-sonnet", 3.00, 15.00},
	{"claude-3-haiku", 0.25, 1.25},
	{"claude-fable", 5.00, 25.00},
	{"gemini-1.5-pro", 1.25, 5.00},
	{"gemini-1.5-flash", 0.075, 0.30},
	{"gemini", 1.00, 3.00},
	{"mock", 0.0, 0.0}, // the dev/mock model is free
	// Claude family fallbacks - catch versioned ids the specific rows above miss,
	// e.g. "claude-3-5-haiku-20241022" contains neither "claude-haiku" nor
	// "claude-3-haiku". Listed LAST so a specific price always wins first. These bare
	// keywords are gated to claude-* ids in priceFor (see claudeFamilyKeyword) so a
	// non-Claude model whose id merely contains the word can't borrow Anthropic pricing.
	{"haiku", 0.80, 4.00},
	{"sonnet", 3.00, 15.00},
	{"opus", 15.00, 75.00},
}

// claudeFamilyKeyword reports whether a price-table match is a bare Claude family
// keyword (the family-fallback rows). Such rows only apply to Anthropic ids.
func claudeFamilyKeyword(match string) bool {
	return match == "haiku" || match == "sonnet" || match == "opus"
}

// aiDefaultPrice is the conservative fallback for an unrecognized model.
var aiDefaultPrice = modelPrice{"", 1.00, 3.00}

// estimateAICost returns the USD cost of a call given the model id and token
// counts, using the price table above.
func estimateAICost(model string, inTokens, outTokens float64) float64 {
	p := priceFor(model)
	return (inTokens/1e6)*p.inPerM + (outTokens/1e6)*p.outPerM
}

// estimateAICostTotal prices a call when ONLY a combined token total is known (no
// input/output split, as some upstreams report), using the blended average of the
// model's input and output rates. A conservative middle-ground so a total-only
// response still contributes a non-zero cost to budgets/cost-quotas.
func estimateAICostTotal(model string, totalTokens float64) float64 {
	p := priceFor(model)
	return (totalTokens / 1e6) * (p.inPerM + p.outPerM) / 2
}

func priceFor(model string) modelPrice {
	m := strings.ToLower(strings.TrimSpace(model))
	for _, p := range aiModelPrices {
		if p.match == "" || !strings.Contains(m, p.match) {
			continue
		}
		// Bare family keywords (haiku/sonnet/opus) are Anthropic-only: don't let a
		// non-Claude model whose id merely contains the word borrow Claude pricing.
		if claudeFamilyKeyword(p.match) && !strings.HasPrefix(m, "claude") {
			continue
		}
		return p
	}
	return aiDefaultPrice
}
