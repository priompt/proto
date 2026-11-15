// Package validate is the ACID "consistency" guarantee: a prompt that fails
// here never enters a valid state (rejected at commit, never served).
package validate

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Scheme is the required prefix of every prompt URI.
const Scheme = "priompt://"

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
	used, err := scan(template)
	if err != nil {
		return err
	}

	found := map[string]bool{}
	for _, s := range used {
		found[s] = true
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

// Slots returns the slot names a template uses, in order of first use, with
// the same rules Template validates against. Clients that derive slots from a
// template (promptctl, the web UI) should use it rather than a regexp, so that
// "{{name}}" — a literal "{name}" — is not mistaken for a slot.
func Slots(template string) ([]string, error) {
	used, err := scan(template)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range used {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, nil
}

// scan walks a template once and returns the slots it uses. The syntax is
// Python's str.format, which both SDK READMEs demonstrate:
//
//   - {name} is a slot (letters, digits, underscore);
//   - {{ and }} are a literal { and }, so JSON examples and other literal
//     braces are written {{"answer": "..."}} and template.format(...) renders
//     them as single braces;
//   - any other { or } is a typo and is rejected, because an unclosed {name is
//     just literal text to a formatter and the slot would silently disappear.
func scan(t string) ([]string, error) {
	var slots []string
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case '{':
			if i+1 < len(t) && t[i+1] == '{' {
				i++ // literal {
				continue
			}
			j := i + 1
			for j < len(t) && isSlotByte(t[j]) {
				j++
			}
			if j >= len(t) || t[j] != '}' || j == i+1 {
				return nil, fmt.Errorf("template has an unclosed or malformed placeholder near %q "+
					"(write {{ for a literal brace)", trunc(t[i:]))
			}
			slots = append(slots, t[i+1:j])
			i = j
		case '}':
			if i+1 < len(t) && t[i+1] == '}' {
				i++ // literal }
				continue
			}
			return nil, fmt.Errorf("template has an unmatched %q near %q (write }} for a literal brace)",
				"}", trunc(t[max(0, i-8):]))
		}
	}
	return slots, nil
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
