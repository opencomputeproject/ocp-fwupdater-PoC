package wiznet

import (
	"fmt"
	"log"
	"machine"
	"net"
	"os"
	"time"

	"tinygo.org/x/drivers/netdev"
	"tinygo.org/x/drivers/netlink"
)

const socketNum uint8 = 8

// Enable debug prints.

var Debug int = 0

// Device is a W5500 chip interface
type Device struct {
	Bus       *machine.SPI
	SelectPin machine.Pin
	hostname  string
}

// Configure SPI interface and select pin
func (w *Device) Configure() error {
	time.Sleep(time.Millisecond * 600)

	if err := w.Bus.Configure(machine.SPIConfig{
		LSBFirst: false,
		SCK:      machine.GP18, SDO: machine.GP19, SDI: machine.GP16,
		Mode:      machine.Mode3,
		Frequency: 62_500_000,
	}); err != nil {
		return err
	}

	w.SelectPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	w.SelectPin.High()

	if err := w.isW5500(); err != nil {
		return fmt.Errorf("could not detect W5500 chip:%v", err)
	}

	return nil
}

func (w *Device) isW5500() error {
	if err := w.Reset(); err != nil {
		return fmt.Errorf("isW5500:%v", err)
	}

	if false { // original code set PPOE?
		if err := w.WriteByte(CommonRegister, ModeRegister, 0x08); err != nil {
			return fmt.Errorf("%v:WriteByte(CommonRegister(%#02x), ModeRegister(%#02x), %#02x): %v", w, CommonRegister, ModeRegister, 0x08, err)
		}
		r, err := w.ReadByte(CommonRegister, ModeRegister)
		if err != nil {
			return fmt.Errorf("%v.ReadByte(CommonRegister(%#02x), ModeRegister(%#02x): %v", w, CommonRegister, ModeRegister, err)
		}
		if r != 0x08 {
			return fmt.Errorf("%v: type is %#02x, not %#02x:%v", w, r, 8, os.ErrNotExist)
		}
	}

	if true { // why disable ping?
		if err := w.WriteByte(CommonRegister, ModeRegister, 0x10); err != nil {
			return fmt.Errorf("%v:WriteByte(CommonRegister(%#02x), ModeRegister(%#02x), %#02x): %v", w, CommonRegister, ModeRegister, 0x10, err)
		}

		r, err := w.ReadByte(CommonRegister, ModeRegister)

		if err != nil {
			return fmt.Errorf("%v.ReadByte(CommonRegister(%#02x), ModeRegister(%#02x): %v", w, CommonRegister, ModeRegister, err)
		}
		if r != 0x10 {
			return fmt.Errorf("%v: type is %#02x, not %#02x:%v", w, r, 0x10, os.ErrNotExist)
		}

	}
	if false { // what was this all about?
		if err := w.WriteByte(CommonRegister, ModeRegister, 0x0); err != nil {
			return fmt.Errorf("%v:WriteByte(CommonRegister(%#02x), ModeRegister(%#02x), %#02x): %v", w, CommonRegister, ModeRegister, 0x0, err)
		}
		r, err := w.ReadByte(CommonRegister, ModeRegister)

		if err != nil {
			return fmt.Errorf("%v.ReadByte(CommonRegister(%#02x), ModeRegister(%#02x): %v", w, CommonRegister, ModeRegister, err)
		}
		if r != 0x0 {
			return fmt.Errorf("%v: type is %#02x, not %#02x:%v", w, r, 0x10, os.ErrNotExist)
		}
	}

	if false { // what was this supposed to do?
		r, err := w.ReadByte(CommonRegister, ModeRegister)

		if err != nil {
			return fmt.Errorf("%v.ReadByte(CommonRegister(%#02x), ModeRegister(%#02x): %v", w, CommonRegister, ModeRegister, err)
		}
		if r != 4 {
			return fmt.Errorf("%v: type is %#02x, not %#02x:%v", w, r, 0x4, os.ErrNotExist)
		}
	}

	return nil
}

func (w *Device) Reset() error {
	_, err := w.SetPHYConfiguration(0)
	if err != nil {
		log.Printf("Set phy config to 0: %v", err)
	}

	for i := 0; i < 1000; i++ {
		bb := w.GetPHYConfiguration()
		if bb == 0 {
			break
		}
	}

	_, err = w.SetPHYConfiguration(0x80)
	if err != nil {
		log.Printf("Set phy config to 0x80: %v", err)
	}

	if err := w.WriteByte(CommonRegister, ModeRegister, 0x80); err != nil {
		return fmt.Errorf("%v.Reset(): %v", w, err)
	}

	var (
		b byte
	)
	for i := 0; i < 20; i++ {
		b, err = w.ReadByte(CommonRegister, ModeRegister)
		if err != nil {
			return fmt.Errorf("%v.ReadByte(CommonRegister(%#02x), ModeRegister(%#02x)): %v", w, CommonRegister, ModeRegister, err)
		}

		if b == 0x0 {
			break
		}

		time.Sleep(1 * time.Millisecond)
	}

	if b != 0 {
		return fmt.Errorf("reset fails: common register is %#02x, not 0", b)
	}
	return nil
}

func (w *Device) NewSocket(port uint8) (*Socket, error) {
	if port > SocketMax {
		return nil, fmt.Errorf("port %d: only ports 0..%d are valid:%w", port, SocketMax, os.ErrNotExist)
	}

	return &Socket{
		Num: port,
		d:   w,
	}, nil
}

type PHYConfiguration byte

func (c PHYConfiguration) IsLinkUp() bool {
	return (c & 0x01) != 0
}

func (c PHYConfiguration) Is100MbpsLink() bool {
	return (c & 0x02) != 0
}

func (w *Device) GetPHYConfiguration() PHYConfiguration {
	b, _ := w.ReadByte(CommonRegister, PHYConfigurationRegister)
	return PHYConfiguration(b)
}

func (w *Device) SetPHYConfiguration(b byte) (PHYConfiguration, error) {
	err := w.WriteByte(CommonRegister, PHYConfigurationRegister, b)
	bb := w.GetPHYConfiguration()
	if err != nil {
		return bb, err
	}
	if b != byte(bb) {
		return bb, fmt.Errorf("tried to set %#02x; got %#02x:%w", b, bb, os.ErrInvalid)
	}

	return PHYConfiguration(bb), nil
}

// SetIPAddress sets client IP address
func (w *Device) SetIPAddress(ip net.IP) error {
	return w.Write(CommonRegister, IPAddressRegister, ip)
}

// GetIPAddress reads client ip address from a W5500 chip
func (w *Device) GetIPAddress() net.IP {
	b, _ := w.Read(CommonRegister, IPAddressRegister, 4)
	return net.IP(b)
}

// SetGatewayAddress sets client IP address
func (w *Device) SetGatewayAddress(ip net.IP) error {
	return w.Write(CommonRegister, GatewayIPAddressRegister, ip)
}

// GetGatewayAddress reads client ip address from a W5500 chip
func (w *Device) GetGatewayAddress() net.IP {
	b, _ := w.Read(CommonRegister, GatewayIPAddressRegister, 4)
	return net.IP(b)
}

// SetSubnetMask sets subnet mask
func (w *Device) SetSubnetMask(mask net.IPMask) error {
	return w.Write(CommonRegister, SubnetMaskRegister, mask)
}

// GetSubnetMask reads subnet mask from a W5500 chip
func (w *Device) GetSubnetMask() net.IPMask {
	b, _ := w.Read(CommonRegister, SubnetMaskRegister, 4)
	return net.IPMask(b)
}

// SetIPNet sets gateway and net mask
func (w *Device) SetIPNet(net net.IPNet) error {
	if err := w.SetGatewayAddress(net.IP); err != nil {
		return err
	}
	if err := w.SetSubnetMask(net.Mask); err != nil {
		return err
	}

	return nil
}

// GetIPNet reads network from W5500 chip
func (w *Device) GetIPNet() net.IPNet {
	gw := w.GetGatewayAddress()
	mask := w.GetSubnetMask()

	return net.IPNet{IP: gw, Mask: mask}
}

// SetHardwareAddr sets a mac address on a W5500 chip
func (w *Device) SetHardwareAddr(mac net.HardwareAddr) error {
	return w.Write(CommonRegister, HardwareAddressRegister, mac)
}

// Read reads a len of bytes
func (w *Device) Read(bsb BlockSelect, address Register, len uint16) ([]byte, error) {
	control := uint8(bsb) << 3
	control |= byte(ReadAccessMode) | byte(VariableLengthMode)

	data := []byte{
		byte((address & 0xFF00) >> 8),
		byte((address & 0x00FF) >> 0),
		control,
	}

	w.SelectPin.Low()
	defer w.SelectPin.High()

	if err := w.Bus.Tx(data, nil); err != nil {
		return nil, err
	}

	buf := make([]byte, len)
	err := w.Bus.Tx(nil, buf)

	if Debug > 1 {
		dumpSPI(control, address, buf)
	}

	return buf, err
}

func (w *Device) ReadByte(control BlockSelect, address Register) (byte, error) {
	b, err := w.Read(control, address, 1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (w *Device) Read16(control BlockSelect, address Register) (uint16, error) {
	b, err := w.Read(control, address, 2)
	if err != nil {
		return 0, err
	}

	return (uint16(b[0]) << 8) | uint16(b[1]), nil
}

func dumpSPI(control uint8, address Register, buf []byte) {
	addr := BlockSelect(control >> 3).String()
	wr := ReadWrite(control & byte(WriteAccessMode)).String()
	amt := Size(control & 3).String()

	log.Printf("Hello %s %s %s %s %#02x", wr, addr, address.String(), amt, buf)

}

// Write writes a buf slice
func (w *Device) Write(bsb BlockSelect, address Register, buf []byte) error {
	control := byte(bsb) << 3
	control |= byte(WriteAccessMode) | byte(VariableLengthMode)

	data := []byte{
		byte((address & 0xFF00) >> 8),
		byte((address & 0x00FF) >> 0),
		control,
	}

	w.SelectPin.Low()
	defer w.SelectPin.High()
	if err := w.Bus.Tx(data, nil); err != nil {
		return fmt.Errorf("Writing data %#02x:%w", data, err)
	}
	if err := w.Bus.Tx(buf, nil); err != nil {
		return fmt.Errorf("Writing data %d bytes:%w", len(data), err)
	}

	if Debug > 1 {
		dumpSPI(control, address, buf)
	}

	return nil
}

func (w *Device) WriteByte(control BlockSelect, address Register, buf byte) error {
	return w.Write(control, address, []byte{buf})
}

func (w *Device) Write16(control BlockSelect, address Register, buf uint16) error {
	return w.Write(control, address, []byte{byte(buf >> 8), byte(buf & 0xFF)})
}

func Probe(n *net.HardwareAddr) (netlink.Netlinker, netdev.Netdever) {
	d, err := New(n)
	if err != nil {
		log.Fatalf("new netdev: %v", err)
		return nil, nil
	}

	// at some, we do this differently ...
	d.SetIPAddress(net.IP{10, 0, 0, 2})
	d.SetGatewayAddress(net.IP{10, 0, 0, 1})
	d.SetSubnetMask(net.IPMask{255, 255, 255, 0})

	s, err := d.NewSocket(0)
	if err != nil {
		log.Fatalf("socket for %v:%v", d, err)
	}

	if err := s.Open("raw", 0); err != nil {
		log.Fatal("%v", err)
	}

	netdev.UseNetdev(d)
	d.hostname = "default"

	return d, d
}
