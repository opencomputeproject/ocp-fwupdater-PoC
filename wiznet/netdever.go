package wiznet

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"time"

	"tinygo.org/x/drivers/netdev"
)

type sockfd int
type socket struct {
	*Device
	conn net.Conn
	net  string
	addr string
}

var sockets = map[sockfd]*socket{}
var nextsockfd uint32

func (d *Device) GetHostByName(name string) (netip.Addr, error) {
	i, err := netip.ParseAddr(name)
	if Debug > 1 {
		log.Printf("GetHostByName(%s) -> %v, %v", name, i, err)
	}
	return i, err
}

func (d *Device) Addr() (netip.Addr, error) {
	log.Panicf("Addr")
	return netip.Addr{}, os.ErrInvalid
}

func (d *Device) Socket(domain int, stype int, protocol int) (int, error) {
	// oh dear. It's BSD.
	// map to go.
	num := "4"
	proto := "tcp"
	switch domain {
	default:
		return -1, fmt.Errorf("domain must be AF_INET or AF_INET6, not %d:%w", domain, os.ErrInvalid)
	case 0x1e: // syscall.AF_INET6:
		num = "6"
	case 2: // syscallnet.AF_INET:
	}

	switch stype {
	default:
		return -1, fmt.Errorf("stype must be SOCK_{STREAM,DGRAM} (1 or 2) not %d:%w", stype, os.ErrInvalid)
	case 2: // syscall.SOCK_DGRAM:
		proto = "udo"
	case 1: // syscall.SOCK_STREAM:
	}

	fd := sockfd(atomic.AddUint32(&nextsockfd, 1))
	sock := &socket{
		net: proto + num,
	}
	sockets[fd] = sock
	if Debug > 1 {
		log.Printf("Allocated sockfd %d, sock %v", fd, sock)
	}

	return int(fd), nil
}

func (d *Device) Bind(sockfd int, ip netip.AddrPort) error {
	log.Panicf("Bind")
	return os.ErrInvalid
}

func (d *Device) Connect(sockfd int, host string, ip netip.AddrPort) error {
	log.Panicf("Connect")
	return os.ErrInvalid
}

func (d *Device) Listen(sockfd int, backlog int) error {
	log.Panicf("Listen")
	return os.ErrInvalid
}

func (d *Device) Accept(sockfd int) (int, netip.AddrPort, error) {
	log.Panicf("Accept")
	return -1, netip.AddrPort{}, os.ErrInvalid
}

func (d *Device) Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	log.Panicf("Send")
	return -1, os.ErrInvalid
}

func (d *Device) Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	log.Panicf("Recv")
	return -1, os.ErrInvalid
}

func (d *Device) Close(sockfd int) error {
	log.Panicf("Close")
	return os.ErrInvalid
}

func (d *Device) SetSockOpt(sockfd int, level int, opt int, value interface{}) error {
	log.Panicf("SetSockOpt")
	return os.ErrInvalid
}

var _ netdev.Netdever = &Device{}
