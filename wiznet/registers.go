package wiznet

//go:generate stringer -type=BlockSelect,Register,ReadWrite,Size

type BlockSelect byte

const (
	CommonRegister BlockSelect = iota

	Socket0Register BlockSelect = 0b00001
	Socket0TxBuffer BlockSelect = 0b00010
	Socket0RxBuffer BlockSelect = 0b00011

	Socket1Register BlockSelect = 0b00101
	Socket1TxBuffer BlockSelect = 0b00110
	Socket1RxBuffer BlockSelect = 0b00111

	Socket2Register BlockSelect = 0b01001
	Socket2TxBuffer BlockSelect = 0b01010
	Socket2RxBuffer BlockSelect = 0b01011

	Socket3Register BlockSelect = 0b01101
	Socket3TxBuffer BlockSelect = 0b01110
	Socket3RxBuffer BlockSelect = 0b01111

	Socket4Register BlockSelect = 0b10001
	Socket4TxBuffer BlockSelect = 0b10010
	Socket4RxBuffer BlockSelect = 0b10011

	Socket5Register BlockSelect = 0b10101
	Socket5TxBuffer BlockSelect = 0b10110
	Socket5RxBuffer BlockSelect = 0b10111

	Socket6Register BlockSelect = 0b11001
	Socket6TxBuffer BlockSelect = 0b11010
	Socket6RxBuffer BlockSelect = 0b11011

	Socket7Register BlockSelect = 0b11101
	Socket7TxBuffer BlockSelect = 0b11110
	Socket7RxBuffer BlockSelect = 0b11111

	SocketMax = 7
)

type ReadWrite byte

const (
	ReadAccessMode  ReadWrite = 0x00 << 2
	WriteAccessMode ReadWrite = 0x01 << 2
)

type Size byte

const (
	VariableLengthMode Size = iota
	FixedLengthOneMode
	FixedLengthTwoMode
	FixedLengthFourMode
)

type Register uint16

const (
	ModeRegister                   Register = 0x0000 // 1 byte
	GatewayIPAddressRegister       Register = 0x0001 // 4 bytes
	SubnetMaskRegister             Register = 0x0005 // 4 bytes
	HardwareAddressRegister        Register = 0x0009 // 6 bytes
	IPAddressRegister              Register = 0x000F // 4 bytes
	InterruptLowLevelTimerRegister Register = 0x0013 // 2 bytes
	InterruptRegister              Register = 0x0015 // 1 byte
	InterruptMaskRegister          Register = 0x0016 // 1 byte
//	SocketInterruptRegister		Register = 0x0017 // 1 byte
	SocketInterruptMaskRegister Register = 0x0018 // 1 byte
	PHYConfigurationRegister    Register = 0x002E // 1 byte
	VersionRegister             Register = 0x0039 // 1 byte
)

const (
	SocketModeRegister               Register = 0x0000
	SocketCommandRegister            Register = 0x0001
	SocketInterruptRegister          Register = 0x0002
	SocketStatusRegister             Register = 0x0003
	SocketSourcePort                 Register = 0x0004 // 2 bytes
	SocketDestinationHardwareAddress Register = 0x0006 // 6 bytes
	SocketDestinationIPAddress       Register = 0x000C // 4 bytes
	SocketDestinationPort            Register = 0x0010 // 2 bytes
	SocketMaximumSegmentSize         Register = 0x0012 // 2 bytes
	SocketIPTOS                      Register = 0x0015
	SocketIPTTL                      Register = 0x0016
	SocketReceiveBufferSize          Register = 0x001E
	SocketTransmitBufferSize         Register = 0x001F
	SocketTransmitFreeSize           Register = 0x0020 // 2 bytes
	SocketTransmitReadPointer        Register = 0x0022 // 2 bytes
	SocketTransmitWritePointer       Register = 0x0024 // 2 bytes
	SocketReceiveReceivedSize        Register = 0x0026 // 2 bytes
	SocketReceiveReadPointer         Register = 0x0028 // 2 bytes
	SocketReceiveWritePointer        Register = 0x002A // 2 bytes
	SocketInterruptMask              Register = 0x002C
	SocketFragmentOffsetInIPHeader   Register = 0x002D // 2 bytes
	KeepaliveTimer                   Register = 0x002F
)
