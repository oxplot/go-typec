//go:build !tinygo

package main

import (
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"

	"github.com/oxplot/go-typec/tcpcdriver"
)

const busNumber = "1"

func getI2C() tcpcdriver.I2C {
	if _, err := host.Init(); err != nil {
		panic(err)
	}
	b, err := i2creg.Open(busNumber)
	if err != nil {
		panic(err)
	}
	b.SetSpeed(1000000)
	return b
}
