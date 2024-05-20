package yamagiconf_test

import (
	"encoding"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/romshark/yamagiconf"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func LoadSrc[T any](src string) (*T, error) {
	var c T
	if err := yamagiconf.Load(src, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func TestLoadFile(t *testing.T) {
	type Container struct {
		AnyString string `yaml:"any-string"`
	}
	type Embedded struct {
		AnyString         string                `yaml:"any-string"`
		NotATag           string                `yaml:"not-a-tag"`
		StrQuotedNull     string                `yaml:"str-quoted-null"`
		StrDubQuotNull    string                `yaml:"str-doublequoted-null"`
		StrBlockNull      string                `yaml:"str-block-null"`
		StrFoldBlockNull  string                `yaml:"str-foldblock-null"`
		RequiredString    string                `yaml:"required-string" validate:"required"`
		MapStringString   map[string]string     `yaml:"map-string-string"`
		MapInt16Int16     map[int16]int16       `yaml:"map-int16-int16"`
		MapInt16Int16Null map[int16]int16       `yaml:"map-int16-int16-null"`
		MapContainer      map[string]Container  `yaml:"map-string-container"`
		MapContainerNull  map[string]*Container `yaml:"map-string-ptr-container"`
		SliceStr          []string              `yaml:"slice-str"`
		SliceInt64        []int64               `yaml:"slice-int64"`
		SliceInt64Null    []int64               `yaml:"slice-int64-null"`
		Time              time.Time             `yaml:"time"`

		UnmarshalerYAML        YAMLUnmarshaler  `yaml:"unmarshaler-yaml"`
		UnmarshalerText        TextUnmarshaler  `yaml:"unmarshaler-text"`
		PtrUnmarshalerYAML     *YAMLUnmarshaler `yaml:"ptr-unmarshaler-yaml"`
		PtrUnmarshalerText     *TextUnmarshaler `yaml:"ptr-unmarshaler-text"`
		PtrUnmarshalerYAMLNull *YAMLUnmarshaler `yaml:"ptr-unmarshaler-yaml-null"`
		PtrUnmarshalerTextNull *TextUnmarshaler `yaml:"ptr-unmarshaler-text-null"`

		// ignored must be ignored by yamagiconf even though it's
		// of type int which is unsupported.
		//lint:ignore U1000 no need to use it.
		ignored int
	}
	type TestConfig struct {
		Embedded  `yaml:",inline"`
		Container Container `yaml:"container"`
		Int32     int32     `yaml:"int32"`
		Enabled   bool      `yaml:"enabled"`
	}

	p := filepath.Join(t.TempDir(), "test-config.yaml")
	err := os.WriteFile(p, []byte(`# test YAML file
any-string: 'any string'
not-a-tag: '!tag value'
str-quoted-null: 'null'
str-doublequoted-null: "null"
str-block-null: |
  null
str-foldblock-null: >
  null
required-string: 'OK'
map-string-container:
  foo:
    any-string: foo
  bar:
    any-string: bar
map-string-ptr-container:
  foo:
    any-string: foo
  bar: null
slice-str:
  - 1
  - 2
  - 3
slice-int64: [1, 2, 3]
slice-int64-null: null
map-string-string:
  foo: &test-anchor val
  bazz: *test-anchor
map-int16-int16:
  2: 4
  4: 8
map-int16-int16-null: null
time: 2024-05-09T20:19:22Z
unmarshaler-yaml: YAML unmarshaler non-pointer non-null
unmarshaler-text: Text unmarshaler non-pointer non-null
ptr-unmarshaler-yaml: YAML unmarshaler pointer
ptr-unmarshaler-text: Text unmarshaler pointer
ptr-unmarshaler-yaml-null: null
ptr-unmarshaler-text-null: null
container:
  any-string: 'any string'
int32: 42
enabled: true`), 0o664)
	require.NoError(t, err)

	var c TestConfig
	err = yamagiconf.LoadFile(p, &c)
	require.NoError(t, err)

	require.Equal(t, `any string`, c.AnyString)
	require.Equal(t, `!tag value`, c.NotATag)
	require.Equal(t, `null`, c.StrQuotedNull)
	require.Equal(t, `null`, c.StrDubQuotNull)
	require.Equal(t, "null\n", c.StrBlockNull)
	require.Equal(t, "null\n", c.StrFoldBlockNull)
	require.Equal(t, `OK`, c.RequiredString)
	require.Equal(t, int32(42), c.Int32)
	require.Equal(t, map[string]string{"foo": "val", "bazz": "val"}, c.MapStringString)
	require.Equal(t, map[int16]int16{2: 4, 4: 8}, c.MapInt16Int16)
	require.Nil(t, c.MapInt16Int16Null)
	require.Equal(t, map[string]Container{
		"foo": {AnyString: "foo"},
		"bar": {AnyString: "bar"},
	}, c.MapContainer)
	require.Equal(t, map[string]*Container{
		"foo": {AnyString: "foo"},
		"bar": nil,
	}, c.MapContainerNull)
	require.Equal(t, []string{"1", "2", "3"}, c.SliceStr)
	require.Equal(t, []int64{1, 2, 3}, c.SliceInt64)
	require.Nil(t, c.SliceInt64Null)
	require.Equal(t, time.Date(2024, 5, 9, 20, 19, 22, 0, time.UTC), c.Time)
	require.Equal(t, `YAML unmarshaler non-pointer non-null`, c.UnmarshalerYAML.Str)
	require.Equal(t, `Text unmarshaler non-pointer non-null`, c.UnmarshalerText.Str)
	require.Equal(t, `YAML unmarshaler pointer`, c.PtrUnmarshalerYAML.Str)
	require.Equal(t, `Text unmarshaler pointer`, c.PtrUnmarshalerText.Str)
	require.Nil(t, c.PtrUnmarshalerYAMLNull)
	require.Nil(t, c.PtrUnmarshalerTextNull)
}

func TestLoadErrInvalidUTF8(t *testing.T) {
	type TestConfig struct {
		Str string `yaml:"str"`
	}

	t.Run("control_char_in_string", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("str: abc\u0000defg")
		require.ErrorIs(t, err, yamagiconf.ErrMalformedYAML)
		require.Equal(t,
			`malformed YAML: yaml: control characters are not allowed`,
			err.Error())
	})

	t.Run("control_char_in_key", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("s\u0000tr: abcdefg")
		require.ErrorIs(t, err, yamagiconf.ErrMalformedYAML)
		require.Equal(t,
			`malformed YAML: yaml: control characters are not allowed`,
			err.Error())
	})

	t.Run("overlong_encoding_of_U+0000", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("\xc0\x80: not ok")
		require.ErrorIs(t, err, yamagiconf.ErrMalformedYAML)
		require.Equal(t,
			`malformed YAML: yaml: invalid length of a UTF-8 sequence`,
			err.Error())
	})

	t.Run("surrogate_half_U+D800_U+DFFF", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("\xed\xa0\x80: not ok")
		require.ErrorIs(t, err, yamagiconf.ErrMalformedYAML)
		require.Equal(t,
			`malformed YAML: yaml: invalid Unicode character`,
			err.Error())
	})

	t.Run("code_point_beyond_max_unicode_value", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("\xef\xbf\xff: not ok")
		require.ErrorIs(t, err, yamagiconf.ErrMalformedYAML)
		require.Equal(t,
			`malformed YAML: yaml: invalid trailing UTF-8 octet`,
			err.Error())
	})
}

func TestLoadErrYAMLAnchorRedefined(t *testing.T) {
	type Container struct {
		One string `yaml:"one"`
		Two string `yaml:"two"`
	}
	type TestConfig struct {
		One       string    `yaml:"one"`
		Two       string    `yaml:"two"`
		Container Container `yaml:"container"`
	}
	t.Run("level_0", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`
one: &x v1
two: &x
container:
  one: *x
  two: *x
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLAnchorRedefined)
		require.Equal(t, `at 3:6: redefined anchor "x" at 2:6: `+
			`yaml anchors must be unique throughout the whole document`, err.Error())
	})

	t.Run("level_1", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`
one: &x v1
two: *x
container:
  one: &x
  two: *x
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLAnchorRedefined)
		require.Equal(t, `at 5:8: redefined anchor "x" at 2:6: `+
			`yaml anchors must be unique throughout the whole document`, err.Error())
	})
}

func TestLoadErrMissingYAMLTag(t *testing.T) {
	t.Run("level_0", func(t *testing.T) {
		type TestConfig struct {
			HasYAMLTag string `yaml:"has-yaml-tag"`
			NoYAMLTag  string
		}
		_, err := LoadSrc[TestConfig]("has-yaml-tag: 'OK'\nNoYAMLTag: 'NO'\n")
		require.ErrorIs(t, err, yamagiconf.ErrMissingYAMLTag)
		require.Equal(t, "at TestConfig.NoYAMLTag: missing yaml struct tag", err.Error())
	})
	t.Run("level_1", func(t *testing.T) {
		type Foo struct{ NoYAMLTag string }
		type TestConfig struct {
			HasYAMLTag string `yaml:"has-yaml-tag"`
			Foo        Foo    `yaml:"foo"`
		}
		_, err := LoadSrc[TestConfig]("has-yaml-tag: 'OK'\nfoo:\n  NoYAMLTag: 'NO'\n")
		require.ErrorIs(t, err, yamagiconf.ErrMissingYAMLTag)
		require.Equal(t,
			"at TestConfig.Foo.NoYAMLTag: missing yaml struct tag",
			err.Error())
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
		require.Equal(t, "at TestConfig.Slice.NoYAMLTag: "+
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
		require.Equal(t, "at TestConfig.Array.NoYAMLTag: "+
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
		require.Equal(t, "at TestConfig.Wrong: invalid env struct tag: "+
			"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("wrong_start", func(t *testing.T) {
		type TestConfig struct {
			Wrong string `yaml:"wrong" env:"1NOTOK"`
		}
		_, err := LoadSrc[TestConfig]("wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t, "at TestConfig.Wrong: invalid env struct tag: "+
			"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("illegal_char", func(t *testing.T) {
		type TestConfig struct {
			Wrong string `yaml:"wrong" env:"NOT-OK"`
		}
		_, err := LoadSrc[TestConfig]("wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvTag)
		require.Equal(t, "at TestConfig.Wrong: invalid env struct tag: "+
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
			"at TestConfig.Container.Wrong: invalid env struct tag: "+
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Container: "+
				"env var on unsupported type: yamagiconf_test.Container", err.Error())
	})

	t.Run("on_ptr_struct", func(t *testing.T) {
		type Container struct {
			OK string `yaml:"ok" env:"OK"`
		}
		type TestConfig struct {
			Container *Container `yaml:"container" env:"NOTOK"`
		}
		_, err := LoadSrc[TestConfig]("container:\n  ok: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Container: "+
				"env var on unsupported type: *yamagiconf_test.Container", err.Error())
	})

	t.Run("on_slice", func(t *testing.T) {
		type TestConfig struct {
			Wrong []string `yaml:"wrong" env:"WRONG"`
		}
		_, err := LoadSrc[TestConfig]("wrong:\n  - ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Wrong: "+
				"env var on unsupported type: []string", err.Error())
	})

	t.Run("on_yaml_unmarshaler", func(t *testing.T) {
		type TestConfig struct {
			Wrong YAMLUnmarshaler `yaml:"wrong" env:"WRONG"`
		}
		_, err := LoadSrc[TestConfig]("wrong:\n  - ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Wrong: env var on unsupported type: "+
				"yamagiconf_test.YAMLUnmarshaler", err.Error())
	})

	t.Run("on_ptr_yaml_unmarshaler", func(t *testing.T) {
		type TestConfig struct {
			Wrong *YAMLUnmarshaler `yaml:"wrong" env:"WRONG"`
		}
		_, err := LoadSrc[TestConfig]("wrong:\n  - ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Wrong: env var on unsupported type: "+
				"*yamagiconf_test.YAMLUnmarshaler", err.Error())
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
		require.Equal(t, "at TestConfigRecurThroughContainerPtr.Container.Recurs: "+
			"recursive type", err.Error())
	})
	t.Run("ptr_through_container", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurPtrThroughContainer](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrRecursiveType)
		require.Equal(t, "at TestConfigRecurPtrThroughContainer.Container.Recurs: "+
			"recursive type", err.Error())
	})

	t.Run("through_slice", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurThroughSlice](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrRecursiveType)
		require.Equal(t, "at TestConfigRecurThroughSlice.Recurs: "+
			"recursive type", err.Error())
	})

	t.Run("ptr_through_slice", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurPtrThroughSlice](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrRecursiveType)
		require.Equal(t, "at TestConfigRecurPtrThroughSlice.Recurs: "+
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

		_, err := LoadSrc[TestConfig](`int: 42`)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Int: unsupported type: int, "+
			"use integer type with specified width, "+
			"such as int8, int16, int32 or int64 instead of int", err.Error())
	})

	t.Run("uint", func(t *testing.T) {
		type TestConfig struct {
			Uint uint `yaml:"uint"`
		}

		_, err := LoadSrc[TestConfig](`uint: 42`)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Uint: unsupported type: uint, "+
			"use unsigned integer type with specified width, "+
			"such as uint8, uint16, uint32 or uint64 instead of uint", err.Error())
	})

	t.Run("ptr_ptr", func(t *testing.T) {
		type TestConfig struct {
			PtrPtr **int `yaml:"ptr-ptr"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t, "at TestConfig.PtrPtr: unsupported pointer type", err.Error())
	})

	t.Run("ptr_ptr_in_slice", func(t *testing.T) {
		type TestConfig struct {
			Slice []**int `yaml:"slice"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t, "at TestConfig.Slice: unsupported pointer type", err.Error())
	})

	t.Run("ptr_ptr_in_map", func(t *testing.T) {
		type TestConfig struct {
			Map map[string]**int `yaml:"map"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t,
			"at TestConfig.Map[value]: unsupported pointer type",
			err.Error())
	})

	t.Run("ptr_slice", func(t *testing.T) {
		type TestConfig struct {
			PtrSlice *[]string `yaml:"ptr-slice"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t, "at TestConfig.PtrSlice: unsupported pointer type", err.Error())
	})

	t.Run("ptr_map", func(t *testing.T) {
		type TestConfig struct {
			PtrMap *map[string]string `yaml:"ptr-map"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedPtrType)
		require.Equal(t, "at TestConfig.PtrMap: unsupported pointer type", err.Error())
	})

	t.Run("channel", func(t *testing.T) {
		type TestConfig struct {
			Chan chan int `yaml:"chan"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Chan: unsupported type: chan int", err.Error())
	})

	t.Run("func", func(t *testing.T) {
		type TestConfig struct {
			Func func() `yaml:"func"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Func: unsupported type: func()", err.Error())
	})

	t.Run("unsafe_pointer", func(t *testing.T) {
		type TestConfig struct {
			UnsafePointer unsafe.Pointer `yaml:"unsafe-pointer"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.UnsafePointer: "+
			"unsupported type: unsafe.Pointer", err.Error())
	})

	t.Run("interface", func(t *testing.T) {
		type TestConfig struct {
			Interface interface{ Write() ([]byte, int) } `yaml:"interface"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Interface: "+
			"unsupported type: interface { Write() ([]uint8, int) }", err.Error())
	})

	t.Run("empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Anything: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("slice_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything []any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  - foo\n  - bar")
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Anything: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("array_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything [2]any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  - foo\n  - bar")
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Anything: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("map_key_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything map[any]string `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  foo: bar")
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Anything[key]: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("map_value_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything map[string]any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  foo: bar")
		require.ErrorIs(t, err, yamagiconf.ErrUnsupportedType)
		require.Equal(t, "at TestConfig.Anything[value]: "+
			"unsupported type: interface {}", err.Error())
	})
}

func TestValidateTypeErrIllegalRootType(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		err := yamagiconf.ValidateType[string]()
		require.ErrorIs(t, err, yamagiconf.ErrIllegalRootType)
		require.Equal(t, fmt.Sprintf(
			"at string: %s", yamagiconf.ErrIllegalRootType.Error(),
		), err.Error())
	})

	t.Run("implements_yaml_unmarshaler", func(t *testing.T) {
		err := yamagiconf.ValidateType[YAMLUnmarshaler]()
		require.ErrorIs(t, err, yamagiconf.ErrIllegalRootType)
		require.Equal(t, fmt.Sprintf(
			"at YAMLUnmarshaler: %s", yamagiconf.ErrIllegalRootType.Error(),
		), err.Error())
	})

	t.Run("implements_text_unmarshaler", func(t *testing.T) {
		err := yamagiconf.ValidateType[TextUnmarshaler]()
		require.ErrorIs(t, err, yamagiconf.ErrIllegalRootType)
		require.Equal(t, fmt.Sprintf(
			"at TextUnmarshaler: %s", yamagiconf.ErrIllegalRootType.Error(),
		), err.Error())
	})
}

func TestValidateTypeErrYAMLTagOnUnexported(t *testing.T) {
	type TestConfig struct {
		Ok string `yaml:"okay"`
		//lint:ignore U1000 ignore for testing purposes
		unexported string `yaml:"unexported"`
	}
	err := yamagiconf.ValidateType[TestConfig]()
	require.ErrorIs(t, err, yamagiconf.ErrYAMLTagOnUnexported)
	require.Equal(t,
		"at TestConfig.unexported: yaml tag on unexported field",
		err.Error())
}

func TestValidateTypeErrYAMLInlineNonAnon(t *testing.T) {
	t.Run("struct", func(t *testing.T) {
		type Container struct {
			Str string `yaml:"str"`
		}
		type TestConfig struct {
			Container Container `yaml:",inline"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrYAMLInlineNonAnon)
		require.Equal(t,
			"at TestConfig.Container: inline yaml on non-embedded field",
			err.Error())
	})

	t.Run("struct_with_tag", func(t *testing.T) {
		type Container struct {
			Str string `yaml:"str"`
		}
		type TestConfig struct {
			Container Container `yaml:"container,inline"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrYAMLInlineNonAnon)
		require.Equal(t,
			"at TestConfig.Container: inline yaml on non-embedded field",
			err.Error())
	})

	t.Run("after_other_struct_tags", func(t *testing.T) {
		type Container struct {
			Str string `yaml:"str"`
		}
		type TestConfig struct {
			Container Container `yaml:",omitempty,inline"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrYAMLInlineNonAnon)
		require.Equal(t,
			"at TestConfig.Container: inline yaml on non-embedded field",
			err.Error())
	})

	t.Run("string", func(t *testing.T) {
		type TestConfig struct {
			Str string `yaml:",inline"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrYAMLInlineNonAnon)
		require.Equal(t,
			"at TestConfig.Str: inline yaml on non-embedded field",
			err.Error())
	})

	t.Run("string_with_tag", func(t *testing.T) {
		type TestConfig struct {
			Str string `yaml:"str,inline"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrYAMLInlineNonAnon)
		require.Equal(t,
			"at TestConfig.Str: inline yaml on non-embedded field",
			err.Error())
	})
}

func TestValidateTypeErrYAMLNoInlineOpt(t *testing.T) {
	t.Run("no_inline", func(t *testing.T) {
		type Container struct {
			Str string `yaml:"str"`
		}
		type TestConfig struct {
			Container `yaml:"container"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrYAMLInlineOpt)
		require.Equal(t,
			"at TestConfig.Container: use `yaml:\",inline\"` for embedded fields",
			err.Error())
	})

	t.Run("named_inline", func(t *testing.T) {
		type Container struct {
			Str string `yaml:"str"`
		}
		type TestConfig struct {
			Container `yaml:"container,inline"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrYAMLInlineOpt)
		require.Equal(t,
			"at TestConfig.Container: use `yaml:\",inline\"` for embedded fields",
			err.Error())
	})
}

func TestValidateTypeErrYAMLTagRedefined(t *testing.T) {
	type TestConfig struct {
		First  string `yaml:"x"`
		Second string `yaml:"x"`
	}
	err := yamagiconf.ValidateType[TestConfig]()
	require.ErrorIs(t, err, yamagiconf.ErrYAMLTagRedefined)
	require.Equal(t, `at TestConfig.Second: yaml tag "x" `+
		`previously defined on field TestConfig.First: `+
		`a yaml tag must be unique`, err.Error())
}

func TestValidateTypeErrEnvTagOnUnexported(t *testing.T) {
	type TestConfig struct {
		Ok string `yaml:"okay"`
		//lint:ignore U1000 ignore for testing purposes
		unexported string `env:"ok"`
	}
	err := yamagiconf.ValidateType[TestConfig]()
	require.ErrorIs(t, err, yamagiconf.ErrEnvTagOnUnexported)
	require.Equal(t,
		"at TestConfig.unexported: env tag on unexported field",
		err.Error())
}

func TestValidateTypeErrNoExportedFields(t *testing.T) {
	type TestConfig struct {
		//lint:ignore U1000 ignore for testing purposes
		unexported1 string
		//lint:ignore U1000 ignore for testing purposes
		unexported2 string
	}
	err := yamagiconf.ValidateType[TestConfig]()
	require.ErrorIs(t, err, yamagiconf.ErrNoExportedFields)
	require.Equal(t, "at TestConfig: no exported fields", err.Error())
}

func TestValidateTypeErrTagOnInterfaceImpl(t *testing.T) {
	t.Run("NoopTextUnmarshalerWithYAMLTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopTextUnmarshalerWithYAMLTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements encoding.TextUnmarshaler `+
			`but field contains tag "yaml" ("illegal"): `+
			yamagiconf.ErrTagOnInterfaceImpl.Error(), err.Error())
	})

	t.Run("NoopYAMLUnmarshalerWithYAMLTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopYAMLUnmarshalerWithYAMLTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements yaml.Unmarshaler `+
			`but field contains tag "yaml" ("illegal"): `+
			yamagiconf.ErrTagOnInterfaceImpl.Error(), err.Error())
	})

	t.Run("NoopTextUnmarshalerWithEnvTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopTextUnmarshalerWithEnvTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements encoding.TextUnmarshaler `+
			`but field contains tag "env" ("illegal"): `+
			yamagiconf.ErrTagOnInterfaceImpl.Error(), err.Error())
	})

	t.Run("NoopYAMLUnmarshalerWithEnvTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopYAMLUnmarshalerWithEnvTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements yaml.Unmarshaler `+
			`but field contains tag "env" ("illegal"): `+
			yamagiconf.ErrTagOnInterfaceImpl.Error(), err.Error())
	})
}

type (
	NoopTextUnmarshalerWithYAMLTag struct {
		HasYAMLTag string `yaml:"illegal"`
	}
	NoopYAMLUnmarshalerWithYAMLTag struct {
		HasYAMLTag string `yaml:"illegal"`
	}
	NoopTextUnmarshalerWithEnvTag struct {
		HasEnvTag string `env:"illegal"`
	}
	NoopYAMLUnmarshalerWithEnvTag struct {
		HasEnvTag string `env:"illegal"`
	}
)

func (u *NoopTextUnmarshalerWithYAMLTag) UnmarshalText([]byte) error     { return nil }
func (u *NoopYAMLUnmarshalerWithYAMLTag) UnmarshalYAML(*yaml.Node) error { return nil }
func (u *NoopTextUnmarshalerWithEnvTag) UnmarshalText([]byte) error      { return nil }
func (u *NoopYAMLUnmarshalerWithEnvTag) UnmarshalYAML(*yaml.Node) error  { return nil }

var (
	_ encoding.TextUnmarshaler = new(NoopTextUnmarshalerWithYAMLTag)
	_ yaml.Unmarshaler         = new(NoopYAMLUnmarshalerWithYAMLTag)
	_ encoding.TextUnmarshaler = new(NoopTextUnmarshalerWithEnvTag)
	_ yaml.Unmarshaler         = new(NoopYAMLUnmarshalerWithEnvTag)
)

func TestAnonymousStructErrorPath(t *testing.T) {
	var c struct {
		MissingYAMLTag string
	}
	err := yamagiconf.Load(`ok: ok`, &c)
	require.ErrorIs(t, err, yamagiconf.ErrMissingYAMLTag)
	require.Equal(t,
		"at struct{...}.MissingYAMLTag: missing yaml struct tag",
		err.Error())
}

func TestLoadErrMissingConfig(t *testing.T) {
	type TestConfig struct {
		OK      string `yaml:"ok,omitempty"`
		Missing string `yaml:"missing,omitempty"`
	}
	_, err := LoadSrc[TestConfig]("ok: 'OK'")
	require.ErrorIs(t, err, yamagiconf.ErrMissingConfig)
	require.Equal(t,
		`at TestConfig.Missing (as "missing"): missing field in config file`,
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

func TestLoadErrYAMLTagUsed(t *testing.T) {
	type TestConfig struct {
		Str string `yaml:"str"`
	}

	t.Run("str", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("str: !!str NOTOK")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLTagUsed)
		require.Equal(t,
			`at 1:6: "str" (TestConfig.Str): tag "!!str": avoid using YAML tags`,
			err.Error())
	})

	t.Run("custom", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("str: !.custom NOTOK")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLTagUsed)
		require.Equal(t,
			`at 1:6: "str" (TestConfig.Str): tag "!.custom": avoid using YAML tags`,
			err.Error())
	})

	t.Run("custom_novalue", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("str: !0")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLTagUsed)
		require.Equal(t,
			`at 1:6: "str" (TestConfig.Str): tag "!0": avoid using YAML tags`,
			err.Error())
	})

	t.Run("custom_at_level_1", func(t *testing.T) {
		type TestConfig struct {
			Container struct {
				Str string `yaml:"str"`
			} `yaml:"container"`
		}

		_, err := LoadSrc[TestConfig]("container:\n  str: !.custom NOTOK")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLTagUsed)
		require.Equal(t,
			`at 2:8: "str" (TestConfig.Container.Str): tag "!.custom": `+
				`avoid using YAML tags`,
			err.Error())
	})

	t.Run("custom_at_level_1_map", func(t *testing.T) {
		type TestConfig struct {
			Container struct {
				Map map[string]string `yaml:"map"`
			} `yaml:"container"`
		}

		_, err := LoadSrc[TestConfig]("container:\n  map:\n    key: !:custom value")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLTagUsed)
		require.Equal(t,
			`at 3:10: "map" (TestConfig.Container.Map["key"]): tag "!:custom": `+
				`avoid using YAML tags`,
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

	t.Run("nil_config", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "test-config.yaml")
		_, err := os.Create(p)
		require.NoError(t, err)
		err = yamagiconf.LoadFile[TestConfig](p, nil)
		require.ErrorIs(t, err, yamagiconf.ErrNilConfig)
		require.Equal(t, "cannot load into nil config", err.Error())
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
	type Foo struct {
		Foo string `yaml:"foo" env:"FOO"`
	}
	type Map2D = map[string]map[string]string
	type TestConfig struct {
		BoolFalse bool          `yaml:"bool_false" env:"BOOL_FALSE"`
		BoolTrue  bool          `yaml:"bool_true" env:"BOOL_TRUE"`
		String    string        `yaml:"string" env:"STRING"`
		Float32   float32       `yaml:"float32" env:"FLOAT_32"`
		Float64   float64       `yaml:"float64" env:"FLOAT_64"`
		Int8      int8          `yaml:"int8" env:"INT_8"`
		Uint8     uint8         `yaml:"uint8" env:"UINT_8"`
		Int16     int16         `yaml:"int16" env:"INT_16"`
		Uint16    uint16        `yaml:"uint16" env:"UINT_16"`
		Int32     int32         `yaml:"int32" env:"INT_32"`
		Uint32    uint32        `yaml:"uint32" env:"UINT_32"`
		Int64     int64         `yaml:"int64" env:"INT_64"`
		Uint64    uint64        `yaml:"uint64" env:"UINT_64"`
		Time      time.Time     `yaml:"time" env:"TIME"`
		Duration  time.Duration `yaml:"duration" env:"DURATION"`

		PtrBoolFalse *bool          `yaml:"ptr-bool_false" env:"PTR_BOOL_FALSE"`
		PtrBoolTrue  *bool          `yaml:"ptr-bool_true" env:"PTR_BOOL_TRUE"`
		PtrString    *string        `yaml:"ptr-string" env:"PTR_STRING"`
		PtrFloat32   *float32       `yaml:"ptr-float32" env:"PTR_FLOAT_32"`
		PtrFloat64   *float64       `yaml:"ptr-float64" env:"PTR_FLOAT_64"`
		PtrInt8      *int8          `yaml:"ptr-int8" env:"PTR_INT_8"`
		PtrUint8     *uint8         `yaml:"ptr-uint8" env:"PTR_UINT_8"`
		PtrInt16     *int16         `yaml:"ptr-int16" env:"PTR_INT_16"`
		PtrUint16    *uint16        `yaml:"ptr-uint16" env:"PTR_UINT_16"`
		PtrInt32     *int32         `yaml:"ptr-int32" env:"PTR_INT_32"`
		PtrUint32    *uint32        `yaml:"ptr-uint32" env:"PTR_UINT_32"`
		PtrInt64     *int64         `yaml:"ptr-int64" env:"PTR_INT_64"`
		PtrUint64    *uint64        `yaml:"ptr-uint64" env:"PTR_UINT_64"`
		PtrTime      *time.Time     `yaml:"ptr-time" env:"PTR_TIME"`
		PtrDuration  *time.Duration `yaml:"ptr-duration" env:"PTR_DURATION"`

		PtrBoolNull     *bool          `yaml:"ptr-bool-null" env:"PTR_BOOL_NULL"`
		PtrStringNull   *string        `yaml:"ptr-string-null" env:"PTR_STRING_NULL"`
		PtrFloat32Null  *float32       `yaml:"ptr-float32-null" env:"PTR_FLOAT_32_NULL"`
		PtrFloat64Null  *float64       `yaml:"ptr-float64-null" env:"PTR_FLOAT_64_NULL"`
		PtrInt8Null     *int8          `yaml:"ptr-int8-null" env:"PTR_INT_8_NULL"`
		PtrUint8Null    *uint8         `yaml:"ptr-uint8-null" env:"PTR_UINT_8_NULL"`
		PtrInt16Null    *int16         `yaml:"ptr-int16-null" env:"PTR_INT_16_NULL"`
		PtrUint16Null   *uint16        `yaml:"ptr-uint16-null" env:"PTR_UINT_16_NULL"`
		PtrInt32Null    *int32         `yaml:"ptr-int32-null" env:"PTR_INT_32_NULL"`
		PtrUint32Null   *uint32        `yaml:"ptr-uint32-null" env:"PTR_UINT_32_NULL"`
		PtrInt64Null    *int64         `yaml:"ptr-int64-null" env:"PTR_INT_64_NULL"`
		PtrUint64Null   *uint64        `yaml:"ptr-uint64-null" env:"PTR_UINT_64_NULL"`
		PtrTimeNull     *time.Time     `yaml:"ptr-time-null" env:"PTR_TIME_NULL"`
		PtrDurationNull *time.Duration `yaml:"ptr-duration-null" env:"PTR_DURATION_NULL"`

		Foo       Foo             `yaml:"foo"`
		MapFoo    map[string]Foo  `yaml:"map-foo"`
		MapPtrFoo map[string]*Foo `yaml:"map-ptr-foo"`
		SliceFoo  []Foo           `yaml:"slice-foo"`
		ArrayFoo  [1]Foo          `yaml:"array-foo"`
		Map2D     Map2D           `yaml:"map-2d"`

		UnmarshalerText    TextUnmarshaler  `yaml:"unm-text" env:"UNMARSH_TEXT"`
		PtrUnmarshalerText *TextUnmarshaler `yaml:"ptr-unm-text" env:"PTR_UNMARSH_TEXT"`

		// ignored must be ignored by yamagiconf even though it's
		// of type int which is unsupported.
		//lint:ignore U1000 no need to use it.
		ignored int
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
	t.Setenv("TIME", "2000-10-10T10:10:10Z")
	t.Setenv("DURATION", "30m")

	t.Setenv("PTR_BOOL_FALSE", "false")
	t.Setenv("PTR_BOOL_TRUE", "true")
	t.Setenv("PTR_STRING", "test text")
	t.Setenv("PTR_FLOAT_32", "3.14")
	t.Setenv("PTR_FLOAT_64", "3.1415")
	t.Setenv("PTR_INT_8", "-1")
	t.Setenv("PTR_UINT_8", "1")
	t.Setenv("PTR_INT_16", "-1")
	t.Setenv("PTR_UINT_16", "1")
	t.Setenv("PTR_INT_32", "-1")
	t.Setenv("PTR_UINT_32", "1")
	t.Setenv("PTR_INT_64", "-1")
	t.Setenv("PTR_UINT_64", "1")
	t.Setenv("PTR_TIME", "2000-10-10T10:10:10Z")
	t.Setenv("PTR_DURATION", "12s")

	t.Setenv("PTR_BOOL_NULL", "null")
	t.Setenv("PTR_STRING_NULL", "null")
	t.Setenv("PTR_FLOAT_32_NULL", "null")
	t.Setenv("PTR_FLOAT_64_NULL", "null")
	t.Setenv("PTR_INT_8_NULL", "null")
	t.Setenv("PTR_UINT_8_NULL", "null")
	t.Setenv("PTR_INT_16_NULL", "null")
	t.Setenv("PTR_UINT_16_NULL", "null")
	t.Setenv("PTR_INT_32_NULL", "null")
	t.Setenv("PTR_UINT_32_NULL", "null")
	t.Setenv("PTR_INT_64_NULL", "null")
	t.Setenv("PTR_UINT_64_NULL", "null")
	t.Setenv("PTR_TIME_NULL", "null")
	t.Setenv("PTR_DURATION_NULL", "null")

	t.Setenv("FOO", "bar")
	t.Setenv("UNMARSH_TEXT", "ut")
	t.Setenv("PTR_UNMARSH_TEXT", "ptr_ut")

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
time: 1999-10-10T10:10:10Z
duration: 0s

ptr-bool_false: true
ptr-bool_true: false
ptr-string: ''
ptr-float32: 0
ptr-float64: 0
ptr-int8: 0
ptr-uint8: 0
ptr-int16: 0
ptr-uint16: 0
ptr-int32: 0
ptr-uint32: 0
ptr-int64: 0
ptr-uint64: 0
ptr-time: 1999-10-10T10:10:10Z
ptr-duration: 0s

ptr-bool-null: true
ptr-string-null: ''
ptr-float32-null: 0
ptr-float64-null: 0
ptr-int8-null: 0
ptr-uint8-null: 0
ptr-int16-null: 0
ptr-uint16-null: 0
ptr-int32-null: 0
ptr-uint32-null: 0
ptr-int64-null: 0
ptr-uint64-null: 0
ptr-time-null: 1999-10-10T10:10:10Z
ptr-duration-null: 0s

foo:
  foo: foo
map-foo:
  key:
    foo: fuzz
map-ptr-foo:
  bar:
    foo: fuzz
  bazz: null
slice-foo:
  - foo: fuzz
  - foo: fuzz
array-foo:
  - foo: fuzz
map-2d:
  foo:
    bar: bazz
    muzz: tazz
  kraz:
    fraz: sazz
unm-text: x
ptr-unm-text: null
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
	require.Equal(t, time.Date(2000, 10, 10, 10, 10, 10, 0, time.UTC), c.Time)
	require.Equal(t, 30*time.Minute, c.Duration)

	require.Equal(t, false, *c.PtrBoolFalse)
	require.Equal(t, true, *c.PtrBoolTrue)
	require.Equal(t, "test text", *c.PtrString)
	require.Equal(t, float32(3.14), *c.PtrFloat32)
	require.Equal(t, float64(3.1415), *c.PtrFloat64)
	require.Equal(t, int8(-1), *c.PtrInt8)
	require.Equal(t, uint8(1), *c.PtrUint8)
	require.Equal(t, int16(-1), *c.PtrInt16)
	require.Equal(t, uint16(1), *c.PtrUint16)
	require.Equal(t, int32(-1), *c.PtrInt32)
	require.Equal(t, uint32(1), *c.PtrUint32)
	require.Equal(t, int64(-1), *c.PtrInt64)
	require.Equal(t, uint64(1), *c.PtrUint64)
	require.Equal(t, time.Date(2000, 10, 10, 10, 10, 10, 0, time.UTC), *c.PtrTime)
	require.Equal(t, 12*time.Second, *c.PtrDuration)

	require.Nil(t, c.PtrBoolNull)
	require.Nil(t, c.PtrStringNull)
	require.Nil(t, c.PtrFloat32Null)
	require.Nil(t, c.PtrFloat64Null)
	require.Nil(t, c.PtrInt8Null)
	require.Nil(t, c.PtrUint8Null)
	require.Nil(t, c.PtrInt16Null)
	require.Nil(t, c.PtrUint16Null)
	require.Nil(t, c.PtrInt32Null)
	require.Nil(t, c.PtrUint32Null)
	require.Nil(t, c.PtrInt64Null)
	require.Nil(t, c.PtrUint64Null)
	require.Nil(t, c.PtrTimeNull)
	require.Nil(t, c.PtrDurationNull)

	require.Equal(t, Foo{Foo: "bar"}, c.Foo)
	require.Equal(t, map[string]Foo{"key": {Foo: "bar"}}, c.MapFoo)
	require.Equal(t, map[string]*Foo{
		"bar":  {Foo: "bar"},
		"bazz": nil,
	}, c.MapPtrFoo)
	require.Equal(t, []Foo{{Foo: "bar"}, {Foo: "bar"}}, c.SliceFoo)
	require.Equal(t, [1]Foo{{Foo: "bar"}}, c.ArrayFoo)
	require.Equal(t, Map2D{
		"foo":  {"bar": "bazz", "muzz": "tazz"},
		"kraz": {"fraz": "sazz"},
	}, c.Map2D)
	require.Equal(t, "ut", c.UnmarshalerText.Str)
	require.Equal(t, "ptr_ut", c.PtrUnmarshalerText.Str)
}

func TestLoadEnvVarNoOverwrite(t *testing.T) {
	type TestConfig struct {
		BoolFalse bool          `yaml:"bool_false" env:"BOOL_FALSE"`
		BoolTrue  bool          `yaml:"bool_true" env:"BOOL_TRUE"`
		String    string        `yaml:"string" env:"STRING"`
		Float32   float32       `yaml:"float32" env:"FLOAT_32"`
		Float64   float64       `yaml:"float64" env:"FLOAT_64"`
		Int8      int8          `yaml:"int8" env:"INT_8"`
		Uint8     uint8         `yaml:"uint8" env:"UINT_8"`
		Int16     int16         `yaml:"int16" env:"INT_16"`
		Uint16    uint16        `yaml:"uint16" env:"UINT_16"`
		Int32     int32         `yaml:"int32" env:"INT_32"`
		Uint32    uint32        `yaml:"uint32" env:"UINT_32"`
		Int64     int64         `yaml:"int64" env:"INT_64"`
		Uint64    uint64        `yaml:"uint64" env:"UINT_64"`
		Time      time.Time     `yaml:"time" env:"TIME"`
		Duration  time.Duration `yaml:"duration" env:"DURATION"`

		PtrBoolFalse *bool          `yaml:"ptr-bool_false" env:"PTR_BOOL_FALSE"`
		PtrBoolTrue  *bool          `yaml:"ptr-bool_true" env:"PTR_BOOL_TRUE"`
		PtrString    *string        `yaml:"ptr-string" env:"PTR_STRING"`
		PtrFloat32   *float32       `yaml:"ptr-float32" env:"PTR_FLOAT_32"`
		PtrFloat64   *float64       `yaml:"ptr-float64" env:"PTR_FLOAT_64"`
		PtrInt8      *int8          `yaml:"ptr-int8" env:"PTR_INT_8"`
		PtrUint8     *uint8         `yaml:"ptr-uint8" env:"PTR_UINT_8"`
		PtrInt16     *int16         `yaml:"ptr-int16" env:"PTR_INT_16"`
		PtrUint16    *uint16        `yaml:"ptr-uint16" env:"PTR_UINT_16"`
		PtrInt32     *int32         `yaml:"ptr-int32" env:"PTR_INT_32"`
		PtrUint32    *uint32        `yaml:"ptr-uint32" env:"PTR_UINT_32"`
		PtrInt64     *int64         `yaml:"ptr-int64" env:"PTR_INT_64"`
		PtrUint64    *uint64        `yaml:"ptr-uint64" env:"PTR_UINT_64"`
		PtrTime      *time.Time     `yaml:"ptr-time" env:"PTR_TIME"`
		PtrDuration  *time.Duration `yaml:"ptr-duration" env:"PTR_DURATION"`

		PtrBoolNull     *bool          `yaml:"ptr-bool-null" env:"PTR_BOOL_NULL"`
		PtrStringNull   *string        `yaml:"ptr-string-null" env:"PTR_STRING_NULL"`
		PtrFloat32Null  *float32       `yaml:"ptr-float32-null" env:"PTR_FLOAT_32_NULL"`
		PtrFloat64Null  *float64       `yaml:"ptr-float64-null" env:"PTR_FLOAT_64_NULL"`
		PtrInt8Null     *int8          `yaml:"ptr-int8-null" env:"PTR_INT_8_NULL"`
		PtrUint8Null    *uint8         `yaml:"ptr-uint8-null" env:"PTR_UINT_8_NULL"`
		PtrInt16Null    *int16         `yaml:"ptr-int16-null" env:"PTR_INT_16_NULL"`
		PtrUint16Null   *uint16        `yaml:"ptr-uint16-null" env:"PTR_UINT_16_NULL"`
		PtrInt32Null    *int32         `yaml:"ptr-int32-null" env:"PTR_INT_32_NULL"`
		PtrUint32Null   *uint32        `yaml:"ptr-uint32-null" env:"PTR_UINT_32_NULL"`
		PtrInt64Null    *int64         `yaml:"ptr-int64-null" env:"PTR_INT_64_NULL"`
		PtrUint64Null   *uint64        `yaml:"ptr-uint64-null" env:"PTR_UINT_64_NULL"`
		PtrTimeNull     *time.Time     `yaml:"ptr-time-null" env:"PTR_TIME_NULL"`
		PtrDurationNull *time.Duration `yaml:"ptr-duration-null" env:"PTR_DURATION_NULL"`
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
time: 2024-01-01T01:01:01Z
duration: 10s

ptr-bool_false: true
ptr-bool_true: false
ptr-string: ''
ptr-float32: 0
ptr-float64: 0
ptr-int8: 0
ptr-uint8: 0
ptr-int16: 0
ptr-uint16: 0
ptr-int32: 0
ptr-uint32: 0
ptr-int64: 0
ptr-uint64: 0
ptr-time: 2024-01-01T01:01:01Z
ptr-duration: 10s

ptr-bool-null: null
ptr-string-null: null
ptr-float32-null: null
ptr-float64-null: null
ptr-int8-null: null
ptr-uint8-null: null
ptr-int16-null: null
ptr-uint16-null: null
ptr-int32-null: null
ptr-uint32-null: null
ptr-int64-null: null
ptr-uint64-null: null
ptr-time-null: null
ptr-duration-null: null
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
	require.Equal(t, time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC), c.Time)
	require.Equal(t, 10*time.Second, c.Duration)

	require.Equal(t, true, *c.PtrBoolFalse)
	require.Equal(t, false, *c.PtrBoolTrue)
	require.Equal(t, "", *c.PtrString)
	require.Equal(t, float32(0), *c.PtrFloat32)
	require.Equal(t, float64(0), *c.PtrFloat64)
	require.Equal(t, int8(0), *c.PtrInt8)
	require.Equal(t, uint8(0), *c.PtrUint8)
	require.Equal(t, int16(0), *c.PtrInt16)
	require.Equal(t, uint16(0), *c.PtrUint16)
	require.Equal(t, int32(0), *c.PtrInt32)
	require.Equal(t, uint32(0), *c.PtrUint32)
	require.Equal(t, int64(0), *c.PtrInt64)
	require.Equal(t, uint64(0), *c.PtrUint64)
	require.Equal(t, time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC), *c.PtrTime)
	require.Equal(t, 10*time.Second, *c.PtrDuration)

	require.Nil(t, c.PtrBoolNull)
	require.Nil(t, c.PtrStringNull)
	require.Nil(t, c.PtrFloat32Null)
	require.Nil(t, c.PtrFloat64Null)
	require.Nil(t, c.PtrInt8Null)
	require.Nil(t, c.PtrUint8Null)
	require.Nil(t, c.PtrInt16Null)
	require.Nil(t, c.PtrUint16Null)
	require.Nil(t, c.PtrInt32Null)
	require.Nil(t, c.PtrUint32Null)
	require.Nil(t, c.PtrInt64Null)
	require.Nil(t, c.PtrUint64Null)
	require.Nil(t, c.PtrTimeNull)
	require.Nil(t, c.PtrDurationNull)
}

func TestLoadEnvVarErr(t *testing.T) {
	t.Run("map_of_slice", func(t *testing.T) {
		type Dur struct {
			Duration time.Duration `yaml:"duration" env:"DURATION"`
		}
		type Container struct {
			Map map[string][]Dur `yaml:"map"`
		}
		type TestConfig struct {
			Container Container `yaml:"container"`
		}
		t.Setenv("DURATION", "10minutes") // Invalid time.Duration value
		_, err := LoadSrc[TestConfig](`
container:
  map:
    foo:
      - duration: 12s
    bar:
      - duration: 12m
`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, `at TestConfig.Container.Map[bar][0].Duration: `+
			`invalid env var DURATION: expected time.Duration: time: `+
			`unknown unit "minutes" in duration "10minutes"`, err.Error())
	})

	t.Run("map_of_map", func(t *testing.T) {
		type Dur struct {
			Duration time.Duration `yaml:"duration" env:"DURATION"`
		}
		type Container struct {
			Map map[string]map[string]*Dur `yaml:"map"`
		}
		type TestConfig struct {
			Container Container `yaml:"container"`
		}
		t.Setenv("DURATION", "10minutes") // Invalid time.Duration value
		_, err := LoadSrc[TestConfig](`
container:
  map:
    foo:
      fazz:
        duration: 12s
    bar:
      bazz:
        duration: 12m
`)
		require.ErrorIs(t, err, yamagiconf.ErrInvalidEnvVar)
		require.Equal(t, `at TestConfig.Container.Map[bar][bazz].Duration: `+
			`invalid env var DURATION: expected time.Duration: time: `+
			`unknown unit "minutes" in duration "10minutes"`, err.Error())
	})
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

type (
	TextUnmarshaler        struct{ Str string }
	TextUnmarshalerCopyRcv struct{ Str *string }
	YAMLUnmarshaler        struct{ Str string }
)

func (u *TextUnmarshaler) UnmarshalText(d []byte) error {
	u.Str = string(d)
	return nil
}

func (u TextUnmarshalerCopyRcv) UnmarshalText(d []byte) error {
	*u.Str = string(d)
	return nil
}

func (u *YAMLUnmarshaler) UnmarshalYAML(n *yaml.Node) error {
	u.Str = n.Value
	return nil
}

var (
	_ encoding.TextUnmarshaler = new(TextUnmarshaler)
	_ encoding.TextUnmarshaler = new(TextUnmarshalerCopyRcv)
	_ yaml.Unmarshaler         = new(YAMLUnmarshaler)
)

func TestLoadTextUnmarshaler(t *testing.T) {
	var u2str string
	c := struct {
		U1    TextUnmarshaler        `yaml:"u1"`
		U2    TextUnmarshalerCopyRcv `yaml:"u2"`
		U1Ptr *TextUnmarshaler       `yaml:"u1_ptr"`
	}{
		U2: TextUnmarshalerCopyRcv{Str: &u2str},
	}
	err := yamagiconf.Load("u1: t1\nu2: t2\nu1_ptr: t3", &c)
	require.NoError(t, err)
	require.Equal(t, "t1", c.U1.Str)
	require.Equal(t, "t2", *c.U2.Str)
	require.Equal(t, "t3", c.U1Ptr.Str)
}
