package yamagiconf_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/romshark/yamagiconf"
	"github.com/stretchr/testify/require"
)

func LoadSrc[T any](src string) (*T, error) {
	var c T
	if err := yamagiconf.Load(src, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func TestLoadFile(t *testing.T) {
	type Embedded struct {
		AnyString        string            `yaml:"any-string"`
		StrQuotedNull    string            `yaml:"str-quoted-null"`
		StrDubQuotNull   string            `yaml:"str-doublequoted-null"`
		StrBlockNull     string            `yaml:"str-block-null"`
		StrFoldBlockNull string            `yaml:"str-foldblock-null"`
		RequiredString   string            `yaml:"required-string" validate:"required"`
		MapStringString  map[string]string `yaml:"map-string-string"`
		MapIntInt        map[int16]int16   `yaml:"map-int-int"`
		SliceStr         []string          `yaml:"slice-str"`
		SliceInt         []int64           `yaml:"slice-int"`
	}
	type TestConfig struct {
		Embedded `yaml:"embedded"`
		Int32    int32 `yaml:"int32"`
		Enabled  bool  `yaml:"enabled"`
	}

	p := filepath.Join(t.TempDir(), "test-config.yaml")
	err := os.WriteFile(p, []byte(`# test YAML file
int32: 42
embedded:
  any-string: 'any string'
  str-quoted-null: 'null'
  str-doublequoted-null: "null"
  str-block-null: |
    null
  str-foldblock-null: >
    null
  required-string: 'OK'
  slice-str:
    - 1
    - 2
    - 3
  slice-int: [1, 2, 3]
  map-string-string:
    foo: bar
    bazz: fuzz
  map-int-int:
    2: 4
    4: 8
enabled: true`), 0o664)
	require.NoError(t, err)

	var c TestConfig
	err = yamagiconf.LoadFile(p, &c)
	require.NoError(t, err)

	require.Equal(t, `any string`, c.AnyString)
	require.Equal(t, `null`, c.StrQuotedNull)
	require.Equal(t, `null`, c.StrDubQuotNull)
	require.Equal(t, "null\n", c.StrBlockNull)
	require.Equal(t, "null\n", c.StrFoldBlockNull)
	require.Equal(t, `OK`, c.RequiredString)
	require.Equal(t, int32(42), c.Int32)
	require.Equal(t, map[string]string{"foo": "bar", "bazz": "fuzz"}, c.MapStringString)
	require.Equal(t, map[int16]int16{2: 4, 4: 8}, c.MapIntInt)
	require.Equal(t, []string{"1", "2", "3"}, c.SliceStr)
	require.Equal(t, []int64{1, 2, 3}, c.SliceInt)
}

func TestLoadErrMissingYAMLTag(t *testing.T) {
	t.Run("level_0", func(t *testing.T) {
		type TestConfig struct {
			HasYAMLTag string `yaml:"has-yaml-tag"`
			NoYAMLTag  string
		}
		_, err := LoadSrc[TestConfig]("has-yaml-tag: 'OK'\nNoYAMLTag: 'NO'\n")
		require.ErrorIs(t, err, yamagiconf.ErrMissingYAMLTag)
		require.Equal(t, "TestConfig.NoYAMLTag: missing yaml struct tag", err.Error())
	})
	t.Run("level_1", func(t *testing.T) {
		type Foo struct{ NoYAMLTag string }
		type TestConfig struct {
			HasYAMLTag string `yaml:"has-yaml-tag"`
			Foo        Foo    `yaml:"foo"`
		}
		_, err := LoadSrc[TestConfig]("has-yaml-tag: 'OK'\nfoo:\n  NoYAMLTag: 'NO'\n")
		require.ErrorIs(t, err, yamagiconf.ErrMissingYAMLTag)
		require.Equal(t, "TestConfig.Foo.NoYAMLTag: missing yaml struct tag", err.Error())
	})

	t.Run("slice_item", func(t *testing.T) {
		type Item struct{ NoYAMLTag string }
		type TestConfig struct {
			HasYAMLTag string `yaml:"has-yaml-tag"`
			Slice      []Item `yaml:"slice"`
		}
		_, err := LoadSrc[TestConfig](`
has-yaml-tag: OK
slice:
  - NoYAMLTag: NO
`)
		require.ErrorIs(t, err, yamagiconf.ErrMissingYAMLTag)
		require.Equal(t, "TestConfig.Slice.NoYAMLTag: "+
			"missing yaml struct tag", err.Error())
	})

	t.Run("array_item", func(t *testing.T) {
		type Item struct{ NoYAMLTag string }
		type TestConfig struct {
			HasYAMLTag string  `yaml:"has-yaml-tag"`
			Array      [8]Item `yaml:"array"`
		}
		_, err := LoadSrc[TestConfig](`
has-yaml-tag: OK
slice:
  - NoYAMLTag: NO
`)
		require.ErrorIs(t, err, yamagiconf.ErrMissingYAMLTag)
		require.Equal(t, "TestConfig.Array.NoYAMLTag: "+
			"missing yaml struct tag", err.Error())
	})
}

func TestLoadInvalidEnvTag(t *testing.T) {
	t.Run("lower_case", func(t *testing.T) {
		type TestConfig struct {
			Wrong string `yaml:"wrong" env:"notok"`
		}
		_, err := LoadSrc[TestConfig]("wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t, "TestConfig.Wrong: invalid env struct tag: "+
			"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("wrong_start", func(t *testing.T) {
		type TestConfig struct {
			Wrong string `yaml:"wrong" env:"1NOTOK"`
		}
		_, err := LoadSrc[TestConfig]("wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t, "TestConfig.Wrong: invalid env struct tag: "+
			"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("illegal_char", func(t *testing.T) {
		type TestConfig struct {
			Wrong string `yaml:"wrong" env:"NOT-OK"`
		}
		_, err := LoadSrc[TestConfig]("wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t, "TestConfig.Wrong: invalid env struct tag: "+
			"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("level_1", func(t *testing.T) {
		type Container struct {
			Wrong string `yaml:"wrong" env:"NOT-OK"`
		}
		type TestConfig struct {
			Container Container `yaml:"container"`
		}
		_, err := LoadSrc[TestConfig]("container:\n  wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t,
			"TestConfig.Container.Wrong: invalid env struct tag: "+
				"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("on_struct", func(t *testing.T) {
		type Container struct {
			OK string `yaml:"ok" env:"OK"`
		}
		type TestConfig struct {
			Container Container `yaml:"container" env:"NOTOK"`
		}
		_, err := LoadSrc[TestConfig]("container:\n  ok: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t,
			"TestConfig.Container: invalid env struct tag: "+
				"env var of unsupported type: yamagiconf_test.Container", err.Error())
	})

	t.Run("on_ptr_struct", func(t *testing.T) {
		type Container struct {
			OK string `yaml:"ok" env:"OK"`
		}
		type TestConfig struct {
			Container *Container `yaml:"container" env:"NOTOK"`
		}
		_, err := LoadSrc[TestConfig]("container:\n  ok: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t,
			"TestConfig.Container: invalid env struct tag: "+
				"env var of unsupported type: *yamagiconf_test.Container", err.Error())
	})

	t.Run("on_slice", func(t *testing.T) {
		type TestConfig struct {
			Wrong []string `yaml:"wrong" env:"WRONG"`
		}
		_, err := LoadSrc[TestConfig]("wrong:\n  - ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t,
			"TestConfig.Wrong: invalid env struct tag: "+
				"env var of unsupported type: []string", err.Error())
	})
}

type TestConfigRecurThroughContainerPtr struct {
	Str       string     `yaml:"str"`
	Container *Container `yaml:"container"`
}

type Container struct {
	Recurs TestConfigRecurThroughContainerPtr `yaml:"recurs"`
}

type TestConfigRecurPtrThroughContainer struct {
	Str       string           `yaml:"str"`
	Container ContainerWithPtr `yaml:"container"`
}

type ContainerWithPtr struct {
	Recurs *TestConfigRecurPtrThroughContainer `yaml:"recurs"`
}

type TestConfigRecurThroughSlice struct {
	Str    string                        `yaml:"str"`
	Recurs []TestConfigRecurThroughSlice `yaml:"recurs"`
}

type TestConfigRecurPtrThroughSlice struct {
	Str    string                            `yaml:"str"`
	Recurs []*TestConfigRecurPtrThroughSlice `yaml:"recurs"`
}

func TestLoadErrRecursiveType(t *testing.T) {
	// These contents don't really matter because
	// the type is going to be checked before the unmarshaling.
	const yamlContents = `
str: OK
container:
  recurs:
    str: 'not ok'
    container: null
recurs:
  - str: a
  - str: b
`

	t.Run("through_container_ptr", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurThroughContainerPtr](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrRecursiveType)
		require.Equal(t, "TestConfigRecurThroughContainerPtr.Container.Recurs: "+
			"recursive type", err.Error())
	})
	t.Run("ptr_through_container", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurPtrThroughContainer](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrRecursiveType)
		require.Equal(t, "TestConfigRecurPtrThroughContainer.Container.Recurs: "+
			"recursive type", err.Error())
	})

	t.Run("through_slice", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurThroughSlice](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrRecursiveType)
		require.Equal(t, "TestConfigRecurThroughSlice.Recurs: "+
			"recursive type", err.Error())
	})

	t.Run("ptr_through_slice", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurPtrThroughSlice](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrRecursiveType)
		require.Equal(t, "TestConfigRecurPtrThroughSlice.Recurs: "+
			"recursive type", err.Error())
	})
}

func TestLoadErrUnsupportedType(t *testing.T) {
	// These contents don't really matter because
	// the type is going to be checked before the unmarshaling.
	const yamlContents = `
str: OK
container:
  recurs:
    str: 'not ok'
    container: null
recurs:
  - str: a
  - str: b
`

	t.Run("int", func(t *testing.T) {
		type TestConfig struct {
			Int int `yaml:"int"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "TestConfig.Int: unsupported type: int, "+
			"use integer type with specified width, "+
			"such as int32 or int64 instead of int", err.Error())
	})

	t.Run("ptr_ptr", func(t *testing.T) {
		type TestConfig struct {
			PtrPtr **int `yaml:"ptr-ptr"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t, "TestConfig.PtrPtr: unsupported pointer type", err.Error())
	})

	t.Run("ptr_slice", func(t *testing.T) {
		type TestConfig struct {
			PtrSlice *[]string `yaml:"ptr-slice"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t, "TestConfig.PtrSlice: unsupported pointer type", err.Error())
	})

	t.Run("ptr_map", func(t *testing.T) {
		type TestConfig struct {
			PtrMap **map[string]string `yaml:"ptr-map"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t, "TestConfig.PtrMap: unsupported pointer type", err.Error())
	})

	t.Run("channel", func(t *testing.T) {
		type TestConfig struct {
			Chan chan int `yaml:"chan"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "TestConfig.Chan: unsupported type: chan int", err.Error())
	})

	t.Run("func", func(t *testing.T) {
		type TestConfig struct {
			Func func() `yaml:"func"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "TestConfig.Func: unsupported type: func()", err.Error())
	})

	t.Run("unsafe_pointer", func(t *testing.T) {
		type TestConfig struct {
			UnsafePointer unsafe.Pointer `yaml:"unsafe-pointer"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "TestConfig.UnsafePointer: "+
			"unsupported type: unsafe.Pointer", err.Error())
	})

	t.Run("interface", func(t *testing.T) {
		type TestConfig struct {
			Interface interface{ Write() ([]byte, int) } `yaml:"interface"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "TestConfig.Interface: "+
			"unsupported type: interface { Write() ([]uint8, int) }", err.Error())
	})

	t.Run("empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "TestConfig.Anything: "+
			"unsupported type: interface {}", err.Error())
	})
}

func TestLoadErrMissingConfig(t *testing.T) {
	type TestConfig struct {
		OK      string `yaml:"ok,omitempty"`
		Missing string `yaml:"missing,omitempty"`
	}
	_, err := LoadSrc[TestConfig]("ok: 'OK'")
	require.ErrorIs(t, err, yamagiconf.ErrMissingConfig)
	require.Equal(t,
		`config "missing" (TestConfig.Missing): missing field in config file`,
		err.Error())
}

func TestLoadNullOnNonPointer(t *testing.T) {
	t.Run("on_string", func(t *testing.T) {
		type TestConfig struct {
			Ok  string `yaml:"ok"`
			Str string `yaml:"str"`
		}
		_, err := LoadSrc[TestConfig]("ok: OK\nstr: null")
		require.ErrorIs(t, err, yamagiconf.ErrNullOnNonPointer)
		require.Equal(t,
			`at 2:6: "str" (TestConfig.Str): cannot assign null to non-pointer type`,
			err.Error())
	})
	t.Run("on_uint32", func(t *testing.T) {
		type TestConfig struct {
			Ok     string `yaml:"ok"`
			Uint32 uint32 `yaml:"uint32"`
		}
		_, err := LoadSrc[TestConfig]("ok: OK\nuint32: null")
		require.ErrorIs(t, err, yamagiconf.ErrNullOnNonPointer)
		require.Equal(t,
			`at 2:9: "uint32" (TestConfig.Uint32): cannot assign null to non-pointer type`,
			err.Error())
	})
}

func TestLoadErrNilConfig(t *testing.T) {
	type TestConfig struct {
		Foo int8 `yaml:"foo"`
	}
	err := yamagiconf.Load[TestConfig]("non-existing.yaml", nil)
	require.ErrorIs(t, err, yamagiconf.ErrNilConfig)
}

func TestLoadFileErrNotExist(t *testing.T) {
	type TestConfig struct {
		Foo int8 `yaml:"foo"`
	}
	var c TestConfig
	err := yamagiconf.LoadFile("non-existing.yaml", &c)
	require.Error(t, err)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Zero(t, c)
}

func TestLoadErr(t *testing.T) {
	type TestConfig struct {
		Foo int8 `yaml:"foo"`
	}

	t.Run("file_empty", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "test-config.yaml")
		_, err := os.Create(p)
		require.NoError(t, err)
		var c TestConfig
		err = yamagiconf.LoadFile(p, &c)
		require.ErrorIs(t, err, yamagiconf.ErrEmptyFile)
		require.Zero(t, c)
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		// Using tabs is illegal
		c, err := LoadSrc[TestConfig]("x:\n\ttabs: 'TABS'\n  spaces: 'SPACES'\n")
		require.ErrorIs(t, err, yamagiconf.ErrMalformedYAML)
		require.True(t, strings.HasPrefix(err.Error(), "malformed YAML: yaml: line 2:"))
		require.Zero(t, c)
	})

	t.Run("unsupported_boolean_literal", func(t *testing.T) {
		type TestConfig struct {
			Boolean bool `yaml:"boolean"`
		}
		_, err := LoadSrc[TestConfig]("\nboolean: yes")
		require.ErrorIs(t, err, yamagiconf.ErrBadBoolLiteral)
		require.True(t, strings.HasPrefix(err.Error(),
			`at 2:10: "boolean" (TestConfig.Boolean): must be either false or true`))
	})

	t.Run("unsupported_null_literal_tilde", func(t *testing.T) {
		type TestConfig struct {
			Nullable *bool `yaml:"nullable"`
		}
		_, err := LoadSrc[TestConfig]("\nnullable: ~")
		require.ErrorIs(t, err, yamagiconf.ErrBadNullLiteral)
		require.Equal(t, `at 2:11: "nullable" (TestConfig.Nullable): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_tilde_in_map", func(t *testing.T) {
		type TestConfig struct {
			Map map[*string]string `yaml:"map"`
		}
		_, err := LoadSrc[TestConfig]("map:\n  ~: string")
		require.ErrorIs(t, err, yamagiconf.ErrBadNullLiteral)
		require.Equal(t, `at 2:3: "map" (TestConfig.Map["~"]): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_tilde_in_map_value", func(t *testing.T) {
		type TestConfig struct {
			Map map[string]*string `yaml:"map"`
		}
		_, err := LoadSrc[TestConfig]("map:\n  notok: ~")
		require.ErrorIs(t, err, yamagiconf.ErrBadNullLiteral)
		require.Equal(t, `at 2:10: "map" (TestConfig.Map["notok"]): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_tilde_in_slice", func(t *testing.T) {
		type TestConfig struct {
			Slice []*string `yaml:"slice"`
		}
		_, err := LoadSrc[TestConfig]("slice:\n  - ~")
		require.ErrorIs(t, err, yamagiconf.ErrBadNullLiteral)
		require.Equal(t, `at 2:5: "slice" (TestConfig.Slice[0]): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_title", func(t *testing.T) {
		type TestConfig struct {
			Nullable *bool `yaml:"nullable"`
		}
		_, err := LoadSrc[TestConfig]("\nnullable: Null")
		require.ErrorIs(t, err, yamagiconf.ErrBadNullLiteral)
		require.Equal(t, `at 2:11: "nullable" (TestConfig.Nullable): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_uppercase", func(t *testing.T) {
		type TestConfig struct {
			Nullable *bool `yaml:"nullable"`
		}
		_, err := LoadSrc[TestConfig]("\nnullable: NULL")
		require.ErrorIs(t, err, yamagiconf.ErrBadNullLiteral)
		require.Equal(t, `at 2:11: "nullable" (TestConfig.Nullable): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})
}

func TestValidation(t *testing.T) {
	type TestConfig struct {
		Validation struct {
			// Tag `validate:"required"` requires it to be non-zero.
			RequiredStr string `yaml:"required-str" validate:"required"`
		} `yaml:"validation"`
	}

	t.Run("required_ok", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("validation:\n  required-str: 'ok'")
		require.NoError(t, err)
	})

	t.Run("required_error", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("validation:\n  required-str: ''")
		require.ErrorIs(t, err, yamagiconf.ErrValidateTagViolation)
		require.Equal(t,
			`at 2:17: "required-str" violates validation rule: "required"`,
			err.Error())
	})
}

type TestConfWithValid struct {
	Foo       string          `yaml:"foo" validate:"required"`
	Bar       string          `yaml:"bar"`
	Container ContainerStruct `yaml:"container"`
}

var _ yamagiconf.Validator = new(TestConfWithValid)

func (v *TestConfWithValid) Validate() error {
	if v.Foo == "" {
		return errors.New("foo must not be empty")
	}
	if v.Bar == "" {
		return errors.New("bar must not be empty")
	}
	return nil
}

type ValidatedMap map[ValidatedString]ValidatedString

type ContainerStruct struct {
	ValidatedString       ValidatedString     `yaml:"validated-string"`
	PtrValidatedString    *ValidatedString    `yaml:"ptr-validated-string"`
	ValidatedStringPtr    ValidatedStringPtr  `yaml:"validated-string-ptr"`
	PtrValidatedStringPtr *ValidatedStringPtr `yaml:"ptr-validated-string-ptr"`
	Slice                 []ValidatedString   `yaml:"slice"`
	Map                   ValidatedMap        `yaml:"map"`
}

type ValidatedString string

func (v ValidatedString) Validate() error {
	if v != "valid" {
		return fmt.Errorf("is not 'valid'")
	}
	return nil
}

type ValidatedStringPtr string

func (v *ValidatedStringPtr) Validate() error {
	if *v != "valid" {
		return fmt.Errorf("is not 'valid'")
	}
	return nil
}

func PtrTo[T any](t T) *T { return &t }

func TestValidator(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		c, err := LoadSrc[TestConfWithValid](`
foo: a
bar: b
container:
  validated-string: valid
  ptr-validated-string: valid
  validated-string-ptr: valid
  ptr-validated-string-ptr: valid
  slice:
    - valid
    - valid
  map:
    valid: valid
`)
		require.NoError(t, err)
		require.Equal(t, TestConfWithValid{
			Foo: "a",
			Bar: "b",
			Container: ContainerStruct{
				ValidatedString:       "valid",
				PtrValidatedString:    PtrTo(ValidatedString("valid")),
				ValidatedStringPtr:    "valid",
				PtrValidatedStringPtr: PtrTo(ValidatedStringPtr("valid")),
				Slice:                 []ValidatedString{"valid", "valid"},
				Map:                   ValidatedMap{"valid": "valid"},
			},
		}, *c)
	})

	t.Run("ok_null", func(t *testing.T) {
		c, err := LoadSrc[TestConfWithValid](`
foo: a
bar: b
container:
  validated-string: valid
  ptr-validated-string: null
  validated-string-ptr: valid
  ptr-validated-string-ptr: null
  slice: null
  map: null
`)
		require.NoError(t, err)
		require.Equal(t, TestConfWithValid{
			Foo: "a",
			Bar: "b",
			Container: ContainerStruct{
				ValidatedString:    "valid",
				ValidatedStringPtr: "valid",
			},
		}, *c)
	})

	t.Run("error_at_level_0", func(t *testing.T) {
		_, err := LoadSrc[TestConfWithValid](`
foo: ''
bar: b
container:
  validated-string: valid
  ptr-validated-string: valid
  validated-string-ptr: valid
  ptr-validated-string-ptr: valid
  slice:
    - valid
  map:
    valid: valid
`)
		require.ErrorIs(t, err, yamagiconf.ErrValidation)
		errMsg := err.Error()
		require.Equal(t, `at 2:1: validation: foo must not be empty`, errMsg)
	})

	t.Run("error_at_level_1", func(t *testing.T) {
		_, err := LoadSrc[TestConfWithValid](`
foo: a
bar: b
container:
  validated-string: invalid
  ptr-validated-string: invalid
  validated-string-ptr: invalid
  ptr-validated-string-ptr: invalid
  slice:
    - valid
  map:
    valid: valid
`)

		require.Error(t, err)
		errMsg := err.Error()
		require.Equal(t, `at 5:21: validation: is not 'valid'`, errMsg)
	})

	t.Run("error_in_slice", func(t *testing.T) {
		_, err := LoadSrc[TestConfWithValid](`
foo: a
bar: b
container:
  validated-string: valid
  ptr-validated-string: valid
  validated-string-ptr: valid
  ptr-validated-string-ptr: valid
  slice:
    - valid
    - invalid
  map:
    valid: valid
`)

		require.Error(t, err)
		errMsg := err.Error()
		require.Equal(t, `at 11:7: validation: is not 'valid'`, errMsg)
	})

	t.Run("error_in_map_key", func(t *testing.T) {
		_, err := LoadSrc[TestConfWithValid](`
foo: a
bar: b
container:
  validated-string: valid
  ptr-validated-string: valid
  validated-string-ptr: valid
  ptr-validated-string-ptr: valid
  slice:
    - valid
    - valid
  map:
    valid: valid
    invalid: valid
`)

		require.Error(t, err)
		errMsg := err.Error()
		require.Equal(t, `at 14:5: validation: is not 'valid'`, errMsg)
	})

	t.Run("error_in_map_value", func(t *testing.T) {
		_, err := LoadSrc[TestConfWithValid](`
foo: a
bar: b
container:
  validated-string: valid
  ptr-validated-string: valid
  validated-string-ptr: valid
  ptr-validated-string-ptr: valid
  slice:
    - valid
    - valid
  map:
    valid: invalid
`)
		require.Error(t, err)
		errMsg := err.Error()
		require.Equal(t, `at 13:12: validation: is not 'valid'`, errMsg)
	})
}

func TestLoadEnvVar(t *testing.T) {
	type TestConfig struct {
		BoolFalse     bool    `yaml:"bool_false" env:"BOOL_FALSE"`
		BoolTrue      bool    `yaml:"bool_true" env:"BOOL_TRUE"`
		String        string  `yaml:"string" env:"STRING"`
		Float32       float32 `yaml:"float32" env:"FLOAT_32"`
		Float64       float64 `yaml:"float64" env:"FLOAT_64"`
		Int8          int8    `yaml:"int8" env:"INT_8"`
		Uint8         uint8   `yaml:"uint8" env:"UINT_8"`
		Int16         int16   `yaml:"int16" env:"INT_16"`
		Uint16        uint16  `yaml:"uint16" env:"UINT_16"`
		Int32         int32   `yaml:"int32" env:"INT_32"`
		Uint32        uint32  `yaml:"uint32" env:"UINT_32"`
		Int64         int64   `yaml:"int64" env:"INT_64"`
		Uint64        uint64  `yaml:"uint64" env:"UINT_64"`
		PtrUint64     *uint64 `yaml:"ptr-uint64" env:"PTR_UINT_64"`
		PtrUint64Null *uint64 `yaml:"ptr-uint64-null" env:"PTR_UINT_64_NULL"`
	}
	t.Setenv("BOOL_FALSE", "false")
	t.Setenv("BOOL_TRUE", "true")
	t.Setenv("STRING", "test text")
	t.Setenv("FLOAT_32", "3.14")
	t.Setenv("FLOAT_64", "3.1415")
	t.Setenv("INT_8", "-1")
	t.Setenv("UINT_8", "1")
	t.Setenv("INT_16", "-1")
	t.Setenv("UINT_16", "1")
	t.Setenv("INT_32", "-1")
	t.Setenv("UINT_32", "1")
	t.Setenv("INT_64", "-1")
	t.Setenv("UINT_64", "1")
	t.Setenv("PTR_UINT_64", "1")
	t.Setenv("PTR_UINT_64_NULL", "null")
	c, err := LoadSrc[TestConfig](`
bool_false: true
bool_true: false
string: ''
float32: 0
float64: 0
int8: 0
uint8: 0
int16: 0
uint16: 0
int32: 0
uint32: 0
int64: 0
uint64: 0
ptr-uint64: 0
ptr-uint64-null: 0
`)
	require.NoError(t, err)
	require.Equal(t, false, c.BoolFalse)
	require.Equal(t, true, c.BoolTrue)
	require.Equal(t, "test text", c.String)
	require.Equal(t, float32(3.14), c.Float32)
	require.Equal(t, float64(3.1415), c.Float64)
	require.Equal(t, int8(-1), c.Int8)
	require.Equal(t, uint8(1), c.Uint8)
	require.Equal(t, int16(-1), c.Int16)
	require.Equal(t, uint16(1), c.Uint16)
	require.Equal(t, int32(-1), c.Int32)
	require.Equal(t, uint32(1), c.Uint32)
	require.Equal(t, int64(-1), c.Int64)
	require.Equal(t, uint64(1), c.Uint64)
	require.Equal(t, uint64(1), *c.PtrUint64)
	require.Nil(t, c.PtrUint64Null)
}

func TestLoadEnvVarNoOverwrite(t *testing.T) {
	type TestConfig struct {
		BoolFalse     bool    `yaml:"bool_false" env:"BOOL_FALSE"`
		BoolTrue      bool    `yaml:"bool_true" env:"BOOL_TRUE"`
		String        string  `yaml:"string" env:"STRING"`
		Float32       float32 `yaml:"float32" env:"FLOAT_32"`
		Float64       float64 `yaml:"float64" env:"FLOAT_64"`
		Int8          int8    `yaml:"int8" env:"INT_8"`
		Uint8         uint8   `yaml:"uint8" env:"UINT_8"`
		Int16         int16   `yaml:"int16" env:"INT_16"`
		Uint16        uint16  `yaml:"uint16" env:"UINT_16"`
		Int32         int32   `yaml:"int32" env:"INT_32"`
		Uint32        uint32  `yaml:"uint32" env:"UINT_32"`
		Int64         int64   `yaml:"int64" env:"INT_64"`
		Uint64        uint64  `yaml:"uint64" env:"UINT_64"`
		PtrUint64     *uint64 `yaml:"ptr-uint64" env:"PTR_UINT_64"`
		PtrUint64Null *uint64 `yaml:"ptr-uint64-null" env:"PTR_UINT_64_NULL"`
	}
	c, err := LoadSrc[TestConfig](`
bool_false: true
bool_true: false
string: ''
float32: 0
float64: 0
int8: 0
uint8: 0
int16: 0
uint16: 0
int32: 0
uint32: 0
int64: 0
uint64: 0
ptr-uint64: 0
ptr-uint64-null: null
`)
	require.NoError(t, err)
	require.Equal(t, true, c.BoolFalse)
	require.Equal(t, false, c.BoolTrue)
	require.Equal(t, "", c.String)
	require.Equal(t, float32(0), c.Float32)
	require.Equal(t, float64(0), c.Float64)
	require.Equal(t, int8(0), c.Int8)
	require.Equal(t, uint8(0), c.Uint8)
	require.Equal(t, int16(0), c.Int16)
	require.Equal(t, uint16(0), c.Uint16)
	require.Equal(t, int32(0), c.Int32)
	require.Equal(t, uint32(0), c.Uint32)
	require.Equal(t, int64(0), c.Int64)
	require.Equal(t, uint64(0), c.Uint64)
	require.Equal(t, uint64(0), *c.PtrUint64)
	require.Nil(t, c.PtrUint64Null)
}

func TestLoadErrInvalidEnvVar(t *testing.T) {
	t.Run("bool", func(t *testing.T) {
		type TestConfig struct {
			Bool bool `yaml:"bool" env:"BOOL"`
		}
		t.Setenv("BOOL", "yes")
		_, err := LoadSrc[TestConfig](`bool: false`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t,
			"at TestConfig.Bool: invalid env var BOOL: expected bool",
			err.Error())
	})

	t.Run("float32", func(t *testing.T) {
		type TestConfig struct {
			Float32 float32 `yaml:"float32" env:"FLOAT_32"`
		}
		t.Setenv("FLOAT_32", "not_a_float32")
		_, err := LoadSrc[TestConfig](`float32: 3.14`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Float32: invalid env var FLOAT_32: "+
			"expected float32: "+
			"strconv.ParseFloat: parsing \"not_a_float32\": invalid syntax", err.Error())
	})

	t.Run("float64", func(t *testing.T) {
		type TestConfig struct {
			Float64 float64 `yaml:"float64" env:"FLOAT_64"`
		}
		t.Setenv("FLOAT_64", "not_a_float64")
		_, err := LoadSrc[TestConfig](`float64: 3.14`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Float64: invalid env var FLOAT_64: "+
			"expected float64: "+
			"strconv.ParseFloat: parsing \"not_a_float64\": invalid syntax", err.Error())
	})

	t.Run("int8", func(t *testing.T) {
		type TestConfig struct {
			Int8 int8 `yaml:"int8" env:"INT_8"`
		}
		t.Setenv("INT_8", "257")
		_, err := LoadSrc[TestConfig](`int8: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Int8: invalid env var INT_8: "+
			"expected int8: "+
			"strconv.ParseInt: parsing \"257\": value out of range", err.Error())
	})

	t.Run("uint8", func(t *testing.T) {
		type TestConfig struct {
			Uint8 uint8 `yaml:"uint8" env:"UINT_8"`
		}
		t.Setenv("UINT_8", "-1")
		_, err := LoadSrc[TestConfig](`uint8: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Uint8: invalid env var UINT_8: "+
			"expected uint8: "+
			"strconv.ParseUint: parsing \"-1\": invalid syntax", err.Error())
	})

	t.Run("int16", func(t *testing.T) {
		type TestConfig struct {
			Int16 int16 `yaml:"int16" env:"INT_16"`
		}
		t.Setenv("INT_16", "65536")
		_, err := LoadSrc[TestConfig](`int16: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Int16: invalid env var INT_16: "+
			"expected int16: "+
			"strconv.ParseInt: parsing \"65536\": value out of range", err.Error())
	})

	t.Run("uint16", func(t *testing.T) {
		type TestConfig struct {
			Uint16 uint16 `yaml:"uint16" env:"UINT_16"`
		}
		t.Setenv("UINT_16", "-1")
		_, err := LoadSrc[TestConfig](`uint16: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Uint16: invalid env var UINT_16: "+
			"expected uint16: "+
			"strconv.ParseUint: parsing \"-1\": invalid syntax", err.Error())
	})

	t.Run("int32", func(t *testing.T) {
		type TestConfig struct {
			Int32 int32 `yaml:"int32" env:"INT_32"`
		}
		t.Setenv("INT_32", "4294967296")
		_, err := LoadSrc[TestConfig](`int32: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Int32: invalid env var INT_32: "+
			"expected int32: "+
			"strconv.ParseInt: parsing \"4294967296\": value out of range", err.Error())
	})

	t.Run("uint32", func(t *testing.T) {
		type TestConfig struct {
			Uint32 uint32 `yaml:"uint32" env:"UINT_32"`
		}
		t.Setenv("UINT_32", "-1")
		_, err := LoadSrc[TestConfig](`uint32: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Uint32: invalid env var UINT_32: "+
			"expected uint32: "+
			"strconv.ParseUint: parsing \"-1\": invalid syntax", err.Error())
	})

	t.Run("int64", func(t *testing.T) {
		type TestConfig struct {
			Int64 int64 `yaml:"int64" env:"INT_64"`
		}

		t.Setenv("INT_64", "9223372036854775808")
		_, err := LoadSrc[TestConfig](`int64: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Int64: invalid env var INT_64: "+
			"expected int64: "+
			"strconv.ParseInt: parsing \"9223372036854775808\": value out of range",
			err.Error())
	})

	t.Run("uint64", func(t *testing.T) {
		type TestConfig struct {
			Uint64 uint64 `yaml:"uint64" env:"UINT_64"`
		}
		t.Setenv("UINT_64", "-1")
		_, err := LoadSrc[TestConfig](`uint64: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.Uint64: invalid env var UINT_64: "+
			"expected uint64: "+
			"strconv.ParseUint: parsing \"-1\": invalid syntax", err.Error())
	})

	t.Run("ptr_uint64", func(t *testing.T) {
		type TestConfig struct {
			PtrUint64 *uint64 `yaml:"uint64" env:"PTR_UINT_64"`
		}
		t.Setenv("PTR_UINT_64", "-1")
		_, err := LoadSrc[TestConfig](`uint64: 0`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, "at TestConfig.PtrUint64: invalid env var PTR_UINT_64: "+
			"expected uint64: "+
			"strconv.ParseUint: parsing \"-1\": invalid syntax", err.Error())
	})
}
