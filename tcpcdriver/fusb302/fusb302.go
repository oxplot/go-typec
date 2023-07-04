// Package fusb302 implements type-C port controller driver for FUSB302 from
// ONSemi.
package fusb302

import (
	"errors"
	"time"

	"github.com/oxplot/go-typec"
	"github.com/oxplot/go-typec/pdmsg"
	"github.com/oxplot/go-typec/tcpcdriver"
)

// MPN represents the manufacturer part number
type MPN uint8

// I2CAddress returns the I2C address of the FUSB302.
func (m MPN) I2CAddress() uint8 {
	return uint8(m)
}

// Manufacturer part numbers
const (
	FUSB302BUCX   MPN = 0b100010
	FUSB302BMPX   MPN = 0b100010
	FUSB302VMPX   MPN = 0b100010
	FUSB302B01MPX MPN = 0b100011
	FUSB302B10MPX MPN = 0b100100
	FUSB302B11MPX MPN = 0b100101
)

// FUSB302 represents a type-C port controller for FUSB302 IC.
type FUSB302 struct {
	port tcpcdriver.I2C
	addr uint16

	intA uint8 // cache

	// We use go channel here as a fixed size queue and drop messages when
	// queue is full. This is not the optimal behavior but it's simple and given
	// large enough a queue, unlikely to ever be a problem.
	msgs chan pdmsg.Message

	// Buffer used for tx and rx, defined once here instead to avoid heap
	// allocations in each method used.
	buf [pdmsg.MaxMessageBytes + 10]byte
}

const msgQueueSize = 10

// New creates a new controller and allocates all necessary memory for all future operations.
//
// I2C port must have <=1Mhz frequency.
func New(port tcpcdriver.I2C, mpn MPN) *FUSB302 {
	return &FUSB302{
		port: port,
		addr: uint16(mpn.I2CAddress()),
		msgs: make(chan pdmsg.Message, msgQueueSize),
	}
}

func (f *FUSB302) write(r uint8, d byte) error {
	f.buf[0] = r
	f.buf[1] = d
	return f.port.Tx(f.addr, f.buf[:2], nil)
}

func (f *FUSB302) read(r uint8) (byte, error) {
	f.buf[0] = r
	err := f.port.Tx(f.addr, f.buf[:1], f.buf[1:2])
	return f.buf[1], err
}

func (f *FUSB302) writeMany(r uint8, d []byte) error {
	f.buf[0] = r
	copy(f.buf[1:], d)
	return f.port.Tx(f.addr, f.buf[:len(d)+1], nil)
}

func (f *FUSB302) readMany(r uint8, d []byte) error {
	f.buf[0] = r
	err := f.port.Tx(f.addr, f.buf[:1], f.buf[1:len(d)+1])
	if err == nil {
		copy(d, f.buf[1:len(d)+1])
	}
	return err
}

// Init initializes the controller.
func (f *FUSB302) Init() error {

	// Reset the chip and registers to default

	if err := f.write(regReset, regResetSWReset); err != nil {
		return err
	}

	// Flush the rx buffer

	if err := f.write(regControl1, 0b100); err != nil {
		return err
	}

	// Flush the receive queue

FlushReceiveQueue:
	for {
		select {
		case <-f.msgs:
		default:
			break FlushReceiveQueue
		}
	}

	// Turn on all power

	if err := f.write(regPower, regPowerPwrAll); err != nil {
		return err
	}

	// Turn on auto detect CC in sink mode

	if err := f.write(regControl2, 0b00000101); err != nil {
		return err
	}

	// Turn on auto retry

	if err := f.write(regControl3, 0b111); err != nil {
		return err
	}

	return nil
}

// Tx transmits a message.
func (f *FUSB302) Tx(m pdmsg.Message) error {

	// Flush TX FIFO

	if err := f.write(regControl0, 0b01100100); err != nil {
		return err
	}

	// Construct and send the message

	buf := make([]byte, 9+pdmsg.MaxMessageBytes)
	copy(buf, []byte{fifoTokenSync1, fifoTokenSync1, fifoTokenSync1, fifoTokenSync2})
	mlen := m.ToBytes(buf[5:])
	buf[4] = fifoTokenPackSym | mlen
	copy(buf[5+mlen:], []byte{fifoTokenJamCRC, fifoTokenEOP, fifoTokenTxOff, fifoTokenTxOn})
	plen := 9 + mlen

	if err := f.writeMany(regFIFOs, buf[:plen]); err != nil {
		return err
	}

	// Wait until either:
	// - GoodCRC is received: tx successful
	// - Auto Retry failed: tx failed
	// - ~10 millisecond has passed: tx failed

	for i := 0; i < 10; i++ {
		r, err := f.read(regInterruptA)
		f.intA |= r
		if err != nil {
			return err
		}
		if r&regInterruptATxSuccess != 0 { // received GoodCRC
			return nil
		}
		if r&regInterruptARetryFail != 0 {
			return typec.ErrTxFailed
		}
		time.Sleep(time.Millisecond)
	}

	return typec.ErrTxFailed
}

// Rx returns a received message.
func (f *FUSB302) Rx() (pdmsg.Message, error) {
	select {
	case n := <-f.msgs:
		return n, nil
	default:
		return pdmsg.Message{}, typec.ErrRxEmpty
	}
}

func (f *FUSB302) rx(m *pdmsg.Message) error {
	var err error
	var reg uint8

	// Is there a message waiting to be read?

	reg, err = f.read(regStatus1)
	if err != nil {
		return err
	}
	if reg&regStatus1RxEmpty != 0 {
		return typec.ErrRxEmpty
	}

	// Read the header

	buf := make([]byte, pdmsg.MaxMessageBytes+4) // 4 extra for CRC at the end which we will discard
	if err = f.readMany(regFIFOs, buf[:3]); err != nil {
		return err
	}
	m.Header = uint16(buf[2])<<8 | uint16(buf[1])
	l := m.DataObjectCount()

	// Read data objects

	if l > 0 {
		if err = f.readMany(regFIFOs, buf[:l*4+4]); err != nil {
			return err
		}
		for i := uint8(0); i < l; i++ {
			s := i * 4
			m.Data[i] = uint32(buf[s]) | uint32(buf[s+1])<<8 | uint32(buf[s+2])<<16 | uint32(buf[s+3])<<24
		}
	} else {

		// Discard the CRC

		if err = f.readMany(regFIFOs, buf[:4]); err != nil {
			return err
		}
	}
	return nil
}

// SendReset send a hard reset message to the port partner.
func (f *FUSB302) SendReset() error {
	r, err := f.read(regControl3)
	if err != nil {
		return err
	}
	if err := f.write(regControl3, r|regControl3SendHardReset); err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		intA, err := f.read(regInterruptA)
		if err != nil {
			return err
		}
		f.intA |= intA
		if intA&regInterruptAHardSent != 0 {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return typec.ErrTxFailed
}

// ErrInvalidCCState is returned when the CC state is invalid.
var ErrInvalidCCState = errors.New("invalid cc state")

// Alert processes all pending interrupts and returns any event generated as a
// result.
func (f *FUSB302) Alert() (e typec.Event, err error) {
	regs := make([]byte, 7)
	if err = f.readMany(regStatus0A, regs); err != nil {
		return
	}
	status0A, status1A, intA, _, status0, status1, intT := regs[0], regs[1], regs[2], regs[3], regs[4], regs[5], regs[6]
	intA |= f.intA
	f.intA = 0

	_, _, _ = intT, status0, status1

	// Report soft and hard resets

	if intA&regInterruptASoftReset != 0 && status0A&regStatus0ARxSoftReset != 0 {
		e.Add(typec.EventResetReceived)
	}
	if intA&regInterruptAHardReset != 0 && status0A&regStatus0ARxHardReset != 0 {
		e.Add(typec.EventResetReceived)
	}

	// Set CC polarity after CC is settled

	if intA&regInterruptATogDone != 0 {

		// Determine host current capabilities at 5V

		switch status0 & 0b11 {
		case 1:
			e.Add(typec.EventPower0A5)
		case 2:
			e.Add(typec.EventPower1A5)
		case 3:
			e.Add(typec.EventPower3A0)
		}

		// Turn off auto detect function

		if err = f.write(regControl2, 0); err != nil {
			return
		}

		// Enable tx and rx on the detected CC line

		var pol uint8
		var meas uint8

		if (status1A>>regStatus1ATogSSPos)&(regStatus1ATogSSMask) == regStatus1ATogSSSnk1 {
			pol = regSwitches1TxCC1En
			meas = regSwitches0MeasCC1
		} else if (status1A>>regStatus1ATogSSPos)&(regStatus1ATogSSMask) == regStatus1ATogSSSnk2 {
			pol = regSwitches1TxCC2En
			meas = regSwitches0MeasCC2
		} else {
			return e, ErrInvalidCCState
		}
		if err = f.write(regSwitches1, regSwitches1SpecRev1|regSwitches1AutoGCRC|pol); err != nil {
			return
		}
		if err = f.write(regSwitches0, meas|regSwitches0CC1PdEn|regSwitches0CC2PdEn); err != nil {
			return
		}

	}

	// VBUS detection

	if intT&regInterruptVBusOK != 0 {
		if status0&regStatus0VBusOK == 0 {
			e.Add(typec.EventDetached)
		} else {
			e.Add(typec.EventAttached)
		}
	}

	// Message received

	if intT&regInterruptCRCChk != 0 {
		// Read all messages into a queue as quickly as possible.
		for {
			var msg pdmsg.Message
			if err = f.rx(&msg); err != nil {
				if err == typec.ErrRxEmpty {
					err = nil
					break
				}
				return
			}
			if !msg.IsData() && msg.Type() == pdmsg.TypeGoodCRC {
				continue
			}
			// Queue message without blocking (ie drop if queue is full which should be rare).
			select {
			case f.msgs <- msg:
			default:
			}
		}
		e.Add(typec.EventRx)
	}

	return
}

const (
	regSwitches0        = 0x02
	regSwitches0MeasCC2 = 1 << 3
	regSwitches0MeasCC1 = 1 << 2
	regSwitches0CC2PdEn = 1 << 1
	regSwitches0CC1PdEn = 1 << 0

	regSwitches1         = 0x03
	regSwitches1SpecRev1 = 1 << 6
	regSwitches1AutoGCRC = 1 << 2
	regSwitches1TxCC2En  = 1 << 1
	regSwitches1TxCC1En  = 1 << 0

	regMeasure         = 0x04
	regMeasureMDACMask = 0x3F
	regMeasureVBus     = 1 << 6

	regControl0 = 0x06
	regControl1 = 0x07
	regControl2 = 0x08

	regControl3              = 0x09
	regControl3SendHardReset = 1 << 6

	regMask = 0x0A

	regPower       = 0x0B
	regPowerPwrAll = 0xF

	regReset        = 0x0C
	regResetSWReset = 1 << 0

	regStatus0A            = 0x3C
	regStatus0ARxSoftReset = 1 << 1
	regStatus0ARxHardReset = 1 << 0

	regStatus1A = 0x3D

	regStatus1ATogSSSnk1 = 0b101
	regStatus1ATogSSSnk2 = 0b110
	regStatus1ATogSSPos  = 3
	regStatus1ATogSSMask = 0x7

	regInterruptA          = 0x3E
	regInterruptATogDone   = 1 << 6
	regInterruptARetryFail = 1 << 4
	regInterruptAHardSent  = 1 << 3
	regInterruptATxSuccess = 1 << 2
	regInterruptASoftReset = 1 << 1
	regInterruptAHardReset = 1 << 0

	regStatus0       = 0x40
	regStatus0VBusOK = 1 << 7

	regStatus1        = 0x41
	regStatus1RxEmpty = 1 << 5

	regInterrupt       = 0x42
	regInterruptVBusOK = 1 << 7
	regInterruptCRCChk = 1 << 4

	regFIFOs = 0x43

	fifoTokenTxOn    = 0xA1
	fifoTokenSync1   = 0x12
	fifoTokenSync2   = 0x13
	fifoTokenPackSym = 0x80
	fifoTokenJamCRC  = 0xFF
	fifoTokenEOP     = 0x14
	fifoTokenTxOff   = 0xFE
)
