// TCLogger prints power profiles of the connected power source to the
// terminal.
package main

import (
	"context"
	"machine"
	"os"

	"github.com/oxplot/go-typec/tcdpm"
	"github.com/oxplot/go-typec/tcpcdriver/fusb302"
	"github.com/oxplot/go-typec/tcpe"
)

const mpn = fusb302.FUSB302BMPX

func main() {
	i2c := machine.I2C1
	i2c.Configure(machine.I2CConfig{
		Frequency: 1000000,
		SDA:       machine.GPIO2,
		SCL:       machine.GPIO3,
	})
	pc := fusb302.New(i2c, mpn)
	pe := tcpe.New(pc)
	dpm := tcdpm.NewLogger(os.Stdout, "\r\n", nil)
	pe.SetDPM(dpm)
	pe.Run(context.Background())
}
