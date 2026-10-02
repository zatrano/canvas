//go:build canvas_poison

package rt

// poisonOnRelease fills the writer's buffer capacity with 0xDE so retained
// Bytes() aliases are detectably corrupted after ReleaseWriter.
func poisonOnRelease(w *Writer) {
	if w == nil {
		return
	}
	b := w.b
	c := cap(b)
	if c == 0 {
		return
	}
	full := b[:c]
	for i := range full {
		full[i] = 0xDE
	}
	w.b = full[:0]
}
