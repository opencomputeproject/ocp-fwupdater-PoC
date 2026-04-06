# Hardware Setup Description

## Overview
- Purpose: Proof-of-concept (PoC) development setup for firmware and driver testing using a Raspberry Pi Pico 2 (RP2350) as SPI master, a W5500 Ethernet adapter, and a local SPI‑NOR flash (Winbond 32MB) multiplexed with a TMUX1574.
- Use-case: Development and validation of SPI drivers, Ethernet stack interaction, and SPI‑NOR access on a lab breadboard.
- Primary components: Raspberry Pi Pico 2, Wiznet W5500 (SPI → Ethernet), TMUX1574 (SPI multiplexer), Winbond SPI‑NOR (25Q256JVFQ / 32 MByte), passive components (resistors, capacitor, diode).

## Bill of Materials (BOM)
- Raspberry Pi Pico 2 (RP2350) — 1
  - https://www.raspberrypi.com/documentation/microcontrollers/pico-series.html
  - RP2350 datasheet: https://pip.raspberrypi.com/documents/RP-008373-DS-rp2350-datasheet.pdf
- Wiznet W5500 SPI Ethernet module — 1
  - https://docs.wiznet.io/Product/Chip/Ethernet/W5500
- Winbond SPI‑NOR flash, 25Q256JVFQ (32 MByte / 256 Mbit) — 1
- TI TMUX1574 (TSSOP16 mounted on TSSOP→DIP adapter) — 1
  - https://www.ti.com/product/TMUX1574
- SOIC‑16 → DIP adapter for SPI‑NOR — 1
- 30 Ω series resistors, 1/4 W — 3 (INT, CS, RESET)
- Pull resistors (values as required for Errata workaround) — as needed (see note)
- 0.1 μF ceramic decoupling capacitor (placed close to TMUX VCC) — 1
- Diode on 3.3 V rail for isolation (Schottky recommended) — 1
- Breadboard, jumper wires, connectors, standoffs, ground wiring

## Hardware Diagram (simple ASCII)
[Host] -- (optional) -- [SEL control] ---------+  
&nbsp;&nbsp;&nbsp;|&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;|  
[Pico2 (SPI master)] --[30Ω series]-- [TMUX1574] -- [SPI‑NOR (25Q256)]  
&nbsp;&nbsp;&nbsp;|&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;  
&nbsp;&nbsp;&nbsp;+-- SPI0 (GPIO16..19) -----------> W5500 (SPI)  
&nbsp;&nbsp;&nbsp;|-- GPIO20 (W5500 RESET)  
&nbsp;&nbsp;&nbsp;|-- GPIO21 (W5500 INT)  
VBUS -> VSYS  
GND -> common ground

## Components and Interfaces

### Raspberry Pi Pico 2 (Controller / Main Board)
- MCU: RP2350
- Docs: Pico series documentation and RP2350 datasheet above.
- Key interfaces used:
  - SPI0 (GPIO16, GPIO17, GPIO18, GPIO19)
  - GPIO20 (W5500 RESET)
  - GPIO21 (W5500 INT)
  - One GPIO for TMUX SEL
- Power: Pico2 3.3 V regulator used (limited to ~300 mA). VBUS wired to VSYS. Pico2 provides GND to the breadboard.

### Wiznet W5500 (Ethernet)
- Interface: SPI (standard mode; driven by SPI0 of Pico2)
- Wiring:
  - SCLK → SPI0 SCLK (GPIO18)
  - MOSI → SPI0 TX / MOSI (GPIO19)  (confirm physical mapping to your board)
  - MISO → SPI0 RX / MISO (GPIO16)
  - CS   → SPI0 CS (GPIO17)
  - INT  → GPIO21
  - RESET → GPIO20
- Note: Use W5500 docs to tune performance and driver parameters.

### TMUX1574 (SPI multiplexer)
- Purpose: Multiplex SPI‑NOR between Pico2 and an external host. TMUX1574 has two groups of four S inputs and four D outputs (supports standard SPI only; no dual/quad modes).
- Connection:
  - Pico2 (SPI1 bus master) connected to one S group of TMUX.
  - SPI‑NOR connected to corresponding D lines.
  - SEL pin of TMUX connected to a Pico2 GPIO to select which side drives the SPI‑NOR.
- Power sensitivity:
  - Place a 0.1 μF decoupling capacitor as close as possible to TMUX VCC pin (use 0.1 μF exactly).
- Note: TMUX introduces delay and impedance; proper signal conditioning is required.

### SPI‑NOR (Winbond 25Q256JVFQ)
- Package: SOIC‑16 on SOIC→DIP adapter for breadboard use.
- Connected to TMUX D lines.
- Accessed as SPI slave when TMUX directs connection to Pico2.

## Pinout / Wiring (Pico2 → peripherals)
- SPI0 SCLK: GPIO18 → W5500 CLK
- SPI0 MOSI: GPIO19 → W5500 MOSI
- SPI0 MISO: GPIO16 → W5500 MISO
- SPI0 CS:   GPIO17 → W5500 CS 
- W5500 INT: GPIO21 → W5500 IRQ
- W5500 RESET: GPIO20 → W5500 RESET
- TMUX SCLK : S1A → SPI1 SCLK GPIO14 (with 30 Ω series resistor close to Pico2)
- TMUX MOSI : S2A → SPI1 MOSI GPIO15 (30 Ω series resistor)
- TMUX MISO : S3A → SPI1 MISO GPIO12 
- TMUX CS: S4A → SPI1 CS GPIO13 (30 Ω series resistor)
- TMUX SEL:  Connect to a spare Pico2 GPIO (used to flip SPI‑NOR control between Pico2 and host)
- TMUX VCC: 3.3V + 0.1uF capacitor to GND
- VBUS → VSYS (Pico2 powering breadboard 3.3 V rail)
- Common GND: Pico2 GND → all devices
- SPI-NOR SCLK: D1
- SPI-NOR MOSI: D2
- SPI-NOR MISO: D3
- SPI-NOR CS: D4
- SPI-NOR VCC: 3.3V 
- SPI-NOR GND: GND
- 3.3 V isolation: Add diode on Pico2 3.3 V rail when sharing SPI‑NOR with an external host to prevent current backflow between power domains.

(Adjust physical GPIO numbers if using alternate pin mapping on your board; above uses your provided mapping.)

## Signal Conditioning and PCB/Breadboard Notes
- Errata 9 (RP2350): For RP2350 revisions <= A4, pull resistors may be required on certain SPI signals to maintain stability. Implement pull-ups or pull-downs as needed — in this setup pull up (47k) resistors were added on INT, CS, and RESET.
- Series resistors: Add 30 Ω, 1/4 W series resistors on MOSI, SCLK, and CS close to the Pico2 (driver side) to reduce overshoot and ringing introduced by TMUX delay and breadboard wiring.
- Wire length: Use equal length wires for SPI signals (MOSI, MISO, SCLK, CS) to reduce skew and maintain signal integrity on high-speed SPI.
- Decoupling: Place 0.1 μF ceramic capacitor close to TMUX VCC pin. Also ensure adequate decoupling for W5500 and SPI‑NOR devices per their datasheets.
- Breadboard limitations: Despite being on a breadboard, the described conditioning allows reliable operation up to 60 MHz SPI in this setup (no external retimer used).

## Power and Grounding
- Power source: Pico2 onboard 3.3 V regulator (limit ~300 mA). Ensure combined current draw of W5500, SPI‑NOR, TMUX and any LEDs or peripherals stays within this limit.
- VBUS is wired to VSYS on the Pico2.
- Ground: Common ground across Pico2, W5500, TMUX, SPI‑NOR, and the host (if present).
- 3.3 V isolation: When the SPI‑NOR is shared with another host that has its own 3.3 V rail, add a diode on the Pico2 3.3 V output to prevent power backfeed between rails. Confirm diode orientation and voltage drop do not prevent proper powering of devices during intended operation.

## Assembly / Wiring Instructions (high-level)
1. Mount Pico2 on the breadboard and wire VBUS → VSYS; connect GND to breadboard ground rail.
2. Mount TMUX1574 on TSSOP→DIP adapter; place 0.1 μF decoupling capacitor as close to TMUX VCC pin as possible.
3. Mount SPI‑NOR on SOIC→DIP adapter and insert into breadboard.
4. Wire TMUX D outputs to SPI‑NOR pins (MOSI, MISO, SCLK, CS, GND, VCC).
5. Wire Pico2 SPI1 pins to the TMUX S1 inputs for the Pico side. Put 30 Ω series resistors on MOSI, SCLK and CS near the Pico2.
6. Wire W5500 to Pico2 SPI pins (same SPI0 lines) and control pins (INT to GPIO21, RESET to GPIO20). Add pull resistors where required (see Errata note).
7. Wire TMUX SEL to a Pico2 GPIO to control selection between Pico2 and host.
8. Add diode on Pico2 3.3 V rail for isolation when sharing with another host; ensure common ground.
9. Verify all connections, add pull resistors per RP2350 errata if chip revision <= A4.

## Power-Up and Validation
- Initial power-up:
  - Confirm power rails: Pico2 3.3 V stable, ground common.
  - With TMUX unselected or selected appropriately, verify SPI‑NOR / W5500 power present.
- Expected indicators:
  - Pico2 power LED on.
  - W5500 link/activity LEDs as applicable (module dependent).
- Tests:
  - Verify SPI communication at low clock (e.g., 1–4 MHz) first.
  - Confirm read/write to SPI‑NOR (JEDEC ID, basic read/write).
  - Bring SPI up to target 60 MHz in steps, watching signal integrity and transfer correctness.
  - Validate W5500 initialization and basic Ethernet TX/RX.

## Troubleshooting
- No SPI‑NOR response:
  - Check TMUX SEL state and wiring.
  - Confirm CS polarity and pull resistor states.
  - Verify series resistors not open or miswired.
  - Check decoupling capacitor and VCC to TMUX.
- Signal integrity issues at higher SPI speeds:
  - Use shorter, equal-length wires.
  - Confirm series resistors are close to Pico2 driver pins.
  - Inspect for breadboard noise and poor contacts; consider transfer to protoboard/PCB if persistent.
- W5500 not responding:
  - Check RESET and INT wiring and states.
  - Verify CS, SCLK, MOSI, MISO mapping.
  - Confirm 3.3 V rail can supply W5500 peak currents; consider external regulator if needed.

## Safety and Maintenance
- Stay within the Pico2 regulator current limits (≈300 mA). If you add other peripherals or load, use an external 3.3 V regulator with adequate current headroom.
- Visually inspect adapters and solder joints on TSSOP and SOIC adapters for shorts.
- Use ESD precautions when handling ICs on adapters.

## Appendix / References
- Raspberry Pi Pico series: https://www.raspberrypi.com/documentation/microcontrollers/pico-series.html
- RP2350 datasheet (errata reference): https://pip.raspberrypi.com/documents/RP-008373-DS-rp2350-datasheet.pdf
- W5500 documentation: https://docs.wiznet.io/Product/Chip/Ethernet/W5500
- TMUX1574 product page: https://www.ti.com/product/TMUX1574
- Winbond 25Q256JVFQ datasheet: (refer to Winbond product pages / datasheet for pinout and timing)

If you want, I can produce:
- A printable wiring table (component → pin → Pico GPIO)
- A schematic-style diagram (PNG / SVG) showing exact pin connections
- Recommended values for pull resistors for the RP2350 errata case (if you provide the chip revision or preferred pull direction)
