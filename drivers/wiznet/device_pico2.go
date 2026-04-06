package wiznet

import (
	"fmt"
	"log"
	"machine"
	"net"
)

// New returns a new Device. It always returns a device, even if there is an error,
// to allow error recovery.
func New(n *net.HardwareAddr) (*Device, error) {
	machine.SPI0.Configure(machine.SPIConfig{Frequency: 1_000_000, SCK: machine.GP18, SDO: machine.GP19, SDI: machine.GP16})
	device := &Device{
		Bus:       machine.SPI0,
		SelectPin: machine.GP17,
	}
	if err := device.Configure(); err != nil {
		return device, fmt.Errorf("device configure err: %v\n", err)
	}

	macaddr, err := device.GetHardwareAddr()
	if err != nil {
		return nil, err
	}
	// slices.Equal worketh not.
	if (macaddr[0] == 0) && (macaddr[1] == 0) && (macaddr[2] == 0 && (macaddr[3] == 0) && macaddr[4] == 0) && (macaddr[5] == 0) {
		copy(macaddr, []byte{0x02, 0x00, 0x10, 0x00, 0x00, 0x02})
		if err := device.SetHardwareAddr(macaddr); err != nil {
			log.Printf("=========> OH NO! Trying to set: %s gets error %v", macaddr, err)
		}
	}

	return device, nil
}
