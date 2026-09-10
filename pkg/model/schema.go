package model

import "strings"

// ClassSchema represents an OPS5 element class definition declared via (literalize ...).
type ClassSchema struct {
	Class      string         // Normalized class name (lowercase)
	Attributes []string       // Ordered list of normalized attribute names
	attrMap    map[string]int // Attribute name -> 0-based index
}

// NewClassSchema creates a new ClassSchema.
func NewClassSchema(class string, attributes []string) *ClassSchema {
	normClass := strings.ToLower(class)
	normAttrs := make([]string, 0, len(attributes))
	attrMap := make(map[string]int, len(attributes))

	for _, a := range attributes {
		norm := NormalizeAttribute(a)
		if norm == "" {
			continue
		}
		if _, exists := attrMap[norm]; !exists {
			attrMap[norm] = len(normAttrs)
			normAttrs = append(normAttrs, norm)
			attrMap[norm] = len(normAttrs) - 1
		}
	}

	return &ClassSchema{
		Class:      normClass,
		Attributes: normAttrs,
		attrMap:    attrMap,
	}
}

// HasAttribute returns true if the attribute is declared in the schema.
func (s *ClassSchema) HasAttribute(attr string) bool {
	_, ok := s.attrMap[NormalizeAttribute(attr)]
	return ok
}

// AttributeAt returns the attribute name at 0-based position.
func (s *ClassSchema) AttributeAt(index int) (string, bool) {
	if index < 0 || index >= len(s.Attributes) {
		return "", false
	}
	return s.Attributes[index], true
}

// IndexOf returns the 0-based index of an attribute name.
func (s *ClassSchema) IndexOf(attr string) (int, bool) {
	idx, ok := s.attrMap[NormalizeAttribute(attr)]
	return idx, ok
}
