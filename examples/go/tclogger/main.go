// TCLogger prints power profiles of the connected power source to the
// terminal.
package main

import (
	"context"
	"log"
	"os"

	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"

	"github.com/oxplot/go-typec/tcdpm"
	"github.com/oxplot/go-typec/tcpcdriver/fusb302"
	"github.com/oxplot/go-typec/tcpe"
)

const (
	mpn       = fusb302.FUSB302BMPX
	busNumber = "1"
)

func main() {
	log.SetFlags(0)
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}
	b, err := i2creg.Open(busNumber)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()
	b.SetSpeed(1000000)

	pc := fusb302.New(b, mpn)
	dpm := tcdpm.NewLogger(os.Stdout, "\n", nil)
	pe := tcpe.New(pc)
	pe.SetDPM(dpm)
	pe.Run(context.Background())
}
