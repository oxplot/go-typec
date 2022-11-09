// Package typec defines high level interfaces and types for implementing a full
// USB Type-C power delivery stack.
package typec

import (
	"errors"

	"github.com/oxplot/go-typec/pdmsg"
)

// Event can store multiple events and return them in priority order.
type Event uint16

// Pop returns the next high priority event and clears it.
func (e *Event) Pop() Event {
	if *e == 0 {
		return EventNone
	}
	for r := Event(1); r <= 0x8000; r <<= 1 {
		if *e&r != 0 {
			*e &= ^r
			return r
		}
	}
	return EventNone // will never get here
}

// Add adds the events v to the set.
func (e *Event) Add(v Event) {
	*e |= v
}

// Has returns true if the event v is set without clearing it.
func (e Event) Has(v Event) bool {
	return e&v != 0
}

func (e Event) String() string {
	switch e {
	case EventNone:
		return "None"
	case EventResetReceived:
		return "ResetReceived"
	case EventSendReset:
		return "SendReset"
	case EventPower0A5:
		return "Power0A5"
	case EventPower1A5:
		return "Power1A5"
	case EventPower3A0:
		return "Power3A0"
	case EventAttached:
		return "Attached"
	case EventDetached:
		return "Detached"
	case EventRx:
		return "Rx"
	case EventTimerTimeout:
		return "TimerTimeout"
	default:
		return "INVALID"
	}
}

// EventNone represents no event.
const EventNone Event = 0

// The events are listed in order of priority from highest to lowest. This
// means that in presence of multiple pending events, highest priority one is
// attended to first.
const (
	EventResetReceived Event = 1 << iota // Hard reset received
	EventSendReset                       // Request to send hard reset signal to port partner
	EventPower0A5                        // 5V@0.5A non-PD power source
	EventPower1A5                        // 5V@1.5A non-PD power source
	EventPower3A0                        // 5V@3A non-PD power source
	EventAttached                        // VBUS power detected
	EventDetached                        // VBUS power lost
	EventRx                              // Received a message
	EventTimerTimeout                    // Active timer has timed out
)

// PortController provides an interface to operate a device, often an IC
// such as FUSB302, in USB Power Delivery sink mode only. The implementer must
// handle the physical and some protocol layer of the USB Power Delivery as
// detailed below.
//
// Sink port controllers must:
//
//   - Handle entirety of Atomic Message Sequence from the protocol layer all the
//     way to physical layer. This includes starting CRCReceiveTimer, GoodCRC
//     message ID matching, retries, etc. Message ID counters are tracked in the
//     policy engine however.
//   - Configure the port for sink operation after Init is called.
//   - Detect and set correct CC polarity upon attachment.
//   - Detect and report host current provided by the source as EventPower*
//     events.
//
// Port controllers should try to avoid heap allocation after initialization
// stage as much as possible, since they may be running on microcontrollers with
// limited/expensive garbage collectors.
type PortController interface {

	// Init (re-)initializes the state of the controller to a known initial
	// working state. Init must be called at least once and before any other
	// method of this interface. It may be called multiple times thereafter to
	// bring the controller back to its initial state (for instance after a
	// reset).
	Init() error

	// Tx sends a power delivery message to the port partner. CRC is
	// automatically calculated and appended to the header and the data. Tx will
	// block until a GoodCRC response is received or all auto-retries have
	// failed. ErrTxFailed will be returned if auto-retries fail. Alert must be
	// called after each call to Tx to check for possible events generated as
	// result of the call to Tx.
	Tx(pdmsg.Message) error

	// Rx returns a single received message. If no messages are left, ErrRxEmpty
	// is returned. Rx does not return GoodCRC messages and instead, discards
	// them internally. Alert must be called after each call to Rx to check for
	// possible events generated as result of the call to Rx.
	Rx() (pdmsg.Message, error)

	// SendReset sends a hard reset message to the port partner and blocks until
	// send is complete. Alert must be called after each call to SendReset to
	// check for possible events generated as result of the call to SendReset.
	SendReset() error

	// Alert is called by the policy engine either periodically or as a result of
	// hardware/software interrupts, to let the port conroller know it needs to
	// check on the hardware status/interrupts. It must also be called after each
	// call to Tx, Rx and SendReset.
	//
	// Port controller can respond with list of events resulting from processing
	// of interrupts. If the controller generates events outside of Alert(), it
	// must cache those and return them on the next Alert() call.
	Alert() (Event, error)
}

var (
	// ErrTxFailed is returned by Tx() if all auto-retries have failed.
	ErrTxFailed = errors.New("failed to send pd message")

	// ErrRxEmpty is returned by Rx() if no more messages are left to read.
	ErrRxEmpty = errors.New("no more messages to read")
)
