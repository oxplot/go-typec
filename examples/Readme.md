Examples are divided between those that run on:

- [`go`](./go): [Full fledged Go](https://go.dev/) which can run on
  all platforms supported by Go and the [periph](https://periph.io/)
  peripherals I/O module.

- [`tinygo`](./tinygo): [TinyGo](https://tinygo.org/) which has limited
  Go functionality and built-in peripheral libraries for various
  microcontrollers.

You need a supported port controller hardware to run these examples. The
only one currently supported by this module is the popular
[FUSB302](https://www.onsemi.com/products/interfaces/usb-type-c/fusb302b)
by ON Semiconductor. The easiest way to use this chip is to purchase a
[breakout/development
board](https://www.google.com.au/search?q=fusb302+breakout).
