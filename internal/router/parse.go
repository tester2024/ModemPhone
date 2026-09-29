package router

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// decodeJSONLoose parses JSON that the firmware emits with a UTF-8 byte order
// mark and stray control characters, which encoding/json rejects.
func decodeJSONLoose(s string, v any) error {
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.TrimSpace(s)
	return json.Unmarshal([]byte(s), v)
}

// The firmware inlines page data as JavaScript string literals, for example:
//
//	var smsListInfo = "…";
//
// It also appends <br><br> where the message had line breaks, and may emit
// unescaped quotes from message text, so the literal is scanned by hand rather
// than with a regular expression that assumes no embedded quotes.
var jsStringRe = regexp.MustCompile(`(?s)var\s+` + `([A-Za-z_][A-Za-z0-9_]*)` + `\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`)

// jsQuotedRe matches a JavaScript string assignment, accepting either quote
// style. The firmware is inconsistent: message lists use double quotes while
// some settings variables use single quotes.
var jsQuotedRe = regexp.MustCompile(`(?s)var\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')\s*;`)

// extractJSString returns the value of the named JavaScript string variable,
// whichever quote style the firmware used for it.
func extractJSString(page, name string) (string, bool) {
	quoted := regexp.QuoteMeta(name)
	re := regexp.MustCompile(`(?s)var\s+` + quoted + `\s*=\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')\s*;`)
	m := re.FindStringSubmatch(page)
	if m == nil {
		return "", false
	}
	if m[1] != "" {
		return unescapeJS(m[1]), true
	}
	return unescapeJS(m[2]), true
}

// unescapeJS turns the escape sequences the firmware emits into real
// characters. Unknown escapes keep the escaped character.
func unescapeJS(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\', '"', '/', '\'':
			b.WriteByte(s[i])
		case 'u':
			if i+4 < len(s) {
				var v rune
				ok := true
				for k := 1; k <= 4; k++ {
					d, good := hexVal(s[i+k])
					if !good {
						ok = false
						break
					}
					v = v<<4 | rune(d)
				}
				if ok {
					b.WriteRune(v)
					i += 4
					continue
				}
			}
			b.WriteByte('\\')
			b.WriteByte(s[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

// cleanBody turns the markup the firmware puts inside message bodies into
// plain text. It converts the <br> pairs it uses for newlines, strips any
// other tags and decodes the handful of entities that occur.
func cleanBody(s string) string {
	if s == "" {
		return s
	}
	// The firmware writes a line break as <br><br>.
	s = strings.ReplaceAll(s, "<br><br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&apos;", "'",
	).Replace(s)
	return strings.TrimRight(s, "\n")
}

var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// sanitizeUTF8 drops invalid byte sequences so the UI never receives text Go
// cannot hold, while leaving valid Persian untouched.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "")
}

// The firmware serialises a list of records as
//
//	idx}-{stat}-{number}-{time}-{content}-{|,|idx}-{stat}-…
//
// so records split on the "|,|" separator and fields on "}-{". A trailing
// separator leaves an empty final element, which is skipped.
func parseRecords(list string) [][]string {
	if strings.TrimSpace(list) == "" {
		return nil
	}
	var out [][]string
	for _, rec := range strings.Split(list, "|,|") {
		if strings.TrimSpace(rec) == "" {
			continue
		}
		out = append(out, strings.Split(rec, "}-{"))
	}
	return out
}
