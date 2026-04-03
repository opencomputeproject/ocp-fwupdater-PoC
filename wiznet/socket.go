package wiznet

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"time"
)

// Socket w5500
type Socket struct {
	io.ReadCloser
	io.WriteCloser
	d *Device

	BufferSize uint16 // todo: should be moved outside
	Num        uint8

	receivedSize  uint16
	receiveOffset uint16
}

func (s *Socket) initBuffers() error {
	// buffer initialisation should be moved outside probably
	if s.BufferSize == 0 {
		s.BufferSize = 2048
	}

	fmt.Printf("Socket buffer size : %dKB on socket %d S_IntR %d\n", s.BufferSize>>10, s.Num, SocketInterruptRegister)
	err := s.d.WriteByte(s.getRegister(), SocketReceiveBufferSize, byte(s.BufferSize>>10))
	err = s.d.WriteByte(s.getRegister(), SocketTransmitBufferSize, byte(s.BufferSize>>10))

	return err
}

func (s *Socket) Open(protocol string, port uint16) error {
	if err := s.initBuffers(); err != nil {
		return err
	}

	if status := s.Status(); status != SocketClosedStatus {
		log.Printf("open on non-closed socket: 0x%x\n\r", status)

		if err := s.Close(); err != nil {
			return err
		}
	}

	mode, err := newSocketModeFromProtocol(protocol)
	if err != nil {
		return err
	}
	if err := s.writeMode(mode); err != nil {
		return err
	}
	if err := s.clearInterrupt(); err != nil {
		return err
	}

	if port == 0 { // pick random local port instead
		port = 49152 + uint16(rand.Intn(16383))
	}

	if err := s.d.Write16(s.getRegister(), SocketSourcePort, port); err != nil {
		return err
	}

	stat := s.execCommand(socketOpenCommand)

	return stat
}

func (s *Socket) Close() error {
	return s.execCommand(socketCloseCommand)
}

func (s *Socket) Listen() error {
	status := s.Status()
	if status != SocketInitStatus {
		return fmt.Errorf("listen on uninitalized socket: 0x%x", status)
	}

	return s.execCommand(socketListenCommand)
}

func (s *Socket) Connect(ip net.IP, port uint16) error {
	status := s.Status()
	if status != SocketInitStatus {
		return fmt.Errorf("connect on uninitalized socket: 0x%x", status)
	}

	if err := s.d.Write(s.getRegister(), SocketDestinationIPAddress, ip); err != nil {
		return err
	}
	if err := s.d.Write(s.getRegister(), SocketDestinationHardwareAddress, []byte{0x00, 0xe0, 0x4c, 0x68, 0x00, 0xee}); err != nil {
		return err
	}
	if err := s.d.Write16(s.getRegister(), SocketDestinationPort, port); err != nil {
		return err
	}

	if err := s.execCommand(socketConnectCommand); err != nil {
		return err
	}

	start := time.Now()
	for {
		status := s.Status()
		switch status {
		case SocketClosedStatus:
			return errors.New("connect failed")
		case SocketCloseWaitStatus:
		case SocketEstablishedStatus:
			return nil
		}

		if time.Now().Sub(start) > time.Second*5 { // todo: timeout 5s as a configurable paramater
			return errors.New("connect timeout") // todo: better errors
		}

		irq, err := s.d.ReadByte(CommonRegister, InterruptRegister)
		if err != nil {
			log.Printf("reading common irq:%v", err)
		}
		sirq, err := s.ReadInterrupt()
		if err != nil {
			log.Printf("reading socket irq:%v", err)
		}
		log.Printf("socket irq %#02x IRQ %#02x", sirq, irq)
		time.Sleep(time.Millisecond)
	}
}

func (s *Socket) Disconnect() error {
	return s.execCommand(socketDisconnectCommand)
}

type SocketStatus uint8

const (
	SocketClosedStatus      SocketStatus = 0x00
	SocketInitStatus                     = 0x13
	SocketListenStatus                   = 0x14
	SocketSYNSENTStatus                  = 0x15
	SocketRECVStatus                     = 0x16
	SocketEstablishedStatus              = 0x17
	SocketFinWaitStatus                  = 0x18
	SocketClosingStatus                  = 0x1A
	SocketTimeWaitStatus                 = 0x1B
	SocketCloseWaitStatus                = 0x1C
	SocketLastAckStatus                  = 0x1D
	SocketUDPStatus                      = 0x22
	SocketMACRAWStatus                   = 0x42
)

func (s *Socket) Status() SocketStatus {
	b, _ := s.d.ReadByte(s.getRegister(), SocketStatusRegister)
	return SocketStatus(b)
}

func (s *Socket) getRegister() BlockSelect {
	return BlockSelect(s.Num*4 + 1)
}

type socketMode uint8

const (
	socketClosedMode         socketMode = 0x00
	socketTCPProtocolMode               = 0x01 // 0x21
	socketUDPProtocolMode               = 0x02
	socketMacRAWProtocolMode            = 0x04
)

func newSocketModeFromProtocol(protocol string) (socketMode, error) {
	switch protocol {
	case "tcp":
		return socketTCPProtocolMode, nil
	case "udp":
		return socketUDPProtocolMode, nil
	case "raw":
		return socketMode(byte(socketMacRAWProtocolMode) | 0xf4), nil
	}

	return 0, fmt.Errorf("unsupported protocol: %s", protocol)
}

func (s *Socket) clearInterrupt() error {
	return s.d.WriteByte(s.getRegister(), SocketInterruptRegister, 0xff)
}

type socketInterrupt uint8

const (
	socketSendOkInterrupt  socketInterrupt = 0x10
	socketTimeoutInterrupt socketInterrupt = 0x08
	socketRecvInterrupt    socketInterrupt = 0x04
	socketDisconInterrupt  socketInterrupt = 0x02
	socketConInterupt      socketInterrupt = 0x01
)

// ReadInterrupt reads the socket interrupt register
func (s *Socket) ReadInterrupt() (socketInterrupt, error) {
	i, err := s.d.ReadByte(s.getRegister(), SocketInterruptRegister)
	return socketInterrupt(i), err
}

// ReadInterrupt reads the socket interrupt register
func (s *Socket) Readregister(reg Register) (byte, error) {
        value, err := s.d.ReadByte(0x0, reg)
        return value, err
}

func (s *Socket) SetInterruptMask() error {
	// Let's check a few register
	// Let's write a value without generating interrupt
	// when we don't need to like SENTOK
	s.d.WriteByte(s.getRegister(), 0x2c, 0x7)
	_, _ = s.d.ReadByte(s.getRegister(), 0x2c)
	_, _ = s.d.ReadByte(s.getRegister(), 0x00)
	return s.d.WriteByte(0x00, 0x18, 0xff)
}	

// WriteInterrupt reads the socket interrupt register
func (s *Socket) WriteInterrupt(i socketInterrupt) error {
	return s.d.WriteByte(s.getRegister(), SocketInterruptRegister, byte(i))
}

func (s *Socket) readMode() (socketMode, error) {
	b, err := s.d.ReadByte(s.getRegister(), SocketModeRegister)
	return socketMode(b), err
}

func (s *Socket) writeMode(mode socketMode) error {
	return s.d.WriteByte(s.getRegister(), SocketModeRegister, byte(mode))
}


type socketCmd uint8

const (
	socketOpenCommand       socketCmd = 0x01
	socketListenCommand     socketCmd = 0x02
	socketConnectCommand    socketCmd = 0x04
	socketDisconnectCommand socketCmd = 0x08
	socketCloseCommand      socketCmd = 0x10
	socketSendCommand       socketCmd = 0x20
	socketSendMacCommand    socketCmd = 0x21
	socketSendKeepCommand   socketCmd = 0x22
	socketRecvCommand       socketCmd = 0x40
)

func (s *Socket) execCommand(cmd socketCmd) error {
	if err := s.d.WriteByte(s.getRegister(), SocketCommandRegister, byte(cmd)); err != nil {
		return err
	}

	for i := 0; i < 10; i++ {
		b, err := s.d.ReadByte(s.getRegister(), SocketCommandRegister)

		if err != nil {
			return err
		}

		if b == 0x0 {
			return nil
		}

		if socketCmd(b) != cmd {
			return fmt.Errorf("invalid command set: 0x%x", b)
		}

		time.Sleep(time.Millisecond)
	}

	return errors.New("socket command timeout")
}

func (s *Socket) read(start, len uint16) ([]byte, error) {
	start &= s.BufferSize - 1
	address := uint16(s.Num)*2048 + 0xC000 + start

	return s.d.Read(BlockSelect(s.Num*4+3), Register(address), len)
}

func (s *Socket) Read(p []byte) (n int, err error) {
	l := uint16(len(p))
	if l == 0 {
		return 0, nil
	}

	availableSize, err := s.d.Read16(s.getRegister(), SocketReceiveReceivedSize)
	if err != nil {
		return 0, err
	}

	if availableSize == 0 {
		status := s.Status()

		if status == SocketListenStatus || status == SocketClosedStatus || status == SocketCloseWaitStatus {
			return 0, io.EOF
		}

		return 0, nil
	}

	if availableSize > l {
		availableSize = l
	}

	pointer, err := s.d.Read16(s.getRegister(), SocketReceiveReadPointer)
	if err != nil {
		return 0, err
	}

	buf, err := s.read(pointer, availableSize)
	if err != nil {
		return 0, err
	}

	copy(p, buf) // ?

	pointer += availableSize

	if err := s.d.Write16(s.getRegister(), SocketReceiveReadPointer, pointer); err != nil {
		return int(availableSize), err
	}

	if err := s.execCommand(socketRecvCommand); err != nil {
		return int(availableSize), err
	}

	return int(availableSize), nil
}

func (s *Socket) write(data []byte) error {
	pointer, err := s.d.Read16(s.getRegister(), SocketTransmitWritePointer)
	if err != nil {
		return err
	}

	// offset := pointer & (s.BufferSize - 1)
	// address := offset + (uint16(s.Num)*s.BufferSize + 0x4000)

	address := pointer + uint16(s.Num)*s.BufferSize
	// Writing to
	// log.Printf("address is %d", address)
	if err := s.d.Write(BlockSelect(s.Num*4+2), Register(address), data); err != nil {
		return err
	}

	pointer += uint16(len(data))
	return s.d.Write16(s.getRegister(), SocketTransmitWritePointer, pointer)
}

func (s *Socket) Write(p []byte) (n int, err error) {
	l := len(p)
	if l > int(s.BufferSize) { // todo split buffer into chunks
		return 0, fmt.Errorf("%d exceeds available socket buffer (%d)", l, s.BufferSize)
	}

	status := s.Status()
	if status != SocketCloseWaitStatus && status != SocketEstablishedStatus && status != SocketMACRAWStatus {
		return 0, fmt.Errorf("write on closed socket (0x%.2x)", status)
	}

	var freeSize uint16
	for {
		freeSize, err = s.d.Read16(s.getRegister(), SocketTransmitFreeSize)
		if err != nil {
			return 0, fmt.Errorf("Reading SocketTransmitFreeSize:%w", SocketTransmitFreeSize)
		}
//		log.Printf("freeSize is %d", freeSize)
		if freeSize >= uint16(l) {
			break
		}
		status := s.Status()
		if status != SocketCloseWaitStatus && status != SocketEstablishedStatus && status != SocketMACRAWStatus {
			return 0, fmt.Errorf("Status is %#x, not one of SocketCloseWaitStatus, SocketEstablishedStatus, SocketMACRAWStatus:%w", status, io.EOF) // todo: better error
		}

		time.Sleep(time.Millisecond)
	}

	if err := s.write(p); err != nil {
		return 0, err
	}

	// make sure IRQ is clear.
	for {
		i, err := s.ReadInterrupt()
		if err != nil {
			return -1, fmt.Errorf("Read: can not read IRQ before xmit:%w", err)
		}

		if i&socketSendOkInterrupt != socketSendOkInterrupt {
			break
		}
		status := s.Status()
		if status == SocketClosedStatus {
			return 0, errors.New("socket closed during write")
		}

		if err := s.WriteInterrupt(socketSendOkInterrupt); err != nil {
			return -1, fmt.Errorf("Read: can not clear IRQ before xmit:%w", err)
		}

		time.Sleep(time.Millisecond)
	}

	// We will NEVER ARP.
	if err := s.execCommand(socketSendMacCommand /*socketSendCommand*/); err != nil {
		return 0, err
	}

	for {
		i, err := s.ReadInterrupt()
		if err != nil {
			return -1, fmt.Errorf("Read: can not read IRQ after xmit:%w", err)
		}
		if i&socketSendOkInterrupt == socketSendOkInterrupt {
			break
		}
		status := s.Status()
		if status == SocketClosedStatus {
			return 0, errors.New("socket closed during write")
		}

		time.Sleep(time.Millisecond)
	}

	if err := s.WriteInterrupt(socketSendOkInterrupt); err != nil {
		return -1, fmt.Errorf("Read: can not clear IRQ after xmit:%w", err)
	}

	if err := s.d.Write16(s.getRegister(), SocketTransmitWritePointer, 0); err != nil {
		return -1, fmt.Errorf("Read: zero SocketTransmitWritePointer: %w", err)
	}

	return l, nil
}
