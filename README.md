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
keeps your configs simple and consistent.

If you hate [YAML](https://yaml.org/), and you're afraid of
[YAML documents from hell](https://ruudvanasseldonk.com/2023/01/11/the-yaml-document-from-hell), and you can't stand complex configurations then yamagiconf is for you!

## (anti-)Features

- Forbids the use of `no`, `yes`, `on` and `off` for `bool`,
  allows only `true` and `false`.
- Forbids the use of `~` and `Null`, allows only `null` for nilables.
- Forbids assigning `null` to non-nilables (which is causing them to use their zero value).
- Requires `"yaml"` struct tags throught your configuration struct type.
- Requires `"env"` struct tags to be POSIX-style and
  forbids any non-primitive env var fields.
- Forbids recursive Go types.
- Forbids the use of `any`, `int` & `uint` (unspecified width), and other types.
  Only maps, slices, arrays and deterministic primitives are allowed.
- Requires fields specified in the configuration type to be present in the YAML file.
- Reports errors by `line:column` when it can.
- If any type within your configuration struct implements the `Validate` interface,
  then its validation method will be called.
  If it returns an error - the error will be reported.
  Keep your validation logic close to your configuration type definitions.

## Example

```yaml
list:
  - foo: valid
    bar: valid
  - foo: valid
    bar: valid
map:
  valid: valid
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
}

type Struct struct {
	Foo string `yaml:"foo"`
	Bar string `yaml:"bar"`
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
	c, err := yamagiconf.Load[Config]("./config.yaml")
	if err != nil {
		fmt.Println("Whoops, something is wrong with your config!", err)
	}
	fmt.Printf("%#v\n", c)
}
```