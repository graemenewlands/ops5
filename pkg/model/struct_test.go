package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/graemenewlands/ops5/pkg/model"
)

type CustomerOrder struct {
	Timetag     int64      `ops5:",timetag"`
	ID          int64      `ops5:"order_id"`
	Customer    string     `ops5:"customer"`
	Status      string     `ops5:"status,symbol"`
	Total       float64    `ops5:"total"`
	IsExpedited bool       `ops5:"is_expedited"`
	Tags        []string   `ops5:"tags,vector"`
	ItemsCount  int        `ops5:"items_count"`
	CreatedAt   time.Time  `ops5:"created_at,utc"`
	DueDate     time.Time  `ops5:"due_date,date"`
	ShippedAt   *time.Time `ops5:"shipped_at,datetime,omitempty"`
	Notes       string     `ops5:"notes,omitempty"`
	Ignored     string     `ops5:"-"`
}

func (CustomerOrder) OPS5ClassName() string {
	return "order"
}

type OrderWithoutInterface struct {
	ID    int64   `ops5:"id,class=custom_order"`
	Total float64 `ops5:"total"`
}

type BaseEntity struct {
	ID        int64     `ops5:"id"`
	CreatedAt time.Time `ops5:"created_at,utc"`
}

type EmbeddedItem struct {
	BaseEntity
	Name  string  `ops5:"name"`
	Price float64 `ops5:"price"`
}

func TestClassSchemaFromStruct(t *testing.T) {
	schema, err := model.ClassSchemaFromStruct(CustomerOrder{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if schema.Class != "order" {
		t.Errorf("expected class 'order', got %q", schema.Class)
	}

	expectedAttrs := []string{
		"order_id", "customer", "status", "total", "is_expedited",
		"tags", "items_count", "created_at", "due_date", "shipped_at", "notes",
	}

	for _, attr := range expectedAttrs {
		if !schema.HasAttribute(attr) {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}

	// Timetag and Ignored should NOT be in attributes
	if schema.HasAttribute("timetag") {
		t.Errorf("schema should not have timetag attribute")
	}
	if schema.HasAttribute("ignored") {
		t.Errorf("schema should not have ignored attribute")
	}

	// Vector attribute check
	if !schema.IsVectorAttribute("tags") {
		t.Errorf("expected 'tags' to be vector attribute")
	}

	// Field types check
	if schema.FieldType("order_id") != "int64" {
		t.Errorf("expected order_id type 'int64', got %q", schema.FieldType("order_id"))
	}
	if schema.FieldType("status") != "symbol" {
		t.Errorf("expected status type 'symbol', got %q", schema.FieldType("status"))
	}
	if schema.FieldType("total") != "float64" {
		t.Errorf("expected total type 'float64', got %q", schema.FieldType("total"))
	}
	if schema.FieldType("created_at") != "dateutctime" {
		t.Errorf("expected created_at type 'dateutctime', got %q", schema.FieldType("created_at"))
	}
	if schema.FieldType("due_date") != "date" {
		t.Errorf("expected due_date type 'date', got %q", schema.FieldType("due_date"))
	}
	if schema.FieldType("shipped_at") != "datetime" {
		t.Errorf("expected shipped_at type 'datetime', got %q", schema.FieldType("shipped_at"))
	}

	// Structural fingerprint must be non-empty and deterministic
	fp := schema.Fingerprint
	if fp == "" {
		t.Errorf("expected non-empty structural fingerprint")
	}

	schema2, _ := model.ClassSchemaFromStruct(&CustomerOrder{})
	if schema2.Fingerprint != fp {
		t.Errorf("expected matching fingerprints, got %s and %s", fp, schema2.Fingerprint)
	}
}

func TestClassSchemaClassTagOverride(t *testing.T) {
	schema, err := model.ClassSchemaFromStruct(OrderWithoutInterface{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema.Class != "custom_order" {
		t.Errorf("expected class 'custom_order', got %q", schema.Class)
	}
}

func TestClassSchemaEmbeddedStruct(t *testing.T) {
	schema, err := model.ClassSchemaFromStruct(EmbeddedItem{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema.Class != "embeddeditem" {
		t.Errorf("expected class 'embeddeditem', got %q", schema.Class)
	}
	if !schema.HasAttribute("id") || !schema.HasAttribute("created_at") || !schema.HasAttribute("name") || !schema.HasAttribute("price") {
		t.Errorf("expected embedded attributes to be present, got %v", schema.Attributes)
	}
}

func TestMarshalWME_And_UnmarshalWME(t *testing.T) {
	utcTime := time.Date(2026, 10, 7, 14, 30, 0, 0, time.UTC)
	dueDate := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	shippedAt := time.Date(2026, 10, 8, 9, 0, 0, 0, time.Local)

	src := CustomerOrder{
		Timetag:     999, // Should be ignored during marshal
		ID:          1001,
		Customer:    "Acme Corp",
		Status:      "pending",
		Total:       149.95,
		IsExpedited: true,
		Tags:        []string{"urgent", "fragile"},
		ItemsCount:  3,
		CreatedAt:   utcTime,
		DueDate:     dueDate,
		ShippedAt:   &shippedAt,
		Notes:       "Deliver after 10am",
		Ignored:     "do not export",
	}

	className, attrs, err := model.MarshalWME(src)
	if err != nil {
		t.Fatalf("MarshalWME error: %v", err)
	}

	if className != "order" {
		t.Errorf("expected class 'order', got %q", className)
	}

	// Verify timetag and ignored were not marshaled into attributes
	if _, ok := attrs["timetag"]; ok {
		t.Errorf("timetag should not be marshaled as an attribute")
	}
	if _, ok := attrs["ignored"]; ok {
		t.Errorf("ignored field should not be marshaled")
	}

	// Verify types of marshaled attributes
	if attrs["order_id"].Type() != model.TypeInteger || attrs["order_id"].Raw().(int64) != 1001 {
		t.Errorf("order_id mismatch: %v", attrs["order_id"])
	}
	if attrs["status"].Type() != model.TypeSymbol || attrs["status"].Raw().(string) != "pending" {
		t.Errorf("status symbol mismatch: %v", attrs["status"])
	}
	if attrs["created_at"].Type() != model.TypeDateUTCTime {
		t.Errorf("created_at type mismatch: %v", attrs["created_at"].Type())
	}
	if attrs["due_date"].Type() != model.TypeDate {
		t.Errorf("due_date type mismatch: %v", attrs["due_date"].Type())
	}
	if attrs["shipped_at"].Type() != model.TypeDateTime {
		t.Errorf("shipped_at type mismatch: %v", attrs["shipped_at"].Type())
	}
	if attrs["tags"].Type() != model.TypeVector {
		t.Errorf("tags type mismatch: %v", attrs["tags"].Type())
	}

	// Construct WME with timetag 42
	wme := model.NewWME(42, className, attrs)

	// Unmarshal back into a fresh struct
	var dest CustomerOrder
	if err := model.UnmarshalWME(wme, &dest); err != nil {
		t.Fatalf("UnmarshalWME error: %v", err)
	}

	if dest.Timetag != 42 {
		t.Errorf("expected timetag 42, got %d", dest.Timetag)
	}
	if dest.ID != 1001 {
		t.Errorf("expected ID 1001, got %d", dest.ID)
	}
	if dest.Customer != "Acme Corp" {
		t.Errorf("expected customer Acme Corp, got %q", dest.Customer)
	}
	if dest.Status != "pending" {
		t.Errorf("expected status pending, got %q", dest.Status)
	}
	if dest.Total != 149.95 {
		t.Errorf("expected total 149.95, got %f", dest.Total)
	}
	if !dest.IsExpedited {
		t.Errorf("expected IsExpedited true")
	}
	if len(dest.Tags) != 2 || dest.Tags[0] != "urgent" || dest.Tags[1] != "fragile" {
		t.Errorf("tags mismatch: %v", dest.Tags)
	}
	if dest.ItemsCount != 3 {
		t.Errorf("expected ItemsCount 3, got %d", dest.ItemsCount)
	}
	if !dest.CreatedAt.Equal(utcTime) {
		t.Errorf("createdAt mismatch: expected %v, got %v", utcTime, dest.CreatedAt)
	}
	if dest.DueDate.Year() != 2026 || dest.DueDate.Month() != 10 || dest.DueDate.Day() != 15 {
		t.Errorf("dueDate mismatch: %v", dest.DueDate)
	}
	if dest.ShippedAt == nil || !dest.ShippedAt.Equal(shippedAt) {
		t.Errorf("shippedAt mismatch: %v", dest.ShippedAt)
	}
	if dest.Notes != "Deliver after 10am" {
		t.Errorf("notes mismatch: %q", dest.Notes)
	}
	if dest.Ignored != "" {
		t.Errorf("expected ignored to remain zero, got %q", dest.Ignored)
	}

	// Also verify via (*WME).Unmarshal
	var dest2 CustomerOrder
	if err := wme.Unmarshal(&dest2); err != nil {
		t.Fatalf("wme.Unmarshal error: %v", err)
	}
	if dest2.ID != 1001 || dest2.Timetag != 42 {
		t.Errorf("dest2 mismatch: %+v", dest2)
	}
}

func TestMarshalWME_OmitEmpty(t *testing.T) {
	src := CustomerOrder{
		ID:       55,
		Customer: "Alice",
		// Status is empty string, but not omitempty -> should be marshaled
		Status: "",
		// Notes has omitempty -> should NOT be marshaled
		Notes: "",
		// ShippedAt is nil pointer with omitempty -> should NOT be marshaled
		ShippedAt: nil,
	}

	_, attrs, err := model.MarshalWME(src)
	if err != nil {
		t.Fatalf("MarshalWME error: %v", err)
	}

	if _, ok := attrs["notes"]; ok {
		t.Errorf("expected 'notes' to be omitted")
	}
	if _, ok := attrs["shipped_at"]; ok {
		t.Errorf("expected 'shipped_at' to be omitted")
	}
	if _, ok := attrs["status"]; !ok {
		t.Errorf("expected 'status' to be present because it lacks omitempty")
	}
}

func TestUnmarshalWME_UnknownAttributesAndMissingFields(t *testing.T) {
	attrs := map[string]model.Value{
		"order_id": model.NewInt(777),
		"customer": model.NewString("Bob"),
		"extra_1":  model.NewString("should be ignored"),
		"extra_2":  model.NewInt(999),
	}
	wme := model.NewWME(15, "order", attrs)

	var target CustomerOrder
	if err := wme.Unmarshal(&target); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if target.Timetag != 15 {
		t.Errorf("expected timetag 15, got %d", target.Timetag)
	}
	if target.ID != 777 {
		t.Errorf("expected ID 777, got %d", target.ID)
	}
	if target.Customer != "Bob" {
		t.Errorf("expected customer Bob, got %q", target.Customer)
	}
	// Missing fields stay as zero-values
	if target.Total != 0 {
		t.Errorf("expected Total 0, got %f", target.Total)
	}
	if target.Status != "" {
		t.Errorf("expected Status empty, got %q", target.Status)
	}
}

type StructA struct {
	ID    int64   `ops5:"id,class=item"`
	Total float64 `ops5:"total"`
}

type StructA_Reordered struct {
	Total float64 `ops5:"total,class=item"`
	ID    int64   `ops5:"id"`
}

type StructA_ChangedType struct {
	ID    string  `ops5:"id,class=item"`
	Total float64 `ops5:"total"`
}

type StructA_AddedField struct {
	ID     int64   `ops5:"id,class=item"`
	Total  float64 `ops5:"total"`
	Status string  `ops5:"status"`
}

func TestStructuralFingerprintDeterminismAndDrift(t *testing.T) {
	schemaA, err := model.ClassSchemaFromStruct(StructA{})
	if err != nil {
		t.Fatal(err)
	}

	schemaReordered, err := model.ClassSchemaFromStruct(StructA_Reordered{})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Reordering struct fields MUST NOT change the structural fingerprint
	if schemaA.Fingerprint != schemaReordered.Fingerprint {
		t.Errorf("field reordering should produce identical fingerprint, got %s != %s",
			schemaA.Fingerprint, schemaReordered.Fingerprint)
	}

	// 2. Changing a field type MUST change the structural fingerprint
	schemaChangedType, _ := model.ClassSchemaFromStruct(StructA_ChangedType{})
	if schemaA.Fingerprint == schemaChangedType.Fingerprint {
		t.Errorf("changing field type should produce different fingerprint")
	}

	// 3. Adding a field MUST change the structural fingerprint
	schemaAddedField, _ := model.ClassSchemaFromStruct(StructA_AddedField{})
	if schemaA.Fingerprint == schemaAddedField.Fingerprint {
		t.Errorf("adding field should produce different fingerprint")
	}
}

func TestSchemaManifestExportAndValidation(t *testing.T) {
	manifest := model.NewSchemaManifest()

	schemaA, _ := model.ClassSchemaFromStruct(StructA{})
	schemaOrder, _ := model.ClassSchemaFromStruct(CustomerOrder{})

	manifest.AddSchema(schemaA)
	manifest.AddSchema(schemaOrder)

	jsonBytes, err := manifest.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	if !strings.Contains(string(jsonBytes), `"fingerprint"`) {
		t.Errorf("expected JSON to contain fingerprint")
	}

	parsed, err := model.ParseSchemaManifestJSON(jsonBytes)
	if err != nil {
		t.Fatalf("ParseSchemaManifestJSON error: %v", err)
	}

	if len(parsed.Schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(parsed.Schemas))
	}

	// Validating against self should succeed
	if err := manifest.Validate(parsed); err != nil {
		t.Errorf("validation against self should succeed: %v", err)
	}

	// Validating against drifted manifest should fail
	driftedManifest := model.NewSchemaManifest()
	driftedA, _ := model.ClassSchemaFromStruct(StructA_ChangedType{})
	driftedManifest.AddSchema(driftedA)

	if err := manifest.Validate(driftedManifest); err == nil {
		t.Errorf("expected drift validation error, but got nil")
	}
}

func TestUnmarshalErrors(t *testing.T) {
	wme := model.NewWME(1, "test", nil)

	// Non-pointer
	if err := model.UnmarshalWME(wme, CustomerOrder{}); err == nil {
		t.Errorf("expected error for non-pointer target")
	}

	// Nil pointer
	var nilPtr *CustomerOrder
	if err := model.UnmarshalWME(wme, nilPtr); err == nil {
		t.Errorf("expected error for nil pointer target")
	}

	// Pointer to non-struct
	var notStruct int
	if err := model.UnmarshalWME(wme, &notStruct); err == nil {
		t.Errorf("expected error for pointer to non-struct target")
	}
}
