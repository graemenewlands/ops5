package model

import (
	"testing"
)

func TestValueEquality(t *testing.T) {
	tests := []struct {
		name  string
		v1    Value
		v2    Value
		equal bool
	}{
		{"same symbol", NewSymbol("foo"), NewSymbol("foo"), true},
		{"diff symbol", NewSymbol("foo"), NewSymbol("bar"), false},
		{"same int", NewInt(42), NewInt(42), true},
		{"diff int", NewInt(42), NewInt(43), false},
		{"int and float equal", NewInt(42), NewFloat(42.0), true},
		{"int and float diff", NewInt(42), NewFloat(42.5), false},
		{"string equality", NewString("hello"), NewString("hello"), true},
		{"variable equality", NewVariable("<x>"), NewVariable("x"), true},
		{"symbol vs string", NewSymbol("hello"), NewString("hello"), false},
		{"boolean equality", NewBoolean(true), NewBoolean(true), true},
		{"boolean diff", NewBoolean(true), NewBoolean(false), false},
		{"boolean and symbol cross-equality", NewBoolean(true), NewSymbol("true"), true},
		{"boolean and symbol false cross-equality", NewBoolean(false), NewSymbol("false"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v1.Equal(tt.v2); got != tt.equal {
				t.Errorf("Equal() = %v, want %v", got, tt.equal)
			}
		})
	}
}

func TestValueCompare(t *testing.T) {
	tests := []struct {
		name    string
		v1      Value
		v2      Value
		want    int
		wantErr bool
	}{
		{"int less", NewInt(10), NewInt(20), -1, false},
		{"int greater", NewInt(20), NewInt(10), 1, false},
		{"int equal", NewInt(10), NewInt(10), 0, false},
		{"int and float less", NewInt(10), NewFloat(10.5), -1, false},
		{"symbol less", NewSymbol("alpha"), NewSymbol("beta"), -1, false},
		{"incompatible types", NewSymbol("alpha"), NewInt(10), 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.v1.Compare(tt.v2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Compare() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAutoValue(t *testing.T) {
	tests := []struct {
		token   string
		wantTyp ValueType
		wantStr string
	}{
		{"active", TypeSymbol, "active"},
		{"123", TypeInteger, "123"},
		{"3.14", TypeFloat, "3.14"},
		{"<x>", TypeVariable, "<x>"},
		{`"quoted text"`, TypeString, `"quoted text"`},
		{"true", TypeBoolean, "true"},
		{"false", TypeBoolean, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			val := AutoValue(tt.token)
			if val.Type() != tt.wantTyp {
				t.Errorf("AutoValue(%q).Type() = %v, want %v", tt.token, val.Type(), tt.wantTyp)
			}
			if val.String() != tt.wantStr {
				t.Errorf("AutoValue(%q).String() = %v, want %v", tt.token, val.String(), tt.wantStr)
			}
		})
	}
}

func TestVectorValue(t *testing.T) {
	v1 := NewVector([]Value{NewFloat(42.36), NewFloat(-71.05)})
	v2 := NewVector([]Value{NewFloat(42.36), NewFloat(-71.05)})
	v3 := NewVector([]Value{NewFloat(42.36), NewFloat(-70.00)})

	if !v1.IsVector() {
		t.Fatalf("expected v1 to be vector")
	}
	if v1.Type() != TypeVector {
		t.Fatalf("expected v1.Type() == TypeVector, got %v", v1.Type())
	}
	if !v1.Equal(v2) {
		t.Fatalf("expected v1 equal v2")
	}
	if v1.Equal(v3) {
		t.Fatalf("expected v1 not equal v3")
	}
	if v1.String() != "42.36 -71.05" {
		t.Fatalf("expected '42.36 -71.05', got %q", v1.String())
	}
	if len(v1.VectorElements()) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(v1.VectorElements()))
	}

	// Vectors of strings, integers, booleans, and heterogeneous elements
	strVec := NewVector([]Value{NewString("Beantown"), NewString("The Hub")})
	if strVec.String() != `"Beantown" "The Hub"` {
		t.Fatalf("expected '\"Beantown\" \"The Hub\"', got %q", strVec.String())
	}

	intVec := NewVector([]Value{NewInt(2101), NewInt(2108), NewInt(2115)})
	if intVec.String() != "2101 2108 2115" {
		t.Fatalf("expected '2101 2108 2115', got %q", intVec.String())
	}

	boolVec := NewVector([]Value{NewBoolean(true), NewBoolean(false), NewBoolean(true)})
	if boolVec.String() != "true false true" {
		t.Fatalf("expected 'true false true', got %q", boolVec.String())
	}

	hetVec := NewVector([]Value{NewString("alpha"), NewInt(10), NewFloat(3.14), NewBoolean(true)})
	if hetVec.String() != `"alpha" 10 3.14 true` {
		t.Fatalf("expected '\"alpha\" 10 3.14 true', got %q", hetVec.String())
	}
}
