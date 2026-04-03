// OSRB OSPO220224-2371
// This is a demo firmware for RP2350 attached to a Wiznet 5500
// It provides support for DHCP, TFTP, and ARP discover
// The firmware will then download the file received into the TFTP request answer
// The Maximum TFTP blocksize is 1452
// The Maximum TFTP Window size is 4 (due to w5500 limitation and stack needs on the uC)
// AUTHORS: verdun@hpe.com

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"machine"
	"net"
	"runtime"
	"strconv"
	"time"
	"w5500"
)

var (
	uart = machine.Serial
)

var Start time.Time
var End time.Time

type ethPacket struct {
	length int
	block  uint16
	frame  []byte
}

// TFTP opcodes
const (
	OpcodeData = 3
	OpcodeAck  = 4
)

// TFTPDataPacket represents a parsed TFTP DATA packet.
type TFTPDataPacket struct {
	Opcode uint16 // should be 3 for DATA
	Block  uint16 // block number
	Data   []byte // up to 512 bytes
}

var rxQueue chan bool
var txQueue chan ethPacket
var AckPkts chan ethPacket

type EthernetConfig struct {
	MAC        net.HardwareAddr `json:"mac"`
	MTU        int              `json:"mtu"`
	AutoNeg    bool             `json:"autoneg"`
	SpeedMbps  int              `json:"speed_mbps"` // 0 = auto
	DuplexFull bool             `json:"duplex_full"`
	VLAN       *int             `json:"vlan,omitempty"` // nil = no VLAN
}

// IPv4Config contains IPv4 addressing and routing info.
// WARNING Any allocation to net.IP must be done through temporary variables
// and copying the slice content to the original definition of the structure
// otherwise we can have stack / heap issue

type IPv4Config struct {
	Enabled     bool      `json:"enabled"`
	Address     net.IP    `json:"address"`    // single address
	PrefixLen   int       `json:"prefix_len"` // CIDR prefix length (e.g. 20)
	Netmask     net.IP    `json:"netmask"`
	Gateway     net.IP    `json:"gateway"`
	DNSServers  []net.IP  `json:"dns_servers"`
	LeaseExpiry time.Time `json:"lease_expiry,omitempty"` // DHCP lease expiry if applicable
	UseDHCP     bool      `json:"use_dhcp"`
}

type BootConfig struct {
	BootFile                     string           `json:"bootfile"`
	TFTPport                     uint16           `json:"tftpport"`
	TFTPServerport               uint16           `json:"tftpserverport"`
	TFTPblksize                  uint16           `json:"tftpblksize"`
	TFTPPreviousTransferredBytes int              `json:"tftpprevioustransferredbytes"`
	TFTPTransferredBytes         int              `json:"tftptrasnferredbytes"`
	BootServerIP                 net.IP           `json:"bootserverip"`
	BootServerMac                net.HardwareAddr `json:"bootservermac"`
}

type ClientNetworkConfig struct {
	Name     string         `json:"name"`
	Eth      EthernetConfig `json:"ethernet"`
	IPv4     IPv4Config     `json:"ipv4"`
	Hostname string         `json:"hostname,omitempty"`
	Boot     BootConfig     `json:"bootconfig"`
	// optional extra metadata
	MTUDiscovery bool `json:"mtu_discovery"`
}

var DefaultClientConfig = ClientNetworkConfig{
	Name: "pico2-01",
	Eth: EthernetConfig{
		MAC:        net.HardwareAddr{0x08, 0x00, 0x27, 0x7c, 0xc1, 0x88},
		MTU:        1500,
		AutoNeg:    true,
		SpeedMbps:  0,
		DuplexFull: true,
		VLAN:       nil,
	},
	IPv4: IPv4Config{
		Enabled:    true,
		Address:    net.ParseIP("10.10.10.100"),
		PrefixLen:  24,
		Netmask:    net.ParseIP("255.255.255.0"),
		Gateway:    net.ParseIP("10.10.10.1"),
		DNSServers: []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("8.8.4.4")},
		UseDHCP:    true,
	},
	Boot: BootConfig{
		BootFile:       string(""),
		TFTPport:       0,
		TFTPServerport: 0,
		BootServerIP:   net.ParseIP("10.10.10.100"),
		BootServerMac:  net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	},
	Hostname:     "pico2-01.local",
	MTUDiscovery: false,
}

const (
	ethHeaderLen    = 14
	vlanTagLen      = 4
	ipHeaderMinLen  = 20
	udpHeaderLen    = 8
	etherTypeIPv4   = 0x0800
	protoUDP        = 17
	dhcpMagicCookie = "\x63\x82\x53\x63"

	dhcpServerPort = 67
	dhcpClientPort = 68
)

// ParseTFTPDataPacket parses a UDP payload of a TFTP DATA packet.
// It returns an error if the payload is malformed or not a DATA packet.
//
//go:section .ramfuncs
var totalTime time.Duration
var totalPacket time.Duration
var totalInterrupt time.Duration
var totalWait time.Duration
var profile bool

func ParseTFTPDataPacket(buf []byte, n int) (TFTPDataPacket, error) {
	var begin, end time.Time
	var pkt TFTPDataPacket
	// Parse Ethernet header
	if profile {
		begin = time.Now()
	}
	pkt.Opcode = 0
	ethType := binary.BigEndian.Uint16(buf[12:14])

	offset := ethHeaderLen
	// We only care about IPv4
	if ethType != 0x0800 {
		return pkt, nil
	}
	// Ensure we have at least IP header
	if n < offset+ipHeaderMinLen+udpHeaderLen {
		return pkt, nil
	}

	ipHdrStart := offset
	verIhl := buf[ipHdrStart]
	ihl := int((verIhl & 0x0f) * 4)
	if ihl < ipHeaderMinLen {
		return pkt, nil
	}

	// Protocol must be UDP (17)
	proto := buf[ipHdrStart+9]
	if proto != 17 {
		return pkt, nil
	}

	udpStart := offset + ihl
	if udpStart+udpHeaderLen > n {
		return pkt, nil
	}

	udpPayload := buf[udpStart+udpHeaderLen : n]

	// Minimum length: 2 bytes opcode + 2 bytes block = 4
	if len(udpPayload) < 4 {
		return pkt, nil
	}

	op := binary.BigEndian.Uint16(udpPayload[0:2])
	pkt.Opcode = op
	if op != OpcodeData {
		return pkt, nil
	}

	block := binary.BigEndian.Uint16(udpPayload[2:4])
	data := udpPayload[4:]

	// vejmarie Weird needed to be added
	data = data[:len(data)-2]

	// TFTP data block must be <= 512 bytes
	if len(data) > int(DefaultClientConfig.Boot.TFTPblksize) {
		return pkt, errors.New("tftp: DATA payload exceeds expectations")
	}

	pkt.Opcode = op
	pkt.Block = block
	// copy data to avoid retaining reference to caller slice if desired
	pkt.Data = make([]byte, len(data))
	copy(pkt.Data, data)
	if profile {
		end = time.Now()
		totalTime = totalTime + end.Sub(begin)
	}
	//	log.Printf("TFTP parsing time ... %d", end.Sub(begin) )

	return pkt, nil
}

func BuildDHCPRequest(xid uint32, chaddr net.HardwareAddr, requestedIP, serverID net.IP) ([]byte, error) {
	if len(chaddr) != 6 {
		return nil, fmt.Errorf("chaddr must be 6 bytes")
	}

	// BOOTP header is 236 bytes:
	// op(1), htype(1), hlen(1), hops(1), xid(4), secs(2), flags(2),
	// ciaddr(4), yiaddr(4), siaddr(4), giaddr(4), chaddr(16), sname(64), file(128)
	packet := make([]byte, 236)

	// op: 1 for BOOTREQUEST
	packet[0] = 1
	// htype: 1 for ethernet
	packet[1] = 1
	// hlen: hardware address length (6 for MAC)
	packet[2] = 6
	// hops: 0
	packet[3] = 0
	// xid:
	binary.BigEndian.PutUint32(packet[4:8], xid)
	// secs: 0
	binary.BigEndian.PutUint16(packet[8:10], 0x0001) // secs: 1
	// flags: 0x8000 to indicate broadcast (optional)
	// flags: 0x0000 to indicate unicast (optional)

	binary.BigEndian.PutUint16(packet[10:12], 0x0000) // broadcast flag

	// ciaddr, yiaddr, siaddr, giaddr left zero

	// chaddr (client hardware addr): first 16 bytes reserved, we put MAC in first 6
	copy(packet[28:34], chaddr)

	// sname and file (next 64 + 128 bytes) are zero by default

	// Now append DHCP magic cookie + options
	// magic cookie 99,130,83,99
	opts := []byte{99, 130, 83, 99}

	// DHCP Message Type = DHCPREQUEST (option 53 = 3)
	opts = append(opts, 53, 1, 3)

	// Client Identifier (option 61): type (1 for HW type Ethernet) + MAC
	opts = append(opts, 61, byte(1+len(chaddr)))
	opts = append(opts, 1)         // hw type 1 (ethernet)
	opts = append(opts, chaddr...) // client MAC

	// Requested IP (option 50) if provided
	if requestedIP != nil && requestedIP.To4() != nil {
		opts = append(opts, 50, 4)
		opts = append(opts, requestedIP.To4()...)
	}

	// Server Identifier (option 54) if provided
	if serverID != nil && serverID.To4() != nil {
		opts = append(opts, 54, 4)
		opts = append(opts, serverID.To4()...)
	}

	// Parameter Request List (option 55): ask for common options
	prl := []byte{
		1,  // subnet mask
		2,  // Time Offset
		3,  // router
		6,  // DNS
		12, // Hostname
		15, // domain name
		17,
		26,
		28, // broadcast address
		33,
		40,
		41,
		42,
		119,
		121,
		249,
		252,
	}
	// We must request option 51 (lease time) and
	// 66 and 67 to get the server boot parameter
	// for the client
	leaseTime := []byte{0x0, 0x0, 0x0, 0x0}
	opts = append(opts, 51, byte(0x4))
	opts = append(opts, leaseTime...)

	bootfile := "INI="
	opts = append(opts, 67, 0x04)
	opts = append(opts, bootfile...)
	bootserver := "unknown"
	opts = append(opts, 66, 0x07)
	opts = append(opts, bootserver...)

	opts = append(opts, 55, byte(len(prl)))
	opts = append(opts, prl...)

	maxSize := []byte{0x2, 0x40}
	opts = append(opts, 57, byte(len(maxSize)))
	opts = append(opts, maxSize...)

	hostname := "pico2"
	opts = append(opts, 12, byte(len(hostname)))
	opts = append(opts, hostname...)

	// End option
	opts = append(opts, 255)

	// Pad to minimum DHCP size? DHCP minimum is BOOTP(236) + cookie(4) + options variable.
	// Some servers expect packet to be at least 300-ntus; we can optionally pad to 300 bytes.
	packet = append(packet, opts...)

	// RFC says options can be padded with zeros after end; some clients pad to 300 bytes.
	if len(packet) < 300 {
		padding := make([]byte, 300-len(packet))
		packet = append(packet, padding...)
	}

	return packet, nil
}

// IPv4 checksum (for header)

// computeIPv4Checksum calculates IPv4 header checksum.
//
//go:section .ramfuncs
func computeIPv4Checksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i < len(hdr); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(hdr[i : i+2]))
	}
	for (sum >> 16) != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// pseudo header checksum for UDP (for IPv4)
//
//go:section .ramfuncs
func udpChecksum(srcIP, dstIP net.IP, udp []byte) uint16 {
	var sum uint32
	// pseudo-header
	sum += uint32(srcIP[0])<<8 | uint32(srcIP[1])
	sum += uint32(srcIP[2])<<8 | uint32(srcIP[3])
	sum += uint32(dstIP[0])<<8 | uint32(dstIP[1])
	sum += uint32(dstIP[2])<<8 | uint32(dstIP[3])
	sum += uint32(0x00<<8 | protoUDP)
	sum += uint32(len(udp))

	// UDP header + data
	for i := 0; i+1 < len(udp); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(udp[i : i+2]))
	}
	if len(udp)%2 == 1 {
		sum += uint32(udp[len(udp)-1]) << 8
	}

	for (sum >> 16) != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func setmac(b, mac []byte) {
	copy(b[6:], mac)
	copy(b[70:], mac)
}

// BuildEthernetIPv4UDP builds an Ethernet frame containing an IPv4/UDP packet.
// srcMAC, dstMAC: 6 bytes each
// srcIP, dstIP: net.IP (To4)
// srcPort, dstPort: UDP ports
// payload: UDP payload (TFTP RRQ)
//
//go:section .ramfuncs
func BuildEthernetIPv4UDP(srcMAC, dstMAC net.HardwareAddr, srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) ([]byte, error) {
	if len(srcMAC) != 6 || len(dstMAC) != 6 {
		return nil, errors.New("MAC addresses must be 6 bytes")
	}
	sip := srcIP.To4()
	dip := dstIP.To4()
	if sip == nil || dip == nil {
		return nil, errors.New("IPs must be IPv4")
	}

	// Ethernet header: 14 bytes
	frame := make([]byte, 14)

	// dst MAC
	copy(frame[0:6], dstMAC)
	copy(frame[6:12], srcMAC)
	binary.BigEndian.PutUint16(frame[12:14], etherTypeIPv4)

	// IPv4 header (20 bytes, no options)
	// ipStart := 14
	ipLen := 20
	totalLen := ipLen + 8 + len(payload) // IP header + UDP header (8) + payload
	ip := make([]byte, ipLen)
	ip[0] = 0x45 // Version 4, IHL=5
	ip[1] = 0x00 // DSCP/ECN
	binary.BigEndian.PutUint16(ip[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(ip[4:6], 0x0000) // ID
	binary.BigEndian.PutUint16(ip[6:8], 0x4000) // Flags (don't fragment)
	ip[8] = 64                                  // TTL
	ip[9] = protoUDP
	copy(ip[12:16], sip)
	copy(ip[16:20], dip)
	// checksum
	binary.BigEndian.PutUint16(ip[10:12], computeIPv4Checksum(ip))

	// UDP header + payload
	udp := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], uint16(8+len(payload)))
	// checksum fill later
	copy(udp[8:], payload)
	cs := udpChecksum(sip, dip, udp)
	binary.BigEndian.PutUint16(udp[6:8], cs)

	// Compose frame
	frame = append(frame, ip...)
	frame = append(frame, udp...)
	return frame, nil
}

func parseARP(data []byte, n int) int {
	off := 14
	if len(data) < off+8 {
		return 0
	}
	// hwType := binary.BigEndian.Uint16(data[off : off+2])
	// protoType := binary.BigEndian.Uint16(data[off+2 : off+4])
	hwLen := data[off+4]
	protoLen := data[off+5]
	op := binary.BigEndian.Uint16(data[off+6 : off+8])
	off += 8

	// verify remaining length for addresses
	expectedAddrLen := int(hwLen)*2 + int(protoLen)*2
	if len(data) < off+expectedAddrLen {
		return 0
	}
	// Extract addresses in order:
	// sender hardware (hwLen), sender proto (protoLen),
	// target hardware (hwLen), target proto (protoLen)
	shw := net.HardwareAddr(nil)
	if hwLen > 0 {
		shw = net.HardwareAddr(data[off : off+int(hwLen)])
	}
	off += int(hwLen)

	sp := net.IP(nil)
	if protoLen > 0 {
		sp = net.IP(data[off : off+int(protoLen)])
	}

	// check the arp op
	if op != 2 {
		// This is a request
		if op != 1 {
			return 0
		}
		return 1
	}

	// Now we can store the data
	// and generate a random port number to
	// accept TFTP traffic
	srcPort, _ := RandomUDPPort()
	DefaultClientConfig.Boot.TFTPport = srcPort
	DefaultClientConfig.Boot.TFTPServerport = 0
	// vejmarie looks like the allocation of the mac that way doesn't work
	// DefaultClientConfig.Boot.BootServerMac = shw
	// it creates a mess into the heap so better to copy it as one variable is 6 bytes and the
	// target has a variable length
	for k := range shw {
		DefaultClientConfig.Boot.BootServerMac[k] = shw[k]
	}
	DefaultClientConfig.Boot.BootServerIP = sp
	return 2
}

func parseDHCP(buf []byte, n int) {
	// Parse Ethernet header

	ethType := binary.BigEndian.Uint16(buf[12:14])

	offset := ethHeaderLen
	var vlanPresent bool
	var vlanID int
	var vlanPCP uint8
	var vlanDEI uint8
	// Check for 802.1Q tag (TPID 0x8100). Also handle 0x88a8 (QinQ) if desired.
	if ethType == 0x8100 || ethType == 0x88a8 {
		// Ensure buffer long enough for VLAN tag + inner ethertype
		if n < ethHeaderLen+vlanTagLen {
			return
		}
		vlanPresent = true
		// VLAN TCI is 2 bytes at offset 14..15
		tci := binary.BigEndian.Uint16(buf[14:16])
		vlanID = int(tci & 0x0FFF)
		vlanPCP = uint8((tci >> 13) & 0x07)
		vlanDEI = uint8((tci >> 12) & 0x01)
		// inner ethertype after TCI is at bytes 16:18
		ethType = binary.BigEndian.Uint16(buf[16:18])
		offset += vlanTagLen
	}
	// We only care about IPv4
	if ethType != 0x0800 {
		return
	}
	// Ensure we have at least IP header
	if n < offset+ipHeaderMinLen+udpHeaderLen {
		return
	}

	ipHdrStart := offset
	verIhl := buf[ipHdrStart]
	ihl := int((verIhl & 0x0f) * 4)
	if ihl < ipHeaderMinLen {
		return
	}
	if n < offset+ihl+udpHeaderLen {
		return
	}

	// Protocol must be UDP (17)
	proto := buf[ipHdrStart+9]
	if proto != 17 {
		return
	}

	udpStart := offset + ihl
	if udpStart+udpHeaderLen > n {
		return
	}
	srcPort := binary.BigEndian.Uint16(buf[udpStart : udpStart+2])
	dstPort := binary.BigEndian.Uint16(buf[udpStart+2 : udpStart+4])

	// Filter on DHCP ports (server/client)
	if !((srcPort == dhcpServerPort && dstPort == dhcpClientPort) || (srcPort == dhcpClientPort && dstPort == dhcpServerPort)) {
		return
	}

	udpPayload := buf[udpStart+udpHeaderLen : n]
	if len(udpPayload) < 240 {
		// minimal BOOTP (236) + cookie (4)
		return
	}
	// Check DHCP magic cookie at offset 236 inside BOOTP (i.e., payload[236:240])
	if string(udpPayload[236:240]) != dhcpMagicCookie {
		return
	}

	// Parse BOOTP
	op := udpPayload[0]
	if !(op == 1 || op == 2) {
		return
	}
	yiaddr := net.IP(udpPayload[16:20])

	// Options start after BOOTP (236) + cookie (4) = 240
	options := udpPayload[240:]

	if vlanPresent {
		fmt.Printf("VLAN: present=true id=%d pcp=%d dei=%d\n", vlanID, vlanPCP, vlanDEI)
	} else {
		fmt.Printf("VLAN: present=false\n")
	}

	localIP := net.IPv4(yiaddr[0], yiaddr[1], yiaddr[2], yiaddr[3])
	for k := range localIP {
		DefaultClientConfig.IPv4.Address[k] = localIP[k]
	}

	if v := parseDHCPOption(options, 51); v != nil && len(v) >= 4 {
		// We shall have received a leaseTime
		// but that option is 4 bytes as this is the lease in secod
		// So we need to print it that will be highly frustrating
		leaseTime := binary.BigEndian.Uint32(v)
		DefaultClientConfig.IPv4.LeaseExpiry = time.Now()
		DefaultClientConfig.IPv4.LeaseExpiry.Add(time.Second * time.Duration(leaseTime))
	}

	if v := parseDHCPOption(options, 54); v != nil && len(v) >= 4 {
		fmt.Printf("Server IP: %s\n", net.IP(v).String())
	}
	if v := parseDHCPOption(options, 1); v != nil && len(v) >= 4 {
		DefaultClientConfig.IPv4.Netmask = net.IP(v)
	}
	if v := parseDHCPOption(options, 3); v != nil && len(v) >= 4 {
		// vejmarie THE BUG IS HERE IT LOOKS LIKE !!!
		// DefaultClientConfig.IPv4.Gateway = net.IP(v)
		myGw := net.IPv4(v[0], v[1], v[2], v[3])
		for k := range myGw {
			DefaultClientConfig.IPv4.Gateway[k] = myGw[k]
		}
	}
	if v := parseDHCPOption(options, 6); v != nil && len(v) >= 4 {
		DefaultClientConfig.IPv4.DNSServers = nil
		for i := 0; i+4 <= len(v); i += 4 {
			DefaultClientConfig.IPv4.DNSServers = append(DefaultClientConfig.IPv4.DNSServers, net.IP(v[i:i+4]))
		}
	}
	if v := parseDHCPOption(options, 66); v != nil && len(v) >= 4 {
		// Warning it can be an hostname while we are looking for an IP
		DefaultClientConfig.IPv4.Gateway = net.ParseIP(string(v))
	}
	if v := parseDHCPOption(options, 67); v != nil && len(v) >= 4 {
		DefaultClientConfig.Boot.BootFile = string(v)
	}
}

func parseDHCPOption(options []byte, code byte) []byte {
	i := 0
	for i < len(options) {
		c := options[i]
		if c == 255 { // end
			break
		}
		if c == 0 { // pad
			i++
			continue
		}
		if i+1 >= len(options) {
			break
		}
		l := int(options[i+1])
		if i+2+l > len(options) {
			break
		}
		if c == code {
			return options[i+2 : i+2+l]
		}
		i += 2 + l
	}
	return nil
}

func parseDHCPMessageType(options []byte) string {
	v := parseDHCPOption(options, 53)
	if v == nil || len(v) == 0 {
		return "Unknown"
	}
	switch v[0] {
	case 1:
		return "DHCPDISCOVER"
	case 2:
		return "DHCPOFFER"
	case 3:
		return "DHCPREQUEST"
	case 4:
		return "DHCPDECLINE"
	case 5:
		return "DHCPACK"
	case 6:
		return "DHCPNAK"
	case 7:
		return "DHCPRELEASE"
	case 8:
		return "DHCPINFORM"
	default:
		return fmt.Sprintf("Unknown(%d)", v[0])
	}
}

func BuildARPpacket(srcMAC net.HardwareAddr, srcIP net.IP, dstIP net.IP, opcode uint16, targetMac net.HardwareAddr) ([]byte, error) {

	// Validate inputs
	if len(srcMAC) != 6 {
		return nil, errors.New("srcMAC must be 6 bytes")
	}
	srcIP4 := srcIP.To4()
	if srcIP4 == nil {
		return nil, errors.New("srcIP must be an IPv4 address")
	}
	dstIP4 := dstIP.To4()
	if dstIP4 == nil {
		return nil, errors.New("dstIP must be an IPv4 address")
	}

	// Ethernet header: 14 bytes
	// - dst MAC (6) : broadcast for ARP request
	// - src MAC (6)
	// - EtherType (2) : 0x0806 for ARP

	frame := make([]byte, 14+28) // 14 bytes Ethernet header + 28 bytes ARP payload

	// Destination: broadcast
	for i := 0; i < 6; i++ {
		frame[i] = 0xff
	}
	copy(frame[0:6], targetMac)
	// Source MAC
	copy(frame[6:12], srcMAC)
	// EtherType = 0x0806 (ARP)
	binary.BigEndian.PutUint16(frame[12:14], 0x0806)

	// ARP payload starts at offset 14
	arp := frame[14:]
	// Hardware type: 1 (Ethernet) 2 bytes
	binary.BigEndian.PutUint16(arp[0:2], 0x0001)
	// Protocol type: 0x0800 (IPv4) 2 bytes
	binary.BigEndian.PutUint16(arp[2:4], 0x0800)
	// Hardware size: 6
	arp[4] = 6
	// Protocol size: 4
	arp[5] = 4
	// Opcode: 1 for request, 2 for reply
	binary.BigEndian.PutUint16(arp[6:8], opcode)
	// Sender hardware address (SHA) 6 bytes
	copy(arp[8:14], srcMAC)
	// Sender protocol address (SPA) 4 bytes
	copy(arp[14:18], srcIP4)
	// Target hardware address (THA) 6 bytes -> zero for request
	for i := 18; i < 24; i++ {
		arp[i] = 0x00
	}
	// Target protocol address (TPA) 4 bytes
	copy(arp[24:28], dstIP4)
	return frame, nil
}

// BuildTFTPRRQ builds a TFTP RRQ packet bytes for a given filename and mode ("octet").
func BuildTFTPRRQ(filename, mode string, chunk uint16) []byte {
	fmt.Printf("TFTP Transfer starts\n")
	var blksize string
	totalTime = 0
	totalPacket = 0
	totalInterrupt = 0
	totalWait = 0
	blksize = strconv.Itoa(int(chunk))
	Start = time.Now()
	DefaultClientConfig.Boot.TFTPServerport = 0
	// RRQ opcode = 1
	b := make([]byte, 2+len(filename)+1+len(mode)+1+len("blksize")+1+len(blksize)+1)
	binary.BigEndian.PutUint16(b[0:2], 1)
	pos := 2
	copy(b[pos:], []byte(filename))
	pos += len(filename)
	b[pos] = 0
	pos++
	copy(b[pos:], []byte(mode))
	pos += len(mode)
	b[pos] = 0
	pos++
	copy(b[pos:], []byte("blksize"))
	pos += len("blksize")
	b[pos] = 0
	pos++
	// binary.BigEndian.PutUint16(packet[8:10], 0x0001)
	copy(b[pos:], []byte(blksize))
	pos += len(blksize)
	b[pos] = 0
	// We need to setup the blksize
	DefaultClientConfig.Boot.TFTPblksize = chunk
	return b
}

// BuildAckFromDataPacket builds a TFTP ACK packet for the provided DATA packet.
// Returns a 4 byte slice: [0,4][block hi][block lo].
//
//go:section .ramfuncs
func BuildAckFromDataPacket(pkt TFTPDataPacket) ([]byte, error) {
	if pkt.Opcode != OpcodeData && pkt.Opcode != 6 {
		log.Printf("tftp: provided packet is not DATA %d", pkt.Opcode)
		return nil, errors.New("tftp: provided packet is not DATA")
	}
	ack := make([]byte, 4)
	binary.BigEndian.PutUint16(ack[0:2], OpcodeAck)
	binary.BigEndian.PutUint16(ack[2:4], pkt.Block)
	return ack, nil
}

var count int

func handleTxQueue(s *wiznet.Socket, DefaultClientConfig ClientNetworkConfig) {
	count = 0
	for {
		txFrame := <-txQueue
		s.Write(txFrame.frame[:txFrame.length])
	}
}

func RandomUDPPort() (uint16, error) {
	const min = 1024
	const max = 65535
	const rangeLen = max - min + 1 // 64512

	// To avoid modulo bias, accept only values in [0, maxAccept-1]
	// where maxAccept is the largest multiple of rangeLen <= 65536.
	// 65536 % 64512 = 1024, so maxAccept = 65536 - 1024 = 64512.
	const maxAccept = 65536 - (65536 % rangeLen) // 64512
	for {
		test, _ := machine.GetRNG()
		if uint16(test) < min {
			continue
		}
		return uint16(test), nil
	}
}

var tx [1516]byte

var rx [16384]byte

//go:section .ramfuncs
func main() {
	var err error

	txQueue = make(chan ethPacket, 2)
	rxQueue = make(chan bool, 2)
	AckPkts = make(chan ethPacket, 2)
	time.Sleep(2 * time.Second)
	mac := DefaultClientConfig.Eth.MAC
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Example transaction ID
	var xid uint32 = 0x3903F326

	// If you received a DHCPOFFER and want to send REQUEST for the offered IP:
	requestedIP := DefaultClientConfig.IPv4.Address
	var serverID net.IP

	payload, err := BuildDHCPRequest(xid, mac, requestedIP, serverID)
	if err != nil {
		fmt.Println("build error:", err)
	}
	profile = true

	srcIP := net.IPv4zero.To4()
	dstIP := net.IPv4bcast.To4()
	dstMac := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	var srcPort, dstPort uint16
	dstPort = 67
	srcPort = 68
	frame, err := BuildEthernetIPv4UDP(mac, dstMac, srcIP, dstIP, srcPort, dstPort, payload)
	if err != nil {
		log.Fatalf("build frame: %v", err)
	}

	copy(tx[:], frame)

	// use default settings for UART

	uart.Configure(machine.UARTConfig{})
	uart.Write([]byte("Echo console enabled. Type something then press enter:\r\n"))
	time.Sleep(2 * time.Second)

	cs, err := wiznet.New(&net.HardwareAddr{})
	if err != nil {
		log.Fatalf("%v", err)
	}

	cs.SetHardwareAddr(mac)
	macaddr, err := cs.GetHardwareAddr()
	if err != nil {
		log.Printf("gethardwareaddr: %v", err)
	}
	fmt.Printf("Macaddr is set to %s\n", macaddr)
	// slices.Equal worketh not.
	if (macaddr[0] == 0) && (macaddr[1] == 0) && (macaddr[2] == 0 && (macaddr[3] == 0) && macaddr[4] == 0) && (macaddr[5] == 0) {
		copy(macaddr, mac)
		copy(macaddr, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xef})
		if err := cs.SetHardwareAddr(macaddr); err != nil {
			log.Printf("=========> OH NO! Trying to set: %s gets error %v", macaddr, err)
		}
	}
	macaddr, err = cs.GetHardwareAddr()

	s, err := cs.NewSocket(0)
	if err != nil {
		log.Fatalf("socket for %v:%v", cs, err)
	}

	s.BufferSize = 16 * 1024
	if err := s.Open("raw", 0); err != nil {
		log.Fatal("%v", err)
	}
	// The socket is open
	// So we can now send from it
	go handleTxQueue(s, DefaultClientConfig)
	// And we shall be able to read from it
	// The packet are going to be received and processed into different threads
	if cs.GetPHYConfiguration().Is100MbpsLink() {
		fmt.Printf("100mbps Link detected\n")
	}

	input := make([]byte, 64)
	i := 0
	if true {
		s.SetInterruptMask()
		// This go routine is going to prepare next acknowledge packet while
		// we wait for interrupt because otherwise this is useless cycles
		// The problem is that we do not know the Port which is changing between each transfer
		// But is constant between all transfer !
		// so we might be building some ack packet which will be wasted, but this has proven to
		// be efficient. The DefaultClientConfig.Boot.TFTPServerport is reset to 0 between transfer
		go func() {
			var pktID uint16
			var txFrame ethPacket
			pktID = 1
			for {

				var pkt TFTPDataPacket
				if DefaultClientConfig.Boot.TFTPServerport != 0 {
					pkt.Opcode = 3
					pkt.Block = pktID
					pktID = pktID + 1
					srcP := DefaultClientConfig.Boot.TFTPServerport
					dstP := DefaultClientConfig.Boot.TFTPport
					ackPkt, _ := BuildAckFromDataPacket(pkt)
					srcPort = DefaultClientConfig.Boot.TFTPport
					frame, err := BuildEthernetIPv4UDP(mac, DefaultClientConfig.Boot.BootServerMac,
						DefaultClientConfig.IPv4.Address,
						DefaultClientConfig.IPv4.Gateway, dstP, srcP, ackPkt)
					if err != nil {
						log.Fatalf("build frame: %v", err)
					}
					txFrame.length = len(frame)
					txFrame.block = pktID
					txFrame.frame = frame
					// txQueue <- txFrame
					AckPkts <- txFrame
				} else {
					pktID = 1
				}
				// We need to explicitely call the scheduler or we will be hanging for ever as this is pure compute loop
				runtime.Gosched()
			}
		}()
		go func() {
			var txFrame ethPacket
			// Configure LED
			led := machine.LED
			led.Configure(machine.PinConfig{Mode: machine.PinOutput})
			interruptPin := machine.GPIO21
			interruptPin.Configure(machine.PinConfig{Mode: machine.PinInput})
			interruptPin.SetInterrupt(machine.PinFalling, func(p machine.Pin) {
				rxQueue <- true
			})
			for {
				// Instead of sleeping we just read from the rxQueue if we received
				// and interrupt telling us that an event occurs at the MAC level
				var waitPacketE time.Time
				var waitPacketS time.Time
				var interuptE time.Time
				var interuptS time.Time
				var packetE time.Time
				var packetS time.Time
				if profile {
					waitPacketS = time.Now()
				}
				//				time.Sleep(2 * time.Second)
				_ = <-rxQueue
				if profile {
					waitPacketE = time.Now()
					totalWait = totalWait + waitPacketE.Sub(waitPacketS)
					interuptE = time.Now()
				}
				sirq, err := s.ReadInterrupt()

				if err != nil {
					log.Printf("reading socket irq:%v", err)
				}
				if sirq&4 == 0 {
					continue
				}
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)

				/*                		if ( memStats.HeapInuse > 500000 ) {
				        count = count + 1
				}
				*/

				if err := s.WriteInterrupt(sirq); err != nil {
					log.Printf("clearing socket irq:%v", err)
				}
				if profile {
					interuptS = time.Now()
					totalInterrupt = totalInterrupt + interuptS.Sub(interuptE)
					// done
					packetE = time.Now()
				}
				// reading the packet

				n, err := s.Read(rx[:])
				//				log.Printf("Read ... %d bytes\n", n)
				if profile {
					packetS = time.Now()
					totalPacket = totalPacket + packetS.Sub(packetE)
				}

				if n < ethHeaderLen+ipHeaderMinLen+udpHeaderLen {
					log.Printf("Packet received with %d bytes", n)
					continue
				}
				// The buffer can contain multiple packets
				// which is enhancing the performance and this is in the case
				// the TFTP server is supporting windowing

				packetCount := n / 1500

				if packetCount == 0 {
					packetCount = 1
				}
				for i := 0; i < packetCount; i++ {
					// We shall check what kind of packet we are receiving
					var ethType uint16
					var pktIndex int
					pktIndex = i * 1500
					ethType = binary.BigEndian.Uint16(rx[pktIndex+14 : pktIndex+16])
					switch ethType {
					case 0x0800:
						// We must check the src port and destination
						// port
						udpStart := 14 + 20 + 2
						udp := rx[pktIndex+udpStart : pktIndex+udpStart+4]
						srcP := binary.BigEndian.Uint16(udp[0:2])
						dstP := binary.BigEndian.Uint16(udp[2:4])
						if srcP == 67 && dstP == 68 {
							// We process a DHCP request
							// within an IPv4 / UDP packet
							// parseDHCP(rx[pkIndex + 2:],n, &DefaultClientConfig)
							parseDHCP(rx[pktIndex+2:], n)
							// So now we can check the ARP state
							// And then we need to parse the answer ...
							// that will be complex with the current processing
							// 0x0001 is for a request
							// 0x0002 is for a reply

							targetMAC := net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
							// vejmarie I am not using the global variable
							mygateway := DefaultClientConfig.IPv4.Gateway
							frame, _ := BuildARPpacket(DefaultClientConfig.Eth.MAC,
								DefaultClientConfig.IPv4.Address,
								mygateway, 0x0001, targetMAC)

							// We need to keep the DHCP request up to date
							// And send a renew packet
							// based on the lease time

							// Let's send the ARP request
							txFrame.length = len(frame)
							txFrame.frame = frame
							txQueue <- txFrame
							continue
						}
						// We can process also TFTP incoming packet which are Ipv4 / UDP
						// DefaultClientConfig.Boot.TFTPport
						// The srcP shall be saved somewhere
						if DefaultClientConfig.Boot.TFTPServerport == 0 {
							DefaultClientConfig.Boot.TFTPServerport = srcP
						}
						if srcP == DefaultClientConfig.Boot.TFTPServerport && dstP == DefaultClientConfig.Boot.TFTPport {
							// This is where we parse the pkt, but we can probably build an answer
							// in parallel for the next packet
							// last paremeter was n
							// and it shall be either 1500 or n - pktIndex
							lastByte := pktIndex + 1500
							pktSize := 1500
							if lastByte > n {
								lastByte = n
								pktSize = (n - pktIndex)
							}
							pkt, _ := ParseTFTPDataPacket(rx[pktIndex+2:lastByte], pktSize)
							// We can print a # every 128kB
							DefaultClientConfig.Boot.TFTPTransferredBytes += len(pkt.Data)
							if DefaultClientConfig.Boot.TFTPTransferredBytes >
								(DefaultClientConfig.Boot.TFTPPreviousTransferredBytes + 128*1024) {
								uart.Write([]byte("#"))
								DefaultClientConfig.Boot.TFTPPreviousTransferredBytes = DefaultClientConfig.Boot.TFTPTransferredBytes
							}
							switch pkt.Opcode {
							case 3:
								// So now we can send an answer to get the next one
								// it is created into a specific go routine
								// runtime.Gosched()
								answerPkt := <-AckPkts
								// we might have to purge up to the time we are getting
								// a packet which is matching the expected it
								for (answerPkt.block - 1) != pkt.Block {
									answerPkt = <-AckPkts
								}
								if len(pkt.Data) < int(DefaultClientConfig.Boot.TFTPblksize) {
									led.Set(false)
									End = time.Now()
									uart.Write([]byte("\n"))
									fmt.Println("Transfer time ", float64(End.Sub(Start))/float64(time.Second))
									fmt.Printf("Data transferred: %d bytes\n", DefaultClientConfig.Boot.TFTPTransferredBytes)
									fmt.Printf("Bandwidth: %.2f MB/s\n", (float64(DefaultClientConfig.Boot.TFTPTransferredBytes)/
										(1024.0*1024.0))/(float64(End.Sub(Start))/float64(time.Second)))
									fmt.Printf("TFTP transfer done\n")
									fmt.Printf("Memory full : %d times\n", count)
									if profile {
										fmt.Printf("Time to parse TFTP packet: %.2f\n", float64(totalTime)/float64(time.Second))
										fmt.Printf("Time to waiting TFTP packet: %.2f\n", float64(totalWait)/float64(time.Second))
										fmt.Printf("Time to reset interrupt: %.2f\n", float64(totalInterrupt)/float64(time.Second))
										fmt.Printf("Time to read Packet: %.2f\n", float64(totalPacket)/float64(time.Second))
									}
									count = 0
									DefaultClientConfig.Boot.TFTPServerport = 0
									DefaultClientConfig.Boot.TFTPport = 0
								}
								txQueue <- answerPkt
							case 6:
								// ok for the moment we ack the data transfer size but we shall
								// check if it match our needs !
								pkt.Block = 0
								ackPkt, _ := BuildAckFromDataPacket(pkt)
								srcPort = DefaultClientConfig.Boot.TFTPport
								frame, err := BuildEthernetIPv4UDP(mac, DefaultClientConfig.Boot.BootServerMac,
									DefaultClientConfig.IPv4.Address,
									DefaultClientConfig.IPv4.Gateway, dstP, srcP, ackPkt)
								if err != nil {
									log.Fatalf("build frame: %v", err)
								}
								txFrame.length = len(frame)
								txFrame.frame = frame
								txQueue <- txFrame
							default:
								log.Printf("unsupported Opcode: %d", pkt.Opcode)
							}
						}
					case 0x0806:
						// This is an ARP packet
						// We need to parse it now
						switch parseARP(rx[2:], n) {
						case 1:
							// We need to answer to an ARP request by sending an answer
							// 0x0002 is the reply
							// but we need to answer to only the relevant machine and not broadcast
							targetMAC := net.HardwareAddr(rx[24:30])
							frame, _ := BuildARPpacket(DefaultClientConfig.Eth.MAC,
								DefaultClientConfig.IPv4.Address,
								DefaultClientConfig.IPv4.Gateway, 0x0002, targetMAC)
							txFrame.length = len(frame)
							txFrame.frame = frame
							txQueue <- txFrame
						case 2:
							// If everything is ok, we can now
							// initiate a TFTP transfer
							// TFTP will be then processed as receiving packed
							// Build a simple TFTP RRQ for file "test.bin" in octet mode.
							led.Set(true)
							rrq := BuildTFTPRRQ("bootme", "octet", 1452)
							DefaultClientConfig.Boot.TFTPPreviousTransferredBytes = 0
							DefaultClientConfig.Boot.TFTPTransferredBytes = 0
							srcPort = DefaultClientConfig.Boot.TFTPport
							frame, err := BuildEthernetIPv4UDP(mac, DefaultClientConfig.Boot.BootServerMac, DefaultClientConfig.IPv4.Address,
								DefaultClientConfig.Boot.BootServerIP, srcPort, 69, rrq)
							if err != nil {
								log.Fatalf("build frame: %v", err)
							}
							println("sending TFTP request")
							txFrame.length = len(frame)
							txFrame.frame = frame
							txQueue <- txFrame
						default:
						}
					default:
						// We don't know what it is
						// we drop it
					}
					// end loop on packet
				}
			}
		}()
	}
	fmt.Println("Please press enter\n")
	for {
		if uart.Buffered() > 0 {
			data, err := uart.ReadByte()
			if err != nil {
				log.Printf("ReadByte:%v", err)
				continue
			}
			switch data {
			case 13:
				// return key
				uart.Write([]byte("\r\n"))
				uart.Write([]byte("You typed: "))
				uart.Write(input[:i])
				uart.Write([]byte("\r\n"))
				i = 0
			default:
				// just echo the character
				uart.WriteByte(data)
				input[i] = data
				i++
			}

			var newFrame ethPacket
			newFrame.length = len(frame)
			newFrame.frame = frame
			// Ok I am sending here a DHCP request packet
			txQueue <- newFrame
		}
		time.Sleep(200 * time.Millisecond)
	}
}
