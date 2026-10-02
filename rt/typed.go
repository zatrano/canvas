package rt

// Typed stream API — what `canvas gen` emits for production builds.
// Direct []byte writes, no map[string]any, no io.Writer indirection.
// This is the absolute floor path for typed list-page streams.

// ListItem is the typed row used by StreamListPage.
type ListItem struct {
	Name string
}

// StreamListPage writes the canonical list page shape with typed data.
// Equivalent HTML: <h1>{{title}}</h1> + N× <li>{{name}}</li>
func StreamListPage(w *Writer, title string, items []ListItem) {
	w.WriteString("<h1>")
	writeEscapedString(w, title)
	w.WriteString("</h1>")
	for i := 0; i < len(items); i++ {
		w.WriteString("<li>")
		writeEscapedString(w, items[i].Name)
		w.WriteString("</li>")
	}
}

// StreamListNames is the ultra-flat variant (string slice, no struct field load).
func StreamListNames(w *Writer, title string, names []string) {
	w.WriteString("<h1>")
	writeEscapedString(w, title)
	w.WriteString("</h1>")
	for i := 0; i < len(names); i++ {
		w.WriteString("<li>")
		writeEscapedString(w, names[i])
		w.WriteString("</li>")
	}
}
