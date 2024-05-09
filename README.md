![yamagiconf](https://github.com/romshark/yamagiconf/assets/9574743/9d4f5b77-a461-47b2-8f6f-65194755b4f1)

<a href="https://pkg.go.dev/github.com/romshark/yamagiconf">
    <img src="https://godoc.org/github.com/romshark/yamagiconf?status.svg" alt="GoDoc">
</a>
<a href="https://goreportcard.com/report/github.com/romshark/yamagiconf">
    <img src="https://goreportcard.com/badge/github.com/romshark/yamagiconf" alt="GoReportCard">
</a>
<a href='https://coveralls.io/github/romshark/yamagiconf?branch=main'>
    <img src='https://coveralls.io/repos/github/romshark/yamagiconf/badge.svg?branch=main' alt='Coverage Status' />
</a>

# yamagiconf

The heavily opinionated **YA**ML **Magi**c **Conf**iguration framework for Go
keeps your configs simple and consistent
by being *more restrictive than your regular YAML parser* 🚷 allowing only a subset of YAML and enforcing some restrictions to the target Go type.

If you hate [YAML](https://yaml.org/), and you're afraid of
[YAML documents from hell](https://ruudvanasseldonk.com/2023/01/11/the-yaml-document-from-hell), and you can't stand complex configurations then yamagiconf is for you!

🪄 It's magic because it uses [reflect](https://pkg.go.dev/reflect) to find recursively all
values of types that implement `interface { Validate() error }` and calls them reporting
an error annotated with line and column in the YAML file if necessary.

## (anti-)Features

- Go restrictions:
	- 🚫 Forbids recursive Go types.
	- 🚫 Forbids the use of `any`, `int` & `uint` (unspecified width), and other types.
	Only maps, slices, arrays and deterministic primitives are allowed.
	- ❗️ Requires `"yaml"` struct tags throughout your configuration struct type.
	- ❗️ Requires `"env"` struct tags to be POSIX-style and
	forbids any non-primitive env var fields.
	- ❗️ Allows only primitive fields with the `env` tag to be overwritten with env vars.
  forbids any non-primitive env var fields.
	- 🚫 Forbids non-pointer struct map values.
- YAML restrictions:
	- 🚫 Forbids the use of `no`, `yes`, `on` and `off` for `bool`,
  allows only `true` and `false`.
	- 🚫 Forbids the use of `~` and `Null`, allows only `null` for nilables.
	- 🚫 Forbids assigning `null` to non-nilables (which normally would assign zero value).
	- 🚫 Forbids fields in the YAML file that aren't specified by the Go type.
	- ❗️ Requires fields specified in the configuration type to be present in the YAML file.
- Features:
	- 🪄 If any type within your configuration struct implements the `Validate` interface,
	then its validation method will be called.
	If it returns an error - the error will be reported.
	Keep your validation logic close to your configuration type definitions.
	- Reports errors by `line:column` when it can.
	- Supports `encoding.TextUnmarshaler` and `time.Duration`.
	- Supports [github.com/go-playground/validator](https://github.com/go-playground/validator)
	struct validation tags.
	- Implements `env` struct tags to overwrite fields from env vars if provided.

## Example

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
