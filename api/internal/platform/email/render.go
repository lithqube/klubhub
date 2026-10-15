package email

import (
	"regexp"
	"strings"
)

// templateVar matches one {{key}} variable in a template.
var templateVar = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// Render substitutes {{key}} variables in text. Unknown keys render as empty.
// Values are untrusted (contact names, notes): CR/LF is stripped from every
// substituted value before replacement (header-injection defense for the
// subject — SPEC §7 v2.1). The template's own newlines are untouched.
//
// Substitution is a single pass: a substituted value is data, never
// re-scanned, so a value that itself contains "{{...}}" is inserted as-is.
func Render(text string, vars map[string]string) string {
	if len(vars) == 0 {
		return text
	}
	flatten := strings.NewReplacer("\r", " ", "\n", " ")
	return templateVar.ReplaceAllStringFunc(text, func(m string) string {
		v, ok := vars[m[2:len(m)-2]]
		if !ok {
			return ""
		}
		return flatten.Replace(v)
	})
}
