package model

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ClassNamer allows custom Go types to specify their OPS5 element class name.
type ClassNamer interface {
	OPS5ClassName() string
}

type fieldDescriptor struct {
	fieldIndex   []int
	name         string       // normalized attribute name (lowercase, no ^)
	goType       reflect.Type
	elemType     reflect.Type // if pointer or slice, the underlying element type
	isPointer    bool
	isSlice      bool
	isVector     bool
	isOmitEmpty  bool
	temporalKind string // "", "date", "datetime", "utc"
	isSymbol     bool
	isTimetag    bool
	isIgnored    bool
	fieldType    string // canonical type descriptor for schema (e.g. "int64", "string", "symbol", "vector", ...)
}

type structDescriptor struct {
	targetType   reflect.Type
	className    string
	schema       *ClassSchema
	fields       []fieldDescriptor
	fieldsByAttr map[string]fieldDescriptor
	timetagField *fieldDescriptor
}

var (
	descMu    sync.RWMutex
	descCache = make(map[reflect.Type]*structDescriptor)
)

// ClearStructCache empties the internal struct descriptor cache.
func ClearStructCache() {
	descMu.Lock()
	defer descMu.Unlock()
	descCache = make(map[reflect.Type]*structDescriptor)
}

func getStructDescriptor(t reflect.Type) (*structDescriptor, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct or pointer to struct, got %s", t.Kind())
	}

	descMu.RLock()
	if d, ok := descCache[t]; ok {
		descMu.RUnlock()
		return d, nil
	}
	descMu.RUnlock()

	descMu.Lock()
	defer descMu.Unlock()

	if d, ok := descCache[t]; ok {
		return d, nil
	}

	// 1. Determine class name
	var className string
	if t.Implements(reflect.TypeOf((*ClassNamer)(nil)).Elem()) {
		namer := reflect.Zero(t).Interface().(ClassNamer)
		className = namer.OPS5ClassName()
	} else if reflect.PointerTo(t).Implements(reflect.TypeOf((*ClassNamer)(nil)).Elem()) {
		namer := reflect.New(t).Interface().(ClassNamer)
		className = namer.OPS5ClassName()
	}

	var classOverride string
	var fields []fieldDescriptor
	inspectStructFields(t, nil, &fields, &classOverride)

	if classOverride != "" {
		className = classOverride
	}
	if className == "" {
		className = strings.ToLower(t.Name())
	}
	className = strings.ToLower(strings.TrimSpace(className))

	// 2. Build schema and field indices
	var attrNames []string
	fieldsByAttr := make(map[string]fieldDescriptor, len(fields))
	var timetagField *fieldDescriptor

	for _, f := range fields {
		if f.isIgnored {
			continue
		}
		if f.isTimetag {
			if timetagField == nil {
				tf := f
				timetagField = &tf
			}
			continue
		}
		attrNames = append(attrNames, f.name)
		fieldsByAttr[f.name] = f
	}

	schema := NewClassSchema(className, attrNames)
	for _, f := range fields {
		if f.isIgnored || f.isTimetag {
			continue
		}
		if f.isVector {
			schema.SetVectorAttribute(f.name, true)
		}
		schema.SetFieldType(f.name, f.fieldType)
	}
	schema.ComputeFingerprint()

	desc := &structDescriptor{
		targetType:   t,
		className:    className,
		schema:       schema,
		fields:       fields,
		fieldsByAttr: fieldsByAttr,
		timetagField: timetagField,
	}
	descCache[t] = desc
	return desc, nil
}

func inspectStructFields(t reflect.Type, indexPrefix []int, fields *[]fieldDescriptor, classOverride *string) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			// Unexported non-embedded field
			continue
		}

		currentIndex := append(append([]int(nil), indexPrefix...), i)
		tag := sf.Tag.Get("ops5")
		if tag == "-" {
			continue
		}

		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			inspectStructFields(sf.Type, currentIndex, fields, classOverride)
			continue
		}
		if sf.Anonymous && sf.Type.Kind() == reflect.Pointer && sf.Type.Elem().Kind() == reflect.Struct {
			inspectStructFields(sf.Type.Elem(), currentIndex, fields, classOverride)
			continue
		}

		parts := strings.Split(tag, ",")
		var attrName string
		var opts []string
		if len(parts) > 0 {
			first := strings.TrimSpace(parts[0])
			if strings.Contains(first, "=") {
				opts = append(opts, first)
			} else if first != "" {
				attrName = NormalizeAttribute(first)
			}
			for _, opt := range parts[1:] {
				opt = strings.TrimSpace(opt)
				if opt != "" {
					opts = append(opts, opt)
				}
			}
		}
		if attrName == "" {
			attrName = strings.ToLower(sf.Name)
		}

		fd := fieldDescriptor{
			fieldIndex: currentIndex,
			name:       attrName,
			goType:     sf.Type,
		}

		for _, opt := range opts {
			lowerOpt := strings.ToLower(opt)
			switch {
			case lowerOpt == "omitempty":
				fd.isOmitEmpty = true
			case lowerOpt == "vector":
				fd.isVector = true
			case lowerOpt == "date":
				fd.temporalKind = "date"
			case lowerOpt == "datetime":
				fd.temporalKind = "datetime"
			case lowerOpt == "utc":
				fd.temporalKind = "utc"
			case lowerOpt == "symbol":
				fd.isSymbol = true
			case lowerOpt == "timetag":
				fd.isTimetag = true
			case strings.HasPrefix(lowerOpt, "class="):
				cls := strings.TrimPrefix(opt, "class=")
				cls = strings.TrimPrefix(cls, "CLASS=")
				if cls != "" && *classOverride == "" {
					*classOverride = strings.ToLower(cls)
				}
			}
		}

		if !fd.isTimetag && (attrName == "timetag" || strings.ToLower(sf.Name) == "timetag") && tag == "" {
			fd.isTimetag = true
		}

		ft := sf.Type
		if ft.Kind() == reflect.Pointer {
			fd.isPointer = true
			fd.elemType = ft.Elem()
			ft = ft.Elem()
		} else {
			fd.elemType = ft
		}

		if ft.Kind() == reflect.Slice && ft != reflect.TypeOf([]byte(nil)) {
			fd.isSlice = true
			fd.isVector = true
		}

		// Determine canonical field type descriptor for schema
		if fd.isTimetag {
			fd.fieldType = "int64"
		} else if fd.isVector || fd.isSlice {
			fd.fieldType = "vector"
		} else if fd.temporalKind != "" {
			if fd.temporalKind == "utc" {
				fd.fieldType = "dateutctime"
			} else {
				fd.fieldType = fd.temporalKind
			}
		} else if ft == reflect.TypeOf(time.Time{}) {
			fd.fieldType = "datetime"
		} else if ft == reflect.TypeOf(Value{}) {
			fd.fieldType = "value"
		} else {
			switch ft.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				fd.fieldType = "int64"
			case reflect.Float32, reflect.Float64:
				fd.fieldType = "float64"
			case reflect.Bool:
				fd.fieldType = "boolean"
			case reflect.String:
				if fd.isSymbol {
					fd.fieldType = "symbol"
				} else {
					fd.fieldType = "string"
				}
			default:
				fd.fieldType = "value"
			}
		}

		*fields = append(*fields, fd)
	}
}

// ClassSchemaFromStruct derives an OPS5 element class schema automatically from a Go struct definition.
// The schema includes canonical attribute names, vector designations, field types, and a deterministic SHA-256 fingerprint.
func ClassSchemaFromStruct(v any) (*ClassSchema, error) {
	if v == nil {
		return nil, fmt.Errorf("cannot derive schema from nil")
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct or pointer to struct, got %s", t.Kind())
	}
	desc, err := getStructDescriptor(t)
	if err != nil {
		return nil, err
	}
	return desc.schema.Clone(), nil
}

// MarshalWME converts a Go struct or pointer to struct into an OPS5 class name and attribute map.
func MarshalWME(v any) (string, map[string]Value, error) {
	if v == nil {
		return "", nil, fmt.Errorf("cannot marshal nil value")
	}
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return "", nil, fmt.Errorf("cannot marshal nil pointer")
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return "", nil, fmt.Errorf("expected struct or pointer to struct, got %s", val.Kind())
	}

	desc, err := getStructDescriptor(val.Type())
	if err != nil {
		return "", nil, err
	}

	attrs := make(map[string]Value, len(desc.fields))
	for _, f := range desc.fields {
		if f.isIgnored || f.isTimetag {
			continue
		}

		fv := val.FieldByIndex(f.fieldIndex)
		if f.isPointer {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}

		if f.isOmitEmpty && isZeroValue(fv) {
			continue
		}

		mv, err := toOPS5Value(fv, f)
		if err != nil {
			return "", nil, fmt.Errorf("field %s: %w", f.name, err)
		}
		attrs[f.name] = mv
	}

	return desc.className, attrs, nil
}

func toOPS5Value(fv reflect.Value, f fieldDescriptor) (Value, error) {
	if fv.Type() == reflect.TypeOf(Value{}) {
		return fv.Interface().(Value), nil
	}
	if fv.Type() == reflect.TypeOf(time.Time{}) {
		t := fv.Interface().(time.Time)
		switch f.temporalKind {
		case "date":
			return NewDateFromTime(t), nil
		case "datetime":
			return NewDateTimeFromTime(t), nil
		case "utc":
			return NewDateUTCTimeFromTime(t), nil
		default:
			if t.Location() == time.UTC {
				return NewDateUTCTimeFromTime(t), nil
			}
			return NewDateTimeFromTime(t), nil
		}
	}

	if f.temporalKind == "date" {
		switch fv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return NewDate(fv.Int()), nil
		case reflect.String:
			return ParseDateString(fv.String())
		}
	} else if f.temporalKind == "datetime" && fv.Kind() == reflect.String {
		return ParseDateTime(fv.String())
	} else if f.temporalKind == "utc" && fv.Kind() == reflect.String {
		return ParseDateUTCTime(fv.String())
	}

	if f.isSlice {
		l := fv.Len()
		elems := make([]Value, 0, l)
		for i := 0; i < l; i++ {
			ev := fv.Index(i)
			mv, err := toScalarOPS5Value(ev, f)
			if err != nil {
				return Value{}, err
			}
			elems = append(elems, mv)
		}
		return NewVector(elems), nil
	}

	return toScalarOPS5Value(fv, f)
}

func toScalarOPS5Value(fv reflect.Value, f fieldDescriptor) (Value, error) {
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return NewInt(fv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return NewInt(int64(fv.Uint())), nil
	case reflect.Float32, reflect.Float64:
		return NewFloat(fv.Float()), nil
	case reflect.Bool:
		return NewBoolean(fv.Bool()), nil
	case reflect.String:
		if f.isSymbol {
			return NewSymbol(fv.String()), nil
		}
		return NewString(fv.String()), nil
	default:
		if fv.Type() == reflect.TypeOf(Value{}) {
			return fv.Interface().(Value), nil
		}
		if fv.Type() == reflect.TypeOf(time.Time{}) {
			t := fv.Interface().(time.Time)
			if t.Location() == time.UTC {
				return NewDateUTCTimeFromTime(t), nil
			}
			return NewDateTimeFromTime(t), nil
		}
		return NewString(fmt.Sprint(fv.Interface())), nil
	}
}

func isZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.String:
		return v.Len() == 0
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			return v.Interface().(time.Time).IsZero()
		}
		if v.Type() == reflect.TypeOf(Value{}) {
			return v.Interface().(Value).Raw() == nil
		}
		return v.IsZero()
	}
}

// UnmarshalWME maps the WME's attributes and timetag into a target Go struct pointer.
func UnmarshalWME(wme *WME, target any) error {
	if wme == nil {
		return fmt.Errorf("cannot unmarshal nil WME")
	}
	if target == nil {
		return fmt.Errorf("target must be a non-nil pointer to a struct, got nil")
	}

	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer to a struct, got %T", target)
	}

	structVal := rv.Elem()
	if structVal.Kind() != reflect.Struct {
		return fmt.Errorf("target must be a pointer to a struct, got pointer to %s", structVal.Kind())
	}

	desc, err := getStructDescriptor(structVal.Type())
	if err != nil {
		return err
	}

	// 1. Assign Timetag if designated
	if desc.timetagField != nil {
		tfv := structVal.FieldByIndex(desc.timetagField.fieldIndex)
		switch tfv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			tfv.SetInt(wme.Timetag)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			tfv.SetUint(uint64(wme.Timetag))
		}
	}

	// 2. Populate attributes
	for _, f := range desc.fields {
		if f.isIgnored || f.isTimetag {
			continue
		}

		mv, exists := wme.Get(f.name)
		if !exists {
			continue
		}

		fv := structVal.FieldByIndex(f.fieldIndex)
		targetField := fv
		if f.isPointer {
			if targetField.IsNil() {
				targetField.Set(reflect.New(f.elemType))
			}
			targetField = targetField.Elem()
		}

		if targetField.Type() == reflect.TypeOf(Value{}) {
			targetField.Set(reflect.ValueOf(mv))
			continue
		}

		if targetField.Type() == reflect.TypeOf(time.Time{}) {
			if mv.IsTemporal() {
				targetField.Set(reflect.ValueOf(mv.Time()))
			} else if mv.Type() == TypeInteger {
				d := NewDate(mv.DateInt())
				targetField.Set(reflect.ValueOf(d.Time()))
			} else if mv.Type() == TypeString {
				if t, err := parseDateTimeString(mv.Raw().(string), time.Local); err == nil {
					targetField.Set(reflect.ValueOf(t))
				}
			}
			continue
		}

		if targetField.Kind() == reflect.Slice && targetField.Type() != reflect.TypeOf([]byte(nil)) {
			elemType := targetField.Type().Elem()
			if mv.Type() == TypeVector {
				elems := mv.VectorElements()
				slice := reflect.MakeSlice(targetField.Type(), len(elems), len(elems))
				for i, el := range elems {
					setScalarReflectValue(slice.Index(i), elemType, el)
				}
				targetField.Set(slice)
			} else {
				slice := reflect.MakeSlice(targetField.Type(), 1, 1)
				setScalarReflectValue(slice.Index(0), elemType, mv)
				targetField.Set(slice)
			}
			continue
		}

		setScalarReflectValue(targetField, targetField.Type(), mv)
	}

	return nil
}

func setScalarReflectValue(targetVal reflect.Value, t reflect.Type, mv Value) {
	if t == reflect.TypeOf(Value{}) {
		targetVal.Set(reflect.ValueOf(mv))
		return
	}
	if t == reflect.TypeOf(time.Time{}) {
		if mv.IsTemporal() {
			targetVal.Set(reflect.ValueOf(mv.Time()))
		}
		return
	}

	switch targetVal.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if mv.Type() == TypeInteger {
			targetVal.SetInt(mv.Raw().(int64))
		} else if mv.Type() == TypeFloat {
			targetVal.SetInt(int64(mv.Raw().(float64)))
		} else if mv.Type() == TypeDate {
			targetVal.SetInt(mv.DateInt())
		} else if mv.Type() == TypeString {
			if n, err := strconv.ParseInt(mv.Raw().(string), 10, 64); err == nil {
				targetVal.SetInt(n)
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if mv.Type() == TypeInteger {
			targetVal.SetUint(uint64(mv.Raw().(int64)))
		} else if mv.Type() == TypeFloat {
			targetVal.SetUint(uint64(mv.Raw().(float64)))
		}
	case reflect.Float32, reflect.Float64:
		if mv.Type() == TypeFloat {
			targetVal.SetFloat(mv.Raw().(float64))
		} else if mv.Type() == TypeInteger {
			targetVal.SetFloat(float64(mv.Raw().(int64)))
		}
	case reflect.Bool:
		if mv.Type() == TypeBoolean {
			targetVal.SetBool(mv.Boolean())
		} else if mv.Type() == TypeSymbol {
			targetVal.SetBool(mv.Raw().(string) == "true")
		}
	case reflect.String:
		if mv.Type() == TypeString || mv.Type() == TypeSymbol {
			targetVal.SetString(mv.Raw().(string))
		} else {
			targetVal.SetString(mv.String())
		}
	}
}
