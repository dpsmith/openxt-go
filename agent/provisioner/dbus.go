// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2026 Apertus Solutions, LLC
package provisioner

import (
	"fmt"
	"reflect"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// BuildDBusPropMap exports selected fields of obj as writable D-Bus
// properties. obj must be a pointer to a struct. method is the name of
// a callback with signature func(*prop.Change) *dbus.Error.
//
// fields maps a struct-field prefix to the names to export under that
// prefix. The empty prefix "" is the struct itself. A non-empty prefix
// names an embedded or nested struct field; exported properties are
// named Prefix_Field (for example Source_Type).
func BuildDBusPropMap(fields map[string][]string, obj interface{}, method string) (map[string]*prop.Prop, error) {
	if obj == nil {
		return nil, fmt.Errorf("object is nil")
	}

	objValue := reflect.ValueOf(obj)
	if objValue.Kind() != reflect.Ptr || objValue.IsNil() {
		return nil, fmt.Errorf("object is %T, need pointer to struct", obj)
	}
	objInstance := objValue.Elem()
	if objInstance.Kind() != reflect.Struct {
		return nil, fmt.Errorf("object is %T, not struct", obj)
	}

	callback, err := lookupPropCallback(objValue, method)
	if err != nil {
		return nil, err
	}

	props := make(map[string]*prop.Prop)
	for prefix, names := range fields {
		instance := objInstance
		if prefix != "" {
			instance = objInstance.FieldByName(prefix)
			if !instance.IsValid() {
				return nil, fmt.Errorf("no field %s on %T", prefix, obj)
			}
			if instance.Kind() == reflect.Ptr {
				if instance.IsNil() {
					return nil, fmt.Errorf("field %s on %T is nil", prefix, obj)
				}
				instance = instance.Elem()
			}
			if instance.Kind() != reflect.Struct {
				return nil, fmt.Errorf("field %s on %T is %s, not struct", prefix, obj, instance.Kind())
			}
		}

		if err := addProps(props, instance, prefix, names, callback); err != nil {
			return nil, err
		}
	}

	return props, nil
}

func lookupPropCallback(objValue reflect.Value, method string) (func(*prop.Change) *dbus.Error, error) {
	if method == "" {
		return nil, fmt.Errorf("property callback name is empty")
	}
	cbValue := objValue.MethodByName(method)
	if !cbValue.IsValid() {
		return nil, fmt.Errorf("object has no method %s", method)
	}
	cb, ok := cbValue.Interface().(func(*prop.Change) *dbus.Error)
	if !ok {
		return nil, fmt.Errorf("method %s has type %s, want func(*prop.Change) *dbus.Error", method, cbValue.Type())
	}
	return cb, nil
}

func addProps(dst map[string]*prop.Prop, instance reflect.Value, prefix string, names []string, callback func(*prop.Change) *dbus.Error) error {
	for _, field := range names {
		if field == "" {
			return fmt.Errorf("empty field name under prefix %q", prefix)
		}
		fv := instance.FieldByName(field)
		if !fv.IsValid() {
			return fmt.Errorf("no field %s", qualifiedName(prefix, field))
		}
		if !fv.CanInterface() {
			return fmt.Errorf("field %s is unexported", qualifiedName(prefix, field))
		}

		dst[qualifiedName(prefix, field)] = &prop.Prop{
			Value:    fv.Interface(),
			Writable: true,
			Emit:     prop.EmitTrue,
			Callback: callback,
		}
	}
	return nil
}

func qualifiedName(prefix, field string) string {
	if prefix == "" {
		return field
	}
	return prefix + "_" + field
}
