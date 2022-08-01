The easiest way to run TinyGo examples is to use [Raspberry Pi
Pico](https://www.raspberrypi.com/products/raspberry-pi-pico/). It is
cheap, widely available, well supported and easy to use and work with.

Following guide takes you through how to setup, wire up and run the
examples:

## Wiring

| [RaspberryPi Pico][1] | FUSB302 |
| --------------------- | ------- |
| Pin 3 (GND)           | GND     |
| Pin 4 (I2C1 SDA)      | SDA     |
| Pin 5 (I2C1 SCL)      | SCL     |
| Pin 36 (3.3v)         | VDD     |

You also need to pull up `SDA` and `SCL` lines individually with 4.7KΩ
resistors to 3.3v rail.

## Building and Uploading

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
