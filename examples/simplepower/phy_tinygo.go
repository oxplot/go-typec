//go:build tinygo && pico

package main

import (
	"machine"

	"github.com/oxplot/go-typec/tcpcdriver"
)

func getI2C() tcpcdriver.I2C {
	i2c := machine.I2C1
	i2c.Configure(machine.I2CConfig{
		Frequency: 1000000,
		SDA:       machine.GPIO2,
		SCL:       machine.GPIO3,
	})
	return i2c
}
