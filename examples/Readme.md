Examples can be built for either:

- [Full fledged Go](https://go.dev/) which can run on all platforms
  supported by Go and the [periph](https://periph.io/) peripherals I/O
  module.

- [TinyGo](https://tinygo.org/) which has limited Go functionality and
  built-in peripheral libraries for various microcontrollers.

You need a supported port controller hardware to run these examples. The
only one currently supported by this module is the popular
[FUSB302](https://www.onsemi.com/products/interfaces/usb-type-c/fusb302b)
by ON Semiconductor. The easiest way to use this chip is to purchase a
[breakout/development
board](https://www.google.com.au/search?q=fusb302+breakout).

## Go

Your best bet here is to run the examples on a platform that provides
low latency I2C access such as Raspberry Pi. Following is a partial
guide on how to prepare your Pi and hook it up to FUSB302 breakout
board:

### Configure RPi to enable I2C

1. Install a linux OS on RPi (easiest method is to use the official [RPi
   Imager](https://www.raspberrypi.com/software/).
2. [Configure
   I2C](https://learn.adafruit.com/adafruits-raspberry-pi-lesson-4-gpio-setup/configuring-i2c)
3. Ensure `i2c-tools` are installed and `i2cdetect -l` detects and lists
   your I2C bus.
4. Install `go` SDK using `apt update && apt install golang`.

### Wiring

| [RaspberryPi][1]      | FUSB302 |
| --------------------- | ------- |
| Pin 6 (GND)           | GND     |
| Pin 3 (I2C1 SDA)      | SDA     |
| Pin 5 (I2C1 SCL)      | SCL     |
| Pin 1 (3.3v)          | VDD     |

You also need to pull up `SDA` and `SCL` lines individually with 4.7KΩ
resistors to 3.3v rail.

### Building and Running

Each example has a global `const mpn = ...` that may need to be changed
to the correct manufacturer part number of your specific FUSB302 chip as
some MPNs have different I2C addresses.

To build and run an example, run the following in the example directory:

```
$ go build -o example
$ ./example
```

[1]: https://www.raspberrypi.com/documentation/computers/os.html#gpio-pinout

## TinyGo

The easiest way to run TinyGo examples is to use [Raspberry Pi
Pico](https://www.raspberrypi.com/products/raspberry-pi-pico/). It is
cheap, widely available, well supported and easy to use and work with.

Following guide takes you through how to setup, wire up and run the
examples:

### Wiring

| [RaspberryPi Pico][1] | FUSB302 |
| --------------------- | ------- |
| Pin 3 (GND)           | GND     |
| Pin 4 (I2C1 SDA)      | SDA     |
| Pin 5 (I2C1 SCL)      | SCL     |
| Pin 36 (3.3v)         | VDD     |

You also need to pull up `SDA` and `SCL` lines individually with 4.7KΩ
resistors to 3.3v rail.

### Building and Uploading

Each example has a global `const mpn = ...` that may need to be changed
to the correct manufacturer part number of your specific FUSB302 chip as
some MPNs have different I2C addresses.

You should use TinyGo >= 0.25 which has support for USB UART. For older
TinyGo versions, you will need to hook up extra wires to a UART bridge
to be able to see the logs.

1. Disconnect the Pico from power.
2. Hold down the BOOT button on the board.
3. Connect the USB from your computer to the Pico.
4. Wait until the `RPI-RP2` USB drive shows up.
5. Run the following to build and flash the example program:

    ```
    tinygo flash -target=pico -serial usb
    ```

6. Open the USB serial port using your favorite terminal (e.g. on linux,
   you can run `screen /dev/ttyACM0`).
7. Connect the FUSB302 board to a USB-PD power source.

[1]: https://datasheets.raspberrypi.com/pico/Pico-R3-A4-Pinout.pdf
