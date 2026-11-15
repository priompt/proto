package validate

import (
	"slices"
	"testing"
)

func TestPrompt(t *testing.T) {
	ok := func(uri, tmpl string, slots []string) {
		t.Helper()
		if err := Prompt(uri, tmpl, slots); err != nil {
			t.Errorf("want valid, got %v", err)
		}
	}
	bad := func(uri, tmpl string, slots []string) {
		t.Helper()
		if err := Prompt(uri, tmpl, slots); err == nil {
			t.Errorf("want error for %q / %q", uri, tmpl)
		}
	}

	ok("priompt://o/r/p", "Hello {name}, welcome to {org}.", []string{"name", "org"})
	ok("priompt://o/r/p", "No slots here.", nil)
	ok("priompt://acme/support/agents/tier1/greeting", "Hi.", nil) // any depth
	ok("priompt://acme.ai/support/agent", "Hi.", nil)              // dots inside a segment

	bad("", "x", nil)                                    // empty uri
	bad("priompt://o/r/p", "  ", nil)                    // empty template
	bad("priompt://o/r/p", "Hi {name}", nil)             // undeclared slot
	bad("priompt://o/r/p", "Hi there", []string{"name"}) // unused slot
	bad("priompt://o/r/p", "Hi {name}", []string{""})    // empty slot name
}

// The URI is the authorization boundary — its first segment is the org — and it
// is mapped onto both a NATS subject and a filesystem path. Its shape is a
// security check.
func TestURI(t *testing.T) {
	valid := []string{
		"priompt://acme/onboarding/welcome",
		"priompt://acme/support/agents/tier1/greeting",
		"priompt://acme.ai/x",
		"priompt://acme-corp/a_b/x-1",
	}
	for _, u := range valid {
		if err := URI(u); err != nil {
			t.Errorf("URI(%q) = %v, want valid", u, err)
		}
	}

	invalid := map[string]string{
		"":                         "empty",
		"   ":                      "whitespace only",
		"acme/noscheme/x":          "no scheme",
		"priompt://":               "no org",
		"priompt://acme":           "org but no path",
		"priompt://acme/../evil/x": "parent traversal",
		"priompt://acme/./x":       "current-dir segment",
		"priompt://acme//x":        "empty segment",
		"priompt://acme/a b/x":     "space in segment",
		"priompt://acme/a*/x":      "NATS wildcard *",
		"priompt://acme/a>/x":      "NATS wildcard >",
		"priompt://acme/x ":        "trailing whitespace",
		"priompt://acme/.hidden/x": "segment starting with a dot",
		"priompt://acme/x/":        "trailing slash",
	}
	for u, why := range invalid {
		if err := URI(u); err == nil {
			t.Errorf("URI(%q) = nil, want error (%s)", u, why)
		}
	}
}

func TestOrg(t *testing.T) {
	cases := map[string]string{
		"priompt://acme/a/b": "acme",
		"priompt://acme":     "acme",
		"priompt://":         "",
	}
	for in, want := range cases {
		if got := Org(in); got != want {
			t.Errorf("Org(%q) = %q, want %q", in, got, want)
		}
	}
	// The prefix-boundary bug: "acme" must not be the org of "acmecorp/...".
	if Org("priompt://acmecorp/c/z") == Org("priompt://acme/a/x") {
		t.Error("acmecorp and acme must not share an org")
	}
}

// Unclosed and stray braces produce a template that passes the
// declared-vs-used check and then silently fails to interpolate.
func TestBraceTypos(t *testing.T) {
	bad := []struct{ tmpl, why string }{
		{"Hi {name.", "unclosed placeholder"},
		{"Hi {name}}.", "a slot followed by a lone } (str.format: Single '}' encountered)"},
		{"Hi {name, welcome to {org}.", "unclosed then a valid one"},
		{"Hi {}.", "empty placeholder"},
		{"Hi {na me}.", "space inside a placeholder"},
		{"Hi name}.", "unmatched closing brace"},
	}
	for _, c := range bad {
		if err := Template(c.tmpl, []string{"name", "org"}); err == nil {
			t.Errorf("Template(%q) = nil, want error (%s)", c.tmpl, c.why)
		}
	}

	// A multi-line template must be scanned in full, not just its first line.
	if err := Template("line one is fine\nline two has {oops\n", []string{"oops"}); err == nil {
		t.Error("unclosed brace on a later line was not caught")
	}
	if err := Template("Hi {name},\nwelcome to {org}.\n", []string{"name", "org"}); err != nil {
		t.Errorf("valid multi-line template rejected: %v", err)
	}
}

// {{ and }} are literal braces, exactly as in Python's str.format, so prompts
// with JSON examples can be published and template.format(...) still works.
func TestLiteralBraces(t *testing.T) {
	cases := []struct {
		tmpl  string
		slots []string
	}{
		{`Answer {question}. Reply as JSON: {{"answer": "..."}}`, []string{"question"}},
		{"Use a set like {{1, 2}} for {question}", []string{"question"}},
		{"Write {{name}} literally", nil},              // a literal "{name}", not a slot
		{"Braced value: {{{name}}}", []string{"name"}}, // literal {, slot, literal }
		{"{a}{b} and {a} again", []string{"a", "b"}},
	}
	for _, c := range cases {
		if err := Template(c.tmpl, c.slots); err != nil {
			t.Errorf("Template(%q, %v) = %v, want valid", c.tmpl, c.slots, err)
		}
		got, err := Slots(c.tmpl)
		if err != nil || !slices.Equal(got, c.slots) {
			t.Errorf("Slots(%q) = %v, %v; want %v", c.tmpl, got, err, c.slots)
		}
	}
	// A slot hidden inside an escape is not a slot, so declaring it is an error.
	if err := Template("Write {{name}} literally", []string{"name"}); err == nil {
		t.Error(`"{{name}}" counted as using slot name`)
	}
}
