package h2

import (
	"bytes"
	"encoding/binary"
)

const clientPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

// HTTP/2 帧类型
const (
	frameTypeData         = 0x00
	frameTypeHeaders      = 0x01
	frameTypePriority     = 0x02
	frameTypeRstStream    = 0x03
	frameTypeSettings     = 0x04
	frameTypePushPromise  = 0x05
	frameTypePing         = 0x06
	frameTypeGoAway       = 0x07
	frameTypeWindowUpdate = 0x08
	frameTypeContinuation = 0x09
)

// BuildPreface 构建 HTTP/2 连接前言
func BuildPreface(cfg *FingerprintConfig) []byte {
	var buf bytes.Buffer

	// 1. 发送 HTTP/2 连接前言
	buf.WriteString(clientPreface)

	// 2. 发送 SETTINGS 帧
	settingsPayload := make([]byte, 6*len(cfg.Settings))
	for i, s := range cfg.Settings {
		binary.BigEndian.PutUint16(settingsPayload[i*6:], uint16(s.ID))
		binary.BigEndian.PutUint32(settingsPayload[i*6+2:], s.Val)
	}
	writeFrame(&buf, frameTypeSettings, 0x00, 0, settingsPayload)

	// 3. 发送 WINDOW_UPDATE 帧（连接级别）
	if cfg.WindowUpdateValue > 0 {
		wuPayload := make([]byte, 4)
		binary.BigEndian.PutUint32(wuPayload, cfg.WindowUpdateValue)
		writeFrame(&buf, frameTypeWindowUpdate, 0x00, 0, wuPayload)
	}

	// 4. 发送 PRIORITY 帧（Chrome 指纹核心）
	if cfg.SendPriority && len(cfg.PriorityFrames) > 0 {
		for _, pf := range cfg.PriorityFrames {
			priPayload := BuildPriorityPayload(pf)
			writeFrame(&buf, frameTypePriority, 0x00, pf.StreamID, priPayload)
		}
	}

	return buf.Bytes()
}

// BuildPriorityPayload 构建 PRIORITY 帧的 payload
func BuildPriorityPayload(pf PriorityConfig) []byte {
	payload := make([]byte, 5)

	dep := pf.DependsOn
	if pf.Exclusive {
		dep |= 0x80000000
	}
	binary.BigEndian.PutUint32(payload[0:4], dep)
	payload[4] = pf.Weight

	return payload
}

// writeFrame 写入 HTTP/2 帧
func writeFrame(buf *bytes.Buffer, frameType byte, flags byte, streamID uint32, payload []byte) {
	length := len(payload)

	buf.WriteByte(byte(length >> 16))
	buf.WriteByte(byte(length >> 8))
	buf.WriteByte(byte(length))
	buf.WriteByte(frameType)
	buf.WriteByte(flags)

	var sid [4]byte
	binary.BigEndian.PutUint32(sid[:], streamID&0x7fffffff)
	buf.Write(sid[:])
	buf.Write(payload)
}

// BuildSettingsAckFrame 构建 SETTINGS ACK 帧
func BuildSettingsAckFrame() []byte {
	var buf bytes.Buffer
	writeFrame(&buf, frameTypeSettings, 0x01, 0, nil)
	return buf.Bytes()
}

// BuildPingFrame 构建 PING 帧
func BuildPingFrame(ack bool, data [8]byte) []byte {
	var buf bytes.Buffer
	flags := byte(0x00)
	if ack {
		flags = 0x01
	}
	writeFrame(&buf, frameTypePing, flags, 0, data[:])
	return buf.Bytes()
}

// BuildWindowUpdateFrame 构建 WINDOW_UPDATE 帧
func BuildWindowUpdateFrame(streamID uint32, increment uint32) []byte {
	var buf bytes.Buffer
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, increment&0x7fffffff)
	writeFrame(&buf, frameTypeWindowUpdate, 0x00, streamID, payload)
	return buf.Bytes()
}

// BuildGoAwayFrame 构建 GOAWAY 帧
func BuildGoAwayFrame(lastStreamID uint32, errorCode uint32, debugData []byte) []byte {
	var buf bytes.Buffer
	payload := make([]byte, 8+len(debugData))
	binary.BigEndian.PutUint32(payload[0:4], lastStreamID&0x7fffffff)
	binary.BigEndian.PutUint32(payload[4:8], errorCode)
	if len(debugData) > 0 {
		copy(payload[8:], debugData)
	}
	writeFrame(&buf, frameTypeGoAway, 0x00, 0, payload)
	return buf.Bytes()
}
