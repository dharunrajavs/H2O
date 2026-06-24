package gt06

import "time"

// Packet start markers
const (
	StartShort = 0x7878 // short packet start
	StartLong  = 0x7979 // long packet start
	StopByte1  = 0x0D
	StopByte2  = 0x0A
)

// Protocol numbers defined by the GT06 specification
const (
	ProtoLogin           = 0x01
	ProtoGPS             = 0x10
	ProtoGPSShort        = 0x11
	ProtoHeartbeat       = 0x13
	ProtoStringInfo      = 0x15
	ProtoAlarm           = 0x16
	ProtoExternalPower   = 0x19
	ProtoGPSLBSMulti     = 0x1A
	ProtoGPSSignal       = 0x1E
	ProtoGPSLBSQuery     = 0x25
	ProtoBlindSpot       = 0x26
	ProtoServerResponse  = 0x8001
	ProtoTimeCalibration = 0x8805
)

// Alarm type codes from the GT06 specification
const (
	AlarmSOS        = 0x01
	AlarmPowerCut   = 0x02
	AlarmVibration  = 0x03
	AlarmFence      = 0x04
	AlarmOverSpeed  = 0x05
	AlarmLowBattery = 0x06
	AlarmShock      = 0x09
	AlarmAccOn      = 0x0E
	AlarmAccOff     = 0x0F
)

// RawPacket is the parsed outer frame before protocol-specific decoding
type RawPacket struct {
	StartBit     uint16
	Length       uint16
	ProtocolNum  uint8
	Payload      []byte
	SerialNumber uint16
	CRC          uint16
}

// LoginPacket carries device IMEI and authentication info
type LoginPacket struct {
	IMEI         string
	SerialNumber uint16
	Timestamp    time.Time
}

// GPSPacket is a fully decoded GPS location event
type GPSPacket struct {
	Timestamp    time.Time
	Latitude     float64
	Longitude    float64
	Speed        float64 // km/h
	Heading      float64 // degrees 0–360
	Satellites   int
	GPSRealTime  bool
	IsGPSFixed   bool
	Ignition     bool // ACC (accessory) status
	AlarmType    uint8
	MCC          uint16 // Mobile Country Code
	MNC          uint8  // Mobile Network Code
	LAC          uint16 // Location Area Code
	CellID       uint32
	SerialNumber uint16
	IMEI         string // filled in by handler from session
}

// HeartbeatPacket is a keepalive sent by the device periodically
type HeartbeatPacket struct {
	SerialNumber uint16
	Timestamp    time.Time
	VoltageLevel uint8
	GSMSignal    uint8
	AlarmStatus  uint16
	Language     uint16
}

// AlarmPacket carries alarm event data with an embedded GPSPacket
type AlarmPacket struct {
	GPSPacket
	AlarmType uint8
}

// PowerPacket carries external power voltage info
type PowerPacket struct {
	Voltage      float64
	SerialNumber uint16
	Timestamp    time.Time
}

// ServerResponse is the ACK sent back to the device after each packet
type ServerResponse struct {
	SerialNumber uint16
	ProtocolNum  uint8
}

// GPSEvent is the normalized event published to the event bus
type GPSEvent struct {
	IMEI         string    `json:"imei"`
	TenantID     string    `json:"tenant_id"`
	DeviceID     string    `json:"device_id"`
	Latitude     float64   `json:"lat"`
	Longitude    float64   `json:"lng"`
	Speed        float64   `json:"speed"`
	Heading      float64   `json:"heading"`
	Satellites   int       `json:"satellites"`
	IsGPSFixed   bool      `json:"gps_fixed"`
	Ignition     bool      `json:"ignition"`
	AlarmType    uint8     `json:"alarm_type,omitempty"`
	MCC          uint16    `json:"mcc,omitempty"`
	MNC          uint8     `json:"mnc,omitempty"`
	LAC          uint16    `json:"lac,omitempty"`
	CellID       uint32    `json:"cell_id,omitempty"`
	RecordedAt   time.Time `json:"recorded_at"`
	ReceivedAt   time.Time `json:"received_at"`
	PacketSerial uint16    `json:"packet_serial"`
}

// LoginEvent is published when a device successfully authenticates
type LoginEvent struct {
	IMEI       string    `json:"imei"`
	TenantID   string    `json:"tenant_id"`
	DeviceID   string    `json:"device_id"`
	RemoteAddr string    `json:"remote_addr"`
	LoginAt    time.Time `json:"login_at"`
}

// HeartbeatEvent is published for device keepalives
type HeartbeatEvent struct {
	IMEI         string    `json:"imei"`
	TenantID     string    `json:"tenant_id"`
	VoltageLevel uint8     `json:"voltage_level"`
	GSMSignal    uint8     `json:"gsm_signal"`
	ReceivedAt   time.Time `json:"received_at"`
}

// AlarmEvent wraps a GPS event with alarm-specific fields
type AlarmEvent struct {
	GPSEvent
	AlarmCode uint8  `json:"alarm_code"`
	AlarmName string `json:"alarm_name"`
}

// AlarmName maps an alarm code to a human-readable name
func AlarmName(code uint8) string {
	switch code {
	case AlarmSOS:
		return "SOS"
	case AlarmPowerCut:
		return "power_cut"
	case AlarmVibration:
		return "vibration"
	case AlarmFence:
		return "geofence"
	case AlarmOverSpeed:
		return "overspeed"
	case AlarmLowBattery:
		return "low_battery"
	case AlarmShock:
		return "shock"
	case AlarmAccOn:
		return "acc_on"
	case AlarmAccOff:
		return "acc_off"
	default:
		return "unknown"
	}
}
