package dbus

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/openxt/openxt-go/pkg/argo"
	godbus "github.com/godbus/dbus/v5"
)

const (
	PlatformBusAddress = "argo:domain=0,port=5555"
	SystemBusAddress = "unix:path=/var/run/dbus/system_bus_socket"
)

func ConnectPlatformBus(opts ...godbus.ConnOption) (*godbus.Conn, error) {
	address := os.Getenv("DBUS_SYSTEM_BUS_ADDRESS")
	if address == "" {
		address = PlatformBusAddress
	}

	return Connect(address, opts...)
}

// Connect expands the godbus Connect to accept argo address string
//   address is of the format: `argo:domid={id},port={number}`
func Connect(address string, opts ...godbus.ConnOption) (*godbus.Conn, error) {
	var conn *godbus.Conn
	var err error

	i := strings.IndexRune(address, ':')
	if i == -1 {
		err = errors.New("dbus: invalid bus address (no transport)")
		return nil, err
	}

	if address[:i] == "argo" {
		var domid, port int

		fields := strings.Split(address[i+1:], ",")
		for _, f := range fields {
			i := strings.IndexRune(f, '=')
			switch f[:i] {
			case "domain":
				domid,_ = strconv.Atoi(f[i+1:])
			case "port":
				port,_ = strconv.Atoi(f[i+1:])
			default:
			}
		}

		c, err := argo.Dial(syscall.SOCK_STREAM, domid, port)
		if err != nil {
			return nil, err
		}

		conn, err = godbus.NewConn(c.File(), opts...)
		if err != nil {
			return nil, err
		}
	} else {
		conn, err = godbus.Dial(address, opts...)
		if err != nil {
			return nil, err
		}
	}

	// Should pass conn.auth but it is private and cannot access
	if err = conn.Auth([]godbus.Auth{godbus.AuthAnonymous(),godbus.AuthExternal("root")}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err = conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
