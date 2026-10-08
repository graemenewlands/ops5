package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// SchemaManifest provides a complete, exportable manifest of all registered element schemas,
// their field types, vector flags, and structural fingerprints.
type SchemaManifest struct {
	GeneratedAt time.Time                     `json:"generated_at"`
	Schemas     map[string]ClassSchemaSummary `json:"schemas"`
}

// ClassSchemaSummary captures the serialized schema state of an OPS5 element class.
type ClassSchemaSummary struct {
	Class       string            `json:"class"`
	Fingerprint string            `json:"fingerprint"`
	Attributes  []string          `json:"attributes"`
	FieldTypes  map[string]string `json:"field_types,omitempty"`
	VectorAttrs []string          `json:"vector_attrs,omitempty"`
}

// NewSchemaManifest creates an empty SchemaManifest timestamped to UTC now.
func NewSchemaManifest() *SchemaManifest {
	return &SchemaManifest{
		GeneratedAt: time.Now().UTC(),
		Schemas:     make(map[string]ClassSchemaSummary),
	}
}

// AddSchema adds a ClassSchema to the manifest.
func (m *SchemaManifest) AddSchema(s *ClassSchema) {
	if s == nil {
		return
	}
	fp := s.Fingerprint
	if fp == "" {
		fp = s.ComputeFingerprint()
	}
	var fieldTypes map[string]string
	if len(s.FieldTypes) > 0 {
		fieldTypes = make(map[string]string, len(s.FieldTypes))
		for k, v := range s.FieldTypes {
			fieldTypes[k] = v
		}
	}
	m.Schemas[s.Class] = ClassSchemaSummary{
		Class:       s.Class,
		Fingerprint: fp,
		Attributes:  append([]string(nil), s.Attributes...),
		FieldTypes:  fieldTypes,
		VectorAttrs: s.VectorAttributeNames(),
	}
}

// ToJSON exports the schema manifest as formatted JSON.
func (m *SchemaManifest) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// ParseSchemaManifestJSON parses JSON bytes into a SchemaManifest.
func ParseSchemaManifestJSON(data []byte) (*SchemaManifest, error) {
	var m SchemaManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate verifies that this manifest matches another target manifest.
// Returns an error if any schema in the target manifest has mismatched fingerprints or missing definitions.
func (m *SchemaManifest) Validate(other *SchemaManifest) error {
	if other == nil {
		return fmt.Errorf("target manifest is nil")
	}
	for class, targetSummary := range other.Schemas {
		currSummary, exists := m.Schemas[class]
		if !exists {
			return fmt.Errorf("manifest schema %q is missing", class)
		}
		if currSummary.Fingerprint != targetSummary.Fingerprint {
			return fmt.Errorf("schema drift detected for class %q: current fingerprint %s != target fingerprint %s",
				class, currSummary.Fingerprint, targetSummary.Fingerprint)
		}
	}
	return nil
}
