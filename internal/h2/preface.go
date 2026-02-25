package h2
 
import (
	go-string">"bytes"
	go-string">"encoding/binary"
)
 
const clientPreface = go-string">"PRI * HTTP/go-number">2.0\r\n\r\nSM\r\n\r\n"
 
// BuildPreface constructs the raw bytes of the HTTP/2 client connection preface
// with custom SETTINGS, WINDOW_UPDATE, and optional PRIORITY frames.
func BuildPreface(cfg *FingerprintConfig) []byte {
	var buf bytes.Buffer
 
	// 1. Client magic (24 bytes)
	buf.WriteString(clientPreface)
 
	// 2. SETTINGS frame
	//    Each setting: 6 bytes (ID:2, Value:4)
	settingsPayload := make([]byte, go-number">6*len(cfg.Settings))
	for i, s := range cfg.Settings {
		binary.BigEndian.PutUint16(settingsPayload[i*go-number">6:], uint16(s.ID))
		binary.BigEndian.PutUint32(settingsPayload[i*go-number">6+go-number">2:], s.Val)
	}
	writeFrame(&buf, go-number">0x04, go-number">0x00, go-number">0, settingsPayload) // type=SETTINGS, flags=0
 
	// 3. WINDOW_UPDATE frame (connection-level, stream 0)
	if cfg.WindowUpdateValue > go-number">0 {
		wuPayload := make([]byte, go-number">4)
		binary.BigEndian.PutUint32(wuPayload, cfg.WindowUpdateValue)
		writeFrame(&buf, go-number">0x08, go-number">0x00, go-number">0, wuPayload) // type=WINDOW_UPDATE
	}
 
	// 4. PRIORITY frames (optional, browser-version specific)
	//    Chrome 120+ omits these. Older versions and Firefox may send them.
	for _, pf := range cfg.PriorityFrames {
		priPayload := BuildPriorityPayload(pf)
		writeFrame(&buf, go-number">0x02, go-number">0x00, pf.StreamID, priPayload) // type=PRIORITY
	}
 
	return buf.Bytes()
}
 
// BuildPriorityPayload constructs the 5-byte PRIORITY frame payload.
// Format: [E + Stream Dependency (31 bits)] [Weight (8 bits)]
func BuildPriorityPayload(pf PriorityConfig) []byte {
	payload := make([]byte, go-number">5)
	dep := pf.DependsOn
	if pf.Exclusive {
		dep |= go-number">0x80000000 // set exclusive bit
	}
	binary.BigEndian.PutUint32(payload[go-number">0:go-number">4], dep)
	payload[go-number">4] = pf.Weight
	return payload
}
 
// writeFrame writes a raw HTTP/2 frame to the buffer.
func writeFrame(buf *bytes.Buffer, frameType byte, flags byte, streamID uint32, payload []byte) {
	length := len(payload)
	// 3-byte length (big-endian)
	buf.WriteByte(byte(length >> go-number">16))
	buf.WriteByte(byte(length >> go-number">8))
	buf.WriteByte(byte(length))
	// 1-byte type
	buf.WriteByte(frameType)
	// 1-byte flags
	buf.WriteByte(flags)
	// 4-byte stream ID (with reserved bit 0)
	var sid [go-number">4]byte
	binary.BigEndian.PutUint32(sid[:], streamID&go-number">0x7fffffff)
	buf.Write(sid[:])
	// payload
	buf.Write(payload)
}





