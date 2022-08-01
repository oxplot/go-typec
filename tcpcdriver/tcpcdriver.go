// Package tcpcdriver defines interfaces and helper functions for implementing
// USB Type-C port controller drivers.
//
// The interfaces are either copied or derived from TinyGo source code with
// minor modifications.
package tcpcdriver

// I2C defines a minimum interface to I2C hardware with a single Tx method
// which allows a single driver implementation to work across many different
// µControllers and host platforms. All port controller drivers that
// communicate over I2C use this interface. This interface was originally
// defined in TinyGo.
type I2C interface {

	// Tx performs a write and then a read transfer placing the result in r. Tx
	// must be safe to call concurrently from multiple goroutines.
	//
	// Passing a nil value for w or r skips the transfer corresponding to write
	// or read, respectively.
	//
	//  i2c.Tx(addr, nil, r)
	// Performs only a read transfer.
	//
	//  i2c.Tx(addr, w, nil)
	// Performs only a write transfer.
	Tx(addr uint16, w, r []byte) error
}
