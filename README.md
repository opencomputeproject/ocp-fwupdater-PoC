This code is implementing a basic TFTP client through an RP2350 board
attached to a Wiznet W5500 SPI ethernet adapter

Only ipv4 is supported, and no VLAN

The primary goal is to prove that a firmware storage device (like a spi-nor
which contains BMC code, or ROM ) can be fully validated and updated
before system startup using low cost micro controllers, Ethernet and industry 
standard protocols through high level languages like golang without any human intervention.

The PoC relies on a microcontroller connected to 
- 1 SPI to Ethernet adapter (in our example a Wiznet W5500)
- 1 SPI MUX which will bridge a SPI-NOR between the uC and a target system

The uC starts and issue a DHCP request, then an ARP request, then a TFTP File Request,
and will (not yet implemented) compare the TFTP received data with the SPI-NOR content.
If a missmatch occurs, it will update the SPI-NOR content. When the whole process is done
the uC enter DeepSleep mode and wait for next reset. It switch the SPI MUX to the control
of the main system.

Ultimately, the protocol implemented will be using DHCP, ARP, TFTP to download a secondart
stage firmware, and reset the uC in SRAM to execute that secondary firmware which
will retrieve the SPI-NOR content through HTTP request using TCP connection in 
secure or unsecure mode to update the SPI-NOR.

We do not want to integrate the TCP stack into the potentially immutable uC ROM, only
simple protocol shall be used to reduce attack footprint.

To run this code:

It requires a DHCP server which will deliver an IP address through a DHCP request
Lease time is not taken into account for the moment by the client. It shall be set
in a way the client has enough time to retrieve the TFTP transfer

A bootfile name shall be provided by the DHCP server

The client is then issuing TFTP request to the default Gateway (will be changed
later to a DHCP Option 66/67) 

The transfer happens and the code reports the beginning and end of the transfer

Sample TFTP server can be downloaded here: https://github.com/vejmarie/golang-tftp-example
Sample DHCP server can be downloaded here: https://github.com/coredhcp/coredhcp.git

# To build 

This PoC code requires a tinygo environment

Please use the tasks scheduler for rp2350. The default core scheduler is experiencing a race
condition which is creating a deadlock when the garbage collector is activated. This can
be easily triggered by running multiple transfer through that example. The heap is filled up
to the time memory is exhausted and the Garbage Collector is triggered. Unfortunately the
multicore implementation has a mutex issue, which is leading the use of a more multicore
friendly scheduler

For maximum performances please use the -opt=2 option
The code has also been updated for TFTP transfer block size of 1400 bytes (by default
TFTP is supporting 512 bytes). Please be sure that your TFTP server can adjust to this
requirements

tinygo flash -monitor -target pico2 -scheduler tasks -opt=2 -stack-size=32KB ./main.go

This code has been partially written using AI tools.

You can reach peak performances of a w5500 which is roughly 1.6MB/s through a TFTP transfer.

