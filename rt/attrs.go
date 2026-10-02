package rt

import (
	"fmt"
	"html"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// Attrs is an ordered list of attribute name/value pairs for @attrs($map).
type Attrs []AttrPair

// AttrPair is one HTML attribute.
type AttrPair struct {
	Name  string
	Value any
}

var attrNameOK = regexp.MustCompile(`^[A-Za-z_:][-A-Za-z0-9_:.]*$`)

var attrsURLNames = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true,
	"poster": true, "cite": true, "data": true,
}

// FormatAttrs renders a safe attribute bag for Strict @attrs($x).
// Output starts with a leading space when non-empty: ` name="value"`.
func FormatAttrs(v any) string {
	pairs := normalizeAttrs(v)
	if len(pairs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range pairs {
		name := strings.TrimSpace(p.Name)
		if !attrNameAllowed(name) {
			continue
		}
		lower := strings.ToLower(name)
		if skip, ok := attrsBoolSkip(p.Value); ok {
			if skip {
				continue
			}
			b.WriteByte(' ')
			b.WriteString(name)
			continue
		}
		val := stringifyAttrValue(p.Value)
		if attrsURLNames[lower] {
			val = EscapeURLAttr(val, true)
		} else {
			val = html.EscapeString(val)
		}
		b.WriteByte(' ')
		b.WriteString(name)
		b.WriteString(`="`)
		b.WriteString(val)
		b.WriteByte('"')
	}
	return b.String()
}

func attrNameAllowed(name string) bool {
	if name == "" || len(name) > 128 || !attrNameOK.MatchString(name) {
		return false
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "on") {
		return false
	}
	switch lower {
	case "srcdoc", "style", "srcset":
		return false
	}
	return !strings.HasPrefix(lower, "xmlns")
}

func attrsBoolSkip(v any) (skip bool, isBool bool) {
	if v == nil {
		return true, true
	}
	switch x := v.(type) {
	case bool:
		return !x, true
	case *bool:
		if x == nil {
			return true, true
		}
		return !*x, true
	}
	return false, false
}

func stringifyAttrValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case SafeHTML:
		return string(x) // still escaped by caller — never raw in attrs
	case []byte:
		return string(x)
	case fmt.Stringer:
		return x.String()
	default:
		rv := reflect.ValueOf(v)
		if rv.IsValid() && rv.Kind() == reflect.String {
			return rv.String()
		}
		return fmt.Sprint(v)
	}
}

func normalizeAttrs(v any) []AttrPair {
	if v == nil {
		return nil
	}
	switch a := v.(type) {
	case Attrs:
		return a
	case []AttrPair:
		return a
	case map[string]string:
		keys := make([]string, 0, len(a))
		for k := range a {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]AttrPair, 0, len(keys))
		for _, k := range keys {
			out = append(out, AttrPair{Name: k, Value: a[k]})
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(a))
		for k := range a {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]AttrPair, 0, len(keys))
		for _, k := range keys {
			out = append(out, AttrPair{Name: k, Value: a[k]})
		}
		return out
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() {
			return nil
		}
		if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
			keys := rv.MapKeys()
			strs := make([]string, 0, len(keys))
			for _, k := range keys {
				strs = append(strs, k.String())
			}
			sort.Strings(strs)
			out := make([]AttrPair, 0, len(strs))
			for _, k := range strs {
				out = append(out, AttrPair{Name: k, Value: rv.MapIndex(reflect.ValueOf(k)).Interface()})
			}
			return out
		}
		return nil
	}
}
