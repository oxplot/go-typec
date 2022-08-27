// SimplePower negotiates a constant voltage at a maximum current with the power
// source. This is the most common usage.
//
// To configure, set the global const minVoltage, maxVoltage and maxCurrent to
// your desired values.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/oxplot/go-typec/tcdpm"
	"github.com/oxplot/go-typec/tcpcdriver/fusb302"
	"github.com/oxplot/go-typec/tcpe"
)

const (
	mpn = fusb302.FUSB302BMPX

	minVoltage = 8000  // minimum acceptable voltage in mV
	maxVoltage = 10000 // maximum acceptable voltage in mV
	maxCurrent = 1200  // minimum acceptable current in mA
)

func powerChangeCallback(det tcpe.PowerChangeDetail) {
	if det.On {
		fmt.Printf("Power is on: %d mV, %d mA\r\n", det.Voltage, det.MaxCurrent)
	} else {
		fmt.Print("Power is off\r\n")
	}
}

func main() {
	pc := fusb302.New(getI2C(), mpn)
	pe := tcpe.New(pc)
	dpm := tcdpm.CV{}
	dpm.SetPolicy(tcdpm.CVPolicy{
		MinVoltage: minVoltage,
		MaxVoltage: maxVoltage,
		MaxCurrent: maxCurrent,
	})
	pe.SetDPM(tcdpm.NewLogger(os.Stdout, "\r\n", &dpm))
	pe.NotifyOnPowerChange(powerChangeCallback)
	pe.Run(context.Background())
}
