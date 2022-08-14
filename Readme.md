[![Go
Reference](https://pkg.go.dev/badge/github.com/oxplot/go-typec.svg)](https://pkg.go.dev/github.com/oxplot/go-typec)

**USB Type-C Power Delivery Module for Go**

At the moment, only sink functionality is implemented. Sink refers to a
power consumer. You can use the module to request specific voltage and
current from USB-C power source such as wall adapters, power banks and
more.

Goals:

 - Easy to use, understand and modify.
 - Familiar to Go programmers.
 - Efficient resource utilization so it can run on microcontrollers and
   in embedded environments.
 - Compatibility with various port controller hardware.

Non-goals:

 - Following standard word for word.
 - Implementing all PD functionality.

There are examples in the [examples](./examples) directory to get you
started.

*Note that the timing requirements of PD messaging is rather strict. As
such, in environments where communication latency is high, it may not be
possible to implement a working program. For example, on a linux desktop
using the MCP2221 I2C bridge to communicate with FUSB302 PD port
controller, each I2C transaction latency is too high (~10ms) for things
to work. Your mileage may vary.*
