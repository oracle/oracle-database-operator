// Package statusmerge provides structured merge helpers for status subfields.
package statusmerge

import (
	"fmt"
	"reflect"
)

type PointerMode int

const (
	// PointerCopyIfNil copies pointer values only when destination is nil.
	PointerCopyIfNil PointerMode = iota
	// PointerDeepMerge recursively merges pointer targets.
	PointerDeepMerge
)

// SliceMode controls how slices are merged.
type SliceMode int

const (
	// SliceReplace replaces destination slices with source slices.
	SliceReplace SliceMode = iota
	// SliceMergeByIndex merges slice items by index.
	SliceMergeByIndex
)

// Options controls pointer/slice merge behavior for struct-field merges.
type Options struct {
	PointerMode     PointerMode
	SliceMode       SliceMode
	SkipSliceFields map[string]struct{}
}

// MergeNamedStructField merges one named struct field from srcObj into dstObj.
// Both objects must be pointers to structs containing the field.
func MergeNamedStructField(dstObj, srcObj interface{}, fieldName string, opts Options) error {
	dstVal := reflect.ValueOf(dstObj)
	srcVal := reflect.ValueOf(srcObj)
	if dstVal.Kind() != reflect.Ptr || srcVal.Kind() != reflect.Ptr || dstVal.IsNil() || srcVal.IsNil() {
		return fmt.Errorf("objects must be non-nil pointers")
	}

	dstElem := dstVal.Elem()
	srcElem := srcVal.Elem()
	if dstElem.Kind() != reflect.Struct || srcElem.Kind() != reflect.Struct {
		return fmt.Errorf("objects must point to structs")
	}

	dstField := dstElem.FieldByName(fieldName)
	srcField := srcElem.FieldByName(fieldName)
	if !dstField.IsValid() || !srcField.IsValid() {
		return fmt.Errorf("%s field is not valid in one of the instances", fieldName)
	}
	return mergeStructFields(dstField, srcField, opts)
}

func mergeStructFields(dst, src reflect.Value, opts Options) error {
	if dst.Kind() != reflect.Struct || src.Kind() != reflect.Struct {
		return fmt.Errorf("fields to be merged must be struct")
	}

	for i := 0; i < dst.NumField(); i++ {
		subField := dst.Type().Field(i)
		dstField := dst.Field(i)
		srcField := src.Field(i)

		// Skip unexported or unsettable fields.
		if subField.PkgPath != "" || !dstField.CanSet() {
			continue
		}

		switch srcField.Kind() {
		case reflect.Ptr:
			if srcField.IsNil() {
				continue
			}
			if opts.PointerMode == PointerCopyIfNil {
				if dstField.IsNil() {
					dstField.Set(srcField)
				}
				continue
			}

			// Deep-merge pointer targets.
			if dstField.IsNil() {
				dstField.Set(reflect.New(srcField.Type().Elem()))
			}
			if srcField.Elem().Kind() == reflect.Struct && dstField.Elem().Kind() == reflect.Struct {
				if err := mergeStructFields(dstField.Elem(), srcField.Elem(), opts); err != nil {
					return err
				}
			} else if dstField.Elem().IsZero() {
				dstField.Elem().Set(srcField.Elem())
			}

		case reflect.String:
			if srcField.String() != "" && srcField.String() != "NOT_DEFINED" && dstField.String() == "" {
				dstField.Set(srcField)
			}

		case reflect.Struct:
			if err := mergeStructFields(dstField, srcField, opts); err != nil {
				return err
			}

		case reflect.Slice:
			if _, skip := opts.SkipSliceFields[subField.Name]; skip {
				continue
			}
			if srcField.Len() == 0 {
				continue
			}
			if opts.SliceMode == SliceReplace {
				dstField.Set(srcField)
				continue
			}

			// Merge-by-index behavior.
			if dstField.IsNil() {
				dstField.Set(reflect.MakeSlice(dstField.Type(), 0, srcField.Len()))
			}
			if dstField.Len() == 0 {
				dstField.Set(srcField)
				continue
			}
			for j := 0; j < srcField.Len(); j++ {
				if j < dstField.Len() {
					if srcField.Index(j).Kind() == reflect.Struct {
						if err := mergeStructFields(dstField.Index(j), srcField.Index(j), opts); err != nil {
							return err
						}
					} else if dstField.Index(j).IsZero() {
						dstField.Index(j).Set(srcField.Index(j))
					}
				} else {
					dstField.Set(reflect.Append(dstField, srcField.Index(j)))
				}
			}

		default:
			if reflect.DeepEqual(dstField.Interface(), reflect.Zero(dstField.Type()).Interface()) {
				dstField.Set(srcField)
			}
		}
	}

	return nil
}
