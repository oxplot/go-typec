// Package tcdpm implements some useful device policy managers for common use.
package tcdpm

import (
	"errors"
	"fmt"
	"io"

	"github.com/oxplot/go-typec/pdmsg"
	"github.com/oxplot/go-typec/tcpe"
)

// Policy is the interface which simply embeds CapabilityEvaluator.
type Policy interface {
	// Validate returns an error if the policy parameters are invalid.
	Validate() error
	tcpe.CapabilityEvaluator
}

// CCPolicy defines a constant current policy where the power source is expected
// to drop the voltage if needed to maintain the current under the negotiated
// current. If current is below the negotiated current, the power source is
// expected to increase the voltage up to the negotiated voltage.
//
// Below are some examples of where a constant current supply is useful:
//
//   - Driving LEDs
//   - Charging Li-ion batteries
//
// Constant current capability is only available in PD power sources that
// support Programmable Power Supply (PPS) standard.
//
// WARNING: Most PD power sources are not compliant with PPS standard and do not
// implement constant current capability. There is no way to identify such
// chargers via the PD protocol alone. Always ensure your specific charger
// supports constant current capability before using it in your application by
// running it under load.
type CCPolicy struct {

	// Minimum accepted voltage in millivolts when current is below MaxCurrent.
	MinVoltage uint16

	// Maximum accpeted voltage in millivolts when current is below MaxCurrent.
	MaxVoltage uint16

	// Minimum current in milliamps that should be supplied under all load
	// conditions. Note that per standard, current for this policy (which uses
	// PPS) must be >= 1000mA.
	MinCurrent uint16

	// Maximum current in milliamps that should be supplied under all load
	// conditions. Note that per standard, current for this policy (which uses
	// PPS) must be >= 1000mA.
	// Higher currents up to MaxCurrent are preferred over lower currents.
	MaxCurrent uint16

	// If a source provides multiple profile within the voltage range of a
	// policy, it's possible to prefer lower voltage profiles than the default
	// higher voltage profiles.
	PreferLowerVoltage bool
}

var (
	errCCBadCurrent          = errors.New("tcdpm: current must be >= 1000mA & <= 5000mA")
	errBadVoltage            = errors.New("tcdpm: voltage must be >= 3300mV & <= 21000 mV")
	errCVBadCurrent          = errors.New("tcdpm: current must be >= 0mA & <= 5000mA")
	errMaxCurrentLessThanMin = errors.New("tcdpm: max current must be >= min current")
	errMaxVoltageLessThanMin = errors.New("tcdpm: max voltage must be >= min voltage")
)

// Validate returns an error if the policy parameters are invalid.
func (c CCPolicy) Validate() error {
	if c.MinCurrent < 1000 || c.MaxCurrent < 1000 || c.MinCurrent > 5000 || c.MaxCurrent > 5000 {
		return errCCBadCurrent
	}
	if c.MinVoltage < 3300 || c.MaxVoltage < 3300 || c.MinVoltage > 21000 || c.MaxVoltage > 21000 {
		return errBadVoltage
	}
	if c.MinCurrent > c.MaxCurrent {
		return errMaxCurrentLessThanMin
	}
	if c.MinVoltage > c.MaxVoltage {
		return errMaxVoltageLessThanMin
	}
	return nil
}

// EvaluateCapabilities evaluates the provided power profiles against the policy
// and returns a RequestDO that can be used to negotiate with the power
// source.
func (c CCPolicy) EvaluateCapabilities(pdos []pdmsg.PDO) pdmsg.RequestDO {
	var bestVoltage uint16
	if c.PreferLowerVoltage {
		bestVoltage = ^uint16(0)
	}
	rdo := pdmsg.EmptyRequestDO
	for i, p := range pdos {
		if p.Type() != pdmsg.PDOTypePPS {
			continue
		}
		pps := pdmsg.PPSPDO(p)
		minV, maxV := c.MinVoltage, c.MaxVoltage
		if minV < pps.MinVoltage() {
			minV = pps.MinVoltage()
		}
		if maxV > pps.MaxVoltage() {
			maxV = pps.MaxVoltage()
		}
		if minV <= maxV && pps.MaxCurrent() >= c.MinCurrent {
			cur := pps.MaxCurrent()
			if pps.MaxCurrent() > c.MaxCurrent {
				cur = c.MaxCurrent
			}
			if c.PreferLowerVoltage && minV < bestVoltage {
				rdo.SetSelectedObjectPosition(uint8(i) + 1)
				rdo.SetPPSOutputVoltage(minV)
				rdo.SetPPSOutputCurrent(cur)
				bestVoltage = minV
			} else if !c.PreferLowerVoltage && maxV > bestVoltage {
				rdo.SetSelectedObjectPosition(uint8(i) + 1)
				rdo.SetPPSOutputVoltage(maxV)
				rdo.SetPPSOutputCurrent(cur)
				bestVoltage = maxV
			}
		}
	}
	return rdo
}

// CVPolicy defines a constant voltage policy where the power source is expected
// to maintain the negotiated voltage and to be capable of supplying at least
// the negotiated current.
//
// CVPolicy takes advantage of both fixed and programmable PD profiles. In case
// of programmable, 150mA margin is added to the Current defined by the policy
// to ensure the power supply does not limit current close to the operating
// current.
type CVPolicy struct {

	// Minimum accepted voltage in millivolts.
	MinVoltage uint16

	// Maximum accepted voltage in millivolts.
	MaxVoltage uint16

	// Current in milliamps that the source must be able to supply at the
	// negotiated voltage.
	Current uint16

	// If a source provides multiple profile within the voltage range of a
	// policy, it's possible to prefer lower voltage profiles than the default
	// higher voltage profiles.
	PreferLowerVoltage bool

	// By default, CVPolicy prefers fixed PD profiles unless none can satisfy the
	// requirements in which case PPS profiles are considered. If this is set to
	// true, CVPolicy will prefer PPS profiles over fixed ones.
	PreferPPS bool
}

const cvCurrentMargin = 150 // mA

// Validate returns an error if the policy parameters are invalid.
func (c CVPolicy) Validate() error {
	if c.Current > 5000 {
		return errCVBadCurrent
	}
	if c.MinVoltage < 3300 || c.MaxVoltage < 3300 || c.MinVoltage > 21000 || c.MaxVoltage > 21000 {
		return errBadVoltage
	}
	if c.MinVoltage > c.MaxVoltage {
		return errMaxVoltageLessThanMin
	}
	return nil
}

// EvaluateCapabilities evaluates the provided power profiles against the policy
// and returns a RequestDO that can be used to negotiate with the power
// source.
func (c *CVPolicy) EvaluateCapabilities(pdos []pdmsg.PDO) pdmsg.RequestDO {
	ppsMaxCurrent := c.Current + cvCurrentMargin

	var bestFixedVoltage, bestPPSVoltage uint16
	if c.PreferLowerVoltage {
		bestFixedVoltage = ^uint16(0)
		bestPPSVoltage = ^uint16(0)
	}
	bestFixedRDO := pdmsg.EmptyRequestDO
	bestPPSRDO := pdmsg.EmptyRequestDO
	for i, p := range pdos {
		switch p.Type() {
		case pdmsg.PDOTypeFixedSupply:
			fs := pdmsg.FixedSupplyPDO(p)
			v := fs.Voltage()
			if v >= c.MinVoltage && v <= c.MaxVoltage && fs.MaxCurrent() >= c.Current {
				if (c.PreferLowerVoltage && v < bestFixedVoltage) || (!c.PreferLowerVoltage && v > bestFixedVoltage) {
					bestFixedRDO.SetSelectedObjectPosition(uint8(i) + 1)
					bestFixedRDO.SetFixedMaxOperatingCurrent(c.Current)
					bestFixedRDO.SetFixedOperatingCurrent(c.Current)
					bestFixedVoltage = v
				}
			}
		case pdmsg.PDOTypePPS:
			pps := pdmsg.PPSPDO(p)
			minV, maxV := c.MinVoltage, c.MaxVoltage
			if minV < pps.MinVoltage() {
				minV = pps.MinVoltage()
			}
			if maxV > pps.MaxVoltage() {
				maxV = pps.MaxVoltage()
			}
			if minV <= maxV && ppsMaxCurrent <= pps.MaxCurrent() {
				if c.PreferLowerVoltage && minV < bestPPSVoltage {
					bestPPSRDO.SetSelectedObjectPosition(uint8(i) + 1)
					bestPPSRDO.SetPPSOutputVoltage(minV)
					bestPPSRDO.SetPPSOutputCurrent(c.Current)
					bestPPSVoltage = minV
				} else if !c.PreferLowerVoltage && maxV > bestPPSVoltage {
					bestPPSRDO.SetSelectedObjectPosition(uint8(i) + 1)
					bestPPSRDO.SetPPSOutputVoltage(maxV)
					bestPPSRDO.SetPPSOutputCurrent(c.Current)
					bestPPSVoltage = maxV
				}
			}
		}
	}
	if bestFixedRDO == pdmsg.EmptyRequestDO {
		return bestPPSRDO
	}
	if bestPPSRDO == pdmsg.EmptyRequestDO {
		return bestFixedRDO
	}
	if c.PreferPPS {
		return bestPPSRDO
	}
	return bestFixedRDO
}

// CPPolicy defines a constant power policy where the power source is expected
// to be capabale of supplying at the specified power at the negotiated voltage.
// CPPolicy is a special case of CVPolicy where the current is calculated from
// the power and voltage.
type CPPolicy struct {

	// Minimum accepted voltage in millivolts.
	MinVoltage uint16

	// Maximum accepted voltage in millivolts.
	MaxVoltage uint16

	// Power in milliwatts that the source must be able to supply at the
	// negotiated voltage.
	Power uint16

	// If a source provides multiple profile within the voltage range of a
	// policy, it's possible to prefer lower voltage profiles than the default
	// higher voltage profiles.
	PreferLowerVoltage bool

	// By default, CPPolicy prefers fixed PD profiles unless none can satisfy the
	// requirements in which case PPS profiles are considered. If this is set to
	// true, CPPolicy will prefer PPS profiles over fixed ones.
	PreferPPS bool
}

// EvaluateCapabilities evaluates the provided power profiles against the policy
// and returns a RequestDO that can be used to negotiate with the power
// source.
func (c *CPPolicy) EvaluateCapabilities(pdos []pdmsg.PDO) pdmsg.RequestDO {
	var bestFixedVoltage, bestPPSVoltage uint16
	if c.PreferLowerVoltage {
		bestFixedVoltage = ^uint16(0)
		bestPPSVoltage = ^uint16(0)
	}
	bestFixedRDO := pdmsg.EmptyRequestDO
	bestPPSRDO := pdmsg.EmptyRequestDO
	for i, p := range pdos {
		switch p.Type() {
		case pdmsg.PDOTypeFixedSupply:
			fs := pdmsg.FixedSupplyPDO(p)
			v := fs.Voltage()
			maxCur := c.Power / v
			if v >= c.MinVoltage && v <= c.MaxVoltage && fs.MaxCurrent() >= maxCur {
				if (c.PreferLowerVoltage && v < bestFixedVoltage) || (!c.PreferLowerVoltage && v > bestFixedVoltage) {
					bestFixedRDO.SetSelectedObjectPosition(uint8(i) + 1)
					bestFixedRDO.SetFixedMaxOperatingCurrent(maxCur)
					bestFixedRDO.SetFixedOperatingCurrent(maxCur)
					bestFixedVoltage = v
				}
			}
		case pdmsg.PDOTypePPS:
			pps := pdmsg.PPSPDO(p)
			minV, maxV := c.MinVoltage, c.MaxVoltage
			if minV < pps.MinVoltage() {
				minV = pps.MinVoltage()
			}
			if maxV > pps.MaxVoltage() {
				maxV = pps.MaxVoltage()
			}
			if minV <= maxV {
				maxC := c.Power/maxV + cvCurrentMargin
				minPV := c.Power / (pps.MaxCurrent() - cvCurrentMargin)
				if minPV < minV {
					minPV = minV
				}
				if c.PreferLowerVoltage && minPV < bestPPSVoltage && minPV <= maxV {
					bestPPSRDO.SetSelectedObjectPosition(uint8(i) + 1)
					bestPPSRDO.SetPPSOutputVoltage(minPV)
					bestPPSRDO.SetPPSOutputCurrent(c.Power / minPV)
					bestPPSVoltage = minPV
				} else if !c.PreferLowerVoltage && maxV > bestPPSVoltage && maxC <= pps.MaxCurrent() {
					bestPPSRDO.SetSelectedObjectPosition(uint8(i) + 1)
					bestPPSRDO.SetPPSOutputVoltage(maxV)
					bestPPSRDO.SetPPSOutputCurrent(maxC)
					bestPPSVoltage = maxV
				}
			}
		}
	}
	if bestFixedRDO == pdmsg.EmptyRequestDO {
		return bestPPSRDO
	}
	if bestPPSRDO == pdmsg.EmptyRequestDO {
		return bestFixedRDO
	}
	if c.PreferPPS {
		return bestPPSRDO
	}
	return bestFixedRDO
}

// Logger is a passthrough policy that writes a textual description of source
// capabilities to a given io.Writer. It's mostly used for debugging purposes.
type Logger struct {
	w    io.Writer
	sep  string
	base Policy
}

// NewLogger creates a new logger which will write to the given writer and
// optionally passes through the evaluate calls. If no base is provided,
// this policy will respond with pdmsg.EmptyRequestDO when EvaluateCapabilities
// is called by the policy engine. Line separator is written to the writer after
// each line of output. Some common values are "\n", "\r", "\r\n".
func NewLogger(w io.Writer, lineSep string, base Policy) *Logger {
	return &Logger{
		w:    w,
		sep:  lineSep,
		base: base,
	}
}

// Validate returns nil if the policy is valid.
func (l *Logger) Validate() error {
	if l.base != nil {
		return l.base.Validate()
	}
	return nil
}

// EvaluateCapabilities writes out the textual description of the provided
// power data objects and passes it down to the underlying DPM and returns its
// response.
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
			var powerLimited string
			if pps.IsPowerLimited() {
				powerLimited = " (power limited)"
			}
			minV, maxV, maxC := float32(pps.MinVoltage())/1000, float32(pps.MaxVoltage())/1000, float32(pps.MaxCurrent())/1000
			fmt.Fprintf(l.w, "Programmable %.1f-%.1fV @ max. %.1fA%s", minV, maxV, maxC, powerLimited)
		case pdmsg.PDOTypeBattery:
			fmt.Fprint(l.w, "Battery (not supported)")
		case pdmsg.PDOTypeEPRAVS:
			fmt.Fprint(l.w, "EPRAVS (not supported)")
		default:
			fmt.Fprint(l.w, "INVALID!")
		}
		fmt.Fprint(l.w, l.sep)
	}
	if l.base != nil {
		return l.base.EvaluateCapabilities(pdos)
	}
	return pdmsg.EmptyRequestDO
}
