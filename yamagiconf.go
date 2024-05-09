// Package yamagiconf provides an opinionated configuration file parser
// using a subset of YAML and Go struct type restrictions to allow for
// consistent configuration files, easy validation and good error reporting.
// Supports primitive `env` struct tags used to overwrite fields from env vars.
// Also supports github.com/go-playground/validator struct tags.
package yamagiconf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Errors, including type-specific errors.
var (
	ErrNilConfig            = errors.New("cannot load into nil config")
	ErrEmptyFile            = errors.New("empty file")
	ErrMalformedYAML        = errors.New("malformed YAML")
	ErrMissingYAMLTag       = errors.New("missing yaml struct tag")
	ErrInvalidEnvTag        = errors.New("invalid env struct tag")
	ErrMissingConfig        = errors.New("missing field in config file")
	ErrInvalidEnvVar        = errors.New("invalid env var")
	ErrValidation           = errors.New("validation")
	ErrValidateTagViolation = errors.New("violates validation rule")
	ErrBadBoolLiteral       = errors.New("must be either false or true, " +
		"other variants of boolean literals of YAML are not supported")
	ErrBadNullLiteral = errors.New("must be null, " +
		"any other variants of null are not supported")
	ErrNullOnNonPointer   = errors.New("cannot assign null to non-pointer type")
	ErrRecursiveType      = errors.New("recursive type")
	ErrUnsupportedType    = errors.New("unsupported type")
	ErrUnsupportedPtrType = errors.New("unsupported pointer type")
)

// LoadFile reads and validates the configuration of type T from a YAML file.
// Will return an error if:
//
//   - T contains any struct field without a "yaml" struct tag.
//   - T contains any struct field with an invalid "env" struct tag.
//   - T is recursive.
//   - T contains any unsupported types (signed and unsigned integers with unspecified
//     width, interface (including `any`), function, channel,
//     unsafe.Pointer, pointer to pointer, pointer to slice and pointer to map).
//   - the yaml file is empty or not found.
//   - the yaml file doesn't contain a field specified by T.
//   - the yaml file is missing a field specified by T.
//   - the yaml file contains values that don't pass validation.
//   - the yaml file contains boolean literals other than `true` and `false`.
//   - the yaml file contains null values other than `null` (`~`, etc.).
//   - the yaml file assigns `null` to a non-pointer Go type.
func LoadFile[T any](yamlFilePath string, config *T) error {
	if config == nil {
		return ErrNilConfig
	}

	yamlSrcBytes, err := os.ReadFile(yamlFilePath)
	if err != nil {
		return fmt.Errorf("reading file %q: %w", yamlFilePath, err)
	}
	return Load[T, []byte](yamlSrcBytes, config)
}

// Load reads and validates the configuration of type T from yamlSource.
// Load behaves similar to LoadFile.
func Load[T any, S string | []byte](yamlSource S, config *T) error {
	if config == nil {
		return ErrNilConfig
	}
	if len(yamlSource) == 0 {
		return ErrEmptyFile
	}

	if err := ValidateType[T](); err != nil {
		return err
	}

	dec := newDecoderYAML(yamlSource)
	dec.KnownFields(true)
	err := dec.Decode(config)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMalformedYAML, err)
	}

	var rootNode yaml.Node
	if err := newDecoderYAML(yamlSource).Decode(&rootNode); err != nil {
		return fmt.Errorf("decoding yaml structure: %w", err)
	}

	configType := reflect.TypeOf(config).Elem()

	err = validateYAMLValues("", configType.Name(), configType, rootNode.Content[0])
	if err != nil {
		return err
	}

	err = unmarshalEnv(configType.Name(), "", reflect.ValueOf(config).Elem())
	if err != nil {
		return err
	}

	err = invokeValidateRecursively(reflect.ValueOf(config), rootNode.Content[0])
	if err != nil {
		return err
	}

	err = validator.New(
		validator.WithRequiredStructEnabled(),
	).Struct(config)
	if err != nil {
		if errs, ok := err.(validator.ValidationErrors); ok {
			err := errs[0]
			line, column, yamlTag := mustFindLocationByValidatorNamespace[T](
				err.StructNamespace(), &rootNode,
			)
			return fmt.Errorf("at %d:%d: %q %w: %q",
				line, column, yamlTag, ErrValidateTagViolation, err.Tag())
		}
		return err
	}
	return nil
}

type Validator interface{ Validate() error }

var typeValidator = reflect.TypeOf((*Validator)(nil)).Elem()

// asValidator returns nil if v doesn't implement the Validator interface
// neither as a copy- nor as a pointer receiver.
func asValidator(v reflect.Value) Validator {
	if !v.IsValid() {
		return nil
	}
	tp := v.Type()
	if tp.Kind() == reflect.Pointer && v.IsNil() {
		return nil
	}
	if v.CanInterface() && tp.Implements(typeValidator) {
		return v.Interface().(Validator)
	}
	if v.CanAddr() {
		vp := v.Addr()
		if vp.CanInterface() && vp.Type().Implements(typeValidator) {
			return vp.Interface().(Validator)
		}
	}
	return nil
}

// invokeValidateRecursively runs the Validate method for
// every field of type that implements the Validator interface recursively.
// Assumes that T was previously checked with checkYAMLValues.
func invokeValidateRecursively(v reflect.Value, node *yaml.Node) error {
	tp := v.Type()

	if v := asValidator(v); v != nil {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("at %d:%d: %w: %w",
				node.Line, node.Column, ErrValidation, err)
		}
	}
	for tp.Kind() == reflect.Pointer {
		tp, v = tp.Elem(), v.Elem()
	}

	switch tp.Kind() {
	case reflect.Struct:
		for i := range tp.NumField() {
			fv := v.Field(i)
			yamlTag := getYAMLFieldName(tp.Field(i).Tag)
			nodeValue := findContentNodeByTag(node, yamlTag)
			if err := invokeValidateRecursively(fv, nodeValue); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i, nodeValue := range node.Content {
			if err := invokeValidateRecursively(v.Index(i), nodeValue); err != nil {
				return err
			}
		}
	case reflect.Map:
		mapKeys := v.MapKeys()
		for i := 0; i < len(node.Content); i += 2 {
			for _, k := range mapKeys {
				if k.String() == node.Content[i].Value {
					err := invokeValidateRecursively(k, node.Content[i])
					if err != nil {
						return err
					}
					err = invokeValidateRecursively(v.MapIndex(k), node.Content[i+1])
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func newDecoderYAML[S string | []byte](s S) *yaml.Decoder {
	var reader io.Reader
	switch s := any(s).(type) {
	case []byte:
		reader = bytes.NewReader(s)
	case string:
		reader = strings.NewReader(s)
	}
	return yaml.NewDecoder(reader)
}

// unmarshalEnv traverses v and overwrites the values when an `env` struct tag
// was specified for any given field.
// Assumes that the config type has already been validated.
func unmarshalEnv(path, envVar string, v reflect.Value) error {
	tp := v.Type()

	if tp.Kind() == reflect.Pointer {
		env, ok := os.LookupEnv(envVar)
		if ok {
			if env == "null" {
				v.Set(reflect.Zero(v.Type()))
				return nil
			}
			tp, v = tp.Elem(), v.Elem()
		}
	}

	switch tp.Kind() {
	case reflect.Bool:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		switch env {
		case "true":
			v.SetBool(true)
		case "false":
			v.SetBool(false)
		default:
			return errUnmarshalEnv(path, envVar, v.Type(), nil)
		}
	case reflect.String:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		v.SetString(env)
	case reflect.Float32:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		f, err := strconv.ParseFloat(env, 32)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetFloat(f)
	case reflect.Float64:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		f, err := strconv.ParseFloat(env, 64)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetFloat(f)
	case reflect.Int8:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 8)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetInt(int64(i))
	case reflect.Uint8:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 8)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetUint(uint64(i))
	case reflect.Int16:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 16)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetInt(int64(i))
	case reflect.Uint16:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 16)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetUint(uint64(i))
	case reflect.Int32:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 32)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetInt(int64(i))
	case reflect.Uint32:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 32)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetUint(uint64(i))
	case reflect.Int64:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 64)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetInt(int64(i))
	case reflect.Uint64:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 64)
		if err != nil {
			return errUnmarshalEnv(path, envVar, v.Type(), err)
		}
		v.SetUint(uint64(i))
	case reflect.Struct:
		for i := range tp.NumField() {
			if n := tp.Field(i).Tag.Get("env"); n != "" {
				err := unmarshalEnv(path+"."+tp.Field(i).Name, n, v.Field(i))
				if err != nil {
					return err
				}
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			err := unmarshalEnv("", fmt.Sprintf("%s[%d]", path, i), v.Index(i))
			if err != nil {
				return err
			}
		}
	case reflect.Map:
		for i := range v.Len() {
			err := unmarshalEnv("", fmt.Sprintf("%s[%d]", path, i), v.Index(i))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func errUnmarshalEnv(path, envVar string, tp reflect.Type, err error) error {
	if err != nil {
		return fmt.Errorf("at %s: %w %s: expected %s: %w",
			path, ErrInvalidEnvVar, envVar, tp.String(), err)
	}
	return fmt.Errorf("at %s: %w %s: expected %s",
		path, ErrInvalidEnvVar, envVar, tp.String())
}

// mustFindLocationByValidatorNamespace finds the line and column numbers of the
// validator namespace (field type path).
func mustFindLocationByValidatorNamespace[T any](
	validatorNamespace string, rootNode *yaml.Node,
) (line int, column int, yamlTag string) {
	var t T
	tp := reflect.TypeOf(t)

	// Remove the type prefix, assuming validatorNamespace starts with the type name
	_, validatorNamespace = leftmostPathElement(validatorNamespace)

	currentTp, currentNode := tp, rootNode.Content[0]
	var fieldName string

FOR_PATH:
	for {
		fieldName, validatorNamespace = leftmostPathElement(validatorNamespace)
		if fieldName == "" {
			break
		}
		f, _ := currentTp.FieldByName(fieldName)
		yamlTag = getYAMLFieldName(f.Tag)
		for i := 0; i < len(currentNode.Content); i += 2 {
			if currentNode.Content[i].Value == yamlTag {
				currentTp = f.Type
				currentNode = currentNode.Content[i+1]
				continue FOR_PATH
			}
		}
		break // Not found
	}
	return currentNode.Line, currentNode.Column, yamlTag
}

func leftmostPathElement(s string) (element, rest string) {
	if i := strings.IndexByte(s, '.'); i != -1 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// validateYAMLValues returns an error if the yaml model contains illegal values
// or is missing values specified by T. Assumes that tp has already been validated.
func validateYAMLValues(yamlTag, path string, tp reflect.Type, node *yaml.Node) error {
	if err := validateValue(tp, node); err != nil {
		if yamlTag != "" {
			return fmt.Errorf("at %d:%d: %q (%s): %w",
				node.Line, node.Column, yamlTag, path, err)
		}
		return fmt.Errorf("at %d:%d: %s: %w",
			node.Line, node.Column, path, err)
	}

	switch tp.Kind() {
	case reflect.Struct:
		for i := range tp.NumField() {
			f := tp.Field(i)
			yamlTag := getYAMLFieldName(f.Tag)
			path := path + "." + f.Name
			contentNode := findContentNodeByTag(node, yamlTag)
			if contentNode == nil {
				return fmt.Errorf("config %q (%s): %w",
					yamlTag, path, ErrMissingConfig)
			}
			err := validateYAMLValues(yamlTag, path, f.Type, contentNode)
			if err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		tp := tp.Elem()
		for index, node := range node.Content {
			path := fmt.Sprintf("%s[%d]", path, index)
			if err := validateYAMLValues(yamlTag, path, tp, node); err != nil {
				return err
			}
		}
	case reflect.Map:
		for i := 0; i < len(node.Content); i += 2 {
			path := fmt.Sprintf("%s[%q]", path, node.Content[i].Value)
			// Validate key
			err := validateYAMLValues(yamlTag, path, tp, node.Content[i])
			if err != nil {
				return err
			}
			// Validate value
			err = validateYAMLValues(yamlTag, path, tp, node.Content[i+1])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func validateValue(tp reflect.Type, node *yaml.Node) error {
	if v := node.Value; v == "~" || strings.EqualFold(v, "null") {
		if v != "null" {
			return ErrBadNullLiteral
		}
		switch tp.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map:
		default:
			return ErrNullOnNonPointer
		}
	}
	if tp.Kind() == reflect.Bool {
		if v := node.Value; v != "true" && v != "false" {
			return ErrBadBoolLiteral
		}
	}
	return nil
}

// ValidateType returns an error if T is recursive, contains
// any fields that are missing the `yaml` struct tag, contains
// any fields with malformed `env` struct tag, or contains
// any unsupported types (signed and unsigned integers with unspecified
// width, interface (including `any`), function, channel, unsafe.Pointer,
// pointer to pointer, pointer to slice and pointer to map).
func ValidateType[T any]() error {
	stack := []reflect.Type{}
	var traverse func(path string, tp reflect.Type) error
	traverse = func(path string, tp reflect.Type) error {
		for i := range tp.NumField() {
			f := tp.Field(i)
			yamlTag := getYAMLFieldName(f.Tag)
			if yamlTag == "" {
				return fmt.Errorf("%s: %w", path+"."+f.Name, ErrMissingYAMLTag)
			}

			if err := validateEnvField(f); err != nil {
				return fmt.Errorf("%s: %w: %w", path+"."+f.Name, ErrInvalidEnvTag, err)
			}

			if f.Type.Kind() == reflect.Pointer {
				switch f.Type.Elem().Kind() {
				case reflect.Pointer, reflect.Slice, reflect.Map:
					return fmt.Errorf("%s: %w", path+"."+f.Name, ErrUnsupportedPtrType)
				}
			}

		LOOP:
			for tp := f.Type; ; {
				tp = firstNonPointer(tp)
				switch tp.Kind() {
				case reflect.Struct:
					tp := firstNonPointer(tp)
					for _, p := range stack {
						if p == tp {
							// Recursive type
							return fmt.Errorf("%s: %w",
								path+"."+f.Name, ErrRecursiveType)
						}
					}
					stack = append(stack, tp)
					err := traverse(path+"."+f.Name, tp)
					if err != nil {
						return err
					}
				case reflect.Chan,
					reflect.Func,
					reflect.Interface,
					reflect.UnsafePointer:
					return fmt.Errorf("%s: %w: %s",
						path+"."+f.Name, ErrUnsupportedType, tp.String())
				case reflect.Int,
					reflect.Uint:
					return fmt.Errorf("%s: %w: %s, %s",
						path+"."+f.Name, ErrUnsupportedType, tp.String(),
						"use integer type with specified width, "+
							"such as int32 or int64 instead of int")
				case reflect.Slice, reflect.Array:
					tp = tp.Elem()
					continue LOOP
				}
				break LOOP
			}
		}
		if len(stack) > 0 {
			stack = stack[:len(stack)-1]
		}
		return nil
	}
	var t T
	tp := reflect.TypeOf(t)
	stack = append(stack, tp)
	return traverse(tp.Name(), tp)
}

func findContentNodeByTag(node *yaml.Node, yamlTag string) *yaml.Node {
	// Find value node
	for i, n := range node.Content {
		if n.Value == yamlTag {
			return node.Content[i+1] // The value node is the next node
		}
	}
	return nil
}

func firstNonPointer(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func getYAMLFieldName(t reflect.StructTag) string {
	yamlTag := t.Get("yaml")
	if i := strings.IndexByte(yamlTag, ','); i != -1 {
		yamlTag = yamlTag[:i]
	}
	return yamlTag
}

func validateEnvField(f reflect.StructField) error {
	n, ok := f.Tag.Lookup("env")
	if !ok {
		return nil
	}
	if n == "" {
		return errors.New("empty")
	}
	if !regexEnvVarPOSIX.MatchString(n) {
		return fmt.Errorf(
			"must match the POSIX env var regexp: %s", regexEnvVarPOSIXPattern,
		)
	}
	switch k := f.Type.Kind(); {
	case kindIsPrimitive(k):
		return nil
	case k == reflect.Pointer && kindIsPrimitive(f.Type.Elem().Kind()):
		// Pointer to primitve
		return nil
	}
	return fmt.Errorf("env var of unsupported type: %s", f.Type.String())
}

const regexEnvVarPOSIXPattern = `^[A-Z_][A-Z0-9_]*$`

var regexEnvVarPOSIX = regexp.MustCompile(regexEnvVarPOSIXPattern)

func kindIsPrimitive(k reflect.Kind) bool {
	switch k {
	case reflect.String,
		reflect.Float32,
		reflect.Float64,
		reflect.Int,
		reflect.Uint,
		reflect.Int8,
		reflect.Uint8,
		reflect.Int16,
		reflect.Uint16,
		reflect.Int32,
		reflect.Uint32,
		reflect.Int64,
		reflect.Uint64,
		reflect.Bool:
		return true
	}
	return false
}
