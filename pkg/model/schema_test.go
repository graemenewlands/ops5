package model

import "testing"

func TestClassSchema(t *testing.T) {
	schema := NewClassSchema("Person", []string{"^name", "age", "job", "^salary"})

	if schema.Class != "person" {
		t.Fatalf("expected class 'person', got %s", schema.Class)
	}

	if len(schema.Attributes) != 4 {
		t.Fatalf("expected 4 attributes, got %d", len(schema.Attributes))
	}

	expected := []string{"name", "age", "job", "salary"}
	for i, exp := range expected {
		attr, ok := schema.AttributeAt(i)
		if !ok || attr != exp {
			t.Fatalf("at index %d expected %s, got %s (ok=%v)", i, exp, attr, ok)
		}
		idx, ok := schema.IndexOf(exp)
		if !ok || idx != i {
			t.Fatalf("IndexOf(%s) expected %d, got %d (ok=%v)", exp, i, idx, ok)
		}
	}

	if !schema.HasAttribute("name") || !schema.HasAttribute("^name") {
		t.Fatalf("expected HasAttribute('name') and HasAttribute('^name') to be true")
	}

	if schema.HasAttribute("nonexistent") {
		t.Fatalf("expected HasAttribute('nonexistent') to be false")
	}

	if _, ok := schema.AttributeAt(99); ok {
		t.Fatalf("expected AttributeAt(99) to return false")
	}
}

func TestClassSchemaVectorAttribute(t *testing.T) {
	schema := NewClassSchema("City", []string{"name", "location", "state", "country", "population"})
	schema.SetVectorAttribute("location", true)

	if !schema.IsVectorAttribute("location") || !schema.IsVectorAttribute("^location") {
		t.Fatalf("expected location to be vector attribute")
	}
	if schema.IsVectorAttribute("name") {
		t.Fatalf("expected name to not be vector attribute")
	}

	names := schema.VectorAttributeNames()
	if len(names) != 1 || names[0] != "location" {
		t.Fatalf("expected ['location'], got %v", names)
	}

	schema.SetVectorAttribute("location", false)
	if schema.IsVectorAttribute("location") {
		t.Fatalf("expected location to no longer be vector attribute")
	}
}
