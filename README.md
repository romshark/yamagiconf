![yamagiconf](https://github.com/romshark/yamagiconf/assets/9574743/9d4f5b77-a461-47b2-8f6f-65194755b4f1)

<a href="https://pkg.go.dev/github.com/romshark/yamagiconf/v2">
    <img src="https://godoc.org/github.com/romshark/yamagiconf/v2?status.svg" alt="GoDoc">
</a>
<a href="https://goreportcard.com/report/github.com/romshark/yamagiconf/v2">
    <img src="https://goreportcard.com/badge/github.com/romshark/yamagiconf/v2" alt="GoReportCard">
</a>
<a href='https://coveralls.io/github/romshark/yamagiconf?branch=main'>
    <img src='https://coveralls.io/repos/github/romshark/yamagiconf/badge.svg?branch=main' alt='Coverage Status' />
</a>

# yamagiconf

The heavily opinionated **YA**ML **Magi**c **Conf**iguration framework for Go
keeps your configs simple and consistent
by being *more restrictive than your regular YAML parser* 🚷 allowing only a subset of YAML
and enforcing some restrictions to the target Go type.

If you hate [YAML](https://yaml.org/), and you're afraid of
[YAML documents from hell](https://ruudvanasseldonk.com/2023/01/11/the-yaml-document-from-hell),
and you can't stand complex, unexplorable and unintuitive configurations then yamagiconf is for you!

🪄 It's magic because it uses [reflect](https://pkg.go.dev/reflect) to find recursively all
values of types that implement `interface { Validate() error }` and calls them reporting
an error annotated with line and column in the YAML file if necessary.

## (anti-)Features

- Go restrictions:
	- 🚫 Forbids recursive Go types.
	- 🚫 Forbids the use of `any`, `int` & `uint` (unspecified width), and other types.
	Only maps, slices, arrays and deterministic primitives are allowed.
	- ❗️ Requires `yaml` struct tags on all exported fields.
	- ❗️ Requires `env` struct tags to be POSIX-style.
	- 🚫 Forbids the use of `env` struct tag on non-primitive fields.
	Allows only floats, ints, strings, bool and types that implement the
	[`encoding.TextUnmarshaler`](https://pkg.go.dev/encoding#TextUnmarshaler) interface.
	- 🚫 Forbids the use of `env` on primitive fields implementing
	the [`yaml.Unmarshaler`](https://pkg.go.dev/go.yaml.in/yaml/v4#Unmarshaler) interface.
	- 🚫 Forbids the use of `yaml` and `env` struct tags within implementations of
	[`encoding.TextUnmarshaler`](https://pkg.go.dev/encoding#TextUnmarshaler) and/or
	[`yaml.Unmarshaler`](https://pkg.go.dev/go.yaml.in/yaml/v4#Unmarshaler).
	- 🚫 Forbids the use of YAML struct tag option `"inline"` for non-embedded structs and
	requires embedded structs to use option `"inline"`.
- YAML restrictions:
	- 🚫 Forbids the use of `no`, `yes`, `on` and `off` for `bool`,
	allows only `true` and `false`.
	- 🚫 Forbids the use of `~`, `Null` and other variations, allows only `null` for nilables.
	- 🚫 Forbids assigning `null` to non-nilables (which normally would assign zero value).
	- 🚫 Forbids fields in the YAML file that aren't specified by the Go type.
	- 🚫 Forbids the use of [YAML tags](https://yaml.org/spec/1.2.2/#3212-tags).
	- 🚫 Forbids redeclaration of anchors.
	- 🚫 Forbids unused anchors.
	- 🚫 Forbids anchors with implicit `null` value (no value) like `foo: &bar`.
	- ❗️ Requires fields specified in the configuration type to be present in the YAML file.
	- 🚫 Forbids assigning non-string values to Go types that implement
	the [`encoding.TextUnmarshaler`](https://pkg.go.dev/encoding#TextUnmarshaler) interface.
	- 🚫 Forbids empty array items ([see rationale](#why-are-empty-array-items-forbidden)).
	- 🚫 Forbids multi-document files.
	- 🚫 Forbids [YAML merge keys](https://yaml.org/type/merge.html).
- Features:
	- 🪄 If any type within your configuration struct implements the `Validate` interface,
	then its validation method will be called using reflection
	(doesn't apply to unexported fields which are invisible to `reflect`).
	If it returns an error - the error will be reported.
	Keeps your validation logic close to your configuration type definitions.
	- Reports errors by `line:column` when possible.
	- Supports [github.com/go-playground/validator](https://github.com/go-playground/validator)
	validation struct tags.
	- Implements `env` struct tags to overwrite fields from env vars if provided.
	- Supports [`encoding.TextUnmarshaler`](https://pkg.go.dev/encoding#TextUnmarshaler)
	and [`yaml.Unmarshaler`](https://pkg.go.dev/go.yaml.in/yaml/v4#Unmarshaler)
	(except for the root struct type).
	- Supports `time.Duration`.

## Example

https://go.dev/play/p/PjV0aG7uIUH

```yaml
list:
  - foo: valid
    bar: valid
  - foo: valid
    bar: valid
map:
  valid: valid
secret: 'this will be overwritten from env var SECRET'
required: 'this must not be empty'
```

```go
package main

import (
	"fmt"

	"github.com/romshark/yamagiconf"
)

type Config struct {
	List []Struct                            `yaml:"list"`
	Map  map[ValidatedString]ValidatedString `yaml:"map"`

	// Secret will be overwritten if env var SECRET is set.
	Secret string `yaml:"secret" env:"SECRET"`

	// See https://github.com/go-playground/validator
	// for all available validation tags
	Required string `yaml:"required" validate:"required"`
}

type Struct struct {
	Foo string          `yaml:"foo"`
	Bar ValidatedString `yaml:"bar"`
}

// Validate will automatically be called by yamagiconf
func (v *Struct) Validate() error {
	if v.Foo == "invalid" {
		return fmt.Errorf("invalid foo")
	}
	if v.Bar == "invalid" {
		return fmt.Errorf("invalid bar")
	}
	return nil
}

type ValidatedString string

// Validate will automatically be called by yamagiconf
func (v ValidatedString) Validate() error {
	if v == "invalid" {
		return fmt.Errorf("string is invalid")
	}
	return nil
}

func main() {
	var c Config
	if err := yamagiconf.LoadFile("./config.yaml", &c); err != nil {
		fmt.Println("Whoops, something is wrong with your config!", err)
	}
	fmt.Printf("%#v\n", c)
}
```

## FAQ

### 

### Why are empty array items forbidden?

Consider the following YAML array:

```yaml
array:
  - 
  - ''
  - ""
  - x
```

Even though this YAML array works as expect with a Go array:
`[4]string{"", "", "", "x"}`, parsing the same YAML into a Go slice will result in
the empty item being omitted: `[]string{"", "", "x"}` which is counterintuitive.
Therefore, yamagiconf forbids empty array items in general to keep behavior
consistent and intuitive independent of the Go target type.
