Add a drawing of the PoC board

Add attachement to a SPI-NOR chip and try to dump it on the console

Download through TFTP a usefull content and dump it to the spi-nor through a comparaison method

Break the firmware in two parts, in a way to retrieve the spi-nor content by using HTTP protocol instead of TFTP

Relocate the code into the low memory area as to allow a reboot from the downloaded file which shall become a secondary stage
Secondary stage firmware will check and validate the SPI-NOR content and release it to a potential client
