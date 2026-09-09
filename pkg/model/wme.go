package model

import (
	"fmt"
	"sort"
	"strings"
)

// NormalizeAttribute ensures attribute names are uniform (stripping leading '^' if provided).
func NormalizeAttribute(attr string) string {
	return strings.TrimPrefix(attr, "^")
}

// WME represents a Working Memory Element in OPS5.
// In OPS5, each WME has an immutable timetag, a class name, and attribute-value pairs.
type WME struct {
	Timetag    int64
	Class      string
	Attributes map[string]Value
}

// NewWME creates a new WME with the given timetag, class, and attributes.
func NewWME(timetag int64, class string, attrs map[string]Value) *WME {
	normalized := make(map[string]Value, len(attrs))
	for k, v := range attrs {
		normalized[NormalizeAttribute(k)] = v
	}
	return &WME{
		Timetag:    timetag,
		Class:      class,
		Attributes: normalized,
	}
}

// Get returns the Value for the given attribute, if present.
func (w *WME) Get(attr string) (Value, bool) {
	val, ok := w.Attributes[NormalizeAttribute(attr)]
	return val, ok
}

// Has returns true if the attribute exists on this WME.
func (w *WME) Has(attr string) bool {
	_, ok := w.Attributes[NormalizeAttribute(attr)]
	return ok
}

// Clone creates a shallow copy of the WME with its attributes map copied.
func (w *WME) Clone() *WME {
	attrs := make(map[string]Value, len(w.Attributes))
	for k, v := range w.Attributes {
		attrs[k] = v
	}
	return &WME{
		Timetag:    w.Timetag,
		Class:      w.Class,
		Attributes: attrs,
	}
}

// String returns a readable representation of the WME, e.g.:
// (12: goal ^priority 10 ^status active)
func (w *WME) String() string {
	var keys []string
	for k := range w.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("(%d: %s", w.Timetag, w.Class))
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(" ^%s %s", k, w.Attributes[k].String()))
	}
	b.WriteString(")")
	return b.String()
}
