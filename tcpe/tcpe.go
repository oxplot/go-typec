// Package tcpe provides an implementation of USB Type-C power delivery policy
// engine for sink devices.
package tcpe

import (
	"context"
	"sync"
	"time"

	"github.com/oxplot/go-typec"
	"github.com/oxplot/go-typec/pdmsg"
)

var (
	maxTimerExpiry = time.Unix(1<<63-62135596801, 999999999) // https://stackoverflow.com/a/32620397
	defaultRDO     pdmsg.RequestDO
)

func init() {
	defaultRDO.SetSelectedObjectPosition(1)
	defaultRDO.SetFixedMaxOperatingCurrent(100)
	defaultRDO.SetFixedOperatingCurrent(100)
}

// PowerChangeDetail represents the negotiated power when On==true.
type PowerChangeDetail struct {
	// On is true if the power was successfully negotiated and is available for
	// use. false indicates that power satisfying policies is no longer available.
	On bool

	// Voltage is the voltage in millivolts that was negotiated.
	Voltage uint16

	// MaxCurrent is the maximum current in milliamps that is available for use, as negotiated.
	MaxCurrent uint16

	// CurrentSource is true if the power source is a current source.
	CurrentSource bool
}

// PolicyEngine implements USB Type-C power delivery policy engine for sink
// devices. It uses polling to handle events from the port controller.
type PolicyEngine struct {
	pc typec.PortController
	// On each timer start, expiry is set to the timer + now by the relevant
	// state.
	timerExpiry  time.Time
	sourceCapMsg pdmsg.Message   // Set after source cap message is received
	requestDO    pdmsg.RequestDO // Response from device policy manager
	msgTpl       pdmsg.Message   // Messages to be sent, use this as template
	pdoBuf       [pdmsg.MaxDataObjects]pdmsg.PDO

	// true if an existing successful power negotiation is already in effect.
	explicitContract bool
	// true if received wait message at select cap state.
	waitingOnSource bool

	mu     sync.Mutex
	events typec.Event

	powerChangeCallback struct {
		mu sync.Mutex
		fn func(PowerChangeDetail)
	}

	dpm struct {
		mu  sync.Mutex
		dpm typec.DevicePolicyManager
	}

	v5PDO pdmsg.FixedSupplyPDO // non-PD max current at 5V available from the power source

	nextTxID uint8
	lastRxID uint8
}

// New creates a new policy engine for a given port controller.
func New(pc typec.PortController) *PolicyEngine {
	m := pdmsg.Message{}
	m.SetPowerRole(pdmsg.PowerRoleSink)
	m.SetDataRole(pdmsg.DataRoleUFP)
	m.SetExtended(false)

	v5PDO := pdmsg.NewFixedSupplyPDO()
	v5PDO.SetVoltage(5000)

	return &PolicyEngine{
		pc:          pc,
		timerExpiry: maxTimerExpiry,
		msgTpl:      m,
		v5PDO:       v5PDO,
	}
}

// SetDPM sets the device policy manager to use. Passing nil will result in the
// policy engine rejecting all power negotiations.
func (pe *PolicyEngine) SetDPM(dpm typec.DevicePolicyManager) {
	pe.dpm.mu.Lock()
	pe.dpm.dpm = dpm
	pe.dpm.mu.Unlock()
}

// NotifyOnPowerChange sets up callback to be called with boolean:
//
//   - true: When power negotiation is complete and power is ready to use. The
//     negotiated PDO is the one selected by the invokation of
//     DevicePolicyManager.EvaluateCapabilities. This is usually when the
//     consumer turns on the power gate to the load.
//   - false: When power as was negotiated is no longer (gauranteed to be)
//     available. This is usually when the consumer turns of the power gate to
//     the load.
//
// Pass nil as callback to remove existing one. This method can be called
// concurrently from multiple goroutines.
func (pe *PolicyEngine) NotifyOnPowerChange(callback func(PowerChangeDetail)) {
	pe.powerChangeCallback.mu.Lock()
	pe.powerChangeCallback.fn = callback
	pe.powerChangeCallback.mu.Unlock()
}

// Renegotiate forces power renegotiation with source. Once the source sends
// its capabilities, EvaluateCapabilities of
// [typec.DevicePolicyManager] will be called.
// Renogotiate may be called concurrently from multiple goroutines.
func (pe *PolicyEngine) Renegotiate() {
	pe.mu.Lock()
	pe.events.Add(typec.EventSendReset)
	pe.mu.Unlock()
}

func (pe *PolicyEngine) evalCaps(pdos []pdmsg.PDO) pdmsg.RequestDO {
	pe.dpm.mu.Lock()
	defer pe.dpm.mu.Unlock()
	if pe.dpm.dpm != nil {
		return pe.dpm.dpm.EvaluateCapabilities(pdos)
	}
	return pdmsg.EmptyRequestDO
}

// Run starts the event loop of the policy engine and manages the state
// transitions and delivery of events. Run blocks until ctx is done. Only one
// call to Run must be in progress at any given time.
func (pe *PolicyEngine) Run(ctx context.Context) {
	const loopSleepDuration = 3 * time.Millisecond
	cur := stateSinkStartup // current state
	entering := true

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var next *state // next state
		var err error
		var e typec.Event

		if entering { // Entering a new state

			pe.timerExpiry = maxTimerExpiry
			if cur.Enter != nil {
				next, err = cur.Enter(pe)
			}
			entering = false

		} else { // Waiting in a state

			// Process outstanding events

			if e, err = pe.pc.Alert(); err != nil {
				goto Error
			}
			pe.mu.Lock()
			pe.events.Add(e)
			e = pe.events.Pop()
			pe.mu.Unlock()

			if e == typec.EventNone {

				// No pending events. Check on timers or sleep.

				if time.Now().After(pe.timerExpiry) {
					pe.timerExpiry = maxTimerExpiry // only run timer timeout event once
					next, err = cur.Process(pe, pdmsg.Message{}, typec.EventTimerTimeout)
				} else {
					time.Sleep(loopSleepDuration)
				}

			} else {

				// Handle next event

				switch e {
				case typec.EventPower0A5:
					pe.v5PDO.SetMaxCurrent(500)
				case typec.EventPower1A5:
					pe.v5PDO.SetMaxCurrent(1500)
				case typec.EventPower3A0:
					pe.v5PDO.SetMaxCurrent(3000)
				case typec.EventDetached, typec.EventResetReceived:
					next = stateSinkStartup
				case typec.EventSendReset:
					next = stateSinkHardReset
				case typec.EventRx:
					var m pdmsg.Message
					if m, err = pe.rx(); err == nil {
						next, err = cur.Process(pe, m, typec.EventRx)
						pe.mu.Lock()
						pe.events.Add(typec.EventRx) // there may be multiple messages waiting
						pe.mu.Unlock()
					} else if err == typec.ErrRxEmpty {
						err = nil
					}
				default:
					next, err = cur.Process(pe, pdmsg.Message{}, e)
				}

			}

		}

	Error:

		if err != nil {
			next = stateSinkHardReset
		}

		if next != nil {
			if cur.Exit != nil {
				if err = cur.Exit(pe); err != nil {
					next = stateSinkHardReset
				}
			}
			cur = next
			entering = true
		}

	}

}

func (pe *PolicyEngine) tx(m pdmsg.Message) error {
	m.SetID(pe.nextTxID)
	pe.nextTxID = (pe.nextTxID + 1) % 8
	return pe.pc.Tx(m)
}

func (pe *PolicyEngine) rx() (pdmsg.Message, error) {
	// Discard duplicate messages
	for {
		m, err := pe.pc.Rx()
		if err != nil {
			return pdmsg.Message{}, err
		}
		if m.ID() != pe.lastRxID {
			pe.lastRxID = m.ID()
			return m, nil
		}
	}
}

func (pe *PolicyEngine) startTimer(d time.Duration) {
	pe.timerExpiry = time.Now().Add(d)
}

// ppsNegotiated returns true if the last power negotiation agreed on a PPS
// profile.
func (pe *PolicyEngine) ppsNegotiated() bool {
	p := pe.requestDO.SelectedObjectPosition()
	return p > 0 && pdmsg.PDO(pe.sourceCapMsg.Data[p-1]).Type() == pdmsg.PDOTypePPS
}

func (pe *PolicyEngine) sendRDO(rdo pdmsg.RequestDO) error {
	m := pe.msgTpl
	m.SetType(pdmsg.TypeRequest)
	m.SetDataObjectCount(1)
	m.Data[0] = uint32(rdo)
	return pe.tx(m)
}

func (pe *PolicyEngine) alertPCCB(on bool) {
	pcd := PowerChangeDetail{On: on}
	if on {
		pdo := pdmsg.PDO(pe.sourceCapMsg.Data[pe.requestDO.SelectedObjectPosition()-1])
		switch pdo.Type() {
		case pdmsg.PDOTypeFixedSupply:
			pcd.Voltage = pdmsg.FixedSupplyPDO(pdo).Voltage()
			pcd.MaxCurrent = pe.requestDO.FixedMaxOperatingCurrent()
		case pdmsg.PDOTypePPS:
			pcd.Voltage = pe.requestDO.PPSOutputVoltage()
			pcd.MaxCurrent = pe.requestDO.PPSOutputCurrent()
			pcd.CurrentSource = true
		}
	}
	pe.powerChangeCallback.mu.Lock()
	defer pe.powerChangeCallback.mu.Unlock()
	if pe.powerChangeCallback.fn != nil {
		pe.powerChangeCallback.fn(pcd)
	}
}

// state represents a policy engine state.
type state struct {
	Name string

	// Enter runs actions on entering the state. It may be nil in which case it
	// is ignored. If non-nil next state is returned, the policy engine loop
	// will immediately call the state Exit followed by Enter of the next state.
	//
	// Before each call to Enter, policy engine clears the current timer.
	Enter func(*PolicyEngine) (next *state, err error)

	// Process is called every time:
	//  - a message is received;
	//  - the current timer has timed out;
	//  - an event has occurred;
	// while the policy engine is in this state. Return values are treated the
	// same way as state Enter. Process cannot be nil unless enter returns a
	// next state unconditionally, as otherwise there may be no way for the
	// engine to get out of this state.
	Process func(pe *PolicyEngine, m pdmsg.Message, e typec.Event) (next *state, err error)

	// Exit is called when Enter or Process function of the state returns a
	// non-nil next state. Exit may be nil.
	Exit func(*PolicyEngine) error
}

// The state names are almost the same as those in the PD spec.
var (
	stateNoPD                     *state
	stateSinkStartup              *state
	stateSinkDiscovery            *state
	stateSinkWaitForCapabilities  *state
	stateSinkEvaluateCapabilities *state
	stateSinkSelectCapabilities   *state
	stateSinkTransitionSink       *state
	stateSinkReady                *state
	stateSinkHardReset            *state
)

func init() {

	// Initializing is done here to avoid circular references between states
	// which are not allowed at the package level variable assignments.

	// Psuedo-state which handles non-PD power sources. It creates a fake PDO
	// and calls on the policy manager to see if it accepts it. If it does, the
	// power change callback is called with on state.
	//
	// This hack allows for simpler state management in conjuction with non-PD
	// sources as well as a streamlined device policy manager interface that
	// treats PD and non-PD sources alike.
	stateNoPD = &state{
		Name: "no-pd",
		Enter: func(pe *PolicyEngine) (*state, error) {
			pe.pdoBuf[0] = pdmsg.PDO(pe.v5PDO)
			rdo := pe.evalCaps(pe.pdoBuf[:1])
			pe.alertPCCB(rdo != pdmsg.EmptyRequestDO)
			return nil, nil
		},
		Process: func(pe *PolicyEngine, m pdmsg.Message, e typec.Event) (*state, error) {
			return nil, nil
		},
	}

	stateSinkStartup = &state{
		Name: "sink-startup",
		Enter: func(pe *PolicyEngine) (*state, error) {
			pe.nextTxID = 0
			pe.lastRxID = 8 // impossible ID meaning no message received yet
			pe.alertPCCB(false)
			pe.explicitContract = false
			return stateSinkDiscovery, pe.pc.Init()
		},
	}

	stateSinkDiscovery = &state{
		Name: "sink-discovery",
		Process: func(pe *PolicyEngine, m pdmsg.Message, e typec.Event) (*state, error) {
			if e == typec.EventAttached {
				return stateSinkWaitForCapabilities, nil
			}
			return nil, nil
		},
	}

	stateSinkWaitForCapabilities = &state{
		Name: "sink-wait-for-cap",
		Enter: func(pe *PolicyEngine) (*state, error) {
			pe.startTimer(timerSinkWaitCap)
			return nil, nil
		},
		Process: func(pe *PolicyEngine, m pdmsg.Message, e typec.Event) (*state, error) {
			if e == typec.EventTimerTimeout {
				if pe.v5PDO.MaxCurrent() > 0 {
					return stateNoPD, nil
				}
				return stateSinkHardReset, nil
			}
			if e == typec.EventRx && m.IsData() && m.Type() == pdmsg.TypeSourceCap {
				pe.sourceCapMsg = m
				r := m.Revision()
				if r < pdmsg.Revision30 {
					pe.msgTpl.SetRevision(r)
				} else {
					pe.msgTpl.SetRevision(pdmsg.Revision30)
				}
				return stateSinkEvaluateCapabilities, nil
			}
			return nil, nil
		},
	}

	stateSinkEvaluateCapabilities = &state{
		Name: "sink-eval-cap",
		Enter: func(pe *PolicyEngine) (*state, error) {
			l := pe.sourceCapMsg.DataObjectCount()
			for i, d := range pe.sourceCapMsg.Data[:l] {
				pe.pdoBuf[i] = pdmsg.PDO(d)
			}
			pe.requestDO = pe.evalCaps(pe.pdoBuf[:l])
			return stateSinkSelectCapabilities, nil
		},
	}

	stateSinkSelectCapabilities = &state{
		Name: "sink-select-cap",
		Enter: func(pe *PolicyEngine) (*state, error) {
			rdo := pe.requestDO
			if rdo == pdmsg.EmptyRequestDO {
				rdo = defaultRDO
			}
			if err := pe.sendRDO(rdo); err != nil {
				return nil, err
			}
			pe.startTimer(timerSenderResponse)
			return nil, nil
		},
		Process: func(pe *PolicyEngine, m pdmsg.Message, e typec.Event) (*state, error) {
			if e == typec.EventTimerTimeout {
				return stateSinkHardReset, nil
			}
			if e == typec.EventRx && !m.IsData() {
				switch m.Type() {
				case pdmsg.TypeAccept:
					pe.waitingOnSource = false
					pe.explicitContract = true
					return stateSinkTransitionSink, nil
				case pdmsg.TypeReject:
					// We deviate from standard here as we require the source to
					// satisfy our request. If it can't, no reason to wait any more.
					return stateSinkHardReset, nil
				case pdmsg.TypeWait:
					pe.waitingOnSource = true
					if pe.explicitContract {
						return stateSinkReady, nil
					}
					return stateSinkWaitForCapabilities, nil
				}
			}
			return nil, nil
		},
	}

	stateSinkTransitionSink = &state{
		Name: "sink-transition-sink",
		Enter: func(pe *PolicyEngine) (*state, error) {
			pe.startTimer(timerPSTransition)
			return nil, nil
		},
		Process: func(pe *PolicyEngine, m pdmsg.Message, e typec.Event) (*state, error) {
			if e == typec.EventTimerTimeout {
				return stateSinkHardReset, nil
			}
			if e == typec.EventRx && !m.IsData() && m.Type() == pdmsg.TypePSReady {
				if pe.requestDO != pdmsg.EmptyRequestDO {
					pe.alertPCCB(true)
				}
				return stateSinkReady, nil
			}
			return nil, nil
		},
	}

	stateSinkReady = &state{
		Name: "sink-ready",
		Enter: func(pe *PolicyEngine) (*state, error) {
			if pe.waitingOnSource {
				pe.startTimer(timerSinkRequest)
			} else if pe.ppsNegotiated() {
				pe.startTimer(timerSinkPPSPeriodic)
			}
			return nil, nil
		},
		Process: func(pe *PolicyEngine, m pdmsg.Message, e typec.Event) (*state, error) {
			if e == typec.EventTimerTimeout {
				return stateSinkSelectCapabilities, nil
			} else if e == typec.EventRx && m.IsData() && m.Type() == pdmsg.TypeSourceCap {
				pe.sourceCapMsg = m
				return stateSinkEvaluateCapabilities, nil
			}
			return nil, nil
		},
	}

	stateSinkHardReset = &state{
		Name: "sink-hard-reset",
		Enter: func(pe *PolicyEngine) (*state, error) {
			return stateSinkStartup, pe.pc.SendReset()
		},
	}

}

// Max value for timers used (based on PD standard).
const (
	timerPSTransition    = 550 * time.Millisecond
	timerSenderResponse  = 32 * time.Millisecond
	timerSinkPPSPeriodic = 10 * time.Second
	timerSinkRequest     = 100 * time.Millisecond
	timerSinkWaitCap     = 620 * time.Millisecond
)
