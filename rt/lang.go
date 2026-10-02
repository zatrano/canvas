package rt

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// FormatLang applies translation replacements. Replacement values are always
// HTML-escaped before insertion (all escape modes). The catalog string itself
// is left unchanged — callers decide trusted vs escaped emission by context.
func FormatLang(msg string, repl map[string]any) string {
	if len(repl) == 0 {
		return msg
	}
	keys := make([]string, 0, len(repl))
	for k := range repl {
		keys = append(keys, k)
	}
	// Longer keys first so :username wins over :user.
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		esc := html.EscapeString(fmt.Sprint(repl[k]))
		msg = strings.ReplaceAll(msg, ":"+k, esc)
		msg = strings.ReplaceAll(msg, "{"+k+"}", esc)
	}
	return msg
}

// LangTrustedText reports whether @lang output may be emitted raw
// catalog HTML). Attribute / non-text contexts always require escaping.
func LangTrustedText(ctxName string, escapeCatalog bool) bool {
	return !escapeCatalog && ctxName == "text"
}

// WriteLang emits a formatted translation according to escape kind and catalog policy.
func WriteLang(w *Writer, msg string, kind EscapeKind, ctxName string, escapeCatalog bool) {
	if LangTrustedText(ctxName, escapeCatalog) && kind == EscHTML {
		w.WriteString(msg)
		return
	}
	writeByKind(w, msg, kind)
}
