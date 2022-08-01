// Package tcdpm implements some useful device policy managers for common use.
package tcdpm

import (
	"fmt"
	"io"
	"sync"

	"github.com/oxplot/go-typec"
	"github.com/oxplot/go-typec/pdmsg"
)

// Fallback allows chaining multiple device policy managers.
type Fallback []typec.DevicePolicyManager

// NewFallback returns a new Fallback device policy manager. At power
// negotiation, each manager is consulted in order until a non empty RequestDO
// is returned. The new fallback policy must not be modified after it is set as
// the policy manager for a policy engine.
func NewFallback(policies ...typec.DevicePolicyManager) *Fallback {
	fb := Fallback(make([]typec.DevicePolicyManager, len(policies)))
	for i, p := range policies {
		fb[i] = p
	}
	return &fb
}

// EvaluateCapabilities evaluates the provided power profiles against all the
// policies in the chain and returns the first non empty RequestDO that can be
// used to negotiate with the power source.
func (f *Fallback) EvaluateCapabilities(pdos []pdmsg.PDO) pdmsg.RequestDO {
	for _, p := range *f {
		rdo := p.EvaluateCapabilities(pdos)
		if rdo != pdmsg.EmptyRequestDO {
			return rdo
		}
	}
	return pdmsg.EmptyRequestDO
}

// CCPolicy defines a constant current policy where the power source is expected
// to drop the voltage if needed to maintain the current under the MaxCurrent.
// If current is below MaxCurrent, the power source is expected to increase the
// voltage up to a value between MinVoltage and MaxVoltage.
type CCPolicy struct {

	// Minimum accepted voltage in millivolts when current is below MaxCurrent.
	MinVoltage uint16

	// Maximum accpeted voltage in millivolts when current is below MaxCurrent.
	MaxVoltage uint16

	// Maximum current in milliamps that should be supplied under all load
	// conditions.
	MaxCurrent uint16

	// If a source provides multiple profile within the voltage range of a
	// policy, it's possible to prefer lower voltage profiles than the default
	// higher voltage profiles.
	PreferLowerVoltage bool
}

// CC implements a constant current policy manager. Below are some examples of
// where a constant current supply is useful:
//
//   - LED drivers
//   - Li-ion battery chargers
//
// Constant current capability  is only available in PD power sources that
// support Programmable Power Supply (PPS) standard.
type CC struct {
	mu     sync.Mutex
	policy CCPolicy
}

// SetPolicy updates the existing policy. Any future power negotiations will use
// the new policy. If immediate renegotation of power based on the new policy is
// required, tcpe.PolicyEngine.Renegotiate() must be called.
//
// SetPolicy can be called concurrently from multiple goroutines.
func (c *CC) SetPolicy(p CCPolicy) {
	c.mu.Lock()
	c.policy = p
	c.mu.Unlock()
}

// GetPolicy returns the current policy.
func (c *CC) GetPolicy() CCPolicy {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.policy
}

// EvaluateCapabilities evaluates the provided power profiles against the policy
// and returns a RequestDO that can be used to negotiate with the power
// source.
func (c *CC) EvaluateCapabilities(pdos []pdmsg.PDO) pdmsg.RequestDO {
	c.mu.Lock()
	policy := c.policy
	c.mu.Unlock()

	var bestVoltage uint16
	if policy.PreferLowerVoltage {
		bestVoltage = ^uint16(0)
	}
	rdo := pdmsg.EmptyRequestDO
	for i, p := range pdos {
		if p.Type() != pdmsg.PDOTypePPS {
			continue
		}
		pps := pdmsg.PPSPDO(p)
		minV, maxV := c.policy.MinVoltage, c.policy.MaxVoltage
		if minV < pps.MinVoltage() {
			minV = pps.MinVoltage()
		}
		if maxV > pps.MaxVoltage() {
			maxV = pps.MaxVoltage()
		}
		if minV <= maxV && c.policy.MaxCurrent <= pps.MaxCurrent() {
			if policy.PreferLowerVoltage && minV < bestVoltage {
				rdo.SetSelectedObjectPosition(uint8(i) + 1)
				rdo.SetPPSOutputVoltage(minV)
				rdo.SetPPSOutputCurrent(c.policy.MaxCurrent)
				bestVoltage = minV
			} else if !policy.PreferLowerVoltage && maxV > bestVoltage {
				rdo.SetSelectedObjectPosition(uint8(i) + 1)
				rdo.SetPPSOutputVoltage(maxV)
				rdo.SetPPSOutputCurrent(c.policy.MaxCurrent)
				bestVoltage = maxV
			}
		}
	}
	return rdo
}

// CVPolicy defines a constant voltage policy where the power source is expected
// to be capabale of supplying at least MinCurrent at any voltage between
// MinVoltage and MaxVoltage.
type CVPolicy struct {

	// Minimum accepted voltage in millivolts.
	MinVoltage uint16

	// Maximum accepted voltage in millivolts.
	MaxVoltage uint16

	// Minimum current in milliamps that the source must be able to supply
	// across the entire range of voltages between MinVoltage and MaxVoltage.
	MinCurrent uint16

	// If a source provides multiple profile within the voltage range of a
	// policy, it's possible to prefer lower voltage profiles than the default
	// higher voltage profiles.
	PreferLowerVoltage bool
}

// CV implements a constant voltage policy manager. Constant voltage sources are
// the most common. The can be used to power most devices and are the easiest to
// think about.
//
// CV takes advantage of both fixed and programmable PD profiles. In case of
// programmable, 150mA margin is added to the MinCurrent defined by the policy
// to ensure the power supply does not limit current close to the minimum
// required.
type CV struct {
	mu     sync.Mutex
	policy CVPolicy
}

const cvOvercurrentMargin = 150 // mA

// SetPolicy updates the existing policy. Any future power negotiations will use
// the new policy. If immediate renegotation of power based on the new policy is
// required, tcpe.PolicyEngine.Renegotiate() must be called.
//
// SetPolicy can be called concurrently from multiple goroutines.
func (c *CV) SetPolicy(p CVPolicy) {
	c.mu.Lock()
	c.policy = p
	c.mu.Unlock()
}

// GetPolicy returns the current policy.
func (c *CV) GetPolicy() CVPolicy {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.policy
}

// EvaluateCapabilities evaluates the provided power profiles against the policy
// and returns a RequestDO that can be used to negotiate with the power
// source.
func (c *CV) EvaluateCapabilities(pdos []pdmsg.PDO) pdmsg.RequestDO {
	c.mu.Lock()
	policy := c.policy
	c.mu.Unlock()

	ppsMinCurrent := c.policy.MinCurrent + cvOvercurrentMargin

	var bestVoltage uint16
	if policy.PreferLowerVoltage {
		bestVoltage = ^uint16(0)
	}
	rdo := pdmsg.EmptyRequestDO
	for i, p := range pdos {
		switch p.Type() {
		case pdmsg.PDOTypeFixedSupply:
			fs := pdmsg.FixedSupplyPDO(p)
			v := fs.Voltage()
			if v >= c.policy.MinVoltage && v <= c.policy.MaxVoltage && fs.MaxCurrent() >= c.policy.MinCurrent {
				if (policy.PreferLowerVoltage && v < bestVoltage) || (!policy.PreferLowerVoltage && v > bestVoltage) {
					rdo.SetSelectedObjectPosition(uint8(i) + 1)
					rdo.SetFixedMaxOperatingCurrent(c.policy.MinCurrent)
					rdo.SetFixedOperatingCurrent(c.policy.MinCurrent)
					bestVoltage = v
				}
			}
		case pdmsg.PDOTypePPS:
			pps := pdmsg.PPSPDO(p)
			minV, maxV := c.policy.MinVoltage, c.policy.MaxVoltage
			if minV < pps.MinVoltage() {
				minV = pps.MinVoltage()
			}
			if maxV > pps.MaxVoltage() {
				maxV = pps.MaxVoltage()
			}
			if minV <= maxV && ppsMinCurrent <= pps.MaxCurrent() {
				if policy.PreferLowerVoltage && minV < bestVoltage {
					rdo.SetSelectedObjectPosition(uint8(i) + 1)
					rdo.SetPPSOutputVoltage(minV)
					rdo.SetPPSOutputCurrent(c.policy.MinCurrent)
					bestVoltage = minV
				} else if !policy.PreferLowerVoltage && maxV > bestVoltage {
					rdo.SetSelectedObjectPosition(uint8(i) + 1)
					rdo.SetPPSOutputVoltage(maxV)
					rdo.SetPPSOutputCurrent(c.policy.MinCurrent)
					bestVoltage = maxV
				}
			}
		}
	}
	return rdo
}

// CPPolicy defines a constant power policy where the power source is expected
// to be capabale of supplying at least MinPower at any voltage between
// MinVoltage and MaxVoltage. Since current is a function of voltage and power,
// the minimum expected current capability changes depending on the actual
// voltage output of the supply.
type CPPolicy struct {

	// Mininimum accepted voltage in millivolts.
	MinVoltage uint16

	// Maximum accepted voltage in millivolts.
	MaxVoltage uint16

	// Minimum power in milliwatts that the source must be able to supply
	// across the entire range of voltages between MinVoltage and MaxVoltage.
	MinPower uint16

	// If a source provides multiple profile within the voltage range of a
	// policy, it's possible to prefer lower voltage profiles than the default
	// higher voltage profiles.
	PreferLowerVoltage bool
}

// CP implements a constant power policy manager. Constant power source is
// useful for when the consumer has its own voltage conversion circuitry.
// Usually in these scenarios, the consumer expects a minimum amount of power,
// regardless of the voltage.
type CP struct {
	mu     sync.Mutex
	policy CPPolicy
}

// SetPolicy updates the existing policy. Any future power negotiations will use
// the new policy. If immediate renegotation of power based on the new policy is
// required, tcpe.PolicyEngine.Renegotiate() must be called.
//
// SetPolicy can be called concurrently from multiple goroutines.
func (c *CP) SetPolicy(p CPPolicy) {
	c.mu.Lock()
	c.policy = p
	c.mu.Unlock()
}

// GetPolicy returns the current policy.
func (c *CP) GetPolicy() CPPolicy {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.policy
}

// TODO

// Logger writes a textual description of source capabilities to a given
// io.Writer. It's mostly used for debugging purposes.
type Logger struct {
	w           io.Writer
	sep         string
	passthrough typec.DevicePolicyManager
}

// NewLogger creates a new logger which will write to the given writer and
// optionally passes through the evaluate calls. If no passthrough is provided,
// this DPM will respond with pdmsg.EmptyRequestDO when EvaluateCapabilities is
// called by the policy engine. Line separator is written to the writer after
// each line of output. Some common values are "\n", "\r", "\r\n".
func NewLogger(w io.Writer, lineSep string, passthrough typec.DevicePolicyManager) *Logger {
	return &Logger{
		w:           w,
		sep:         lineSep,
		passthrough: passthrough,
	}
}

// EvaluateCapabilities evaluates the provided power profiles against the policy
// and returns a RequestDO that can be used to negotiate with the power
// source.
func (l *Logger) EvaluateCapabilities(pdos []pdmsg.PDO) pdmsg.RequestDO {
	fmt.Fprintf(l.w, "Received %d profiles:%s", len(pdos), l.sep)
	for i, p := range pdos {
		fmt.Fprintf(l.w, "  %d) ", i+1)
		switch p.Type() {
		case pdmsg.PDOTypeFixedSupply:
			fs := pdmsg.FixedSupplyPDO(p)
			fmt.Fprintf(l.w, "Fixed %.1fV @ max. %.1fA", float32(fs.Voltage())/1000, float32(fs.MaxCurrent())/1000)
		case pdmsg.PDOTypeVariableSupply:
			fmt.Fprint(l.w, "Variable (not supported)")
		case pdmsg.PDOTypePPS:
			pps := pdmsg.PPSPDO(p)
			minV, maxV, maxC := float32(pps.MinVoltage())/1000, float32(pps.MaxVoltage())/1000, float32(pps.MaxCurrent())/1000
			fmt.Fprintf(l.w, "Programmable %.1f-%.1fV @ max. %.1fA", minV, maxV, maxC)
		case pdmsg.PDOTypeBattery:
			fmt.Fprint(l.w, "Battery (not supported)")
		case pdmsg.PDOTypeEPRAVS:
			fmt.Fprint(l.w, "EPRAVS (not supported)")
		default:
			fmt.Fprint(l.w, "INVALID!")
		}
		fmt.Fprint(l.w, l.sep)
	}
	if l.passthrough != nil {
		return l.passthrough.EvaluateCapabilities(pdos)
	}
	return pdmsg.EmptyRequestDO
}
