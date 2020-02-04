// +build libargo

package argo
// #cgo LDFLAGS: -largo
// #include <libargo.h>
// #include <errno.h>
// #include <stdlib.h>
//
import "C"
import (
	"unsafe"
	"os"
	"syscall"
	"fmt"
	"github.com/godbus/dbus"
    )

func libargo_socket(ct C.int) (res C.int, err error) {
	res,err = C.argo_socket(ct)
	return res,err
}

func libargo_close(fd C.int) (res C.int, err error) {
	res,err = C.argo_close(fd)
	return res,err
}

func libargo_bind(fd C.int, addr *C.xen_argo_addr_t, domid C.domid_t) (rv C.int, err error) {
	rv,err = C.argo_bind(fd,addr,domid)
	return rv,err;
}

func libargo_connect(socket C.int, tDomId C.domid_t, port C.xen_argo_port_t) (res C.int,err error) {
	peer := C.struct_xen_argo_addr {
		aport: port,
		domain_id: tDomId,
	}
	res,err = C.argo_connect(socket,&peer)
	return res,err
}

func libargo_vlisten(fd C.int) (res C.int,err error) {
	res,err = C.argo_listen(fd,5)
	return res,err
}

func libargo_xen_argo_addr (domid C.domid_listent, port C.xen_argo_port_t) (rv *C.xen_argo_addr_t) {
	rv = C.make_xen_argo_addr(domid,port)
	return rv
}

func libargo_accept(fd C.int, xa_addr *C.xen_argo_addr_t) (rv C.int, err error) {
	rv,err = C.argo_accept(fd,xa_addr)
	return rv,err
}

func libargo_send(fd C.int, buff []byte, flags C.int) (rv C.ssize_t, err error) {
	i := len(buff)
	rv,err = C.argo_send(fd, unsafe.Pointer(&buff[0]), C.size_t(i), flags)
	return rv,err
}

func Dial(sockType int, domid int, port int) (*Conn, error) {
	c := &Conn{
		addr: Addr{
			Port:   uint32(port),
			Domain: DomainId(domid),
		},
	}

	s,err  := libargo_socket(syscall.SOCK_STREAM)
	if err != nil {
		panic(fmt.Sprintf("socket faild: %v",err))
	}

	retval,err := libargo_connect(s, domid, port)
	if err != nil {
		panic(fmt.Sprintf("connect failed: %v",err))
	}

	switch sockType {
	case syscall.SOCK_STREAM:
		var err error
		c.fd, err = os.NewFile(uintptr(s),"/dev/argo_stream")
		if err != nil {
			return nil, err
		}
	case syscall.SOCK_DGRAM:
		var err error
		c.fd, err = os.NewFile(uintptr(s),"/dev/argo_dgram")
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported socket type")
	}

	if err := syscall.SetNonblock(int(s), false); err != nil {
		return nil, err
	}

	return c, nil
}

func Listen(c *Conn, partner DomainId) (ln *Listener, err error) {
	return nil, errors.New("unsupported function: listen")
}

func (l *Listener) Accept() (*Conn, error) {
	return nil, errors.New("unsupported function: accept")
}
