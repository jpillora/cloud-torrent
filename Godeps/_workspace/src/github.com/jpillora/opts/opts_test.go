package opts

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var spaces = regexp.MustCompile(`\ `)
var newlines = regexp.MustCompile(`\n`)

func readable(s string) string {
	s = spaces.ReplaceAllString(s, "•")
	s = newlines.ReplaceAllString(s, "⏎\n")
	return s
}

func check(t *testing.T, a, b interface{}) {
	if !reflect.DeepEqual(a, b) {
		stra := readable(fmt.Sprintf("%v", a))
		strb := readable(fmt.Sprintf("%v", b))

		extra := ""
		if out, ok := diffstr(stra, strb); ok {
			extra = "\n\n" + out
		}
		t.Fatalf("got '%v', expected '%v'%s", stra, strb, extra)

	}
}

func diffstr(a, b interface{}) (string, bool) {
	stra, oka := a.(string)
	strb, okb := b.(string)
	if !oka || !okb {
		return "", false
	}
	line := 1
	char := 1
	var diff rune
	ra := []rune(stra)
	rb := []rune(strb)
	for i, a := range ra {
		if a == '\n' {
			line++
			char = 1
		} else {
			char++
		}
		var b rune
		if i < len(strb) {
			b = rb[i]
		}
		if a != b {
			log.Printf("%d:%d - %s (%x) %s (%x)", line, char, string(a), a, string(b), b)
			a = diff
			break
		}
	}
	return fmt.Sprintf("Diff on line %d char %d (%d)", line, char, diff), true
}

func TestSimple(t *testing.T) {
	//config
	type Config struct {
		Foo string
		Bar string
	}

	c := &Config{}

	//flag example parse
	New(c).ParseArgs([]string{"--foo", "hello", "--bar", "world"})

	//check config is filled
	check(t, c.Foo, "hello")
	check(t, c.Bar, "world")
}

func TestSubCommand(t *testing.T) {

	type FooConfig struct {
		Ping string
		Pong string
	}

	//config
	type Config struct {
		Cmd string `type:"cmdname"`
		//command (external struct)
		Foo FooConfig
		//command (inline struct)
		Bar struct {
			Zip string
			Zap string
		}
	}

	c := &Config{}

	New(c).ParseArgs([]string{"bar", "--zip", "hello", "--zap", "world"})

	//check config is filled
	check(t, c.Cmd, "bar")
	check(t, c.Foo.Ping, "")
	check(t, c.Foo.Pong, "")
	check(t, c.Bar.Zip, "hello")
	check(t, c.Bar.Zap, "world")
}

func TestUnsupportedType(t *testing.T) {
	//config
	type Config struct {
		Foo string
		Bar map[string]bool
	}
	c := Config{}
	//flag example parse
	err := New(&c).Process([]string{"--foo", "hello", "--bar", "world"})
	if err == nil {
		t.Fatal("Expected error")
	}
	check(t, strings.Contains(err.Error(), "has unsupported type: map"), true)
}

func TestUnsupportedInterfaceType(t *testing.T) {
	//config
	type Config struct {
		Foo string
		Bar interface{}
	}
	c := Config{}
	//flag example parse
	err := New(&c).Process([]string{"--foo", "hello", "--bar", "world"})
	if err == nil {
		t.Fatal("Expected error")
	}
	check(t, strings.Contains(err.Error(), "interface type must implement flag.Value"), true)
}

func TestEnv(t *testing.T) {

	os.Setenv("STR", "helloworld")
	os.Setenv("NUM", "42")
	os.Setenv("BOOL", "true")

	//config
	type Config struct {
		Str  string
		Num  int
		Bool bool
	}

	c := &Config{}

	//flag example parse
	if err := New(c).UseEnv().Process([]string{}); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("STR")
	os.Unsetenv("NUM")
	os.Unsetenv("BOOL")

	//check config is filled
	check(t, c.Str, `helloworld`)
	check(t, c.Num, 42)
	check(t, c.Bool, true)
}

func TestArg(t *testing.T) {

	//config
	type Config struct {
		Foo string `type:"arg"`
		Zip string `type:"arg"`
		Bar string
	}

	c := &Config{}

	//flag example parse
	New(c).ParseArgs([]string{"-b", "wld", "hel", "lo"})

	//check config is filled
	check(t, c.Foo, `hel`)
	check(t, c.Zip, `lo`)
	check(t, c.Bar, `wld`)
}

func TestIgnoreUnexported(t *testing.T) {

	//config
	type Config struct {
		Foo string
		bar string
	}

	c := &Config{}

	//flag example parse
	err := New(c).Process([]string{"-f", "1", "-b", "2"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDocBefore(t *testing.T) {

	//config
	type Config struct {
		Foo string
		bar string
	}

	c := &Config{}

	//flag example parse
	o := New(c)

	l := len(o.order)
	o.DocBefore("usage", "mypara", "hello world this some text\n\n")
	check(t, len(o.order), l+1)
	check(t, o.Help(), `
  hello world this some text

  Usage: opts [options]

  Options:
  --foo
  --help
`)
}

func TestDocAfter(t *testing.T) {

	//config
	type Config struct {
		Foo string
		bar string
	}

	c := &Config{}

	//flag example parse
	o := New(c)

	l := len(o.order)
	o.DocAfter("usage", "mypara", "\nhello world this some text\n")
	check(t, len(o.order), l+1)
	check(t, o.Help(), `
  Usage: opts [options]

  hello world this some text

  Options:
  --foo
  --help
`)
}
