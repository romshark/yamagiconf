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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"go.yaml.in/yaml/v4"
)

// All possible errors.
// Errors in the YAML document begin with ErrYAML...
// Errors in the Go target type begin with ErrType...
// Errors in the env variables begin with ErrEnv...
var (
	ErrConfigNil     = errors.New("cannot load into nil config")
	ErrValidation    = errors.New("validation")
	ErrValidationTag = errors.New("violates validation rule")

	ErrYAMLMultidoc        = errors.New("multi-document YAML files are not supported")
	ErrYAMLEmptyFile       = errors.New("empty file")
	ErrYAMLMalformed       = errors.New("malformed YAML")
	ErrYAMLInlineNonAnon   = errors.New("inline yaml on non-embedded field")
	ErrYAMLInlineOpt       = errors.New("use `yaml:\",inline\"` for embedded fields")
	ErrYAMLTagOnUnexported = errors.New("yaml struct tag on unexported field")
	ErrYAMLTagRedefined    = errors.New("a yaml struct tag must be unique")
	ErrYAMLAnchorRedefined = errors.New("yaml anchors must be unique throughout " +
		"the whole document")
	ErrYAMLAnchorUnused   = errors.New("yaml anchors must be referenced at least once")
	ErrYAMLAnchorNoValue  = errors.New("don't use anchors with implicit null value")
	ErrYAMLMissingConfig  = errors.New("missing field in config file")
	ErrYAMLBadBoolLiteral = errors.New("must be either false or true, " +
		"other variants of boolean literals of YAML are not supported")
	ErrYAMLTagUsed          = errors.New("avoid using YAML tags")
	ErrYAMLNullOnNonPointer = errors.New("cannot assign null to non-pointer type")
	ErrYAMLBadNullLiteral   = errors.New("must be null, " +
		"any other variants of null are not supported")
	ErrYAMLNonStrOnTextUnmarsh = errors.New("value must be a string because the " +
		"target type implements encoding.TextUnmarshaler")
	ErrYAMLMergeKey = errors.New("avoid using YAML merge keys")

	// ErrYAMLEmptyArrayItem applies to both Go arrays and slices even though
	// an empty item would be parsed correctly as zero-value in case of Go arrays
	// to preserve consistency and avoid having exceptional behavior for arrays.
	ErrYAMLEmptyArrayItem = errors.New("avoid empty items in arrays as those will not " +
		"be appended to the target Go slice")

	ErrTypeRecursive   = errors.New("recursive type")
	ErrTypeIllegalRoot = errors.New("root type must be a struct type and must not " +
		"implement encoding.TextUnmarshaler and yaml.Unmarshaler")
	ErrTypeMissingYAMLTag     = errors.New("missing yaml struct tag")
	ErrTypeEnvTagOnUnexported = errors.New("env tag on unexported field")
	ErrTypeTagOnInterfaceImpl = errors.New("implementations of interfaces " +
		"yaml.Unmarshaler or encoding.TextUnmarshaler must not " +
		"contain yaml and env struct tags")
	ErrTypeEnvOnYAMLUnmarsh = errors.New("env var on yaml.Unmarshaler implementation")
	ErrTypeNoExportedFields = errors.New("no exported fields")
	ErrTypeInvalidEnvTag    = fmt.Errorf("invalid env struct tag: "+
		"must match the POSIX env var regexp: %s", regexEnvVarPOSIXPattern)
	ErrTypeEnvVarOnUnsupportedType = errors.New("env var on unsupported type")
	ErrTypeEnvTagInContainer = errors.New(
		"env tag behind pointer, slice, or map is not allowed")
	ErrTypeUnsupported        = errors.New("unsupported type")
	ErrTypeUnsupportedPtrType = errors.New("unsupported pointer type")

	ErrEnvInvalidVar = errors.New("invalid env var")
)

// Option is a functional option for [Load] and [LoadFile].
type Option func(*options)

type options struct {
	strictPresence bool
}

// WithStrictPresence enables strict field-presence checking:
// every field defined in the Go struct type must be present in the YAML source.
// By default yamagiconf does not require all fields to be present,
// allowing fields to be omitted and left at their zero value.
func WithStrictPresence() Option {
	return func(o *options) { o.strictPresence = true }
}

func applyOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// LoadFile reads and validates the configuration of type T from a YAML file.
// Will return an error if:
//   - ValidateType returns an error for T.
//   - the yaml file is empty or not found.
//   - the yaml file doesn't contain a field specified by T.
//   - [WithStrictPresence] is set and the yaml file is missing a field specified by T.
//   - the yaml file contains values that don't pass validation.
//   - the yaml file contains boolean literals other than `true` and `false`.
//   - the yaml file contains null values other than `null` (`~`, etc.).
//   - the yaml file assigns `null` to a non-pointer Go type.
//   - the yaml file contains any YAML tags (https://yaml.org/spec/1.2.2/#3212-tags).
//   - the yaml file contains any redeclared anchors.
//   - the yaml file contains any unused anchors.
//   - the yaml file contains any anchors with implicit null value (no value).
//   - the yaml file assigns non-string values to Go types implementing the
//     encoding.TextUnmarshaler interface.
func LoadFile[T any](yamlFilePath string, config *T, opts ...Option) error {
	if config == nil {
		return ErrConfigNil
	}

	yamlSrcBytes, err := os.ReadFile(yamlFilePath)
	if err != nil {
		return fmt.Errorf("reading file %q: %w", yamlFilePath, err)
	}
	return Load(yamlSrcBytes, config, opts...)
}

// Load reads and validates the configuration of type T from yamlSource.
// Load behaves similar to LoadFile.
func Load[T any, S string | []byte](yamlSource S, config *T, opts ...Option) error {
	if config == nil {
		return ErrConfigNil
	}
	if len(yamlSource) == 0 {
		return ErrYAMLEmptyFile
	}

	if err := ValidateType[T](); err != nil {
		return err
	}

	var rootNode yaml.Node
	{
		dec := newDecoderYAML(yamlSource)
		if err := dec.Decode(&rootNode); err != nil {
			return fmt.Errorf("%w: %w", ErrYAMLMalformed, err)
		}

		// Check if multi-doc
		var n yaml.Node
		if err := dec.Decode(&n); err == nil {
			return fmt.Errorf("at %d:%d: %w", n.Line, n.Column, ErrYAMLMultidoc)
		} else if !errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: %w", ErrYAMLMultidoc, err)
		}
	}

	configType := reflect.TypeFor[T]()

	configTypeName := getConfigTypeName(configType)

	o := applyOptions(opts)
	anchors := make(map[string]*anchor)
	err := validateYAMLValues(
		o, anchors, "", configTypeName, configType, rootNode.Content[0],
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

	{
		dec := newDecoderYAML(yamlSource)
		dec.KnownFields(true)
		if err := dec.Decode(config); err != nil {
			return fmt.Errorf("%w: %w", ErrYAMLMalformed, err)
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

	err = runValidatorStruct(validator.New(
		validator.WithRequiredStructEnabled(),
	), config)
	if err != nil {
		if errs, ok := err.(validator.ValidationErrors); ok {
			err := errs[0]
			line, column, yamlTag := mustFindLocationByValidatorNamespace[T](
				err.StructNamespace(), &rootNode,
			)
			if yamlTag == "-" {
				// TODO: report env var name if any.

				// Ignored field, use Go field name instead of tag.
				return fmt.Errorf("at %s: %w: %q",
					err.StructNamespace(), ErrValidationTag, err.Tag())
			}
			return fmt.Errorf("at %d:%d: %q %w: %q",
				line, column, yamlTag, ErrValidationTag, err.Tag())
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
	err := runValidatorStruct(validator.New(validator.WithRequiredStructEnabled()), t)
	if err != nil {
		if errs, ok := err.(validator.ValidationErrors); ok {
			return fmt.Errorf("at %s: %w: %q",
				errs[0].StructNamespace(), ErrValidationTag, errs[0].Tag())
		}
		return err
	}
	typeName := getConfigTypeName(reflect.TypeOf(t))
	return invokeValidateRecursively(typeName, reflect.ValueOf(t), nil)
}

// Validator defines the interface yamagiconf supports for custom validation code.
// Any implementation of this interface will be found (recursively) and the Validate
// method will be invoked.
type Validator interface{ Validate() error }

// runValidatorStruct wraps v.Struct to recover from panics emitted by
// go-playground/validator when a validate tag is incompatible with the
// field type (e.g. validate:"url" on a custom struct).
func runValidatorStruct(v *validator.Validate, s any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrValidationTag, r)
		}
	}()
	return v.Struct(s)
}

// asIface[I any] returns nil if v doesn't implement the Validator interface
// neither as a copy- nor as a pointer receiver.
func asIface[I any](v reflect.Value, allocateIfNecessary bool) (i I) {
	if !v.IsValid() {
		return i
	}
	ti := reflect.TypeFor[I]()
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
	if pt := reflect.PointerTo(v.Type()); pt.Implements(ti) {
		// Pointer to v implements the interface
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		return ptr.Interface().(I)
	}
	return i
}

func implementsInterface[I any](t reflect.Type) bool {
	ti := reflect.TypeFor[I]()
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

	if node != nil && node.Alias != nil {
		node = node.Alias
	}

	switch tp.Kind() {
	case reflect.Struct:
		for i := range tp.NumField() {
			ft := tp.Field(i)
			if !ft.IsExported() {
				continue
			}
			fv := v.Field(i)
			yamlTag := getYAMLFieldName(ft.Tag)
			var nodeValue *yaml.Node
			if node != nil && yamlTag != "-" {
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
		if node != nil && node.Kind != yaml.SequenceNode {
			node = nil
		}
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
		if node != nil && node.Kind != yaml.MappingNode {
			node = nil
		}
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

	textUnmarshaler := asIface[encoding.TextUnmarshaler](v, true)
	if isPtr := tp.Kind() == reflect.Pointer; isPtr &&
		tp.Elem().Kind() == reflect.Struct && !v.IsNil() && textUnmarshaler == nil {
		// Pointer to a struct type that doesn't implement encoding.TextUnmarshaler
		v, tp = v.Elem(), tp.Elem()
	} else if isPtr {
		env, ok := os.LookupEnv(envVar)
		if ok {
			if env == "null" {
				v.Set(reflect.Zero(v.Type()))
				return nil
			} else if textUnmarshaler != nil {
				if err := textUnmarshaler.UnmarshalText([]byte(env)); err != nil {
					return errUnmarshalEnv(path, envVar, tp, err)
				}
				v.Set(reflect.ValueOf(textUnmarshaler))
				return nil
			}
			newValue := reflect.New(tp.Elem())
			v.Set(newValue) // Set pointer
			v = newValue.Elem()
			tp = tp.Elem()
		}
	}

	if textUnmarshaler != nil {
		env, ok := os.LookupEnv(envVar)
		if !ok {
			return nil
		}
		if err := textUnmarshaler.UnmarshalText([]byte(env)); err != nil {
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
	case reflect.Array:
		for i := range v.Len() {
			err := unmarshalEnv(fmt.Sprintf("%s[%d]", path, i), "", v.Index(i))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

var typeTimeDuration = reflect.TypeFor[time.Duration]()

func errUnmarshalEnv(path, envVar string, tp reflect.Type, err error) error {
	if err != nil {
		return fmt.Errorf("at %s: %w %s: expected %s: %w",
			path, ErrEnvInvalidVar, envVar, tp.String(), err)
	}
	return fmt.Errorf("at %s: %w %s: expected %s",
		path, ErrEnvInvalidVar, envVar, tp.String())
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
		if yamlTag == "-" {
			continue // Ignored field.
		}
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
	if before, after, ok := strings.Cut(s, "."); ok {
		return before, after
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
	opts options, anchors map[string]*anchor, yamlTag, path string, tp reflect.Type, node *yaml.Node,
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
		if node.Value == "" && node.Style != yaml.DoubleQuotedStyle &&
			node.Style != yaml.SingleQuotedStyle && len(node.Content) < 1 {
			return fmt.Errorf("at %d:%d: anchor %q: %w",
				node.Line, node.Column, node.Anchor, ErrYAMLAnchorNoValue)
		}
		anchors[node.Anchor] = &anchor{Node: node, Defined: true}
	}
	if node.Alias != nil {
		anchors[node.Alias.Anchor].IsUsed = true
	}

	if implementsInterface[encoding.TextUnmarshaler](tp) &&
		node.Kind != yaml.ScalarNode {
		return fmt.Errorf("at %d:%d: %w: %s",
			node.Line, node.Column, ErrYAMLNonStrOnTextUnmarsh, tp.String())
	}

	if tp.Kind() == reflect.Pointer {
		if node != nil && node.Tag == "!!null" {
			// Expected a pointer and received "null", all good.
			return nil
		}
		for tp.Kind() == reflect.Pointer {
			tp = tp.Elem()
		}
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
			if yamlTag == "-" {
				continue // Ignored field.
			}
			path := path + "." + f.Name
			contentNode := node
			if !f.Anonymous {
				contentNode = findContentNodeByTag(node, yamlTag)
			}
			if contentNode == nil {
				if opts.strictPresence {
					return fmt.Errorf("at %s (as %q): %w",
						path, yamlTag, ErrYAMLMissingConfig)
				}
				continue
			}
			for _, n := range contentNode.Content {
				if n.Tag == "!!merge" {
					return fmt.Errorf("at %d:%d: %w",
						n.Line, n.Column, ErrYAMLMergeKey)
				}
			}
			err := validateYAMLValues(opts, anchors, yamlTag, path, f.Type, contentNode)
			if err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		tp := tp.Elem()
		for index, node := range node.Content {
			if node.Tag == "!!null" && node.Value == "" {
				// If it's a null item with no value then no zero value item would be
				// appended to a Go slice.
				return fmt.Errorf("at %d:%d: %q (%s): %w",
					node.Line, node.Column, yamlTag, path, ErrYAMLEmptyArrayItem)
			}
			path := fmt.Sprintf("%s[%d]", path, index)
			if err := validateYAMLValues(opts, anchors, yamlTag, path, tp, node); err != nil {
				return err
			}
		}
	case reflect.Map:
		tpKey, tpVal := tp.Key(), tp.Elem()
		for i := 0; i < len(node.Content); i += 2 {
			path := fmt.Sprintf("%s[%q]", path, node.Content[i].Value)
			// Validate key
			err := validateYAMLValues(opts, anchors, yamlTag, path, tpKey, node.Content[i])
			if err != nil {
				return err
			}
			// Validate value
			err = validateYAMLValues(opts, anchors, yamlTag, path, tpVal, node.Content[i+1])
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
			return ErrYAMLNullOnNonPointer
		}
	}
	if v := node.Value; v == "~" || strings.EqualFold(v, "null") {
		if v != "null" {
			return ErrYAMLBadNullLiteral
		}
		switch kind {
		case reflect.Pointer, reflect.Slice, reflect.Map:
		default:
			return ErrYAMLNullOnNonPointer
		}
	}
	if kind == reflect.Bool && node.Alias == nil {
		switch node.Value {
		case "true", "false", "":
		default:
			return ErrYAMLBadBoolLiteral
		}
	}
	return nil
}

// ValidateType returns an error if...
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
func ValidateType[T any]() error {
	stack := []reflect.Type{}
	var traverse func(path string, tp reflect.Type, behindNullable bool) error
	traverse = func(path string, tp reflect.Type, behindNullable bool) error {
		if implementsInterface[encoding.TextUnmarshaler](tp) ||
			implementsInterface[yaml.Unmarshaler](tp) {
			return validateTypeImplementingIfaces(path, tp)
		}

		switch tp.Kind() {
		case reflect.Struct:
			if slices.Contains(stack, tp) {
				// Recursive type
				return fmt.Errorf("at %s: %w",
					path, ErrTypeRecursive)
			}
			stack = append(stack, tp) // Push stack

			exportedFields := 0
			yamlTags := map[string]string{} // tag -> path
			for i := range tp.NumField() {
				f := tp.Field(i)
				yamlTag := getYAMLFieldName(f.Tag)
				yamlIgnored := yamlTag == "-"
				path := path + "." + f.Name
				isExported := f.IsExported()
				if !yamlIgnored {
					isInline := yamlTagIsInline(f.Tag)
					switch {
					case isExported && f.Anonymous && (yamlTag != "" || !isInline):
						return fmt.Errorf("at %s: %w", path, ErrYAMLInlineOpt)
					case isExported && !f.Anonymous && isInline:
						return fmt.Errorf("at %s: %w", path, ErrYAMLInlineNonAnon)
					case yamlTag == "" && isExported && !f.Anonymous:
						return fmt.Errorf("at %s: %w", path, ErrTypeMissingYAMLTag)
					case yamlTag != "" && !isExported:
						return fmt.Errorf("at %s: %w", path, ErrYAMLTagOnUnexported)
					}
				}

				if behindNullable {
					if _, ok := f.Tag.Lookup("env"); ok {
						return fmt.Errorf("at %s: %w", path, ErrTypeEnvTagInContainer)
					}
				}

				if err := validateEnvField(f); err != nil {
					return fmt.Errorf("at %s: %w", path, err)
				}

				if !isExported || yamlIgnored {
					continue
				}
				exportedFields++

				// Avoid checking tag redefinition for embedded fields.
				// For embedded fields yamlTag will always be == "".
				if yamlTag != "" {
					if previous, ok := yamlTags[yamlTag]; ok {
						return fmt.Errorf(
							"at %s: yaml tag %q previously defined on field %s: %w",
							path, yamlTag, previous, ErrYAMLTagRedefined)
					}
					yamlTags[yamlTag] = path
				}
				err := traverse(path, f.Type, behindNullable)
				if err != nil {
					return err
				}
			}
			if exportedFields < 1 {
				return fmt.Errorf("at %s: %w", path, ErrTypeNoExportedFields)
			}
			stack = stack[:len(stack)-1] // Pop stack
			return nil
		case reflect.Chan,
			reflect.Func,
			reflect.Interface,
			reflect.UnsafePointer:
			return fmt.Errorf("at %s: %w: %s", path, ErrTypeUnsupported, tp.String())
		case reflect.Pointer:
			tp = tp.Elem()
			switch tp.Kind() {
			case reflect.Pointer, reflect.Slice, reflect.Map:
				return fmt.Errorf("at %s: %w", path, ErrTypeUnsupportedPtrType)
			}
			return traverse(path, tp, true)
		case reflect.Int:
			return fmt.Errorf("at %s: %w: %s, %s",
				path, ErrTypeUnsupported, tp.String(),
				"use integer type with specified width, "+
					"such as int8, int16, int32 or int64 instead of int")
		case reflect.Uint:
			return fmt.Errorf("at %s: %w: %s, %s",
				path, ErrTypeUnsupported, tp.String(),
				"use unsigned integer type with specified width, "+
					"such as uint8, uint16, uint32 or uint64 instead of uint")
		case reflect.Slice:
			return traverse(path, tp.Elem(), true)
		case reflect.Array:
			return traverse(path, tp.Elem(), behindNullable)
		case reflect.Map:
			if err := traverse(path+"[key]", tp.Key(), true); err != nil {
				return err
			}
			return traverse(path+"[value]", tp.Elem(), true)
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
		return fmt.Errorf("at %s: %w", n, ErrTypeIllegalRoot)
	}
	return traverse(n, tp, false)
}

// validateTypeImplementingIfaces assumes that implementer is
// implementing either encoding.TextUnmarshaler or yaml.Unmarshaler
func validateTypeImplementingIfaces(path string, implementer reflect.Type) error {
	implementedIface := "yaml.Unmarshaler"
	if implementsInterface[encoding.TextUnmarshaler](implementer) {
		implementedIface = "encoding.TextUnmarshaler"
	}
	if implementer.Kind() != reflect.Struct {
		return nil
	}
	for i := range implementer.NumField() {
		f := implementer.Field(i)
		if tag := getYAMLFieldName(f.Tag); tag != "" && tag != "-" {
			return fmt.Errorf("at %s: struct implements %s but field contains tag "+
				"\"yaml\" (%q): %w", path, implementedIface, tag,
				ErrTypeTagOnInterfaceImpl)
		}
		if tag := f.Tag.Get("env"); tag != "" {
			return fmt.Errorf("at %s: struct implements %s but field contains tag "+
				"\"env\" (%q): %w", path, implementedIface, tag,
				ErrTypeTagOnInterfaceImpl)
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
	opts := strings.SplitSeq(yamlTag, ",")
	for opt := range opts {
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
		return ErrTypeEnvTagOnUnexported
	}

	if n == "" || !regexEnvVarPOSIX.MatchString(n) {
		return ErrTypeInvalidEnvTag
	}

	if implementsInterface[yaml.Unmarshaler](f.Type) {
		return fmt.Errorf("%w: %s", ErrTypeEnvOnYAMLUnmarsh, f.Type.String())
	}

	switch k := f.Type.Kind(); {
	case kindIsPrimitive(k):
		return nil
	case k == reflect.Pointer && kindIsPrimitive(f.Type.Elem().Kind()):
		// Pointer to primitive
		return nil
	case implementsInterface[encoding.TextUnmarshaler](f.Type):
		return nil
	}
	return fmt.Errorf("%w: %s", ErrTypeEnvVarOnUnsupportedType, f.Type.String())
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
