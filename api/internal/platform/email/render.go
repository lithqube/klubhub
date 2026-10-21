package email

import (
	"regexp"
	"strings"
)

// templateVar matches one {{key}} variable in a template.
var templateVar = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// Render substitutes {{key}} variables in text. Unknown keys render as empty.
// Values are inserted verbatim (multi-line notes keep their line breaks), so
// use RenderSubject for anything that becomes a header.
//
// Substitution is a single pass: a substituted value is data, never
// re-scanned, so a value that itself contains "{{...}}" is inserted as-is.
func Render(text string, vars map[string]string) string {
	return render(text, vars, false)
}

// RenderSubject is Render for header values: CR/LF is stripped from every
// substituted value (header-injection defense — SPEC §7 v2.1). Values are
// untrusted (contact names, notes). The template's own newlines are untouched.
func RenderSubject(text string, vars map[string]string) string {
	return render(text, vars, true)
}

func render(text string, vars map[string]string, flattenValues bool) string {
	flatten := strings.NewReplacer("\r", " ", "\n", " ")
	return templateVar.ReplaceAllStringFunc(text, func(m string) string {
		v, ok := vars[m[2:len(m)-2]]
		if !ok {
			return ""
		}
		if flattenValues {
			return flatten.Replace(v)
		}
		return v
	})
}
