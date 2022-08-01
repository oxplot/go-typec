Your best bet here is to run the examples on a platform that provides
low latency I2C access such as Raspberry Pi. Following is a partial
guide on how to prepare your Pi and hook it up to FUSB302 breakout
board:

## Configure RPi to enable I2C

1. Install a linux OS on RPi (easiest method is to use the official [RPi
   Imager](https://www.raspberrypi.com/software/).
2. [Configure
   I2C](https://learn.adafruit.com/adafruits-raspberry-pi-lesson-4-gpio-setup/configuring-i2c)
3. Ensure `i2c-tools` are installed and `i2cdetect -l` detects and lists
   your I2C bus.
4. Install `go` SDK using `apt update && apt install golang`.

## Wiring

| [RaspberryPi][1]      | FUSB302 |
| --------------------- | ------- |
| Pin 6 (GND)           | GND     |
| Pin 3 (I2C1 SDA)      | SDA     |
| Pin 5 (I2C1 SCL)      | SCL     |
| Pin 1 (3.3v)          | VDD     |

You also need to pull up `SDA` and `SCL` lines individually with 4.7KΩ
resistors to 3.3v rail.

## Building and Running

Each example has a global `const mpn = ...` that may need to be changed
to the correct manufacturer part number of your specific FUSB302 chip as
some MPNs have different I2C addresses.

To build and run an example, run: `go run main.go` in the example
directory.

[1]: https://www.raspberrypi.com/documentation/computers/os.html#gpio-pinout
