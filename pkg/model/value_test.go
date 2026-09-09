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
