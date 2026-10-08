package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ClassSchema represents an OPS5 element class definition declared via (literalize ...).
type ClassSchema struct {
	Class            string            // Normalized class name (lowercase)
	Attributes       []string          // Ordered list of normalized attribute names
	VectorAttributes map[string]bool   // Attribute name -> true if declared as vector-attribute
	FieldTypes       map[string]string // Attribute name -> type name (e.g. "int64", "float64", "string", "symbol", "boolean", "date", "datetime", "dateutctime", "vector")
	Fingerprint      string            // Deterministic SHA-256 structural fingerprint representing complete schema state
	attrMap          map[string]int    // Attribute name -> 0-based index
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

	s := &ClassSchema{
		Class:            normClass,
		Attributes:       normAttrs,
		VectorAttributes: make(map[string]bool),
		FieldTypes:       make(map[string]string),
		attrMap:          attrMap,
	}
	s.ComputeFingerprint()
	return s
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
		s.ComputeFingerprint()
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
	s.ComputeFingerprint()
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

// SetFieldType records the type descriptor for an attribute and updates the structural fingerprint.
func (s *ClassSchema) SetFieldType(attr, typeName string) {
	if s.FieldTypes == nil {
		s.FieldTypes = make(map[string]string)
	}
	s.FieldTypes[NormalizeAttribute(attr)] = typeName
	s.ComputeFingerprint()
}

// FieldType returns the recorded type descriptor for an attribute, or "" if not recorded.
func (s *ClassSchema) FieldType(attr string) string {
	if s.FieldTypes == nil {
		return ""
	}
	return s.FieldTypes[NormalizeAttribute(attr)]
}

// ComputeFingerprint calculates and stores the deterministic SHA-256 fingerprint for this schema.
func (s *ClassSchema) ComputeFingerprint() string {
	var b strings.Builder
	b.WriteString("class:")
	b.WriteString(s.Class)
	b.WriteString("\n")

	sortedAttrs := make([]string, len(s.Attributes))
	copy(sortedAttrs, s.Attributes)
	sort.Strings(sortedAttrs)

	for _, a := range sortedAttrs {
		ft := ""
		if s.FieldTypes != nil {
			ft = s.FieldTypes[a]
		}
		isVec := false
		if s.VectorAttributes != nil {
			isVec = s.VectorAttributes[a]
		}
		fmt.Fprintf(&b, "attr:%s|type:%s|vec:%t\n", a, ft, isVec)
	}

	hash := sha256.Sum256([]byte(b.String()))
	s.Fingerprint = hex.EncodeToString(hash[:])
	return s.Fingerprint
}

// Clone creates a deep copy of the ClassSchema.
func (s *ClassSchema) Clone() *ClassSchema {
	if s == nil {
		return nil
	}
	attrs := append([]string(nil), s.Attributes...)
	vecAttrs := make(map[string]bool, len(s.VectorAttributes))
	for k, v := range s.VectorAttributes {
		vecAttrs[k] = v
	}
	fieldTypes := make(map[string]string, len(s.FieldTypes))
	for k, v := range s.FieldTypes {
		fieldTypes[k] = v
	}
	attrMap := make(map[string]int, len(s.attrMap))
	for k, v := range s.attrMap {
		attrMap[k] = v
	}
	return &ClassSchema{
		Class:            s.Class,
		Attributes:       attrs,
		VectorAttributes: vecAttrs,
		FieldTypes:       fieldTypes,
		Fingerprint:      s.Fingerprint,
		attrMap:          attrMap,
	}
}
