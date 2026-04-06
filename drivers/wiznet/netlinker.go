package wiznet

import (
	"log"
	"net"

	"tinygo.org/x/drivers/netlink"
)

func (d *Device) NetConnect(params *netlink.ConnectParams) error {
	// we are always connected.
	return nil
}

// Disconnect device from network
func (d *Device) NetDisconnect() {
	log.Panicf("NetDisconnect")
}

// Notify to register callback for network events
func (d *Device) NetNotify(cb func(netlink.Event)) {
	log.Panicf("NetNotify")
}

// GetHardwareAddr reads a mac address from a W5500 chip
func (w *Device) GetHardwareAddr() (net.HardwareAddr, error) {
	b, err := w.Read(CommonRegister, HardwareAddressRegister, 6)
	return net.HardwareAddr(b), err
}

var _ netlink.Netlinker = &Device{}
