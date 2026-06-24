package gt06

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"time"
)

// ErrInvalidPacket is returned when the packet structure is invalid
type ErrInvalidPacket struct {
	Reason string
}

func (e *ErrInvalidPacket) Error() string {
	return fmt.Sprintf("invalid GT06 packet: %s", e.Reason)
}

// Decoder parses GT06 binary protocol packets
type Decoder struct{}

// NewDecoder creates a new GT06 decoder
func NewDecoder() *Decoder {
	return &Decoder{}
}

// DecodeFrame extracts a complete GT06 frame from raw bytes.
// Returns the RawPacket and total bytes consumed (including start+stop bytes).
// Returns an error if the buffer does not contain a complete valid frame.
func (d *Decoder) DecodeFrame(buf []byte) (*RawPacket, int, error) {
	if len(buf) < 5 {
		return nil, 0, &ErrInvalidPacket{Reason: "buffer too short"}
	}

	startBit := binary.BigEndian.Uint16(buf[0:2])
	if startBit != StartShort && startBit != StartLong {
		return nil, 0, &ErrInvalidPacket{Reason: fmt.Sprintf("invalid start bit: 0x%04X", startBit)}
	}

	var packetLen uint16
	var headerSize int

	if startBit == StartShort {
		// 0x7878: length field is 1 byte
		packetLen = uint16(buf[2])
		headerSize = 3 // 2 start + 1 length
	} else {
		// 0x7979: length field is 2 bytes
		if len(buf) < 6 {
			return nil, 0, &ErrInvalidPacket{Reason: "buffer too short for long packet"}
		}
		packetLen = binary.BigEndian.Uint16(buf[2:4])
		headerSize = 4 // 2 start + 2 length
	}

	// Total frame = header + length-counted content + 2 stop bytes
	totalLen := headerSize + int(packetLen) + 2
	if len(buf) < totalLen {
		return nil, 0, &ErrInvalidPacket{Reason: "incomplete packet in buffer"}
	}

	// Verify stop bytes
	if buf[totalLen-2] != StopByte1 || buf[totalLen-1] != StopByte2 {
		return nil, 0, &ErrInvalidPacket{Reason: "missing stop bytes 0x0D 0x0A"}
	}

	// "length" covers: protocol(1) + payload(N) + serial(2) + crc(2)
	content := buf[headerSize : headerSize+int(packetLen)]
	if len(content) < 5 {
		return nil, 0, &ErrInvalidPacket{Reason: "content too short"}
	}

	protocolNum := content[0]
	serialNumber := binary.BigEndian.Uint16(content[len(content)-4 : len(content)-2])
	crc := binary.BigEndian.Uint16(content[len(content)-2:])
	payload := content[1 : len(content)-4]

	// CRC verification over protocol + payload + serial (everything except the CRC itself)
	computedCRC := crcITU(content[:len(content)-2])
	if computedCRC != crc {
		return nil, 0, &ErrInvalidPacket{
			Reason: fmt.Sprintf("CRC mismatch: got 0x%04X, want 0x%04X", crc, computedCRC),
		}
	}

	return &RawPacket{
		StartBit:     startBit,
		Length:       packetLen,
		ProtocolNum:  protocolNum,
		Payload:      payload,
		SerialNumber: serialNumber,
		CRC:          crc,
	}, totalLen, nil
}

// DecodeLogin parses the login packet payload and extracts the IMEI
func (d *Decoder) DecodeLogin(pkt *RawPacket) (*LoginPacket, error) {
	if pkt.ProtocolNum != ProtoLogin {
		return nil, &ErrInvalidPacket{Reason: "not a login packet"}
	}
	if len(pkt.Payload) < 8 {
		return nil, &ErrInvalidPacket{Reason: "login payload too short"}
	}

	// GT06 IMEI encoding: 8 bytes BCD, first nibble is always 0
	imei := decodeIMEI(pkt.Payload[:8])

	return &LoginPacket{
		IMEI:         imei,
		SerialNumber: pkt.SerialNumber,
		Timestamp:    time.Now().UTC(),
	}, nil
}

// DecodeGPS parses a GPS location packet (0x10) or alarm packet (0x16)
func (d *Decoder) DecodeGPS(pkt *RawPacket) (*GPSPacket, error) {
	if pkt.ProtocolNum != ProtoGPS && pkt.ProtocolNum != ProtoAlarm {
		return nil, &ErrInvalidPacket{Reason: "not a GPS or alarm packet"}
	}

	payload := pkt.Payload
	if len(payload) < 22 {
		return nil, &ErrInvalidPacket{Reason: "GPS payload too short"}
	}

	offset := 0

	// Date/Time: YY MM DD HH MM SS (6 bytes)
	ts := time.Date(
		int(payload[offset])+2000,
		time.Month(payload[offset+1]),
		int(payload[offset+2]),
		int(payload[offset+3]),
		int(payload[offset+4]),
		int(payload[offset+5]),
		0, time.UTC,
	)
	offset += 6

	// GPS info length byte — upper 4 bits = satellite count
	gpsInfoLen := payload[offset]
	satellites := int(gpsInfoLen >> 4)
	offset++

	// Latitude: 4 bytes big-endian unsigned int (divide by 1,800,000 for degrees)
	latRaw := binary.BigEndian.Uint32(payload[offset : offset+4])
	offset += 4

	// Longitude: 4 bytes big-endian unsigned int
	lngRaw := binary.BigEndian.Uint32(payload[offset : offset+4])
	offset += 4

	// Speed: 1 byte, km/h
	speed := float64(payload[offset])
	offset++

	// Course/Status word: 2 bytes
	//   bits [9:0]  = heading (0–359°)
	//   bit  [10]   = ACC/ignition status
	//   bit  [11]   = GPS not positioned (0 = positioned)
	//   bit  [12]   = LBS-only locate (1 = LBS, 0 = GPS)
	//   bit  [13]   = South latitude flag
	//   bit  [14]   = West longitude flag
	//   bit  [15]   = GPS real-time flag
	courseStatus := binary.BigEndian.Uint16(payload[offset : offset+2])
	offset += 2

	heading    := float64(courseStatus & 0x03FF)
	isWest     := (courseStatus>>14)&0x01 != 0
	isSouth    := (courseStatus>>13)&0x01 != 0
	gpsRealTime := (courseStatus>>15)&0x01 != 0
	gpsFixed   := (courseStatus>>11)&0x01 == 0 // bit 11 = 0 means "positioned"
	ignition   := (courseStatus>>10)&0x01 != 0

	lat := float64(latRaw) / 1_800_000.0
	lng := float64(lngRaw) / 1_800_000.0
	if isSouth {
		lat = -lat
	}
	if isWest {
		lng = -lng
	}

	if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return nil, &ErrInvalidPacket{
			Reason: fmt.Sprintf("coordinates out of range: %.6f, %.6f", lat, lng),
		}
	}

	// LBS data: MCC(2) + MNC(1) + LAC(2) + CellID(3) = 8 bytes
	var mcc uint16
	var mnc uint8
	var lac uint16
	var cellID uint32

	if offset+8 <= len(payload) {
		mcc = binary.BigEndian.Uint16(payload[offset : offset+2])
		offset += 2
		mnc = payload[offset]
		offset++
		lac = binary.BigEndian.Uint16(payload[offset : offset+2])
		offset += 2
		cellID = uint32(payload[offset])<<16 | uint32(payload[offset+1])<<8 | uint32(payload[offset+2])
		offset += 3
	}

	// Alarm/language byte (present after LBS block)
	var alarmType uint8
	if offset < len(payload) {
		alarmType = payload[offset]
	}

	return &GPSPacket{
		Timestamp:    ts,
		Latitude:     lat,
		Longitude:    lng,
		Speed:        speed,
		Heading:      heading,
		Satellites:   satellites,
		GPSRealTime:  gpsRealTime,
		IsGPSFixed:   gpsFixed,
		Ignition:     ignition,
		AlarmType:    alarmType,
		MCC:          mcc,
		MNC:          mnc,
		LAC:          lac,
		CellID:       cellID,
		SerialNumber: pkt.SerialNumber,
	}, nil
}

// DecodeHeartbeat parses a heartbeat/status packet (0x13)
func (d *Decoder) DecodeHeartbeat(pkt *RawPacket) (*HeartbeatPacket, error) {
	if pkt.ProtocolNum != ProtoHeartbeat {
		return nil, &ErrInvalidPacket{Reason: "not a heartbeat packet"}
	}

	hb := &HeartbeatPacket{
		SerialNumber: pkt.SerialNumber,
		Timestamp:    time.Now().UTC(),
	}

	if len(pkt.Payload) >= 1 {
		hb.VoltageLevel = pkt.Payload[0]
	}
	if len(pkt.Payload) >= 2 {
		hb.GSMSignal = pkt.Payload[1]
	}
	if len(pkt.Payload) >= 4 {
		hb.AlarmStatus = binary.BigEndian.Uint16(pkt.Payload[2:4])
	}
	if len(pkt.Payload) >= 6 {
		hb.Language = binary.BigEndian.Uint16(pkt.Payload[4:6])
	}

	return hb, nil
}

// BuildServerResponse constructs the ACK packet sent back to the GT06 device.
// Format: 0x7878 | len(5) | protocolNum | serialHi | serialLo | crcHi | crcLo | 0x0D 0x0A
func BuildServerResponse(protocolNum uint8, serialNumber uint16) []byte {
	// Content = protocolNum(1) + serialNumber(2) — CRC computed over these
	crcInput := []byte{
		protocolNum,
		uint8(serialNumber >> 8),
		uint8(serialNumber & 0xFF),
	}
	crcVal := crcITU(crcInput)

	return []byte{
		0x78, 0x78,                        // start bits (short packet)
		0x05,                              // length: proto(1) + serial(2) + crc(2) = 5
		protocolNum,                       // echo back the protocol number
		uint8(serialNumber >> 8),          // serial high byte
		uint8(serialNumber & 0xFF),        // serial low byte
		uint8(crcVal >> 8),                // CRC high byte
		uint8(crcVal & 0xFF),              // CRC low byte
		StopByte1, StopByte2,             // 0x0D 0x0A
	}
}

// decodeIMEI converts 8 BCD-encoded bytes to a 15-digit IMEI string.
// GT06 stores the IMEI with the first nibble always 0, so we strip it.
func decodeIMEI(b []byte) string {
	hexStr := hex.EncodeToString(b)
	if len(hexStr) == 16 {
		return hexStr[1:] // strip leading zero nibble → 15 digits
	}
	return hexStr
}

// crcITU computes the CRC-ITU (CRC-16-CCITT, poly 0x1021) checksum used by GT06
func crcITU(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
