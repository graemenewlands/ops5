package model

import "strings"

// ClassSchema represents an OPS5 element class definition declared via (literalize ...).
type ClassSchema struct {
	Class            string          // Normalized class name (lowercase)
	Attributes       []string        // Ordered list of normalized attribute names
	VectorAttributes map[string]bool // Attribute name -> true if declared as vector-attribute
	attrMap          map[string]int  // Attribute name -> 0-based index
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
			normAttrs = append(normAttrs, norm)
			attrMap[norm] = len(normAttrs) - 1
		}
	}

	return &ClassSchema{
		Class:            normClass,
		Attributes:       normAttrs,
		VectorAttributes: make(map[string]bool),
		attrMap:          attrMap,
	}
}

// HasAttribute returns true if the attribute is declared in the schema.
func (s *ClassSchema) HasAttribute(attr string) bool {
	_, ok := s.attrMap[NormalizeAttribute(attr)]
	return ok
}

// AddAttribute appends a new attribute to the schema if not already present.
func (s *ClassSchema) AddAttribute(attr string) {
	norm := NormalizeAttribute(attr)
	if norm == "" {
		return
	}
	if _, exists := s.attrMap[norm]; !exists {
		s.Attributes = append(s.Attributes, norm)
		s.attrMap[norm] = len(s.Attributes) - 1
	}
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

// IsVectorAttribute returns true if the attribute is declared as a vector-attribute.
func (s *ClassSchema) IsVectorAttribute(attr string) bool {
	if s.VectorAttributes == nil {
		return false
	}
	return s.VectorAttributes[NormalizeAttribute(attr)]
}

// SetVectorAttribute marks or unmarks an attribute as a vector-attribute.
func (s *ClassSchema) SetVectorAttribute(attr string, isVector bool) {
	if s.VectorAttributes == nil {
		s.VectorAttributes = make(map[string]bool)
	}
	norm := NormalizeAttribute(attr)
	if isVector {
		s.VectorAttributes[norm] = true
	} else {
		delete(s.VectorAttributes, norm)
	}
}

// VectorAttributeNames returns a list of attribute names designated as vector attributes.
func (s *ClassSchema) VectorAttributeNames() []string {
	var res []string
	for _, a := range s.Attributes {
		if s.VectorAttributes[a] {
			res = append(res, a)
		}
	}
	return res
}
