// Package pdmsg defines types to encode and decode USB-C Power Delivery
// Messages.
package pdmsg

const (
	// MaxDataObjects is the maximum number of data objects that can be stored in
	// a message, as set by the standard.
	MaxDataObjects = 7

	// MaxMessageBytes is the maximum number of bytes in a message which includes
	// the header and the data objects.
	MaxMessageBytes = 2 + 4*MaxDataObjects // 2 bytes header, and 7 data objects, each 32 bits (4 bytes)
)

// Message represents a power delivery message.
// Decoding of extended messages is not supported.
type Message struct {
	Header uint16

	// Data varies depending on the type of the message. For TypeSourceCap and
	// TypeSinkCap, the data element should be converted to PDO, and further to
	// specific PDO type based on PDO.Type().
	//
	// Size of Data is fixed up to maximum allowable message size, to ensure no
	// heap allocations are necessary. To find out how many actual elements are
	// used, use DataObjectCount().
	Data [MaxDataObjects]uint32
}

// ToBytes serializes the message to a byte slice and returns the number of
// bytes written.
func (m Message) ToBytes(b []byte) uint8 {
	b[0] = byte(m.Header & 0xff)
	b[1] = byte((m.Header >> 8) & 0xff)
	c := m.DataObjectCount()
	for i, d := range m.Data[:c] {
		b[2+i*4] = byte(d & 0xff)
		b[3+i*4] = byte((d >> 8) & 0xff)
		b[4+i*4] = byte((d >> 16) & 0xff)
		b[5+i*4] = byte((d >> 24) & 0xff)
	}
	return 2 + c*4
}

// IsExtended returns true if the message has its extended flag set.
func (m Message) IsExtended() bool {
	return m.Header&(1<<15) != 0
}

// SetExtended sets the extended flag in the message.
func (m *Message) SetExtended(e bool) {
	var b uint16
	if e {
		b = 1 << 15
	}
	m.Header = (m.Header & ^(uint16(1) << 15)) | b
}

// ID returns the message ID.
func (m Message) ID() uint8 {
	return uint8((m.Header >> 9) & 0b111)
}

// SetID sets the message ID.
func (m *Message) SetID(id uint8) {
	m.Header = (m.Header & ^(uint16(0b111) << 9)) | (uint16(id) << 9)
}

// DataObjectCount returns the number of data objects in the message.
func (m Message) DataObjectCount() uint8 {
	return uint8((m.Header >> 12) & 0b111)
}

// SetDataObjectCount sets the number of data objects in the message.
func (m *Message) SetDataObjectCount(n uint8) {
	m.Header = (m.Header & ^(uint16(0b111) << 12)) | (uint16(n) << 12)
}

// IsData returns true of the message is a data message, otherwise it's a
// control message.
func (m Message) IsData() bool {
	return m.DataObjectCount() > 0
}

// Type returns the message type. As data and control messages share the same
// value of some types, the user must check IsData in addition to Type, to
// determine the correct type of the message.
func (m Message) Type() Type {
	return Type(m.Header & 0b11111)
}

// SetType sets the message type.
func (m *Message) SetType(t Type) {
	m.Header = (m.Header & ^uint16(0b11111)) | uint16(t)
}

// Type represents the PD message type. For control messages, the value of the
// type is equivalent to that of the PD spec. Actual message type requires
// determining if the message is a control or a data message using IsData().
type Type uint8

// Control message types
const (
	TypeGoodCRC      Type = 0b00001
	TypeAccept       Type = 0b00011
	TypeReject       Type = 0b00100
	TypePing         Type = 0b00101
	TypePSReady      Type = 0b00110
	TypeGetSourceCap Type = 0b00111
	TypeGetSinkCap   Type = 0b01000
	TypeWait         Type = 0b01100
	TypeSoftReset    Type = 0b01101
)

// Data message types
const (
	TypeSourceCap Type = 0b00001
	TypeRequest   Type = 0b00010
	TypeSinkCap   Type = 0b00100
)

// Revision returns the power delivery revision number of the message.
func (m Message) Revision() Revision {
	return Revision((m.Header >> 6) & 0b11)
}

// SetRevision sets the power delivery revision number of the message.
func (m *Message) SetRevision(r Revision) {
	m.Header = (m.Header & ^(uint16(0b11) << 6)) | uint16(r<<6)
}

// Revision represents the power delivery revision number of a message.
type Revision uint8

// Power delivery revision numbers.
const (
	Revision10 Revision = 0b00
	Revision20 Revision = 0b01
	Revision30 Revision = 0b10
)

// PowerRole returns the power role of the sender of the message.
func (m Message) PowerRole() PowerRole {
	return PowerRole((m.Header >> 8) & 1)
}

// SetPowerRole sets the power role of the sender of the message.
func (m *Message) SetPowerRole(r PowerRole) {
	m.Header = (m.Header & ^(uint16(1) << 8)) | (uint16(r) << 8)
}

// PowerRole represents the power role of the sender of a message.
type PowerRole uint8

// Power roles of the sender of a message.
const (
	PowerRoleSink   PowerRole = 0
	PowerRoleSource PowerRole = 1
)

// DataRole returns the data role of the sender of the message.
func (m Message) DataRole() DataRole {
	return DataRole((m.Header >> 5) & 1)
}

// SetDataRole sets the data role of the sender of the message.
func (m *Message) SetDataRole(r DataRole) {
	m.Header = (m.Header & ^(uint16(1) << 5)) | uint16(r<<5)
}

// DataRole represents the data role of the sender of a message.
type DataRole uint8

// Data roles of the sender of a message.
const (
	DataRoleUFP DataRole = 0
	DataRoleDFP DataRole = 1
)

// PDO is a generic Power Data Object. Based on its type, it should be
// converted to specific PDO type to allow extracting various fields.
type PDO uint32

// Type returns the type of the power data object.
func (o PDO) Type() PDOType {
	h := (o >> 30) & 0b11
	if h == 0b11 {
		return PDOType((((o >> 28) & 0b11) << 3) | 0b100 | h)
	}
	return PDOType(h)
}

// PDOType represents the type of a power data object.
type PDOType uint8

// Power data object types.
const (
	PDOTypeFixedSupply    PDOType = 0b00
	PDOTypeBattery        PDOType = 0b01
	PDOTypeVariableSupply PDOType = 0b10
	PDOTypePPS            PDOType = 0b00111 // This value is specific to our library
	PDOTypeEPRAVS         PDOType = 0b01111 // This value is specific to our library
)

// FixedSupplyPDO represents a Fixed Supply Power Data Object
type FixedSupplyPDO uint32

// NewFixedSupplyPDO returns a new blank FixedSupplyPDO.
func NewFixedSupplyPDO() FixedSupplyPDO {
	return FixedSupplyPDO(0)
}

// Voltage returns voltage in millivolts.
func (o FixedSupplyPDO) Voltage() uint16 {
	return uint16(((o >> 10) & (1<<10 - 1)) * 50)
}

// SetVoltage will round the given voltage to the nearest 50mV.
func (o *FixedSupplyPDO) SetVoltage(v uint16) {
	*o = (*o & ^((FixedSupplyPDO(1)<<10 - 1) << 10)) | ((FixedSupplyPDO(v)/50)&(1<<10-1))<<10
}

// MaxCurrent returns maximum current in milliamps
func (o FixedSupplyPDO) MaxCurrent() uint16 {
	return uint16((o & (1<<10 - 1)) * 10)
}

// SetMaxCurrent will round the given current to the nearest 10mV.
func (o *FixedSupplyPDO) SetMaxCurrent(v uint16) {
	*o = (*o & ^(FixedSupplyPDO(1)<<10 - 1)) | (FixedSupplyPDO(v)/10)&(1<<10-1)
}

// PPSPDO represents a Programmable Power Supply Power Data Object
type PPSPDO uint32

// NewPPSPDO returns a new blank programmable power supply power data object.
func NewPPSPDO() PPSPDO {
	return PPSPDO(0b11) << 30
}

// MinVoltage returns minimum voltage in millivolts.
func (o PPSPDO) MinVoltage() uint16 {
	return ((uint16(o) >> 8) & (uint16(1)<<8 - 1)) * 100
}

// SetMinVoltage sets the minimum voltage in millivolts. The voltage will be
// rounded to the nearest 100mV.
func (o *PPSPDO) SetMinVoltage(v uint16) {
	*o = (*o & ^((PPSPDO(1)<<8 - 1) << 8)) | PPSPDO((v/100)&(1<<8-1))<<8
}

// MaxVoltage returns maximum voltage in millivolts.
func (o PPSPDO) MaxVoltage() uint16 {
	return (uint16(o>>17) & (uint16(1)<<8 - 1)) * 100
}

// SetMaxVoltage sets the maximum voltage in millivolts. The voltage will be
// rounded to the nearest 100mV.
func (o *PPSPDO) SetMaxVoltage(v uint16) {
	*o = (*o & ^((PPSPDO(1)<<8 - 1) << 17)) | PPSPDO((v/100)&(1<<8-1))<<17
}

// MaxCurrent returns maximum current in milliamps.
func (o PPSPDO) MaxCurrent() uint16 {
	return (uint16(o) & (uint16(1)<<7 - 1)) * 50
}

// SetMaxCurrent sets the maximum current in milliamps. The current will be
// rounded to the nearest 50mA.
func (o *PPSPDO) SetMaxCurrent(c uint16) {
	*o = (*o & ^(PPSPDO(1)<<8 - 1)) | PPSPDO((c/50)&(1<<7-1))
}

// RequestDO represents a Request Data Object.
type RequestDO uint32

// EmptyRequestDO is returned by device policy managers to indicate that they do
// not accept any of the power profiles supported by the power source.
const EmptyRequestDO RequestDO = 0

// SelectedObjectPosition returns the position number of the PDO in the source
// capability message, starting at 1.
func (o RequestDO) SelectedObjectPosition() uint8 {
	return uint8(o >> 28)
}

// SetSelectedObjectPosition sets the position number of the PDO the source
// capability message, starting at 1.
func (o *RequestDO) SetSelectedObjectPosition(p uint8) {
	*o = (*o & ^(RequestDO(0b1111) << 28)) | RequestDO(p)<<28
}

// CapabilityMismatch returns true if capability mismatch flag of the RDO is
// set.
func (o RequestDO) CapabilityMismatch() bool {
	return o&(1<<26) != 0
}

// SetCapabilityMismatch sets the capability mismatch flag of the RDO.
func (o *RequestDO) SetCapabilityMismatch(m bool) {
	var b RequestDO
	if m {
		b = 1 << 26
	}
	*o = (*o & ^(RequestDO(1) << 26)) | b
}

// FixedOperatingCurrent returns current in milliamps for fixed request
// objects.
func (o RequestDO) FixedOperatingCurrent() uint16 {
	return uint16(((o >> 10) & (1<<10 - 1)) * 10)
}

// SetFixedOperatingCurrent sets current in milliamps rounded to nearest 10mA
// for fixed request objects.
func (o *RequestDO) SetFixedOperatingCurrent(c uint16) {
	*o = (*o & ^((RequestDO(1)<<10 - 1) << 10)) | ((RequestDO(c)/10)&(1<<10-1))<<10
}

// FixedMaxOperatingCurrent returns current in milliamps for fixed request
// objects without GiveBack support.
func (o RequestDO) FixedMaxOperatingCurrent() uint16 {
	return uint16((o & (1<<10 - 1)) * 10)
}

// SetFixedMaxOperatingCurrent sets current in milliamps rounded to nearest
// 10mA for fixed request objects without GiveBack support.
func (o *RequestDO) SetFixedMaxOperatingCurrent(c uint16) {
	*o = (*o & ^(RequestDO(1)<<10 - 1)) | ((RequestDO(c) / 10) & (1<<10 - 1))
}

// PPSOutputVoltage returns voltage in millivolts for PPS data objects.
func (o RequestDO) PPSOutputVoltage() uint16 {
	return uint16(((o >> 9) & (1<<12 - 1)) * 20)
}

// SetPPSOutputVoltage sets voltage in millivolts rounded to nearest 20mV for
// PPS data objects.
func (o *RequestDO) SetPPSOutputVoltage(v uint16) {
	*o = (*o & ^((RequestDO(1)<<12 - 1) << 9)) | ((RequestDO(v)/20)&(1<<12-1))<<9
}

// PPSOutputCurrent returns current in milliamps for PPS data objects.
func (o RequestDO) PPSOutputCurrent() uint16 {
	return uint16((o & (1<<7 - 1)) * 50)
}

// SetPPSOutputCurrent sets current in milliamps rounded to nearest 50mA for
// PPS data objects.
func (o *RequestDO) SetPPSOutputCurrent(v uint16) {
	*o = (*o & ^(RequestDO(1)<<7 - 1)) | (RequestDO(v)/50)&(1<<7-1)
}
