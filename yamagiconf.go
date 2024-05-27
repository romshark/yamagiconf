// Package yamagiconf provides an opinionated configuration file parser
// using a subset of YAML and Go struct type restrictions to allow for
// consistent configuration files, easy validation and good error reporting.
// Supports primitive `env` struct tags used to overwrite fields from env vars.
// Also supports github.com/go-playground/validator struct tags.
package yamagiconf

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Errors, including type-specific errors.
var (
	ErrNilConfig           = errors.New("cannot load into nil config")
	ErrEmptyFile           = errors.New("empty file")
	ErrMalformedYAML       = errors.New("malformed YAML")
	ErrMissingYAMLTag      = errors.New("missing yaml struct tag")
	ErrYAMLInlineNonAnon   = errors.New("inline yaml on non-embedded field")
	ErrYAMLInlineOpt       = errors.New("use `yaml:\",inline\"` for embedded fields")
	ErrYAMLTagOnUnexported = errors.New("yaml tag on unexported field")
	ErrYAMLTagRedefined    = errors.New("a yaml tag must be unique")
	ErrYAMLAnchorRedefined = errors.New("yaml anchors must be unique throughout " +
		"the whole document")
	ErrYAMLAnchorUnused   = errors.New("yaml anchors must be referenced at least once")
	ErrEnvTagOnUnexported = errors.New("env tag on unexported field")
	ErrTagOnInterfaceImpl = errors.New("implementations of interfaces " +
		"yaml.Unmarshaler or encoding.TextUnmarshaler must not " +
		"contain yaml and env tags")
	ErrNoExportedFields = errors.New("no exported fields")
	ErrInvalidEnvTag    = fmt.Errorf("invalid env struct tag: "+
		"must match the POSIX env var regexp: %s", regexEnvVarPOSIXPattern)
	ErrEnvVarOnUnsupportedType = errors.New("env var on unsupported type")
	ErrEnvVarOnYAMLUnmarshImpl = errors.New("env var on yaml.Unmarshaler implementation")
	ErrMissingConfig           = errors.New("missing field in config file")
	ErrInvalidEnvVar           = errors.New("invalid env var")
	ErrValidation              = errors.New("validation")
	ErrValidateTagViolation    = errors.New("violates validation rule")
	ErrBadBoolLiteral          = errors.New("must be either false or true, " +
		"other variants of boolean literals of YAML are not supported")
	ErrBadNullLiteral = errors.New("must be null, " +
		"any other variants of null are not supported")
	ErrNullOnNonPointer = errors.New("cannot assign null to non-pointer type")
	ErrYAMLTagUsed      = errors.New("avoid using YAML tags")
	ErrRecursiveType    = errors.New("recursive type")
	ErrIllegalRootType  = errors.New("root type must be a struct type and must not " +
		"implement encoding.TextUnmarshaler and yaml.Unmarshaler")
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
//     unsafe.Pointer, pointer to pointer, pointer to slice, pointer to map).
//   - T is not a struct or implements yaml.Unmarshaler or encoding.TextUnmarshaler.
//   - T contains any structs with no exported fields.
//   - T contains any structs with yaml and/or env tags assigned to unexported fields.
//   - T contains any struct implementing either yaml.Unmarshaler or
//     encoding.TextUnmarshaler that contains fields with yaml or env struct tags.
//   - T contains any fields with env tag on a type that implements yaml.Unmarshaler.
//   - T contains any struct containing multiple fields with the same yaml tag.
//   - the yaml file is empty or not found.
//   - the yaml file doesn't contain a field specified by T.
//   - the yaml file is missing a field specified by T.
//   - the yaml file contains values that don't pass validation.
//   - the yaml file contains boolean literals other than `true` and `false`.
//   - the yaml file contains null values other than `null` (`~`, etc.).
//   - the yaml file assigns `null` to a non-pointer Go type.
//   - the yaml file contains any YAML tags (https://yaml.org/spec/1.2.2/#3212-tags).
//   - the yaml file contains any redeclared anchors.
//   - the yaml file contains any unused anchors.
func LoadFile[T any](yamlFilePath string, config *T) error {
	if config == nil {
		return ErrNilConfig
	}

	yamlSrcBytes, err := os.ReadFile(yamlFilePath)
	if err != nil {
		return fmt.Errorf("reading file %q: %w", yamlFilePath, err)
	}
	return Load(yamlSrcBytes, config)
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

	configTypeName := getConfigTypeName(configType)

	anchors := make(map[string]*anchor)
	err = validateYAMLValues(
		anchors, "", configTypeName, configType, rootNode.Content[0],
	)
	if err != nil {
		return err
	}

	// Check for unused anchors
	for _, anchor := range anchors {
		if !anchor.IsUsed {
			return fmt.Errorf("at %d:%d: anchor %q: %w",
				anchor.Line, anchor.Column, anchor.Anchor, ErrYAMLAnchorUnused)
		}
	}

	err = unmarshalEnv(configTypeName, "", reflect.ValueOf(config).Elem())
	if err != nil {
		return err
	}

	err = invokeValidateRecursively(
		configTypeName, reflect.ValueOf(config), rootNode.Content[0],
	)
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

// Validate behaves similar to Load and LoadFile just without parsing YAML
// and instead performing the same type and value checks on t.
// Validate will obviously not report line:column error location.
// Validate first validates type T, then validates t according to
// go-playground/validator struct tags, then recursively
// invokes all Validate methods returning an error if any.
func Validate[T any](t T) error {
	if err := ValidateType[T](); err != nil {
		return err
	}
	err := validator.New(validator.WithRequiredStructEnabled()).Struct(t)
	if err != nil {
		if errs, ok := err.(validator.ValidationErrors); ok {
			return fmt.Errorf("at %s: %w: %q",
				errs[0].StructNamespace(), ErrValidateTagViolation, errs[0].Tag())
		}
		return err
	}
	typeName := getConfigTypeName(reflect.TypeOf(t))
	return invokeValidateRecursively(typeName, reflect.ValueOf(t), nil)
}

type Validator interface{ Validate() error }

// asIface[I any] returns nil if v doesn't implement the Validator interface
// neither as a copy- nor as a pointer receiver.
func asIface[I any](v reflect.Value, allocateIfNecessary bool) (i I) {
	if !v.IsValid() {
		return i
	}
	ti := reflect.TypeOf((*I)(nil)).Elem()
	tp := v.Type()
	if tp.Implements(ti) {
		if tp.Kind() == reflect.Pointer && v.IsNil() {
			if !allocateIfNecessary {
				return i
			}
			return reflect.New(tp.Elem()).Interface().(I)
		}
		if v.CanInterface() {
			return v.Interface().(I)
		}
	}
	if v.CanAddr() {
		vp := v.Addr()
		if vp.CanInterface() && vp.Type().Implements(ti) {
			return vp.Interface().(I)
		}
	}
	return i
}

func implementsInterface[I any](t reflect.Type) bool {
	ti := reflect.TypeOf((*I)(nil)).Elem()
	if t.Implements(ti) {
		return true
	} else if t.Kind() != reflect.Pointer {
		return reflect.PointerTo(t).Implements(ti)
	}
	return false
}

func getConfigTypeName(t reflect.Type) string {
	if n := t.Name(); n != "" {
		return n
	}
	return "struct{...}"
}

// invokeValidateRecursively runs the Validate method for
// every field of type that implements the Validator interface recursively.
// Assumes type of v was validated first using ValidateType.
// If node != nil then assumes validateYAMLValues was ran first on it.
func invokeValidateRecursively(path string, v reflect.Value, node *yaml.Node) error {
	tp := v.Type()

	if v := asIface[Validator](v, false); v != nil {
		if err := v.Validate(); err != nil {
			if node == nil {
				return fmt.Errorf("at %s: %w: %w", path, ErrValidation, err)
			}
			return fmt.Errorf("at %d:%d: at %s: %w: %w",
				node.Line, node.Column, path, ErrValidation, err)
		}
	}
	for tp.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		tp, v = tp.Elem(), v.Elem()
	}

	switch tp.Kind() {
	case reflect.Struct:
		if implementsInterface[encoding.TextUnmarshaler](tp) ||
			implementsInterface[yaml.Unmarshaler](tp) {
			return nil
		}
		for i := range tp.NumField() {
			ft := tp.Field(i)
			if !ft.IsExported() {
				continue
			}
			fv := v.Field(i)
			yamlTag := getYAMLFieldName(ft.Tag)
			var nodeValue *yaml.Node
			if node != nil {
				nodeValue = node
				if !ft.Anonymous {
					nodeValue = findContentNodeByTag(node, yamlTag)
				}
			}
			path := path + "." + ft.Name
			if err := invokeValidateRecursively(path, fv, nodeValue); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			path := fmt.Sprintf("%s[%d]", path, i)
			var nodeItem *yaml.Node
			if node != nil {
				nodeItem = node.Content[i]
			}
			err := invokeValidateRecursively(path, v.Index(i), nodeItem)
			if err != nil {
				return err
			}

		}
	case reflect.Map:
		mapKeys := mapKeysSorted(v)
		if node == nil {
			for _, k := range mapKeys {
				err := invokeValidateRecursively(path, k, nil)
				if err != nil {
					return err
				}
				path := fmt.Sprintf("%s[%v]", path, k)
				err = invokeValidateRecursively(path, v.MapIndex(k), nil)
				if err != nil {
					return err
				}
			}
		} else {
			for _, k := range mapKeys {
				for i := 0; i < len(node.Content); i += 2 {
					if k.String() != node.Content[i].Value {
						continue
					}
					err := invokeValidateRecursively(path, k, node.Content[i])
					if err != nil {
						return err
					}
					path := fmt.Sprintf("%s[%v]", path, k)
					err = invokeValidateRecursively(
						path, v.MapIndex(k), node.Content[i+1],
					)
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
			} else if vi := asIface[encoding.TextUnmarshaler](v, true); vi != nil {
				if err := vi.UnmarshalText([]byte(env)); err != nil {
					return errUnmarshalEnv(path, envVar, tp, err)
				}
				v.Set(reflect.ValueOf(vi))
				return nil
			}
			newValue := reflect.New(tp.Elem())
			v.Set(newValue) // Set pointer
			v = newValue.Elem()
			tp = tp.Elem()
		}
	}

	if vi := asIface[encoding.TextUnmarshaler](v, true); vi != nil {
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		if err := vi.UnmarshalText([]byte(env)); err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
	}

	if tp == typeTimeDuration {
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		d, err := time.ParseDuration(env)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetInt(int64(d))
		return nil
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
			return errUnmarshalEnv(path, envVar, tp, nil)
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
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetFloat(f)
	case reflect.Float64:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		f, err := strconv.ParseFloat(env, 64)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetFloat(f)
	case reflect.Int8:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 8)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetInt(int64(i))
	case reflect.Uint8:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 8)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetUint(uint64(i))
	case reflect.Int16:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 16)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetInt(int64(i))
	case reflect.Uint16:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 16)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetUint(uint64(i))
	case reflect.Int32:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 32)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetInt(int64(i))
	case reflect.Uint32:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 32)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetUint(uint64(i))
	case reflect.Int64:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseInt(env, 10, 64)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetInt(int64(i))
	case reflect.Uint64:
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		i, err := strconv.ParseUint(env, 10, 64)
		if err != nil {
			return errUnmarshalEnv(path, envVar, tp, err)
		}
		v.SetUint(uint64(i))
	case reflect.Struct:
		for i := range tp.NumField() {
			f := tp.Field(i)
			if !f.IsExported() {
				continue
			}
			n := f.Tag.Get("env")
			err := unmarshalEnv(path+"."+f.Name, n, v.Field(i))
			if err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			err := unmarshalEnv(fmt.Sprintf("%s[%d]", path, i), "", v.Index(i))
			if err != nil {
				return err
			}
		}
	case reflect.Map:
		keys := mapKeysSorted(v)
		for _, key := range keys {
			path := fmt.Sprintf("%s[%s]", path, key.String())
			value := v.MapIndex(key)

			if tp.Elem().Kind() == reflect.Pointer {
				if value.IsNil() {
					continue
				}
				if err := unmarshalEnv(path, "", value.Elem()); err != nil {
					return err
				}
				continue
			}

			val := reflect.New(value.Type()).Elem()
			val.Set(value)

			if err := unmarshalEnv(path, "", val); err != nil {
				return err
			}
			v.SetMapIndex(key, val)
		}
	}
	return nil
}

var typeTimeDuration = reflect.TypeOf(time.Duration(0))

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

type anchor struct {
	*yaml.Node
	Defined bool
	IsUsed  bool
}

// validateYAMLValues returns an error if the yaml model contains illegal values
// or is missing values specified by T. Assumes that tp has already been validated.
func validateYAMLValues(
	anchors map[string]*anchor, yamlTag, path string, tp reflect.Type, node *yaml.Node,
) error {
	if err := validateValue(tp, node); err != nil {
		if yamlTag != "" {
			return fmt.Errorf("at %d:%d: %q (%s): %w",
				node.Line, node.Column, yamlTag, path, err)
		}
		return fmt.Errorf("at %d:%d: %s: %w",
			node.Line, node.Column, path, err)
	}

	if node.Anchor != "" {
		if p, ok := anchors[node.Anchor]; ok && p.Defined {
			return fmt.Errorf("at %d:%d: redefined anchor %q at %d:%d: %w",
				node.Line, node.Column,
				node.Anchor,
				p.Line, p.Column,
				ErrYAMLAnchorRedefined)
		}
		anchors[node.Anchor] = &anchor{Node: node, Defined: true}
	}
	if node.Alias != nil {
		anchors[node.Alias.Anchor].IsUsed = true
	}

	switch tp.Kind() {
	case reflect.Struct:
		if implementsInterface[encoding.TextUnmarshaler](tp) ||
			implementsInterface[yaml.Unmarshaler](tp) {
			return nil
		}
		for i := range tp.NumField() {
			f := tp.Field(i)
			if !f.IsExported() {
				continue
			}
			yamlTag := getYAMLFieldName(f.Tag)
			path := path + "." + f.Name
			contentNode := node
			if !f.Anonymous {
				contentNode = findContentNodeByTag(node, yamlTag)
			}
			if contentNode == nil {
				return fmt.Errorf("at %s (as %q): %w",
					path, yamlTag, ErrMissingConfig)
			}
			err := validateYAMLValues(anchors, yamlTag, path, f.Type, contentNode)
			if err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		tp := tp.Elem()
		for index, node := range node.Content {
			path := fmt.Sprintf("%s[%d]", path, index)
			if err := validateYAMLValues(anchors, yamlTag, path, tp, node); err != nil {
				return err
			}
		}
	case reflect.Map:
		tpKey, tpVal := tp.Key(), tp.Elem()
		for i := 0; i < len(node.Content); i += 2 {
			path := fmt.Sprintf("%s[%q]", path, node.Content[i].Value)
			// Validate key
			err := validateYAMLValues(anchors, yamlTag, path, tpKey, node.Content[i])
			if err != nil {
				return err
			}
			// Validate value
			err = validateYAMLValues(anchors, yamlTag, path, tpVal, node.Content[i+1])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func validateValue(tp reflect.Type, node *yaml.Node) error {
	if node.Style == yaml.TaggedStyle {
		return fmt.Errorf("tag %q: %w", node.Tag, ErrYAMLTagUsed)
	}
	kind := tp.Kind()
	if kind == reflect.String && node.Value == "null" {
		switch node.Style {
		case yaml.DoubleQuotedStyle, yaml.SingleQuotedStyle:
			return nil
		default:
			return ErrNullOnNonPointer
		}
	}
	if v := node.Value; v == "~" || strings.EqualFold(v, "null") {
		if v != "null" {
			return ErrBadNullLiteral
		}
		switch kind {
		case reflect.Pointer, reflect.Slice, reflect.Map:
		default:
			return ErrNullOnNonPointer
		}
	}
	if kind == reflect.Bool && node.Alias == nil {
		if v := node.Value; v != "true" && v != "false" {
			return ErrBadBoolLiteral
		}
	}
	return nil
}

// ValidateType returns an error if any violation of rules defined by
// LoadFile is determined, otherwise returns nil.
func ValidateType[T any]() error {
	stack := []reflect.Type{}
	var traverse func(path string, tp reflect.Type) error
	traverse = func(path string, tp reflect.Type) error {
		switch tp.Kind() {
		case reflect.Struct:
			if implementsInterface[encoding.TextUnmarshaler](tp) ||
				implementsInterface[yaml.Unmarshaler](tp) {
				return validateTypeImplementingIfaces(path, tp)
			}

			for _, p := range stack {
				if p == tp {
					// Recursive type
					return fmt.Errorf("at %s: %w",
						path, ErrRecursiveType)
				}
			}
			stack = append(stack, tp) // Push stack

			exportedFields := 0
			yamlTags := map[string]string{} // tag -> path
			for i := range tp.NumField() {
				f := tp.Field(i)
				path := path + "." + f.Name
				yamlTag := getYAMLFieldName(f.Tag)
				isInline := yamlTagIsInline(f.Tag)
				isExported := f.IsExported()
				if isExported && f.Anonymous && (yamlTag != "" || !isInline) {
					return fmt.Errorf("at %s: %w", path, ErrYAMLInlineOpt)
				}
				if isExported && !f.Anonymous && isInline {
					return fmt.Errorf("at %s: %w", path, ErrYAMLInlineNonAnon)
				}
				if yamlTag == "" && isExported && !f.Anonymous {
					return fmt.Errorf("at %s: %w", path, ErrMissingYAMLTag)
				} else if yamlTag != "" && !isExported {
					return fmt.Errorf("at %s: %w", path, ErrYAMLTagOnUnexported)
				}

				if err := validateEnvField(f); err != nil {
					return fmt.Errorf("at %s: %w", path, err)
				}

				if !isExported {
					continue
				}
				exportedFields++

				if previous, ok := yamlTags[yamlTag]; ok {
					return fmt.Errorf(
						"at %s: yaml tag %q previously defined on field %s: %w",
						path, yamlTag, previous, ErrYAMLTagRedefined)
				}
				yamlTags[yamlTag] = path

				err := traverse(path, f.Type)
				if err != nil {
					return err
				}
			}
			if exportedFields < 1 {
				return fmt.Errorf("at %s: %w", path, ErrNoExportedFields)
			}
			stack = stack[:len(stack)-1] // Pop stack
			return nil
		case reflect.Chan,
			reflect.Func,
			reflect.Interface,
			reflect.UnsafePointer:
			return fmt.Errorf("at %s: %w: %s", path, ErrUnsupportedType, tp.String())
		case reflect.Pointer:
			tp = tp.Elem()
			switch tp.Kind() {
			case reflect.Pointer, reflect.Slice, reflect.Map:
				return fmt.Errorf("at %s: %w", path, ErrUnsupportedPtrType)
			}
			return traverse(path, tp)
		case reflect.Int:
			return fmt.Errorf("at %s: %w: %s, %s",
				path, ErrUnsupportedType, tp.String(),
				"use integer type with specified width, "+
					"such as int8, int16, int32 or int64 instead of int")
		case reflect.Uint:
			return fmt.Errorf("at %s: %w: %s, %s",
				path, ErrUnsupportedType, tp.String(),
				"use unsigned integer type with specified width, "+
					"such as uint8, uint16, uint32 or uint64 instead of uint")
		case reflect.Slice, reflect.Array:
			return traverse(path, tp.Elem())
		case reflect.Map:
			if err := traverse(path+"[key]", tp.Key()); err != nil {
				return err
			}
			return traverse(path+"[value]", tp.Elem())
		}
		return nil
	}
	var t T
	tp := reflect.TypeOf(t)

	n := tp.Name()
	if n == "" {
		// Anonymous type
		n = "struct{...}"
	}
	if tp.Kind() != reflect.Struct ||
		implementsInterface[encoding.TextUnmarshaler](tp) ||
		implementsInterface[yaml.Unmarshaler](tp) {
		return fmt.Errorf("at %s: %w", n, ErrIllegalRootType)
	}
	return traverse(n, tp)
}

// validateTypeImplementingIfaces assumes that implementer is
// implementing either encoding.TextUnmarshaler or yaml.Unmarshaler
func validateTypeImplementingIfaces(path string, implementer reflect.Type) error {
	implementedIface := "yaml.Unmarshaler"
	if implementsInterface[encoding.TextUnmarshaler](implementer) {
		implementedIface = "encoding.TextUnmarshaler"
	}
	for i := range implementer.NumField() {
		f := implementer.Field(i)
		if tag := getYAMLFieldName(f.Tag); tag != "" {
			return fmt.Errorf("at %s: struct implements %s but field contains tag "+
				"\"yaml\" (%q): %w", path, implementedIface, tag, ErrTagOnInterfaceImpl)
		}
		if tag := f.Tag.Get("env"); tag != "" {
			return fmt.Errorf("at %s: struct implements %s but field contains tag "+
				"\"env\" (%q): %w", path, implementedIface, tag, ErrTagOnInterfaceImpl)
		}
	}
	return nil
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

func getYAMLFieldName(t reflect.StructTag) string {
	yamlTag := t.Get("yaml")
	if i := strings.IndexByte(yamlTag, ','); i != -1 {
		yamlTag = yamlTag[:i]
	}
	return yamlTag
}

func yamlTagIsInline(t reflect.StructTag) bool {
	yamlTag := t.Get("yaml")
	opts := strings.Split(yamlTag, ",")
	for _, opt := range opts {
		if opt == "inline" {
			return true
		}
	}
	return false
}

func validateEnvField(f reflect.StructField) error {
	n, ok := f.Tag.Lookup("env")
	if !ok {
		return nil
	}

	if !f.IsExported() {
		return ErrEnvTagOnUnexported
	}

	if n == "" || !regexEnvVarPOSIX.MatchString(n) {
		return ErrInvalidEnvTag
	}

	if implementsInterface[yaml.Unmarshaler](f.Type) {
		return fmt.Errorf("%w: %s", ErrEnvVarOnYAMLUnmarshImpl, f.Type.String())
	}

	switch k := f.Type.Kind(); {
	case kindIsPrimitive(k):
		return nil
	case k == reflect.Pointer && kindIsPrimitive(f.Type.Elem().Kind()):
		// Pointer to primitve
		return nil
	case implementsInterface[encoding.TextUnmarshaler](f.Type):
		return nil
	}
	return fmt.Errorf("%w: %s", ErrEnvVarOnUnsupportedType, f.Type.String())
}

const regexEnvVarPOSIXPattern = `^[A-Z_][A-Z0-9_]*$`

var regexEnvVarPOSIX = regexp.MustCompile(regexEnvVarPOSIXPattern)

func kindIsPrimitive(k reflect.Kind) bool {
	switch k {
	case reflect.String,
		reflect.Float32,
		reflect.Float64,
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

func mapKeysSorted(m reflect.Value) []reflect.Value {
	keys := m.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
	})
	return keys
}
