package command

import (
	"fmt"
	"reflect"
	"strings"
)

// Schema is a JSON Schema subset describing parameters and results. Struct fields use these tags:
//
//	json:"name"        the field's JSON name
//	desc:"..."         a description for people and agents
//	required:"true"    the field must be present and non-empty
//	enum:"a,b,c"       allowed values
type Schema struct {
	Type        string             `json:"type"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
}

func schemaOf(v any) *Schema {
	if v == nil {
		return nil
	}
	return schemaFor(reflect.TypeOf(v), 0)
}

func schemaFor(t reflect.Type, depth int) *Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}
	case reflect.String:
		return &Schema{Type: "string"}
	case reflect.Slice, reflect.Array:
		return &Schema{Type: "array", Items: schemaFor(t.Elem(), depth+1)}
	case reflect.Map:
		return &Schema{Type: "object"}
	case reflect.Struct:
		s := &Schema{Type: "object", Properties: map[string]*Schema{}}
		if depth > 8 {
			return s
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, ok := jsonName(f)
			if !ok {
				continue
			}
			if f.Anonymous && name == "" {
				embedded := schemaFor(f.Type, depth+1)
				for k, v := range embedded.Properties {
					s.Properties[k] = v
				}
				s.Required = append(s.Required, embedded.Required...)
				continue
			}
			p := schemaFor(f.Type, depth+1)
			p.Description = f.Tag.Get("desc")
			if e := f.Tag.Get("enum"); e != "" {
				p.Enum = strings.Split(e, ",")
			}
			s.Properties[name] = p
			if f.Tag.Get("required") == "true" {
				s.Required = append(s.Required, name)
			}
		}
		if len(s.Properties) == 0 {
			s.Properties = nil
		}
		return s
	}
	return &Schema{Type: "any"}
}

// jsonName returns the field's JSON name, "" for an untagged embedded struct, or false if skipped.
func jsonName(f reflect.StructField) (string, bool) {
	if !f.IsExported() {
		return "", false
	}
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" && !f.Anonymous {
		name = f.Name
	}
	return name, true
}

// checkRequired rejects zero values in required fields and values outside an enum.
func checkRequired(params any) error {
	v := reflect.ValueOf(params)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, ok := jsonName(f)
		if !ok {
			continue
		}
		fv := v.Field(i)
		if f.Anonymous && name == "" {
			if err := checkRequired(fv.Interface()); err != nil {
				return err
			}
			continue
		}
		if f.Tag.Get("required") == "true" && fv.IsZero() {
			return fmt.Errorf("%s is required", name)
		}
		if e := f.Tag.Get("enum"); e != "" && fv.Kind() == reflect.String && fv.String() != "" {
			allowed := strings.Split(e, ",")
			found := false
			for _, a := range allowed {
				found = found || a == fv.String()
			}
			if !found {
				return fmt.Errorf("%s must be one of %s", name, e)
			}
		}
	}
	return nil
}
