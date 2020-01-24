package main

import (
	"fmt"
	"io"
	"os"
	"syscall"

	flag "github.com/spf13/pflag"
	"github.com/openxt/openxt-go/pkg/argo"
)

var (
	domid = flag.IntP("domain", "d", 0, "destination domain id")
	port = flag.IntP("port", "p", 5555, "destination port")
	listen = flag.BoolP("listen", "l", false, "listen for incoming connections")
)

func sender(sockType, domid, port int) {
	s, err := argo.Dial(sockType, domid, port)
	if err != nil {
		panic(err)
	}

	_, err = io.Copy(s, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err: argo connection error: ", err)
	}
}

func listener(sockType, port int) {
	l, err := argo.Listen(sockType, port, argo.XEN_ARGO_DOMID_ANY)
	if err != nil {
		panic(err)
	}

	for {
		incoming, err := l.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "err: argo listen error: ", err)
		} else {
			_, err := io.Copy(os.Stdout, incoming)
			switch err {
			case io.ErrClosedPipe,io.EOF:
				continue
			default:
				fmt.Fprintln(os.Stderr, "err: argo connection error: ", err)
			}
			incoming.Close()
		}
	}
}

func main() {

	flag.Parse()

	if *listen {
		listener(syscall.SOCK_STREAM, *port)
	} else {
		sender(syscall.SOCK_STREAM, *domid, *port)
	}
}
