// Package validate is the ACID "consistency" guarantee: a prompt that fails
// here never enters a valid state (rejected at commit, never served).
package validate

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Scheme is the required prefix of every prompt URI.
const Scheme = "priompt://"

// Slots are written as {name} in the template.
var placeholderRe = regexp.MustCompile(`\{(\w+)\}`)

// segmentRe is the grammar for one URI path segment: letters, digits, and the
// separators . _ - are allowed, but a segment may not be empty, may not be "."
// or "..", and may not contain characters that are meaningful to the pub/sub
// subject mapping (. is remapped, * and > are NATS wildcards) or to a
// filesystem path (the CLI maps a URI onto a file path in a git repo).
var segmentRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)

// URI checks that a prompt address is well formed. The first path segment is
// the owning org and is the authorization boundary, so this is a security
// check, not a cosmetic one: everything downstream (org scoping, the NATS
// subject, the CLI's URI-to-file-path mapping) assumes this shape.
func URI(uri string) error {
	if strings.TrimSpace(uri) == "" {
		return errors.New("uri is empty")
	}
	if uri != strings.TrimSpace(uri) {
		return errors.New("uri has leading or trailing whitespace")
	}
	if !strings.HasPrefix(uri, Scheme) {
		return fmt.Errorf("uri must start with %q", Scheme)
	}
	rest := strings.TrimPrefix(uri, Scheme)
	if rest == "" {
		return errors.New("uri has no org segment")
	}
	segs := strings.Split(rest, "/")
	for i, s := range segs {
		switch {
		case s == "":
			return errors.New("uri has an empty path segment")
		case s == "." || s == "..":
			return fmt.Errorf("uri path segment %q is not allowed", s)
		case !segmentRe.MatchString(s):
			return fmt.Errorf("uri path segment %q contains illegal characters", s)
		}
		if i == 0 && len(segs) == 1 {
			// "priompt://acme" is an org with no prompt path.
			return errors.New("uri has an org but no prompt path")
		}
	}
	return nil
}

// Org returns the owning org of a well-formed URI — the first path segment.
// It returns "" for a URI that does not carry one. Authorization must compare
// this parsed value, never a string prefix of the raw URI.
func Org(uri string) string {
	rest := strings.TrimPrefix(uri, Scheme)
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

// Template checks a template body on its own — well-formedness of the text and
// of its {placeholders}, independent of the address it will be stored at.
func Template(template string, slots []string) error {
	if strings.TrimSpace(template) == "" {
		return errors.New("template is empty")
	}
	if err := braces(template); err != nil {
		return err
	}

	found := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(template, -1) {
		found[m[1]] = true
	}
	declared := map[string]bool{}
	for _, s := range slots {
		if strings.TrimSpace(s) == "" {
			return errors.New("slot name is empty")
		}
		declared[s] = true
	}

	var missing, extra []string
	for s := range found {
		if !declared[s] {
			missing = append(missing, s)
		}
	}
	for s := range declared {
		if !found[s] {
			extra = append(extra, s)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		return fmt.Errorf("template uses undeclared slots: %s", strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		return fmt.Errorf("declared slots never used in template: %s", strings.Join(extra, ", "))
	}
	return nil
}

// Prompt checks the address and the body together. This is the one definition
// of "valid" that the server runs on write and on serve, and that promptctl
// runs at commit time.
func Prompt(uri, template string, slots []string) error {
	if err := URI(uri); err != nil {
		return err
	}
	return Template(template, slots)
}

// braces scans for the two placeholder typos that comparing declared against
// used slots can never catch, because neither produces a placeholder at all.
//
//   - {{name}} — the syntax several other template engines use. placeholderRe
//     matches the inner {name}, so the prompt would validate; but Python's
//     str.format (what both SDK READMEs demonstrate) reads "{{" as an escaped
//     literal brace and substitutes nothing. Silent, and only visible as
//     degraded model output.
//   - {name    — an unclosed brace is just literal text, so the slot silently
//     disappears from the template.
func braces(t string) error {
	for i := 0; i < len(t); i++ {
		if t[i] != '{' {
			continue
		}
		if i+1 < len(t) && t[i+1] == '{' {
			return fmt.Errorf("template uses doubled braces near %q: slots are written {name}; "+
				"{{name}} is an escaped literal brace to a formatter, not a slot", trunc(t[i:]))
		}
		j := i + 1
		for j < len(t) && isSlotByte(t[j]) {
			j++
		}
		if j >= len(t) || t[j] != '}' || j == i+1 {
			return fmt.Errorf("template has an unclosed or malformed placeholder near %q", trunc(t[i:]))
		}
		i = j
	}
	// A closing brace with no opener is equally a typo.
	for i := 0; i < len(t); i++ {
		if t[i] != '}' {
			continue
		}
		k := i - 1
		for k >= 0 && isSlotByte(t[k]) {
			k--
		}
		if k < 0 || t[k] != '{' || k == i-1 {
			return fmt.Errorf("template has an unmatched %q near %q", "}", trunc(t[max(0, i-8):]))
		}
	}
	return nil
}

func isSlotByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

func trunc(s string) string {
	const max = 40
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
