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
	"time"

	"github.com/oxplot/go-typec/pdmsg"
	"github.com/oxplot/go-typec/tcdpm"
	"github.com/oxplot/go-typec/tcpcdriver/fusb302"
	"github.com/oxplot/go-typec/tcpe"
)

const mpn = fusb302.FUSB302BMPX

var policy = tcdpm.CCPolicy{
	MinVoltage: 6000,
	MaxVoltage: 7000,
	MinCurrent: 1000,
	MaxCurrent: 1000,
}

func powerReadyCallback(powerReady bool, pdo pdmsg.PDO, rdo pdmsg.RequestDO) {
	if powerReady {
		v, c := tcdpm.GetVoltageCurrent(pdo, rdo)
		fmt.Printf("Power is on: %d mV, %d mA\r\n", v, c)
	} else {
		fmt.Print("Power is off\r\n")
	}
}

func main() {
	fmt.Print("starting up\r\n")
	pc := fusb302.New(getI2C(), mpn)
	pe := tcpe.New(pc)
	dpm := tcdpm.NewPolicyManager(pe, powerReadyCallback)
	err := dpm.SetPolicy(tcdpm.NewLogger(os.Stdout, "\r\n", policy), true)
	if err != nil {
		for {
			fmt.Printf("Setting policy failed: %s\r\n", err)
			time.Sleep(time.Second)
		}
	}
	pe.Run(context.Background())
}
