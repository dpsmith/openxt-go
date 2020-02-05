package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	flag "github.com/spf13/pflag"
	"github.com/openxt/openxt-go/pkg/argo/dbus"
	godbus "github.com/godbus/dbus/v5"
)

var (
	argPlatform = flag.Bool("platform", false, "Send to the platform message bus")
	argSystem = flag.Bool("system", false, "Send to the system message bus")
	argSession = flag.Bool("session", false, "Send to the session message bus (default)")
	argAddress = flag.String("address", "", "Send to address")
	argDest = flag.String("dest", "", "Specify the name of the connection to receive the message")
	argPrintReply = flag.Bool("print-reply", false, "Block for a reply to the message sent, and print any reply received in a human-readable form")
	argReplyTimeout = flag.Int("reply-timeout", 25, "Wait for a reply for up to MSEC milliseconds")
	argType = flag.String("type", "signal", "Specify method_call or signal")
)

func baseType(s string) (reflect.Type, error) {
	switch s {
	case "int16":
		return reflect.TypeOf((int16)(0)), nil
	case "uint16":
		return reflect.TypeOf((uint16)(0)), nil
	case "int32":
		return reflect.TypeOf((int32)(0)), nil
	case "uint32":
		return reflect.TypeOf((uint32)(0)), nil
	case "int64":
		return reflect.TypeOf((int64)(0)), nil
	case "uint64":
		return reflect.TypeOf((uint64)(0)), nil
	case "double":
		return reflect.TypeOf((float64)(0)), nil
	case "string":
		return reflect.TypeOf((string)("")), nil
	}

	return reflect.TypeOf((interface{})(nil)), fmt.Errorf("type lookup: unknown type %v", s)
}

func baseValue(t, s string) (reflect.Value, error) {
	nilValue := reflect.ValueOf((interface{})(nil))

	switch t {
	case "int16":
		v, err := strconv.ParseInt(s, 10, 16)
		if err != nil {
			return nilValue, fmt.Errorf("invalid int16 %s\n",s)
		}
		return reflect.ValueOf(int16(v)), nil
	case "uint16":
		v, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return nilValue, fmt.Errorf("invalid uint16 %s\n",s)
		}
		return reflect.ValueOf(uint16(v)), nil
	case "int32":
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return nilValue, fmt.Errorf("invalid int32 %s\n",s)
		}
		return reflect.ValueOf(int32(v)), nil
	case "uint32":
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return nilValue, fmt.Errorf("invalid uint32 %s\n",s)
		}
		return reflect.ValueOf(uint32(v)), nil
	case "int64":
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nilValue, fmt.Errorf("invalid int64 %s\n",s)
		}
		return reflect.ValueOf(int64(v)), nil
	case "uint64":
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nilValue, fmt.Errorf("invalid uint64 %s\n",s)
		}
		return reflect.ValueOf(uint64(v)), nil
	case "double":
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nilValue, fmt.Errorf("invalid float64 %s\n",s)
		}
		return reflect.ValueOf(v), nil
	case "string":
		v := strings.Trim(s, "\"'")
		return reflect.ValueOf(v), nil
	}

	return nilValue, fmt.Errorf("value conversion: unknown type %v", t)
}

// array:<type>:<value>[,<value>...]
// "array" will already be peeled off
func parseArryArg(a string) (interface{}, error) {
	var goType reflect.Type

	i := strings.IndexRune(a, ':')
	if i == -1 {
		return nil, fmt.Errorf("invalid array %s\n",a)
	}

	base, entries := a[:i], a[i+1:]
	goType, err := baseType(base)
	if err != nil {
		return nil, err
	}

	slice := reflect.MakeSlice(reflect.SliceOf(goType), 0, 0)

	for _, f := range strings.Split(entries, ",") {
		v, e := baseValue(base, f)
		if e != nil {
			return nil, e
		}
		slice = reflect.Append(slice, v)
	}

	return slice.Interface(), nil
}

func parseContentArgs(args []string) ([]interface{}, error) {
	var content []interface{}

	for _, a := range args {
		i := strings.IndexRune(a, ':')
		if i == -1 {
			return nil, fmt.Errorf("invalid arg %s\n",a)
		}
		base,entry := a[:i], a[i+1:]
		switch base {
		case "int16","uint16","int32","uint32","int64","uint64","double","string":
			v, e := baseValue(base, entry)
			if e != nil {
				return nil, e
			}
			content = append(content, v.Interface())
		case "array":
			v, e := parseArryArg(entry)
			if e != nil {
				return nil, e
			}
			content = append(content, v)
		case "dict","variant":
			return nil, fmt.Errorf("unsupported arg type: %s", base)
		}
	}

	return content, nil
}

func main() {

	flag.Usage = func() {
		u := flag.CommandLine.FlagUsagesWrapped(72)
		fmt.Fprintln(os.Stderr,
			"Usage dbus-send",
			" [--platform | --system | --session | --address=ADDRESS]",
			" [--dest=NAME] [--print-reply] [--reply-timeout=MSEC]",
			" [--type=TYPE] object_path interface.member [contents...]")
		fmt.Fprintln(os.Stderr,u)
	}

	flag.Parse()

	args := flag.Args()

	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "err: both an object_path and interface.member must be specified")
	}

	objPath, intfMethod, args := args[0], args[1], args[2:]

	c, err := parseContentArgs(args)
	if err != nil {
		fmt.Printf("%v\n",err)
		return
	}

	var conn *godbus.Conn
	var connErr error
	switch {
	case *argAddress != "":
		conn, connErr = dbus.Connect(*argAddress)
	case *argPlatform:
		conn, connErr = dbus.ConnectPlatformBus()
	case *argSystem:
		conn, connErr = godbus.ConnectSystemBus()
	case *argSession:
		conn, connErr = godbus.ConnectSessionBus()
	default:
		fmt.Println("must specify a connection type")
		return
	}
	if connErr != nil {
		fmt.Printf("%v\n",connErr)
		return
	}

	var call *godbus.Call
	if len(c) > 0 {
		call = conn.Object(*argDest,godbus.ObjectPath(objPath)).Call(intfMethod, 0, c...)
	} else {
		call = conn.Object(*argDest,godbus.ObjectPath(objPath)).Call(intfMethod, 0)
	}

	if call.Err != nil {
		panic(call.Err)
	}

	if *argPrintReply {
		b, err := json.MarshalIndent(call.Body, "", "  ")
		if err != nil {
			fmt.Println("error:", err)
		}
		fmt.Println(string(b))
	}

	return
}
