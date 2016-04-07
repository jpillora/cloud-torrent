package opts

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var flagValueType = reflect.TypeOf((*flag.Value)(nil)).Elem()

//Opts is the main class, it contains
//all parsing state for a single set of
//arguments
type Opts struct {
	//embed item since an Opts can also be an item
	item
	parent       *Opts
	cmds         map[string]*Opts
	opts         []*item
	args         []*item
	arglist      *argumentlist
	optnames     map[string]bool
	envnames     map[string]bool
	order        []string
	templates    map[string]string
	internalOpts struct {
		//pretend these are in the user struct :)
		Help, Version bool
	}
	cfgPath               string
	erred                 error
	cmdname               *reflect.Value
	repo, author, version string
	pkgrepo, pkgauthor    string
	//LineWidth defines where new-lines
	//are inserted into the help text
	//(defaults to 42)
	LineWidth int
	//PadAll enables padding around the
	//help text (defaults to true)
	PadAll bool
	//PadWidth defines the amount padding
	//when rendering help text (defaults to 2)
	PadWidth int
}

//argumentlist represends a
//named string slice
type argumentlist struct {
	item
	min int
}

//item is the structure representing a
//an opt item
type item struct {
	val       reflect.Value
	name      string
	shortName string
	envName   string
	useEnv    bool
	typeName  string
	help      string
	defstr    string
}

//New creates a new Opts instance
func New(config interface{}) *Opts {
	v := reflect.ValueOf(config)
	//nil parent -> root command
	o := fork(nil, v)
	if o.erred != nil {
		//opts has already encounted an error
		return o
	}

	//attempt to infer package name, repo, author
	pkgpath := v.Elem().Type().PkgPath()
	parts := strings.Split(pkgpath, "/")
	if len(parts) >= 3 {
		o.pkgauthor = parts[1]
		o.Name(parts[2])
		switch parts[0] {
		case "github.com", "bitbucket.org":
			o.pkgrepo = "https://" + strings.Join(parts[0:3], "/")
		}
	}

	return o
}

//Parse(&config) is shorthand for New(&config).Parse()
func Parse(config interface{}) *Opts {
	return New(config).Parse()
}

func fork(parent *Opts, val reflect.Value) *Opts {
	//TODO allow order and template per cmd
	//for now, there is only the root
	var order []string = nil
	var tmpls map[string]string = nil
	if parent == nil {
		order = make([]string, len(DefaultOrder))
		copy(order, DefaultOrder)
		tmpls = map[string]string{}
	} else {
		order = parent.order
		tmpls = parent.templates
	}

	//instantiate
	o := &Opts{
		item: item{
			val: val,
		},
		parent: parent,
		//each cmd/cmd has its own set of names
		optnames: map[string]bool{},
		envnames: map[string]bool{},
		cmds:     map[string]*Opts{},
		opts:     []*item{},
		//these are only set at the root
		order:     order,
		templates: tmpls,
		//public defaults
		LineWidth: 72,
		PadAll:    true,
		PadWidth:  2,
	}

	//all fields from val
	if val.Type().Kind() != reflect.Ptr {
		o.errorf("opts: %s should be a pointer to a struct", val.Type().Name())
		return o
	}
	o.addFields(val.Elem())

	//add help option
	g := reflect.ValueOf(&o.internalOpts).Elem()
	o.addOptArg(g.Type().Field(0), g.Field(0))

	return o
}

func (o *Opts) addFields(c reflect.Value) *Opts {
	t := c.Type()
	k := t.Kind()
	//deref pointer
	if k == reflect.Ptr {
		c = c.Elem()
		t = c.Type()
		k = t.Kind()
	}

	if k != reflect.Struct {
		o.errorf("opts: %s should be a pointer to a struct (got %s)", t.Name(), k)
		return o
	}

	//parse struct fields
	for i := 0; i < c.NumField(); i++ {
		val := c.Field(i)
		//ignore unexported
		if !val.CanSet() {
			continue
		}
		sf := t.Field(i)
		k := sf.Type.Kind()

		if sf.Type.Implements(flagValueType) {
			o.addOptArg(sf, val)
			continue
		}
		switch k {
		case reflect.Ptr, reflect.Struct:
			if sf.Tag.Get("type") == "embedded" {
				o.addFields(val)
			} else {
				o.addCmd(sf, val)
			}
		case reflect.Slice:
			if sf.Type.Elem().Kind() != reflect.String {
				o.errorf("arglist must be of type []string")
				return o
			}
			o.addArgs(sf, val)
		case reflect.Bool, reflect.String, reflect.Int, reflect.Int64:
			if sf.Tag.Get("type") == "cmdname" {
				if k != reflect.String {
					o.errorf("cmdname field '%s' must be a string", sf.Name)
					return o
				}
				o.cmdname = &val
			} else {
				o.addOptArg(sf, val)
			}
		case reflect.Interface:
			o.errorf("Struct field '%s' interface type must implement flag.Value", sf.Name)
			return o
		default:
			o.errorf("Struct field '%s' has unsupported type: %s", sf.Name, k)
			return o
		}
	}

	return o
}

func (o *Opts) errorf(format string, args ...interface{}) *Opts {
	//only store the first error
	if o.erred == nil {
		o.erred = fmt.Errorf(format, args...)
	}
	return o
}

func (o *Opts) addCmd(sf reflect.StructField, val reflect.Value) {

	if o.arglist != nil {
		o.errorf("argslists and commands cannot be used together")
		return
	}

	//requires address
	switch sf.Type.Kind() {
	case reflect.Ptr:
		//if nil ptr, auto-create new struct
		if val.IsNil() {
			ptr := reflect.New(val.Type().Elem())
			val.Set(ptr)
		}
	case reflect.Struct:
		val = val.Addr()
	}

	name := sf.Tag.Get("name")
	if name == "" || name == "!" {
		name = camel2dash(sf.Name) //default to struct field name
	}
	// log.Printf("define cmd: %s =====", subname)
	sub := fork(o, val)
	sub.name = name
	sub.help = sf.Tag.Get("help")
	o.cmds[name] = sub
}

func (o *Opts) addArgs(sf reflect.StructField, val reflect.Value) {

	if len(o.cmds) > 0 {
		o.errorf("argslists and commands cannot be used together")
		return
	}
	if o.arglist != nil {
		o.errorf("only 1 arglist field is allowed ('%s' already defined)", o.arglist.name)
		return
	}

	name := sf.Tag.Get("name")
	if name == "" || name == "!" {
		name = camel2dash(sf.Name) //default to struct field name
	}

	if val.Len() != 0 {
		o.errorf("arglist '%s' is required so it should not be set. "+
			"If you'd like to set a default, consider using an option instead.", name)
		return
	}

	min, _ := strconv.Atoi(sf.Tag.Get("min"))

	//insert
	o.arglist = &argumentlist{
		item: item{
			val:  val,
			name: name,
			help: sf.Tag.Get("help"),
		},
		min: min,
	}
}

var durationType = reflect.TypeOf(time.Second)

func (o *Opts) addOptArg(sf reflect.StructField, val reflect.Value) {

	i := &item{val: val}

	//find name
	i.name = sf.Tag.Get("name")
	if i.name == "" {
		i.name = camel2dash(sf.Name) //default to struct field name
	}

	//specific environment name
	i.envName = sf.Tag.Get("env")
	if i.envName != "" {
		if o.envnames[i.envName] {
			o.errorf("option env name '%s' already in use", i.name)
			return
		}
		o.envnames[i.envName] = true
		i.useEnv = true
	}

	//assume opt, unless arg tag is present
	t := sf.Tag.Get("type")
	if t == "" {
		t = "opt"
	}

	//opt names cannot clash with each other
	if o.optnames[i.name] {
		o.errorf("option name '%s' already in use", i.name)
		return
	}
	o.optnames[i.name] = true

	//get help text
	i.help = sf.Tag.Get("help")

	//the **displayed** default, use 'default' tag, otherwise infer
	defstr := sf.Tag.Get("default")
	if defstr != "" {
		i.defstr = defstr
	} else if def := val.Interface(); def != reflect.Zero(sf.Type).Interface() {
		//not the zero-value, stringify!
		i.defstr = fmt.Sprintf("%v", def)
	}

	switch t {
	case "opt":
		//options can also set short names
		if short := sf.Tag.Get("short"); short != "" {
			if o.optnames[short] {
				o.errorf("option short name '%s' already in use", short)
				return
			} else {
				o.optnames[i.shortName] = true
				i.shortName = short
			}
		}
		// log.Printf("define option: %s %s", name, sf.Type)
		o.opts = append(o.opts, i)
	case "arg":
		//TODO allow other types in 'arg' fields
		if sf.Type.Kind() != reflect.String {
			o.errorf("arg '%s' type must be a string", i.name)
			return
		}
		o.args = append(o.args, i)
	default:
		o.errorf("Invalid optype: %s", t)
	}
}

//Name sets the name of the program
func (o *Opts) Name(name string) *Opts {
	o.name = name
	return o
}

//Version sets the version of the program
//and renders the 'version' template in the help text
func (o *Opts) Version(version string) *Opts {
	//add version option
	g := reflect.ValueOf(&o.internalOpts).Elem()
	o.addOptArg(g.Type().Field(1), g.Field(1))
	o.version = version
	return o
}

//Repo sets the repository link of the program
//and renders the 'repo' template in the help text
func (o *Opts) Repo(repo string) *Opts {
	o.repo = repo
	return o
}

//PkgRepo infers the repository link of the program
//from the package import path of the struct (So note,
//this will not work for 'main' packages)
func (o *Opts) PkgRepo() *Opts {
	if o.pkgrepo == "" {
		return o.errorf("Package repository could not be infered")
	}
	o.Repo(o.pkgrepo)
	return o
}

//Author sets the author of the program
//and renders the 'author' template in the help text
func (o *Opts) Author(author string) *Opts {
	o.author = author
	return o
}

//PkgRepo infers the repository link of the program
//from the package import path of the struct (So note,
//this will not work for 'main' packages)
func (o *Opts) PkgAuthor() *Opts {
	if o.pkgrepo == "" {
		return o.errorf("Package author could not be infered")
	}
	o.Author(o.pkgauthor)
	return o
}

//DocBefore inserts a text block before the specified template
func (o *Opts) DocBefore(target, newid, template string) *Opts {
	return o.docOffset(0, target, newid, template)
}

//DocAfter inserts a text block after the specified template
func (o *Opts) DocAfter(target, newid, template string) *Opts {
	return o.docOffset(1, target, newid, template)
}

func (o *Opts) docOffset(offset int, target, newid, template string) *Opts {
	if _, ok := o.templates[newid]; ok {
		o.errorf("new template already exists: %s", newid)
		return o
	}
	for i, id := range o.order {
		if id == target {
			o.templates[newid] = template
			index := i + offset
			rest := []string{newid}
			if index < len(o.order) {
				rest = append(rest, o.order[index:]...)
			}
			o.order = append(o.order[:index], rest...)
			return o
		}
	}
	o.errorf("target template not found: %s", target)
	return o
}

//DecSet replaces the specified template
func (o *Opts) DocSet(id, template string) *Opts {
	if _, ok := DefaultTemplates[id]; !ok {
		if _, ok := o.templates[id]; !ok {
			o.errorf("template does not exist: %s", id)
			return o
		}
	}
	o.templates[id] = template
	return o
}

//ConfigPath defines a path to a JSON file which matches
//the structure of the provided config. Environment variables
//override JSON Config variables.
func (o *Opts) ConfigPath(path string) *Opts {
	o.cfgPath = path
	return o
}

//UseEnv enables an implicit "env" struct tag option on
//all struct fields, the name of the field is converted
//into an environment variable with the transform
//`FooBar` -> `FOO_BAR`.
func (o *Opts) UseEnv() *Opts {
	o.useEnv = true
	return o
}

//Parse with os.Args
func (o *Opts) Parse() *Opts {
	return o.ParseArgs(os.Args[1:])
}

//ParseArgs with the provided arguments
func (o *Opts) ParseArgs(args []string) *Opts {
	if err := o.Process(args); err != nil {
		fmt.Fprintf(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
	return o
}

//Process is the same as ParseArgs except
//it returns an error on failure
func (o *Opts) Process(args []string) error {

	//cannot be processed - already encountered error - programmer error
	if o.erred != nil {
		return fmt.Errorf("Process error: %s", o.erred)
	}

	//1. set config via JSON file
	if o.cfgPath != "" {
		b, err := ioutil.ReadFile(o.cfgPath)
		if err == nil {
			v := o.val.Interface() //*struct
			err = json.Unmarshal(b, v)
			if err != nil {
				o.erred = fmt.Errorf("Invalid config file: %s", err)
				return errors.New(o.Help())
			}
		}
	}

	flagset := flag.NewFlagSet(o.name, flag.ContinueOnError)
	flagset.SetOutput(ioutil.Discard)

	//pre-loop through the options and
	//add shortnames and env names where possible
	for _, opt := range o.opts {
		//should generate shortname?
		if len(opt.name) >= 3 && opt.shortName == "" {
			//not already taken?
			if s := opt.name[0:1]; !o.optnames[s] {
				opt.shortName = s
				o.optnames[s] = true
			}
		}
		env := camel2const(opt.name)
		if o.useEnv && (opt.envName == "" || opt.envName == "!") &&
			opt.name != "help" && opt.name != "version" &&
			!o.envnames[env] {
			opt.envName = env
		}
	}

	for _, opt := range o.opts {
		// TODO remove debug
		// log.Printf("parse prepare option: %s", opt.name)

		//2. set config via environment
		envVal := ""
		if opt.useEnv || o.useEnv {
			envVal = os.Getenv(opt.envName)
		}
		//3. set config via Go's pkg/flags
		addr := opt.val.Addr().Interface()
		switch addr := addr.(type) {
		case flag.Value:
			flagset.Var(addr, opt.name, "")
			if opt.shortName != "" {
				flagset.Var(addr, opt.shortName, "")
			}
		case *bool:
			str2bool(envVal, addr)
			flagset.BoolVar(addr, opt.name, *addr, "")
			if opt.shortName != "" {
				flagset.BoolVar(addr, opt.shortName, *addr, "")
			}
		case *string:
			str2str(envVal, addr)
			flagset.StringVar(addr, opt.name, *addr, "")
			if opt.shortName != "" {
				flagset.StringVar(addr, opt.shortName, *addr, "")
			}
		case *int:
			str2int(envVal, addr)
			flagset.IntVar(addr, opt.name, *addr, "")
			if opt.shortName != "" {
				flagset.IntVar(addr, opt.shortName, *addr, "")
			}
		case *time.Duration:
			flagset.DurationVar(addr, opt.name, *addr, "")
			if opt.shortName != "" {
				flagset.DurationVar(addr, opt.shortName, *addr, "")
			}
		default:
			return fmt.Errorf("Option '%s' has unsupported type", opt.name)
		}
	}

	// log.Printf("parse %+v", args)
	//set user config
	err := flagset.Parse(args)
	if err != nil {
		//insert flag errors into help text
		o.erred = err
		o.internalOpts.Help = true
	}

	//internal opts (--help and --version)
	if o.internalOpts.Help {
		return errors.New(o.Help())
	} else if o.internalOpts.Version {
		fmt.Println(o.version)
		os.Exit(0)
	}

	//fill each individual arg
	args = flagset.Args()
	for i, argument := range o.args {
		if len(args) > 0 {
			str := args[0]
			args = args[1:]
			argument.val.SetString(str)
		} else if argument.defstr == "" {
			//not-set and no default!
			o.erred = fmt.Errorf("Argument #%d '%s' has no default value", i+1, argument.name)
			return errors.New(o.Help())
		}
	}

	//use command? peek at args
	if len(o.cmds) > 0 && len(args) > 0 {
		a := args[0]
		//matching command, use it
		if sub, exists := o.cmds[a]; exists {
			//user wants name to be set
			if o.cmdname != nil {
				o.cmdname.SetString(a)
			}
			return sub.Process(args[1:])
		}
	}

	//fill arglist? assign remaining as slice
	if o.arglist != nil {
		if len(args) < o.arglist.min {
			o.erred = fmt.Errorf("Too few arguments (expected %d, got %d)", o.arglist.min, len(args))
			return errors.New(o.Help())
		}
		o.arglist.val.Set(reflect.ValueOf(args))
		args = nil
	}

	//we *should* have consumed all args at this point.
	//this prevents:  ./foo --bar 42 -z 21 ping --pong 7
	//where --pong 7 is ignored
	if len(args) != 0 {
		o.erred = fmt.Errorf("Unexpected arguments: %+v", args)
		return errors.New(o.Help())
	}

	return nil
}
