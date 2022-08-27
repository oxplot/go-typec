// TCLogger prints power profiles of the connected power source to the
// terminal.
package main

import (
	"context"
	"os"

	"github.com/oxplot/go-typec/tcdpm"
	"github.com/oxplot/go-typec/tcpcdriver/fusb302"
	"github.com/oxplot/go-typec/tcpe"
)

const mpn = fusb302.FUSB302BMPX

func main() {
	pc := fusb302.New(getI2C(), mpn)
	pe := tcpe.New(pc)
	dpm := tcdpm.NewLogger(os.Stdout, "\r\n", nil)
	pe.SetDPM(dpm)
	pe.Run(context.Background())
}
