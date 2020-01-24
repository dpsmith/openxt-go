// +build !libargo

package argo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"syscall"
	"unsafe"
)

const (
	XEN_ARGO_MAX_RING_SIZE = 0x1000000
	XEN_ARGO_DOMID_ANY     = 0x7FF4
	XEN_ARGO_PORT_ANY      = 0xFFFFFFFF

	typArgo          = 87 // 'W'
	intSize          = 4  // int (assuming 4 byte ints)
	u32Size          = 4  // uint32_t
	argoRingIdSize   = 8  // struct argo_ring_id
	xenArgoAddrSize  = 8  // struct xen_argo_addr
	vIpTablesRulePos = 24 // struct viptables_rule_pos
)

var (
	argoIocSetRingSize   = iow(typArgo, 1, u32Size)
	argoIocBind          = iow(typArgo, 2, argoRingIdSize)
	argoIocGetSockName   = iow(typArgo, 3, argoRingIdSize)
	argoIocGetPeerName   = iow(typArgo, 4, xenArgoAddrSize)
	argoIocConnect       = iow(typArgo, 5, xenArgoAddrSize)
	argoIocGetConnectErr = iow(typArgo, 6, intSize)
	argoIocListen        = iow(typArgo, 7, u32Size)
	argoIocAccept        = iow(typArgo, 8, xenArgoAddrSize)
	argoIocGetSockType   = iow(typArgo, 11, intSize)
	argoIocViptablesAdd  = iow(typArgo, 12, vIpTablesRulePos)
	argoIocViptablesDel  = iow(typArgo, 13, vIpTablesRulePos)
	argoIocViptablesList = iow(typArgo, 14, u32Size)
)

func addrFromC(r io.Reader, a *Addr) error {
	err := binary.Read(r, binary.LittleEndian, &a.Port)
	if err != nil {
		return err
	}

	err = binary.Read(r, binary.LittleEndian, &a.Domain)
	if err != nil {
		return err
	}

	return nil
}

func (a *Addr) toC(w io.Writer) error {
	err := binary.Write(w, binary.LittleEndian, a.Port)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.LittleEndian, a.Domain)
	if err != nil {
		return err
	}

	// uint16 pad which xen will verify is 0
	err = binary.Write(w, binary.LittleEndian, uint16(0))
	if err != nil {
		return err
	}

	return nil
}

func (r *RingId) toC(w io.Writer) error {
	err := binary.Write(w, binary.LittleEndian, r.Domain)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.LittleEndian, r.Partner)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.LittleEndian, r.Port)
	if err != nil {
		return err
	}

	return nil
}

func connect(file *os.File, addr Addr) error {
	var buf bytes.Buffer

	addr.toC(&buf)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(argoIocConnect),
		uintptr(unsafe.Pointer(&buf.Bytes()[0])),
	)

	if errno != 0 {
		return errors.New(errno.Error())
	}

	return nil
}

func bind(file *os.File, id RingId) error {
	var buf bytes.Buffer

	id.toC(&buf)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(argoIocBind),
		uintptr(unsafe.Pointer(&buf.Bytes()[0])),
	)

	if errno != 0 {
		return errors.New(errno.Error())
	}

	return nil
}

func listen(file *os.File, backlog int) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(argoIocListen),
		uintptr(backlog),
	)

	if errno != 0 {
		return errors.New(errno.Error())
	}

	return nil
}

func accept(file *os.File) (*Conn, error) {
	b := make([]byte, xenArgoAddrSize)

	nfd, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(argoIocAccept),
		uintptr(unsafe.Pointer(&b[0])),
	)
	if errno != 0 {
		return nil, errors.New(errno.Error())
	}

	buf := bytes.NewBuffer(b)
	c := &Conn{}

	if err := addrFromC(buf, &c.addr); err != nil {
		return nil, err
	}
	c.file = os.NewFile(nfd, file.Name())
	if c.file == nil {
		return nil, errors.New("accept returned invalid descriptor")
	}

	return c, nil
}

func open(sockType, domid, port int) (*Conn, error) {
	c := &Conn{
		addr: Addr{
			Port:   uint32(port),
			Domain: DomainId(domid),
		},
	}

	switch sockType {
	case syscall.SOCK_STREAM:
		var err error
		c.file, err = os.OpenFile("/dev/argo_stream", syscall.O_RDWR, 0666)
		if err != nil {
			return nil, err
		}
	case syscall.SOCK_DGRAM:
		var err error
		c.file, err = os.OpenFile("/dev/argo_dgram", syscall.O_RDWR, 0666)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported socket type")
	}

	if err := syscall.SetNonblock(int(c.file.Fd()), false); err != nil {
		return nil, err
	}

	return c, nil
}

func Dial(sockType, domid, port int) (*Conn, error) {

	c, err := open(sockType, domid, port)
	if err != nil {
		return nil, err
	}

	if err := connect(c.file, c.addr); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

func Listen(sockType, port int, partner DomainId) (ln *Listener, err error) {

	c, err := open(sockType, XEN_ARGO_DOMID_ANY, port)
	if err != nil {
		return nil, err
	}

	l := &Listener{
		conn: c,
		ring: RingId{
			Domain:  XEN_ARGO_DOMID_ANY,
			Partner: partner,
			Port:    c.addr.Port,
		},
	}

	if err := bind(l.conn.file, l.ring); err != nil {
		return nil, err
	}

	if err := listen(l.conn.file, 5); err != nil {
		return nil, err
	}

	return l, nil
}

func (l *Listener) Accept() (*Conn, error) {
	c, err := accept(l.conn.file)
	if err != nil {
		return nil, err
	}

	if err = syscall.SetNonblock(int(l.conn.file.Fd()), false); err != nil {
		return nil, err
	}

	return c, nil
}
