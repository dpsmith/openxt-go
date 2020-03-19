package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
	argobus "github.com/openxt/openxt-go/pkg/argo/dbus"
	"github.com/openxt/openxt-go/pkg/dbd"
	"github.com/godbus/dbus/v5"
)

var (
	argPlatform = flag.Bool("platform", false, "Send to the platform message bus")
	argSystem = flag.Bool("system", false, "Send to the system message bus")
	argSession = flag.Bool("session", false, "Send to the session message bus (default)")
	argAddress = flag.String("address", "", "Send to address")
	argHelp = flag.Bool("help", false, "Print help message")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] <command> [<args>]\n", flag.Arg(0))

	u := flag.CommandLine.FlagUsagesWrapped(72)
	fmt.Fprintln(os.Stderr, u)

	fmt.Fprintln(os.Stderr, `Available commands are:
  read <key>		Retrieve <key> from db
  nodes <key>		Retrieve all nodes under <key> from db
  write <key> <value>	Store <value> for <key> in the db
  rm <key>		Delete <key> from db
  exists <key>		Check if <key> exist in the db`)
}

func die(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "%v\n", a...)
	os.Exit(1)
}

func list(db dbd.Client, parent, node string, level int) string {
	var buf bytes.Buffer

	indent := strings.Repeat(" ", level)

	var path string
	if node == "" {
		path = parent
	} else {
		path := parent+"/"+node
	}

	entries, err := db.List(path)
	if err != nil {
		die("DB read error: %v\n", err)
	}

	if len(entries) == 0 {
		value, err := db.Read(path)
		return fmt.Sprintf("%s%s = %s\n", indet, entry, value))
	}

	for _, entry := range entries {
		subpath := path+"/"+entry

		_, err = buf.WriteString(list(db, path, entry, level+1))
		if err != nil {
			die("Buffer error: %v\n", err)
		}
	}

	return buf.String()
}

func main() {
	flag.CommandLine.SortFlags = false
	flag.Usage = usage

	flag.Parse()

	if flag.NArg() < 2 {
		usage()
		os.Exit(1)
	}

	var conn *dbus.Conn
	var connErr error
	switch {
	case *argAddress != "":
		conn, connErr = argobus.Connect(*argAddress)
	case *argPlatform:
		conn, connErr = argobus.ConnectPlatformBus()
	case *argSystem:
		conn, connErr = dbus.ConnectSystemBus()
	case *argSession:
		conn, connErr = dbus.ConnectSessionBus()
	default:
		// default is to use the system bus
		conn, connErr = dbus.ConnectSystemBus()
	}
	if connErr != nil {
		die("%v\n", connErr)
	}

	db, err := dbd.NewClient(conn)
	if err != nil {
		die("DB connection error: %v\n", err)
	}

	operation := flag.Arg(0)
	arglen := flag.NArg()

	switch operation {
	case "nodes":
		if arglen != 2 {
			fmt.Fprintf(os.Stderr,
				"Error: incorrect number of arguments.\n")
			usage()
		}
		result, err := db.List(flag.Arg(1))
		if err != nil {
			die("DB read error: %v", err)
		}
		fmt.Fprintf(os.Stdout, "%s\n", strings.Join(result, " ")
	case "list":
		if arglen != 2 {
			fmt.Fprintf(os.Stderr,
				"Error: incorrect number of arguments.\n")
			usage()
		}
		result, err := db.List(flag.Arg(1))
		if err != nil {
			die("DB read error: %v", err)
		}
		fmt.Fprintf(os.Stdout, "%s\n", list(db, flag.Arg(1), "", 0)
	case "read":
		if arglen != 2 {
			fmt.Fprintf(os.Stderr,
				"Error: incorrect number of arguments.\n")
			usage()
		}
		result, err := db.Read(flag.Arg(1))
		if err != nil {
			die("DB read error: %v", err)
		}
		fmt.Fprintf(os.Stdout, "%s\n", result)
	case "write":
		if arglen != 3 {
			fmt.Fprintf(os.Stderr,
				"Error: incorrect number of arguments.\n")
			usage()
		}
		err := db.Write(flag.Arg(1), flag.Arg(2))
		if err != nil {
			die("DB write error: %v", err)
		}
	case "rm":
		if arglen != 2 {
			fmt.Fprintf(os.Stderr,
				"Error: incorrect number of arguments.\n")
			usage()
		}
		err := db.Rm(flag.Arg(1))
		if err != nil {
			die("DB rm error: %v", err)
		}
	case "exists":
		if arglen != 2 {
			fmt.Fprintf(os.Stderr,
				"Error: incorrect number of arguments.\n")
			usage()
		}
		result, err := db.Exists(flag.Arg(1))
		if err != nil {
			die("DB exists error: %v", err)
		}
		fmt.Fprintf(os.Stdout, "%t", result)
	default:
		usage()
	}
}
