package orchestrator

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Gateway DLP: opt-in redaction of PII / secrets in a prompt BEFORE it is
// forwarded to the upstream model, so sensitive data never leaves the perimeter.
// Each string value in the request JSON is scanned; matches are replaced with a
// [REDACTED:type] token (structure preserved - model/params untouched). The
// gateway audits the redaction COUNTS by type, never the values.

type dlpPattern struct {
	name string
	re   *regexp.Regexp
}

// Ordered so more specific patterns run before generic ones.
var dlpPatterns = []dlpPattern{
	{"aws_access_key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"private_key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |)PRIVATE KEY-----`)},
	{"anthropic_key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`)}, // before openai (sk-ant matches sk-)
	{"openai_key", regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`)},
	{"github_token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"ssn", regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
	{"email", regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)},
}

// Card-shaped runs (16-digit 4x4 / 15-digit Amex), Luhn-validated below so a bare
// order number / phone / id is NOT redacted (the old broad \d{13,16} over-redacted).
var creditCardRe = regexp.MustCompile(`\b(?:\d{4}[ -]?){3}\d{4}\b|\b\d{4}[ -]?\d{6}[ -]?\d{5}\b`)

// luhnValid reports whether the digits in s pass the Luhn checksum (real cards do).
func luhnValid(s string) bool {
	var sum, n int
	alt := false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			continue
		}
		d := int(c - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		n++
		alt = !alt
	}
	return n >= 13 && sum%10 == 0
}

// redactDLPString applies every pattern to one string, returning the redacted
// string and per-type hit counts.
func redactDLPString(s string, hits map[string]int) string {
	for _, p := range dlpPatterns {
		s = p.re.ReplaceAllStringFunc(s, func(string) string {
			hits[p.name]++
			return "[REDACTED:" + p.name + "]"
		})
	}
	// Credit cards: only redact card-shaped runs that pass Luhn (avoid redacting
	// order numbers / phones / timestamps).
	s = creditCardRe.ReplaceAllStringFunc(s, func(m string) string {
		if !luhnValid(m) {
			return m
		}
		hits["credit_card"]++
		return "[REDACTED:credit_card]"
	})
	return s
}

// redactDLP walks the request JSON and redacts PII/secrets in every string value.
// Returns the re-encoded payload, the per-type hit counts, and ok=false ONLY when
// matches were found but the redacted payload could not be re-encoded (the caller
// must fail CLOSED then - never forward the un-redacted original). A JSON PARSE
// failure (input isn't JSON, nothing to redact) returns ok=true unchanged.
func redactDLP(payload []byte) (out []byte, hits map[string]int, ok bool) {
	hits = map[string]int{}
	var tree any
	if err := json.Unmarshal(payload, &tree); err != nil {
		return payload, hits, true // not JSON - no string values to scan
	}
	redacted := walkRedact(tree, hits)
	if len(hits) == 0 {
		return payload, hits, true
	}
	enc, err := json.Marshal(redacted)
	if err != nil {
		// We found PII but cannot produce the redacted version - DO NOT forward
		// the original. Signal the caller to fail closed.
		return nil, hits, false
	}
	return enc, hits, true
}

func walkRedact(v any, hits map[string]int) any {
	switch t := v.(type) {
	case string:
		return redactDLPString(t, hits)
	case []any:
		for i := range t {
			t[i] = walkRedact(t[i], hits)
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = walkRedact(t[k], hits)
		}
		return t
	default:
		return v
	}
}

// dlpEnabled reports whether DLP redaction is on for a provider (config "dlp" or
// "dlp_redaction" truthy). Opt-in.
func dlpEnabled(cfg map[string]any) bool {
	for _, k := range []string{"dlp", "dlp_redaction"} {
		switch v := cfg[k].(type) {
		case bool:
			if v {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(v), "true") {
				return true
			}
		}
	}
	return false
}
