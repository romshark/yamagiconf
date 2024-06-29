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
		return &c, err
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
		SliceSliceString  [][]string            `yaml:"slice-slice-string"`
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

		// Fields ignored due to struct tag yaml:"-"
		IgnoredAny  any  `yaml:"-"`
		IgnoredInt  int  `yaml:"-"`
		IgnoredUint uint `yaml:"-"`

		//lint:ignore U1000 no need to use it.
		unexportedIgnoredNoYAML int `yaml:"-"`
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
slice-slice-string:
  - - first
    - second
  - - third
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
	require.Equal(t, [][]string{{"first", "second"}, {"third"}}, c.SliceSliceString)
	require.Equal(t, time.Date(2024, 5, 9, 20, 19, 22, 0, time.UTC), c.Time)
	require.Equal(t, `YAML unmarshaler non-pointer non-null`, string(c.UnmarshalerYAML))
	require.Equal(t, `Text unmarshaler non-pointer non-null`, c.UnmarshalerText.Str)
	require.Equal(t, `YAML unmarshaler pointer`, string(*c.PtrUnmarshalerYAML))
	require.Equal(t, `Text unmarshaler pointer`, c.PtrUnmarshalerText.Str)
	require.Nil(t, c.PtrUnmarshalerYAMLNull)
	require.Nil(t, c.PtrUnmarshalerTextNull)
}

func TestAnchors(t *testing.T) {
	type TestConfig struct {
		Bool      bool `yaml:"bool"`
		BoolAlias bool `yaml:"alias-bool"`

		PtrBool      *bool `yaml:"ptr-bool"`
		PtrBoolAlias *bool `yaml:"alias-ptr-bool"`

		PtrBoolNull      *bool `yaml:"ptr-bool-null"`
		PtrBoolNullAlias *bool `yaml:"alias-ptr-bool-null"`

		Str      string `yaml:"str"`
		StrAlias string `yaml:"alias-str"`

		Float32      float32 `yaml:"float32"`
		Float32Alias float32 `yaml:"alias-float32"`

		Float64      float64 `yaml:"float64"`
		Float64Alias float64 `yaml:"alias-float64"`

		Int8      int8 `yaml:"int8"`
		Int8Alias int8 `yaml:"alias-int8"`

		Int16      int16 `yaml:"int16"`
		Int16Alias int16 `yaml:"alias-int16"`

		Int32      int32 `yaml:"int32"`
		Int32Alias int32 `yaml:"alias-int32"`

		Int64      int64 `yaml:"int64"`
		Int64Alias int64 `yaml:"alias-int64"`

		Uint8      uint8 `yaml:"uint8"`
		Uint8Alias uint8 `yaml:"alias-uint8"`

		Uint16      uint16 `yaml:"uint16"`
		Uint16Alias uint16 `yaml:"alias-uint16"`

		Uint32      uint32 `yaml:"uint32"`
		Uint32Alias uint32 `yaml:"alias-uint32"`

		Uint64      uint64 `yaml:"uint64"`
		Uint64Alias uint64 `yaml:"alias-uint64"`

		SliceStr      []string `yaml:"slice-str"`
		SliceStrAlias []string `yaml:"alias-slice-str"`

		SliceStrNull      []string `yaml:"slice-str-null"`
		SliceStrNullAlias []string `yaml:"alias-slice-str-null"`

		SliceInt64      []int64 `yaml:"slice-int64"`
		SliceInt64Alias []int64 `yaml:"alias-slice-int64"`

		SliceSliceStr      [][]string `yaml:"slice-slice-str"`
		SliceSliceStrAlias [][]string `yaml:"alias-slice-slice-str"`

		MapStrStr      map[string]string `yaml:"map-str-str"`
		MapStrStrAlias map[string]string `yaml:"alias-map-str-str"`

		MapStrStrNull      map[string]string `yaml:"map-str-str-null"`
		MapStrStrNullAlias map[string]string `yaml:"alias-map-str-str-null"`

		MapComplex      map[string]map[string][2][]string `yaml:"map-complex"`
		MapComplexAlias map[string]map[string][2][]string `yaml:"alias-map-complex"`
	}
	var c TestConfig
	err := yamagiconf.Load(`# test YAML file
ptr-bool: &ptr-bool true
ptr-bool-null: &ptr-bool-null null
bool: &bool true
str: &str example text
float32: &float32 3.14
float64: &float64 628.28
int8: &int8 123
int16: &int16 1234
int32: &int32 12345
int64: &int64 123456
uint8: &uint8 123
uint16: &uint16 1234
uint32: &uint32 12345
uint64: &uint64 123456
slice-str: &slice-str [foo, bar]
slice-str-null: &slice-str-null null
slice-int64: &slice-int64 [0, 1, 2]
slice-slice-str: &slice-slice-str [[foo], [], [bar, bazz]]
map-str-str: &map-str-str
  foo: bar
map-str-str-null: &map-str-str-null null
map-complex: &map-complex
  first_level:
    second_level:
      - - one
        - two
      - - three
        - four
alias-ptr-bool: *ptr-bool
alias-ptr-bool-null: *ptr-bool-null
alias-bool: *bool
alias-str: *str
alias-float32: *float32
alias-float64: *float64
alias-int8: *int8
alias-int16: *int16
alias-int32: *int32
alias-int64: *int64
alias-uint8: *uint8
alias-uint16: *uint16
alias-uint32: *uint32
alias-uint64: *uint64
alias-slice-str: *slice-str
alias-slice-str-null: *slice-str-null
alias-slice-int64: *slice-int64
alias-slice-slice-str: *slice-slice-str
alias-map-str-str: *map-str-str
alias-map-str-str-null: *map-str-str-null
alias-map-complex: *map-complex
`, &c)
	require.NoError(t, err)

	require.Equal(t, c.Bool, c.BoolAlias)
	require.Equal(t, c.PtrBool, c.PtrBoolAlias)
	require.Equal(t, c.PtrBoolNull, c.PtrBoolNullAlias)
	require.Equal(t, c.Str, c.StrAlias)
	require.Equal(t, c.Float32, c.Float32Alias)
	require.Equal(t, c.Float64, c.Float64Alias)
	require.Equal(t, c.Int8, c.Int8Alias)
	require.Equal(t, c.Int16, c.Int16Alias)
	require.Equal(t, c.Int32, c.Int32Alias)
	require.Equal(t, c.Int64, c.Int64Alias)
	require.Equal(t, c.Uint8, c.Uint8Alias)
	require.Equal(t, c.Uint16, c.Uint16Alias)
	require.Equal(t, c.Uint32, c.Uint32Alias)
	require.Equal(t, c.Uint64, c.Uint64Alias)
	require.Equal(t, c.SliceStr, c.SliceStrAlias)
	require.Equal(t, c.SliceInt64, c.SliceInt64Alias)
	require.Equal(t, c.SliceSliceStr, c.SliceSliceStrAlias)
	require.Equal(t, c.MapStrStr, c.MapStrStrAlias)
	require.Equal(t, c.MapComplex, c.MapComplexAlias)
}

func TestLoadErrInvalidUTF8(t *testing.T) {
	type TestConfig struct {
		Str string `yaml:"str"`
	}

	t.Run("control_char_in_string", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("str: abc\u0000defg")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMalformed)
		require.Equal(t,
			`malformed YAML: yaml: control characters are not allowed`,
			err.Error())
	})

	t.Run("control_char_in_key", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("s\u0000tr: abcdefg")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMalformed)
		require.Equal(t,
			`malformed YAML: yaml: control characters are not allowed`,
			err.Error())
	})

	t.Run("overlong_encoding_of_U+0000", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("\xc0\x80: not ok")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMalformed)
		require.Equal(t,
			`malformed YAML: yaml: invalid length of a UTF-8 sequence`,
			err.Error())
	})

	t.Run("surrogate_half_U+D800_U+DFFF", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("\xed\xa0\x80: not ok")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMalformed)
		require.Equal(t,
			`malformed YAML: yaml: invalid Unicode character`,
			err.Error())
	})

	t.Run("code_point_beyond_max_unicode_value", func(t *testing.T) {
		_, err := LoadSrc[TestConfig]("\xef\xbf\xff: not ok")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMalformed)
		require.Equal(t,
			`malformed YAML: yaml: invalid trailing UTF-8 octet`,
			err.Error())
	})
}

func TestLoadErrYAMLAnchorRedefined(t *testing.T) {
	type Container struct {
		Three string `yaml:"three"`
		Four  string `yaml:"four"`
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
  three: *x
  four: *x
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
  three: &x
  four: *x
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLAnchorRedefined)
		require.Equal(t, `at 5:10: redefined anchor "x" at 2:6: `+
			`yaml anchors must be unique throughout the whole document`, err.Error())
	})
}

func TestLoadErrYAMLAnchorUnused(t *testing.T) {
	type Container struct {
		Three string `yaml:"three"`
		Four  string `yaml:"four"`
	}
	type TestConfig struct {
		One       string    `yaml:"one"`
		Two       string    `yaml:"two"`
		Container Container `yaml:"container"`
	}
	t.Run("level_0", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`
one: &a ok
two: &b ok
container:
  three: *a
  four: *a
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLAnchorUnused)
		require.Equal(t, `at 3:6: anchor "b": `+
			`yaml anchors must be referenced at least once`, err.Error())
	})

	t.Run("level_1", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`
one: ok
two: ok
container:
  three: &x ""
  four: not ok
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLAnchorUnused)
		require.Equal(t, `at 5:10: anchor "x": `+
			`yaml anchors must be referenced at least once`, err.Error())
	})
}

func TestLoadErrYAMLAnchorNoValue(t *testing.T) {
	t.Run("ok_noerror", func(t *testing.T) {
		type Values struct {
			StrDoubleQuote string            `yaml:"str-double-quote"`
			StrSingleQuote string            `yaml:"str-single-quote"`
			PtrStr         *string           `yaml:"ptr-str"`
			Int64          int64             `yaml:"int64"`
			SliceStr       []string          `yaml:"slice-str"`
			ArraySliceStr  [2][]string       `yaml:"array-slice-str"`
			MapStrStr      map[string]string `yaml:"map-str-str"`
		}
		type TestConfig struct {
			Values  `yaml:",inline"`
			Aliases Values `yaml:"aliases"`
		}

		_, err := LoadSrc[TestConfig](`
str-double-quote: &str-double-quote ok
str-single-quote: &str-single-quote ok
ptr-str: &ptr-str ok
int64: &int64 42
slice-str: &slice-str [foo,bar]
array-slice-str: &array-slice-str [[foo,bar],[]]
map-str-str: &map-str-str
  foo: bar
aliases:
  str-double-quote: *str-double-quote
  str-single-quote: *str-single-quote
  ptr-str: *ptr-str
  int64: *int64
  slice-str: *slice-str
  array-slice-str: *array-slice-str
  map-str-str: *map-str-str
`)
		require.NoError(t, err)
	})

	src := `
anchor: &anchor
alias: *anchor
`
	checkErr := func(t *testing.T, err error) {
		require.ErrorIs(t, err, yamagiconf.ErrYAMLAnchorNoValue)
		require.Equal(t, `at 2:9: anchor "anchor": `+
			`don't use anchors with implicit null value`, err.Error())
	}

	t.Run("err_string", func(t *testing.T) {
		type TestConfig struct {
			Anchor string `yaml:"anchor"`
			Alias  string `yaml:"alias"`
		}
		_, err := LoadSrc[TestConfig](src)
		checkErr(t, err)
	})

	t.Run("err_pointer", func(t *testing.T) {
		type TestConfig struct {
			Anchor *string `yaml:"anchor"`
			Alias  *string `yaml:"alias"`
		}
		_, err := LoadSrc[TestConfig](src)
		checkErr(t, err)
	})

	t.Run("err_slice", func(t *testing.T) {
		type TestConfig struct {
			Anchor []string `yaml:"anchor"`
			Alias  []string `yaml:"alias"`
		}
		_, err := LoadSrc[TestConfig](src)
		checkErr(t, err)
	})

	t.Run("err_array", func(t *testing.T) {
		type TestConfig struct {
			Anchor [1]string `yaml:"anchor"`
			Alias  [1]string `yaml:"alias"`
		}
		_, err := LoadSrc[TestConfig](src)
		checkErr(t, err)
	})

	t.Run("err_map", func(t *testing.T) {
		type TestConfig struct {
			Anchor map[string]string `yaml:"anchor"`
			Alias  map[string]string `yaml:"alias"`
		}
		_, err := LoadSrc[TestConfig](src)
		checkErr(t, err)
	})
}

func TestLoadErrMissingYAMLTag(t *testing.T) {
	t.Run("level_0", func(t *testing.T) {
		type TestConfig struct {
			HasYAMLTag string `yaml:"has-yaml-tag"`
			NoYAMLTag  string
		}
		_, err := LoadSrc[TestConfig]("has-yaml-tag: 'OK'\nNoYAMLTag: 'NO'\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeMissingYAMLTag)
		require.Equal(t, "at TestConfig.NoYAMLTag: missing yaml struct tag", err.Error())
	})
	t.Run("level_1", func(t *testing.T) {
		type Foo struct{ NoYAMLTag string }
		type TestConfig struct {
			HasYAMLTag string `yaml:"has-yaml-tag"`
			Foo        Foo    `yaml:"foo"`
		}
		_, err := LoadSrc[TestConfig]("has-yaml-tag: 'OK'\nfoo:\n  NoYAMLTag: 'NO'\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeMissingYAMLTag)
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeMissingYAMLTag)
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeMissingYAMLTag)
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeInvalidEnvTag)
		require.Equal(t, "at TestConfig.Wrong: invalid env struct tag: "+
			"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("wrong_start", func(t *testing.T) {
		type TestConfig struct {
			Wrong string `yaml:"wrong" env:"1NOTOK"`
		}
		_, err := LoadSrc[TestConfig]("wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeInvalidEnvTag)
		require.Equal(t, "at TestConfig.Wrong: invalid env struct tag: "+
			"must match the POSIX env var regexp: ^[A-Z_][A-Z0-9_]*$", err.Error())
	})

	t.Run("illegal_char", func(t *testing.T) {
		type TestConfig struct {
			Wrong string `yaml:"wrong" env:"NOT-OK"`
		}
		_, err := LoadSrc[TestConfig]("wrong: ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeInvalidEnvTag)
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeInvalidEnvTag)
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeEnvVarOnUnsupportedType)
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Container: "+
				"env var on unsupported type: *yamagiconf_test.Container", err.Error())
	})

	t.Run("on_slice", func(t *testing.T) {
		type TestConfig struct {
			Wrong []string `yaml:"wrong" env:"WRONG"`
		}
		_, err := LoadSrc[TestConfig]("wrong:\n  - ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Wrong: "+
				"env var on unsupported type: []string", err.Error())
	})

	t.Run("on_yaml_unmarshaler", func(t *testing.T) {
		type TestConfig struct {
			Wrong YAMLUnmarshaler `yaml:"wrong" env:"WRONG"`
		}
		_, err := LoadSrc[TestConfig]("wrong:\n  - ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeEnvOnYAMLUnmarsh)
		require.Equal(t,
			"at TestConfig.Wrong: env var on yaml.Unmarshaler implementation: "+
				"yamagiconf_test.YAMLUnmarshaler", err.Error())
	})

	t.Run("on_ptr_yaml_unmarshaler", func(t *testing.T) {
		type TestConfig struct {
			Wrong *YAMLUnmarshaler `yaml:"wrong" env:"WRONG"`
		}
		_, err := LoadSrc[TestConfig]("wrong:\n  - ok\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeEnvOnYAMLUnmarsh)
		require.Equal(t,
			"at TestConfig.Wrong: env var on yaml.Unmarshaler implementation: "+
				"*yamagiconf_test.YAMLUnmarshaler", err.Error())
	})

	t.Run("on_noyaml_field", func(t *testing.T) {
		type TestConfig struct {
			X string `yaml:"x"` // This is here to avoid "no exported fields" error.
			Y int    `yaml:"-" env:"NOTOK"`
		}
		_, err := LoadSrc[TestConfig]("x: x\n")
		require.ErrorIs(t, err, yamagiconf.ErrTypeEnvVarOnUnsupportedType)
		require.Equal(t,
			"at TestConfig.Y: env var on unsupported type: int", err.Error())
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeRecursive)
		require.Equal(t, "at TestConfigRecurThroughContainerPtr.Container.Recurs: "+
			"recursive type", err.Error())
	})
	t.Run("ptr_through_container", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurPtrThroughContainer](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeRecursive)
		require.Equal(t, "at TestConfigRecurPtrThroughContainer.Container.Recurs: "+
			"recursive type", err.Error())
	})

	t.Run("through_slice", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurThroughSlice](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeRecursive)
		require.Equal(t, "at TestConfigRecurThroughSlice.Recurs: "+
			"recursive type", err.Error())
	})

	t.Run("ptr_through_slice", func(t *testing.T) {
		_, err := LoadSrc[TestConfigRecurPtrThroughSlice](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeRecursive)
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
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Int: unsupported type: int, "+
			"use integer type with specified width, "+
			"such as int8, int16, int32 or int64 instead of int", err.Error())
	})

	t.Run("uint", func(t *testing.T) {
		type TestConfig struct {
			Uint uint `yaml:"uint"`
		}

		_, err := LoadSrc[TestConfig](`uint: 42`)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Uint: unsupported type: uint, "+
			"use unsigned integer type with specified width, "+
			"such as uint8, uint16, uint32 or uint64 instead of uint", err.Error())
	})

	t.Run("ptr_ptr", func(t *testing.T) {
		type TestConfig struct {
			PtrPtr **int `yaml:"ptr-ptr"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupportedPtrType)
		require.Equal(t, "at TestConfig.PtrPtr: unsupported pointer type", err.Error())
	})

	t.Run("ptr_ptr_in_slice", func(t *testing.T) {
		type TestConfig struct {
			Slice []**int `yaml:"slice"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupportedPtrType)
		require.Equal(t, "at TestConfig.Slice: unsupported pointer type", err.Error())
	})

	t.Run("ptr_ptr_in_map", func(t *testing.T) {
		type TestConfig struct {
			Map map[string]**int `yaml:"map"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupportedPtrType)
		require.Equal(t,
			"at TestConfig.Map[value]: unsupported pointer type",
			err.Error())
	})

	t.Run("ptr_slice", func(t *testing.T) {
		type TestConfig struct {
			PtrSlice *[]string `yaml:"ptr-slice"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupportedPtrType)
		require.Equal(t, "at TestConfig.PtrSlice: unsupported pointer type", err.Error())
	})

	t.Run("ptr_map", func(t *testing.T) {
		type TestConfig struct {
			PtrMap *map[string]string `yaml:"ptr-map"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupportedPtrType)
		require.Equal(t, "at TestConfig.PtrMap: unsupported pointer type", err.Error())
	})

	t.Run("channel", func(t *testing.T) {
		type TestConfig struct {
			Chan chan int `yaml:"chan"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Chan: unsupported type: chan int", err.Error())
	})

	t.Run("func", func(t *testing.T) {
		type TestConfig struct {
			Func func() `yaml:"func"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Func: unsupported type: func()", err.Error())
	})

	t.Run("unsafe_pointer", func(t *testing.T) {
		type TestConfig struct {
			UnsafePointer unsafe.Pointer `yaml:"unsafe-pointer"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.UnsafePointer: "+
			"unsupported type: unsafe.Pointer", err.Error())
	})

	t.Run("interface", func(t *testing.T) {
		type TestConfig struct {
			Interface interface{ Write() ([]byte, int) } `yaml:"interface"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Interface: "+
			"unsupported type: interface { Write() ([]uint8, int) }", err.Error())
	})

	t.Run("empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig](yamlContents)
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Anything: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("slice_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything []any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  - foo\n  - bar")
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Anything: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("array_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything [2]any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  - foo\n  - bar")
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Anything: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("map_key_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything map[any]string `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  foo: bar")
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Anything[key]: "+
			"unsupported type: interface {}", err.Error())
	})

	t.Run("map_value_empty_interface", func(t *testing.T) {
		type TestConfig struct {
			Anything map[string]any `yaml:"anything"`
		}

		_, err := LoadSrc[TestConfig]("anything:\n  foo: bar")
		require.ErrorIs(t, err, yamagiconf.ErrTypeUnsupported)
		require.Equal(t, "at TestConfig.Anything[value]: "+
			"unsupported type: interface {}", err.Error())
	})
}

func TestValidateTypeErrIllegalRootType(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		err := yamagiconf.ValidateType[string]()
		require.ErrorIs(t, err, yamagiconf.ErrTypeIllegalRoot)
		require.Equal(t, fmt.Sprintf(
			"at string: %s", yamagiconf.ErrTypeIllegalRoot.Error(),
		), err.Error())

		require.Equal(t, err, yamagiconf.Validate(string("ok")))
	})

	t.Run("implements_yaml_unmarshaler", func(t *testing.T) {
		err := yamagiconf.ValidateType[YAMLUnmarshaler]()
		require.ErrorIs(t, err, yamagiconf.ErrTypeIllegalRoot)
		require.Equal(t, fmt.Sprintf(
			"at YAMLUnmarshaler: %s", yamagiconf.ErrTypeIllegalRoot.Error(),
		), err.Error())

		var i YAMLUnmarshaler
		require.Equal(t, err, yamagiconf.Validate(i))
	})

	t.Run("implements_text_unmarshaler", func(t *testing.T) {
		err := yamagiconf.ValidateType[TextUnmarshaler]()
		require.ErrorIs(t, err, yamagiconf.ErrTypeIllegalRoot)
		require.Equal(t, fmt.Sprintf(
			"at TextUnmarshaler: %s", yamagiconf.ErrTypeIllegalRoot.Error(),
		), err.Error())

		var i TextUnmarshaler
		require.Equal(t, err, yamagiconf.Validate(i))
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
		"at TestConfig.unexported: yaml struct tag on unexported field",
		err.Error())

	require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
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

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
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

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
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

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
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

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
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

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
	})
}

func TestValidateTypeErrYAMLInlineOpt(t *testing.T) {
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

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
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

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
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
		`a yaml struct tag must be unique`, err.Error())

	require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
}

func TestErrYAMLMergeKey(t *testing.T) {
	t.Run("in_struct", func(t *testing.T) {
		type Server struct {
			Host string `yaml:"host"`
			Port uint16 `yaml:"port"`
		}
		type TestConfig struct {
			ServerDefault  Server `yaml:"server-default"`
			ServerFallback Server `yaml:"server-fallback"`
		}
		var c TestConfig
		err := yamagiconf.Load[TestConfig](`
server-default: &default
  host: default.server
  port: 12345
server-fallback:
  port: 54321
  <<: *default
`, &c)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMergeKey)
		require.Equal(t, `at 7:3: `+yamagiconf.ErrYAMLMergeKey.Error(), err.Error())
	})

	t.Run("in_map", func(t *testing.T) {
		type TestConfig struct {
			Map1 map[string]string `yaml:"map1"`
			Map2 map[string]string `yaml:"map2"`
		}
		var c TestConfig
		err := yamagiconf.Load[TestConfig](`
map1: &map1
  foo: bar
  bazz: fazz
map2:
  <<: *map1
  mar: tar
`, &c)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMergeKey)
		require.Equal(t, `at 6:3: `+yamagiconf.ErrYAMLMergeKey.Error(), err.Error())
	})

	t.Run("map_multi_merge", func(t *testing.T) {
		type TestConfig struct {
			Map1 map[string]string `yaml:"map1"`
			Map2 map[string]string `yaml:"map2"`
			Map3 map[string]string `yaml:"map3"`
		}
		var c TestConfig
		err := yamagiconf.Load[TestConfig](`
map1: &map1
  foo: bar
map2: &map2
  bazz: fazz
map3:
  <<: [*map1,*map2]
`, &c)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMergeKey)
		require.Equal(t, `at 7:3: `+yamagiconf.ErrYAMLMergeKey.Error(), err.Error())
	})
}

func TestValidateTypeErrTypeEnvTagOnUnexported(t *testing.T) {
	type TestConfig struct {
		Ok string `yaml:"okay"`
		//lint:ignore U1000 ignore for testing purposes
		unexported string `env:"ok"`
	}
	err := yamagiconf.ValidateType[TestConfig]()
	require.ErrorIs(t, err, yamagiconf.ErrTypeEnvTagOnUnexported)
	require.Equal(t,
		"at TestConfig.unexported: env tag on unexported field",
		err.Error())

	require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
}

func TestValidateTypeErrNoExportedFields(t *testing.T) {
	type TestConfig struct {
		//lint:ignore U1000 ignore for testing purposes
		unexported1 string
		//lint:ignore U1000 ignore for testing purposes
		unexported2 string

		ExportedButIgnored string `yaml:"-"`
	}
	err := yamagiconf.ValidateType[TestConfig]()
	require.ErrorIs(t, err, yamagiconf.ErrTypeNoExportedFields)
	require.Equal(t, "at TestConfig: no exported fields", err.Error())

	require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
}

func TestValidateTypeErrTypeTagOnInterfaceImpl(t *testing.T) {
	t.Run("NoopTextUnmarshalerWithYAMLTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopTextUnmarshalerWithYAMLTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTypeTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements encoding.TextUnmarshaler `+
			`but field contains tag "yaml" ("illegal"): `+
			yamagiconf.ErrTypeTagOnInterfaceImpl.Error(), err.Error())

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
	})

	t.Run("NoopYAMLUnmarshalerWithYAMLTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopYAMLUnmarshalerWithYAMLTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTypeTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements yaml.Unmarshaler `+
			`but field contains tag "yaml" ("illegal"): `+
			yamagiconf.ErrTypeTagOnInterfaceImpl.Error(), err.Error())

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
	})

	t.Run("NoopTextUnmarshalerWithEnvTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopTextUnmarshalerWithEnvTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTypeTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements encoding.TextUnmarshaler `+
			`but field contains tag "env" ("illegal"): `+
			yamagiconf.ErrTypeTagOnInterfaceImpl.Error(), err.Error())

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
	})

	t.Run("NoopYAMLUnmarshalerWithEnvTag", func(t *testing.T) {
		type TestConfig struct {
			X NoopYAMLUnmarshalerWithEnvTag `yaml:"x"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.ErrorIs(t, err, yamagiconf.ErrTypeTagOnInterfaceImpl)
		require.Equal(t, `at TestConfig.X: struct implements yaml.Unmarshaler `+
			`but field contains tag "env" ("illegal"): `+
			yamagiconf.ErrTypeTagOnInterfaceImpl.Error(), err.Error())

		require.Equal(t, err, yamagiconf.Validate(TestConfig{}))
	})

	t.Run("NoopTextUnmarshalerWithYAMLTag/ignored", func(t *testing.T) {
		// Make sure NoopTextUnmarshalerWithYAMLTag doesn't cause an error
		// because it's ignored using the yaml:"-" struct tag.
		type TestConfig struct {
			X NoopTextUnmarshalerWithYAMLTag `yaml:"-"`
			Y string                         `yaml:"y"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.NoError(t, err)
	})

	t.Run("NoopYAMLUnmarshalerWithYAMLTag/ignored", func(t *testing.T) {
		// Make sure NoopYAMLUnmarshalerWithYAMLTag_ignored doesn't cause an error
		// because it's ignored using the yaml:"-" struct tag.
		type TestConfig struct {
			X NoopYAMLUnmarshalerWithYAMLTag `yaml:"-"`
			Y string                         `yaml:"y"`
		}
		err := yamagiconf.ValidateType[TestConfig]()
		require.NoError(t, err)
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
	require.ErrorIs(t, err, yamagiconf.ErrTypeMissingYAMLTag)
	require.Equal(t,
		"at struct{...}.MissingYAMLTag: missing yaml struct tag",
		err.Error())

	require.Equal(t, err, yamagiconf.Validate(c))
}

func TestLoadErrMissingConfig(t *testing.T) {
	type TestConfig struct {
		OK      string `yaml:"ok,omitempty"`
		Missing string `yaml:"missing,omitempty"`
	}
	_, err := LoadSrc[TestConfig]("ok: 'OK'")
	require.ErrorIs(t, err, yamagiconf.ErrYAMLMissingConfig)
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
		require.ErrorIs(t, err, yamagiconf.ErrYAMLNullOnNonPointer)
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
		require.ErrorIs(t, err, yamagiconf.ErrYAMLNullOnNonPointer)
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

func TestLoadErrYAMLEmptyArrayItem(t *testing.T) {
	t.Run("in_slice", func(t *testing.T) {
		type TestConfig struct {
			Slice []string `yaml:"slice"`
		}
		_, err := LoadSrc[TestConfig](`
slice:
  - ''
  - ""
  - ok1
  - # this is wrong
  - ok2
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLEmptyArrayItem)
		require.Equal(t,
			`at 6:4: "slice" (TestConfig.Slice): avoid empty items in arrays as those `+
				`will not be appended to the target Go slice`,
			err.Error())
	})

	t.Run("in_array2", func(t *testing.T) {
		type TestConfig struct {
			Array2 [2]string `yaml:"array2"`
		}
		_, err := LoadSrc[TestConfig]("array2:\n  - ''\n  -\n")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLEmptyArrayItem)
		require.Equal(t,
			`at 3:4: "array2" (TestConfig.Array2): avoid empty items in arrays as those `+
				`will not be appended to the target Go slice`,
			err.Error())
	})
}

func TestLoadErrNilConfig(t *testing.T) {
	type TestConfig struct {
		Foo int8 `yaml:"foo"`
	}
	err := yamagiconf.Load[TestConfig]("non-existing.yaml", nil)
	require.ErrorIs(t, err, yamagiconf.ErrConfigNil)
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
		require.ErrorIs(t, err, yamagiconf.ErrYAMLEmptyFile)
		require.Zero(t, c)
	})

	t.Run("nil_config", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "test-config.yaml")
		_, err := os.Create(p)
		require.NoError(t, err)
		err = yamagiconf.LoadFile[TestConfig](p, nil)
		require.ErrorIs(t, err, yamagiconf.ErrConfigNil)
		require.Equal(t, "cannot load into nil config", err.Error())
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		// Using tabs is illegal
		c, err := LoadSrc[TestConfig]("x:\n\ttabs: 'TABS'\n  spaces: 'SPACES'\n")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMalformed)
		require.True(t, strings.HasPrefix(err.Error(), "malformed YAML: yaml: line 2:"))
		require.NotZero(t, c)
	})

	t.Run("multidocument", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`# Second doc contains different data.
---
foo: 1
---
foo: 2
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMultidoc)
		require.Equal(t, "at 4:1: "+yamagiconf.ErrYAMLMultidoc.Error(), err.Error())
	})

	t.Run("multidocument_second_doc_not_to_spec", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`# Second doc can't be decoded to TestConfig.
---
foo: 1
---
bar: 2
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMultidoc)
		require.Equal(t, "at 4:1: "+yamagiconf.ErrYAMLMultidoc.Error(), err.Error())
	})

	t.Run("multidocument_error_in_second_file", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`# Second doc contains a syntax error.
---
foo: 1
---
:
`)
		require.ErrorIs(t, err, yamagiconf.ErrYAMLMultidoc)
		require.Equal(t, yamagiconf.ErrYAMLMultidoc.Error()+
			": yaml: line 4: did not find expected key", err.Error())
	})

	t.Run("unsupported_boolean_literal", func(t *testing.T) {
		type TestConfig struct {
			Boolean bool `yaml:"boolean"`
		}
		_, err := LoadSrc[TestConfig]("boolean: yes")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadBoolLiteral)
		require.Equal(t, `at 1:10: "boolean" (TestConfig.Boolean): `+
			yamagiconf.ErrYAMLBadBoolLiteral.Error(), err.Error())
	})

	t.Run("unsupported_boolean_literal_in_array", func(t *testing.T) {
		type TestConfig struct {
			Booleans []bool `yaml:"booleans"`
		}
		_, err := LoadSrc[TestConfig]("booleans:\n  - false\n  - no")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadBoolLiteral)
		require.Equal(t, `at 3:5: "booleans" (TestConfig.Booleans[1]): `+
			yamagiconf.ErrYAMLBadBoolLiteral.Error(), err.Error())
	})

	t.Run("unsupported_null_literal_tilde", func(t *testing.T) {
		type TestConfig struct {
			Nullable *bool `yaml:"nullable"`
		}
		_, err := LoadSrc[TestConfig]("\nnullable: ~")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadNullLiteral)
		require.Equal(t, `at 2:11: "nullable" (TestConfig.Nullable): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_tilde_in_map", func(t *testing.T) {
		type TestConfig struct {
			Map map[*string]string `yaml:"map"`
		}
		_, err := LoadSrc[TestConfig]("map:\n  ~: string")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadNullLiteral)
		require.Equal(t, `at 2:3: "map" (TestConfig.Map["~"]): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_tilde_in_map_value", func(t *testing.T) {
		type TestConfig struct {
			Map map[string]*string `yaml:"map"`
		}
		_, err := LoadSrc[TestConfig]("map:\n  notok: ~")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadNullLiteral)
		require.Equal(t, `at 2:10: "map" (TestConfig.Map["notok"]): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_tilde_in_slice", func(t *testing.T) {
		type TestConfig struct {
			Slice []*string `yaml:"slice"`
		}
		_, err := LoadSrc[TestConfig]("slice:\n  - ~")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadNullLiteral)
		require.Equal(t, `at 2:5: "slice" (TestConfig.Slice[0]): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_title", func(t *testing.T) {
		type TestConfig struct {
			Nullable *bool `yaml:"nullable"`
		}
		_, err := LoadSrc[TestConfig]("\nnullable: Null")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadNullLiteral)
		require.Equal(t, `at 2:11: "nullable" (TestConfig.Nullable): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})

	t.Run("unsupported_null_literal_uppercase", func(t *testing.T) {
		type TestConfig struct {
			Nullable *bool `yaml:"nullable"`
		}
		_, err := LoadSrc[TestConfig]("\nnullable: NULL")
		require.ErrorIs(t, err, yamagiconf.ErrYAMLBadNullLiteral)
		require.Equal(t, `at 2:11: "nullable" (TestConfig.Nullable): `+
			`must be null, any other variants of null are not supported`, err.Error())
	})
}

func TestValidation(t *testing.T) {
	type MapValVal map[ValidatedString]ValidatedString
	type Container struct {
		// Tag `validate:"required"` requires those fields to be non-zero.
		Str       string `yaml:"required-str" validate:"required"`
		NoYAMLStr string `yaml:"-" env:"NOYAML_STR" validate:"required"`

		Slice          []ValidatedString  `yaml:"slice"`
		SlicePtr       []*ValidatedString `yaml:"slice-ptr"`
		ArrayMapValVal [1]MapValVal       `yaml:"array-map-val-val"`
	}
	type TestConfig struct {
		Container Container `yaml:"container"`
	}

	t.Run("required_ok", func(t *testing.T) {
		t.Setenv("NOYAML_STR", "noyaml_ok")
		c, err := LoadSrc[TestConfig](`
container:
  required-str: ok
  slice:
    - valid
    - valid
  slice-ptr:
    - valid
    - valid
  array-map-val-val:
    - null
`)
		valid := ValidatedString("valid")
		require.NoError(t, err)
		require.Equal(t, TestConfig{Container: Container{
			Str:       "ok",
			Slice:     []ValidatedString{"valid", "valid"},
			SlicePtr:  []*ValidatedString{&valid, &valid},
			NoYAMLStr: "noyaml_ok",
		}}, *c)
		require.NoError(t, yamagiconf.Validate(*c))
	})

	t.Run("required_tag_error", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`
container:
  required-str: ''
  slice:
    - valid
    - valid
  slice-ptr:
    - valid
    - valid
  array-map-val-val:
    - null
`)
		require.ErrorIs(t, err, yamagiconf.ErrValidateTagViolation)
		require.Equal(t,
			`at 3:17: "required-str" violates validation rule: "required"`,
			err.Error())
		validateErr := yamagiconf.Validate(
			TestConfig{Container: Container{Str: ""}},
		)
		require.NoError(t, CompareErrMsgWithPrefix(err, validateErr,
			`at 3:17: "required-str"`, "at TestConfig.Container.Str:"))
	})

	t.Run("validate_err_in_slice", func(t *testing.T) {
		t.Setenv("NOYAML_STR", "noyaml_ok")
		c, err := LoadSrc[TestConfig](`
container:
  required-str: ok
  slice:
    - valid
    - invalid
  slice-ptr:
    - valid
    - valid
  array-map-val-val:
    - null
`)
		require.ErrorIs(t, err, yamagiconf.ErrValidation)
		require.Equal(t,
			`at 6:7: at TestConfig.Container.Slice[1]: `+
				`validation: is not 'valid'`,
			err.Error())
		validateErr := yamagiconf.Validate(*c)
		require.NoError(t, CompareErrMsgWithPrefix(err, validateErr,
			`at 6:7: at TestConfig.Container.Slice[1]:`,
			`at TestConfig.Container.Slice[1]:`))
	})

	t.Run("validate_err_in_slice_ptr", func(t *testing.T) {
		t.Setenv("NOYAML_STR", "noyaml_ok")
		c, err := LoadSrc[TestConfig](`
container:
  required-str: ok
  slice:
    - valid
    - valid
  slice-ptr:
    - valid
    - invalid
  array-map-val-val:
    - null
`)
		require.ErrorIs(t, err, yamagiconf.ErrValidation)
		require.Equal(t,
			`at 9:7: at TestConfig.Container.SlicePtr[1]: `+
				`validation: is not 'valid'`,
			err.Error())
		validateErr := yamagiconf.Validate(*c)
		require.NoError(t, CompareErrMsgWithPrefix(err, validateErr,
			`at 9:7: at TestConfig.Container.SlicePtr[1]:`,
			`at TestConfig.Container.SlicePtr[1]:`))
	})

	t.Run("validate_err_in_map_key", func(t *testing.T) {
		t.Setenv("NOYAML_STR", "noyaml_ok")
		c, err := LoadSrc[TestConfig](`
container:
  required-str: ok
  slice:
    - valid
    - valid
  slice-ptr:
    - valid
    - valid
  array-map-val-val:
    - valid: valid
      invalid: valid
`)
		require.ErrorIs(t, err, yamagiconf.ErrValidation)
		require.Equal(t,
			`at 12:7: at TestConfig.Container.ArrayMapValVal[0]: `+
				`validation: is not 'valid'`,
			err.Error())
		validateErr := yamagiconf.Validate(*c)
		require.NoError(t, CompareErrMsgWithPrefix(err, validateErr,
			`at 12:7: at TestConfig.Container.ArrayMapValVal[0]:`,
			`at TestConfig.Container.ArrayMapValVal[0]:`))
	})

	t.Run("validate_err_in_map_val", func(t *testing.T) {
		t.Setenv("NOYAML_STR", "noyaml_ok")
		c, err := LoadSrc[TestConfig](`
container:
  required-str: ok
  slice:
    - valid
    - valid
  slice-ptr:
    - valid
    - valid
  array-map-val-val:
    - valid: invalid
`)
		require.ErrorIs(t, err, yamagiconf.ErrValidation)
		require.Equal(t,
			`at 11:14: at TestConfig.Container.ArrayMapValVal[0][valid]: `+
				`validation: is not 'valid'`,
			err.Error())
		validateErr := yamagiconf.Validate(*c)
		require.NoError(t, CompareErrMsgWithPrefix(err, validateErr,
			`at 11:14: at TestConfig.Container.ArrayMapValVal[0][valid]:`,
			`at TestConfig.Container.ArrayMapValVal[0][valid]:`))
	})

	t.Run("required_env_error", func(t *testing.T) {
		_, err := LoadSrc[TestConfig](`
container:
  required-str: 'ok'
  slice:
    - valid
    - valid
  slice-ptr:
    - valid
    - valid
  array-map-val-val:
    - null
`)
		require.ErrorIs(t, err, yamagiconf.ErrValidateTagViolation)
		require.Equal(t,
			`at TestConfig.Container.NoYAMLStr: violates validation rule: "required"`,
			err.Error())
		validateErr := yamagiconf.Validate(
			TestConfig{Container: Container{Str: ""}},
		)
		require.NoError(t, CompareErrMsgWithPrefix(err, validateErr,
			`at TestConfig.Container.NoYAMLStr:`, "at TestConfig.Container.Str:"))
	})
}

type TestConfWithValid struct {
	Foo       string          `yaml:"foo" validate:"required"`
	Bar       string          `yaml:"bar"`
	Container ContainerStruct `yaml:"container"`

	NoYAMLStr string `yaml:"-" env:"NOYAML_STR" validate:"required"`
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
		t.Setenv("NOYAML_STR", "noyaml_text")
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
			NoYAMLStr: "noyaml_text",
		}, *c)
	})

	t.Run("ok_null", func(t *testing.T) {
		t.Setenv("NOYAML_STR", "null")
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
			NoYAMLStr: "null",
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
		require.Equal(t, "at 2:1: at TestConfWithValid: validation: "+
			"foo must not be empty", errMsg)
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
		require.Equal(t, "at 5:21: at TestConfWithValid.Container.ValidatedString: "+
			"validation: is not 'valid'", errMsg)
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
		require.Equal(t, "at 11:7: at TestConfWithValid.Container.Slice[1]: "+
			"validation: is not 'valid'", errMsg)
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
		require.Equal(t, "at 14:5: at TestConfWithValid.Container.Map: "+
			"validation: is not 'valid'", errMsg)
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
		require.Equal(t, "at 13:12: at TestConfWithValid.Container.Map[valid]: "+
			"validation: is not 'valid'", errMsg)
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

		NoYAMLStr  string `yaml:"-" env:"NOYAML_STR"`
		NoYAMLStr2 string `yaml:"-" env:"NOYAML_STR"`

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

	t.Setenv("NOYAML_STR", "test_noyaml")

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

	require.Equal(t, c.NoYAMLStr, "test_noyaml")
	require.Equal(t, c.NoYAMLStr2, "test_noyaml")
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

		NoYAMLStr string `yaml:"-" env:"NOYAML_STR"`
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

	require.Zero(t, c.NoYAMLStr)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
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
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
		require.Equal(t, "at TestConfig.PtrUint64: invalid env var PTR_UINT_64: "+
			"expected uint64: "+
			"strconv.ParseUint: parsing \"-1\": invalid syntax", err.Error())
	})

	t.Run("bool_noyaml", func(t *testing.T) {
		type TestConfig struct {
			X string `yaml:"x"` // This is here to avoid "no exported fields" error.

			Bool bool `yaml:"-" env:"BOOL"`
		}
		t.Setenv("BOOL", "no")
		_, err := LoadSrc[TestConfig](`x: x`)
		require.ErrorIs(t, err, yamagiconf.ErrEnvInvalidVar)
		require.Equal(t,
			"at TestConfig.Bool: invalid env var BOOL: expected bool",
			err.Error())
	})
}

type (
	TextUnmarshaler        struct{ Str string }
	TextUnmarshalerCopyRcv struct{ Str *string }
	YAMLUnmarshaler        string
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
	*u = YAMLUnmarshaler(n.Value)
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

// CompareErrMsgWithPrefix compares the suffixes of error messages of a and b,
// assuming that a has prefix aPrefix and b has prefix bPrefix.
func CompareErrMsgWithPrefix(a, b error, aPrefix, bPrefix string) error {
	if a == nil || b == nil {
		return fmt.Errorf("both a (nil? %t) and b (nil? %t) must not be nil",
			a == nil, b == nil)
	}
	aMsg, bMsg := a.Error(), b.Error()
	if !strings.HasPrefix(aMsg, aPrefix) {
		return fmt.Errorf("a (%v) doesn't have prefix %q", a, aPrefix)
	}
	if !strings.HasPrefix(bMsg, bPrefix) {
		return fmt.Errorf("b (%v) doesn't have prefix %q", b, bPrefix)
	}
	aSuffix, bSuffix := aMsg[len(aPrefix):], bMsg[len(bPrefix):]
	if aSuffix != bSuffix {
		return fmt.Errorf("suffixes don't match (a: %q, b: %q)", aSuffix, bSuffix)
	}
	return nil
}

func TestCompareErrMsgWithPrefix(t *testing.T) {
	a, b := errors.New("foo bar bazz"), errors.New("fazz bar bazz")
	t.Run("ok", func(t *testing.T) {
		require.NoError(t, CompareErrMsgWithPrefix(a, b, "foo", "fazz"))
	})
	t.Run("a_is_nil", func(t *testing.T) {
		err := CompareErrMsgWithPrefix(nil, b, "foo", "fazz")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "both a "))
	})
	t.Run("b_is_nil", func(t *testing.T) {
		err := CompareErrMsgWithPrefix(a, nil, "foo", "fazz")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "both a "))
	})
	t.Run("a_prefix_mismatch", func(t *testing.T) {
		err := CompareErrMsgWithPrefix(a, b, "X", "fazz")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "a ("))
	})
	t.Run("b_prefix_mismatch", func(t *testing.T) {
		err := CompareErrMsgWithPrefix(a, b, "foo", "X")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "b ("))
	})
	t.Run("suffix_mismatch", func(t *testing.T) {
		a, b := errors.New("foo bar bazz"), errors.New("fazz bar baXX")
		err := CompareErrMsgWithPrefix(a, b, "foo", "fazz")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "suffixes don't match"))
	})
}

type ValidateWithPointerReceiver struct {
	err           error
	ExportedField bool `yaml:"exported-field"`
}

func (p *ValidateWithPointerReceiver) Validate() error { return p.err }

func TestValidatorPointerReceiver(t *testing.T) {
	ErrValidateWithPointerReceiver := errors.New("this is fine")
	v := ValidateWithPointerReceiver{err: ErrValidateWithPointerReceiver}
	err := yamagiconf.Validate(v)
	require.ErrorIs(t, err, v.err)
}

type (
	IntImplsUnmarshalers       int
	IntImplsYAMLUnmarshaler    int
	AnySlcImplsTextUnmarshaler []**any
	AnySlcImplsYAMLUnmarshaler []**any
)

func (u *IntImplsUnmarshalers) UnmarshalText(t []byte) error          { return nil }
func (u AnySlcImplsTextUnmarshaler) UnmarshalText(t []byte) error     { return nil }
func (u *IntImplsYAMLUnmarshaler) UnmarshalYAML(n *yaml.Node) error   { return nil }
func (u AnySlcImplsYAMLUnmarshaler) UnmarshalYAML(n *yaml.Node) error { return nil }

var (
	_ encoding.TextUnmarshaler = new(IntImplsUnmarshalers)
	_ yaml.Unmarshaler         = new(IntImplsYAMLUnmarshaler)
	_ encoding.TextUnmarshaler = new(AnySlcImplsTextUnmarshaler)
	_ yaml.Unmarshaler         = new(AnySlcImplsYAMLUnmarshaler)
)

// TestForbiddenTypeImplementsUnmarshaler tests whether a named type derived
// from int is accepted when it implements the unmarshaler interfaces,
// because normally types like `int` and `any` are a forbidden data type.
func TestForbiddenTypeImplementsUnmarshaler(t *testing.T) {
	t.Run("IntImplsUnmarshalers", func(t *testing.T) {
		require.NoError(t, yamagiconf.ValidateType[struct {
			Un IntImplsUnmarshalers `yaml:"un"`
		}]())
	})

	t.Run("IntImplsYAMLUnmarshaler", func(t *testing.T) {
		require.NoError(t, yamagiconf.ValidateType[struct {
			Un IntImplsYAMLUnmarshaler `yaml:"un"`
		}]())
	})

	t.Run("AnySlcImplsTextUnmarshaler", func(t *testing.T) {
		require.NoError(t, yamagiconf.ValidateType[struct {
			Un AnySlcImplsTextUnmarshaler `yaml:"un"`
		}]())
	})

	t.Run("AnySlcImplsYAMLUnmarshaler", func(t *testing.T) {
		require.NoError(t, yamagiconf.ValidateType[struct {
			Un AnySlcImplsYAMLUnmarshaler `yaml:"un"`
		}]())
	})
}

type TextUnmarshalerMap map[ValidatedString]ValidatedString

var _ encoding.TextUnmarshaler = new(TextUnmarshalerMap)

func (m *TextUnmarshalerMap) UnmarshalText(t []byte) error {
	*m = TextUnmarshalerMap{ValidatedString(t): ValidatedString(t)}
	return nil
}

func TestTextUnmarshalerMap(t *testing.T) {
	var v struct {
		Map TextUnmarshalerMap `yaml:"map"`
	}
	require.NoError(t, yamagiconf.Load("map: valid", &v))
	require.Equal(t, TextUnmarshalerMap{"valid": "valid"}, v.Map)
}

func TestErrYAMLNonStrOnTextUnmarshMap(t *testing.T) {
	var v struct {
		Map TextUnmarshalerMap `yaml:"map"`
	}
	err := yamagiconf.Load("map:\n  foo: bar", &v)
	require.ErrorIs(t, err, yamagiconf.ErrYAMLNonStrOnTextUnmarsh)
}

type TextUnmarshalerSlice []ValidatedString

var _ encoding.TextUnmarshaler = new(TextUnmarshalerSlice)

func (m *TextUnmarshalerSlice) UnmarshalText(t []byte) error {
	*m = TextUnmarshalerSlice{ValidatedString(t)}
	return nil
}

func TestTextUnmarshalerSlice(t *testing.T) {
	var v struct {
		Slice TextUnmarshalerSlice `yaml:"slice"`
	}
	require.NoError(t, yamagiconf.Load("slice: valid", &v))
	require.Equal(t, TextUnmarshalerSlice{"valid"}, v.Slice)
}

func TestErrYAMLNonStrOnTextUnmarshSlice(t *testing.T) {
	var v struct {
		Slice TextUnmarshalerSlice `yaml:"slice"`
	}
	err := yamagiconf.Load("slice:\n  - valid\n  - valid", &v)
	require.ErrorIs(t, err, yamagiconf.ErrYAMLNonStrOnTextUnmarsh)
}

type TextUnmarshalerArray2 [2]ValidatedString

var _ encoding.TextUnmarshaler = new(TextUnmarshalerArray2)

func (m *TextUnmarshalerArray2) UnmarshalText(t []byte) error {
	*m = TextUnmarshalerArray2{ValidatedString(t), ValidatedString(t)}
	return nil
}

func TestTextUnmarshalerArray2(t *testing.T) {
	var v struct {
		Array2 TextUnmarshalerArray2 `yaml:"array"`
	}
	require.NoError(t, yamagiconf.Load("array: valid", &v))
	require.Equal(t, TextUnmarshalerArray2{"valid", "valid"}, v.Array2)
}

func TestErrYAMLNonStrOnTextUnmarshArray(t *testing.T) {
	var v struct {
		Slice TextUnmarshalerArray2 `yaml:"array"`
	}
	err := yamagiconf.Load("array:\n  - valid\n  - valid", &v)
	require.ErrorIs(t, err, yamagiconf.ErrYAMLNonStrOnTextUnmarsh)
}

func TestErrYAMLNonStrOnTextUnmarshArrayAlias(t *testing.T) {
	var v struct {
		Anchor [2]string             `yaml:"anchor"`
		Slice  TextUnmarshalerArray2 `yaml:"alias"`
	}
	err := yamagiconf.Load("anchor: &a\n  - valid\n  - valid\nalias: *a", &v)
	require.ErrorIs(t, err, yamagiconf.ErrYAMLNonStrOnTextUnmarsh)
}

// TestZeroValue tests whether no value in YAML results in zero Go value.
func TestZeroValue(t *testing.T) {
	type NoValue struct {
		Int8            int8              `yaml:"int8"`
		Int16           int16             `yaml:"int16"`
		Int32           int32             `yaml:"int32"`
		Int64           int64             `yaml:"int64"`
		Uint8           uint8             `yaml:"uint8"`
		Uint16          uint16            `yaml:"uint16"`
		Uint32          uint32            `yaml:"uint32"`
		Uint64          uint64            `yaml:"uint64"`
		Float32         float32           `yaml:"float32"`
		Float64         float64           `yaml:"float64"`
		String          string            `yaml:"string"`
		Bool            bool              `yaml:"bool"`
		ArrayZero       []bool            `yaml:"array-zero"`
		Array2Zero      [2]bool           `yaml:"array2-zero"`
		MapZero         map[string]string `yaml:"map-zero"`
		TextUnmarshaler TextUnmarshaler   `yaml:"text-unmarshaler"`
		YAMLUnmarshaler YAMLUnmarshaler   `yaml:"yaml-unmarshaler"`

		PtrInt8            *int8            `yaml:"ptr-int8"`
		PtrInt16           *int16           `yaml:"ptr-int16"`
		PtrInt32           *int32           `yaml:"ptr-int32"`
		PtrInt64           *int64           `yaml:"ptr-int64"`
		PtrUint8           *uint8           `yaml:"ptr-uint8"`
		PtrUint16          *uint16          `yaml:"ptr-uint16"`
		PtrUint32          *uint32          `yaml:"ptr-uint32"`
		PtrUint64          *uint64          `yaml:"ptr-uint64"`
		PtrFloat32         *float32         `yaml:"ptr-float32"`
		PtrFloat64         *float64         `yaml:"ptr-float64"`
		PtrString          *string          `yaml:"ptr-string"`
		PtrBool            *bool            `yaml:"ptr-bool"`
		PtrArray2Zero      *[2]bool         `yaml:"ptr-array2-zero"`
		PtrTextUnmarshaler *TextUnmarshaler `yaml:"ptr-text-unmarshaler"`
		PtrYAMLUnmarshaler *YAMLUnmarshaler `yaml:"ptr-yaml-unmarshaler"`
	}
	type Container struct {
		String string `yaml:"string"`
	}
	type TestConfig struct {
		NoValue    `yaml:",inline"`
		MapStrStr  map[string]string `yaml:"map-str-str"`
		MapStrBool map[string]bool   `yaml:"map-str-bool"`
		Container  Container         `yaml:"container"`
	}

	var c TestConfig
	err := yamagiconf.Load(`# test YAML file
int8:
int16:
int32:
int64:
uint8:
uint16:
uint32:
uint64:
float32:
float64:
string:
bool:
array-zero:
array2-zero:
map-zero:
text-unmarshaler:
yaml-unmarshaler:
ptr-int8:
ptr-int16:
ptr-int32:
ptr-int64:
ptr-uint8:
ptr-uint16:
ptr-uint32:
ptr-uint64:
ptr-float32:
ptr-float64:
ptr-string:
ptr-bool:
ptr-array2-zero:
ptr-text-unmarshaler:
ptr-yaml-unmarshaler:

# non-zero with zero fields
map-str-str:
  key:
map-str-bool:
  key:
container:
  string:
`, &c)
	require.NoError(t, err)
	require.Zero(t, c.NoValue)

	require.Len(t, c.MapStrStr, 1)
	require.Contains(t, c.MapStrStr, "key")
	require.Zero(t, c.MapStrStr["key"])

	require.Len(t, c.MapStrBool, 1)
	require.Contains(t, c.MapStrBool, "key")
	require.Zero(t, c.MapStrBool["key"])

	require.Zero(t, c.Container)
}
