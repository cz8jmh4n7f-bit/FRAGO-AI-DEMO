package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactDLP(t *testing.T) {
	payload := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"email me at jane@acme.com, key sk-ant-abcdefghij1234567890XYZ, AKIA1234567890ABCDEF"}]}`)
	out, hits, ok := redactDLP(payload)
	if !ok {
		t.Fatal("redactDLP must succeed on a redactable payload")
	}
	s := string(out)
	if strings.Contains(s, "jane@acme.com") || strings.Contains(s, "AKIA1234567890ABCDEF") {
		t.Fatalf("sensitive value not redacted: %s", s)
	}
	if !strings.Contains(s, "[REDACTED:email]") || !strings.Contains(s, "[REDACTED:aws_access_key]") {
		t.Fatalf("redaction token missing: %s", s)
	}
	// model must be untouched (structure preserved)
	var tree map[string]any
	if json.Unmarshal(out, &tree) != nil || tree["model"] != "gpt-4o" {
		t.Fatalf("structure not preserved: %s", s)
	}
	if hits["email"] != 1 || hits["aws_access_key"] != 1 {
		t.Fatalf("hit counts wrong: %v", hits)
	}
}

func TestRedactDLPNoMatch(t *testing.T) {
	payload := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"what is the capital of France?"}]}`)
	out, hits, ok := redactDLP(payload)
	if !ok || len(hits) != 0 || string(out) != string(payload) {
		t.Fatalf("clean payload must pass through unchanged, ok=%v hits=%v", ok, hits)
	}
}

// TestRedactDLPCreditCardLuhn checks the F2 fix: a real (Luhn-valid) card-shaped
// number is redacted, but a similarly-shaped non-card run (order number / id) is
// NOT - avoiding the old broad \d{13,16} over-redaction.
func TestRedactDLPCreditCardLuhn(t *testing.T) {
	// 4111 1111 1111 1111 is the canonical Luhn-valid Visa test number.
	card := []byte(`{"content":"my card is 4111 1111 1111 1111"}`)
	out, hits, ok := redactDLP(card)
	if !ok || hits["credit_card"] != 1 || strings.Contains(string(out), "4111") {
		t.Fatalf("Luhn-valid card must be redacted: ok=%v hits=%v out=%s", ok, hits, out)
	}
	// A 16-digit run that fails Luhn (e.g. an order number) must pass through.
	order := []byte(`{"content":"order 1234 5678 1234 5671"}`)
	out2, hits2, ok2 := redactDLP(order)
	if !ok2 || hits2["credit_card"] != 0 || !strings.Contains(string(out2), "1234 5678 1234 5671") {
		t.Fatalf("non-card numeric run must NOT be redacted: ok=%v hits=%v out=%s", ok2, hits2, out2)
	}
}

func TestDLPEnabled(t *testing.T) {
	if dlpEnabled(map[string]any{}) {
		t.Fatal("default must be off")
	}
	if !dlpEnabled(map[string]any{"dlp": true}) || !dlpEnabled(map[string]any{"dlp_redaction": "true"}) {
		t.Fatal("dlp toggle must enable")
	}
}
