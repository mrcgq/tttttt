package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// FingerprintReport is the JSON output returned to the client.
type FingerprintReport struct {
	JA3Raw         string `json:"ja3_raw"`
	JA3Hash        string `json:"ja3_hash"`
	ALPN           string `json:"alpn"`
	SNI            string `json:"sni"`
	TLSVersion     uint16 `json:"tls_version"`
	CipherSuite    uint16 `json:"cipher_suite"`
	H2Settings     string `json:"h2_settings,omitempty"`
	H2WindowUpdate uint32 `json:"h2_window_update,omitempty"`
	H2PseudoOrder  string `json:"h2_pseudo_order,omitempty"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18443", "listen address")
	certFile := flag.String("cert", "", "TLS cert file (auto-generated if empty)")
	keyFile := flag.String("key", "", "TLS key file (auto-generated if empty)")
	flag.Parse()

	var tlsCert tls.Certificate
	var err error
	if *certFile != "" && *keyFile != "" {
		tlsCert, err = tls.LoadX509KeyPair(*certFile, *keyFile)
	} else {
		log.Println("generating self-signed certificate...")
		tlsCert, err = generateSelfSignedCert()
	}
	if err != nil {
		log.Fatalf("cert error: %v", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("fpserver listening on %s", *addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConnection(conn, tlsCfg)
	}
}

func handleConnection(rawConn net.Conn, tlsCfg *tls.Config) {
	defer rawConn.Close()
	_ = rawConn.SetDeadline(time.Now().Add(10 * time.Second))

	recordHeader := make([]byte, 5)
	if _, err := io.ReadFull(rawConn, recordHeader); err != nil {
		log.Printf("read record header: %v", err)
		return
	}
	recordLen := int(binary.BigEndian.Uint16(recordHeader[3:5]))
	if recordLen > 16384 || recordLen < 4 {
		log.Printf("invalid record length: %d", recordLen)
		return
	}
	recordBody := make([]byte, recordLen)
	if _, err := io.ReadFull(rawConn, recordBody); err != nil {
		log.Printf("read record body: %v", err)
		return
	}
	rawClientHello := append(recordHeader, recordBody...)

	ja3Raw := parseJA3(recordBody)
	ja3Hash := fmt.Sprintf("%x", md5.Sum([]byte(ja3Raw)))

	replay := newReplayConn(rawConn, rawClientHello)
	tlsConn := tls.Server(replay, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("tls handshake: %v", err)
		return
	}
	defer tlsConn.Close()

	state := tlsConn.ConnectionState()
	report := FingerprintReport{
		JA3Raw:      ja3Raw,
		JA3Hash:     ja3Hash,
		ALPN:        state.NegotiatedProtocol,
		TLSVersion:  state.Version,
		CipherSuite: state.CipherSuite,
		SNI:         state.ServerName,
	}

	if state.NegotiatedProtocol == "h2" {
		readH2Fingerprint(tlsConn, &report)
	} else {
		sendH1Response(tlsConn, &report)
		return
	}

	sendH2Response(tlsConn, &report)
}

func parseJA3(handshakeRecord []byte) string {
	if len(handshakeRecord) < 4 || handshakeRecord[0] != 0x01 {
		return ""
	}
	bodyLen := int(handshakeRecord[1])<<16 | int(handshakeRecord[2])<<8 | int(handshakeRecord[3])
	body := handshakeRecord[4:]
	if len(body) < bodyLen {
		body = handshakeRecord[4:]
	}
	if len(body) < 2+32 {
		return ""
	}

	tlsVersion := binary.BigEndian.Uint16(body[0:2])
	pos := 2 + 32

	if pos >= len(body) {
		return ""
	}
	sidLen := int(body[pos])
	pos += 1 + sidLen

	if pos+2 > len(body) {
		return ""
	}
	csLen := int(binary.BigEndian.Uint16(body[pos : pos+2]))
	pos += 2
	var ciphers []string
	for i := 0; i < csLen; i += 2 {
		if pos+2 > len(body) {
			break
		}
		cs := binary.BigEndian.Uint16(body[pos : pos+2])
		pos += 2
		if !isGREASE(cs) {
			ciphers = append(ciphers, strconv.Itoa(int(cs)))
		}
	}

	if pos >= len(body) {
		return ""
	}
	compLen := int(body[pos])
	pos += 1 + compLen

	var extensions []string
	var curves []string
	var pointFormats []string

	if pos+2 <= len(body) {
		extLen := int(binary.BigEndian.Uint16(body[pos : pos+2]))
		pos += 2
		extEnd := pos + extLen
		if extEnd > len(body) {
			extEnd = len(body)
		}

		for pos+4 <= extEnd {
			extType := binary.BigEndian.Uint16(body[pos : pos+2])
			extDataLen := int(binary.BigEndian.Uint16(body[pos+2 : pos+4]))
			pos += 4

			if !isGREASE(extType) {
				extensions = append(extensions, strconv.Itoa(int(extType)))
			}

			extData := body[pos:]
			if extDataLen > len(extData) {
				extDataLen = len(extData)
			}
			extData = extData[:extDataLen]

			switch extType {
			case 0x000a:
				if len(extData) >= 2 {
					listLen := int(binary.BigEndian.Uint16(extData[0:2]))
					for j := 2; j+1 < 2+listLen && j+1 < len(extData); j += 2 {
						g := binary.BigEndian.Uint16(extData[j : j+2])
						if !isGREASE(g) {
							curves = append(curves, strconv.Itoa(int(g)))
						}
					}
				}
			case 0x000b:
				if len(extData) >= 1 {
					fmtLen := int(extData[0])
					for j := 1; j < 1+fmtLen && j < len(extData); j++ {
						pointFormats = append(pointFormats, strconv.Itoa(int(extData[j])))
					}
				}
			}
			pos += extDataLen
		}
	}

	return fmt.Sprintf("%d,%s,%s,%s,%s",
		tlsVersion,
		strings.Join(ciphers, "-"),
		strings.Join(extensions, "-"),
		strings.Join(curves, "-"),
		strings.Join(pointFormats, "-"),
	)
}

func isGREASE(val uint16) bool {
	return (val&0x0f0f) == 0x0a0a && val&0x00ff == val>>8
}

func readH2Fingerprint(conn net.Conn, report *FingerprintReport) {
	magic := make([]byte, 24)
	if _, err := io.ReadFull(conn, magic); err != nil {
		log.Printf("h2: read magic: %v", err)
		return
	}

	framer := http2.NewFramer(conn, conn)
	framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
	framer.MaxHeaderListSize = 262144

	_ = framer.WriteSettings()

	var settingsParts []string
	gotHeaders := false

	for !gotHeaders {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		f, err := framer.ReadFrame()
		if err != nil {
			log.Printf("h2: read frame: %v", err)
			return
		}

		switch fr := f.(type) {
		case *http2.SettingsFrame:
			if !fr.IsAck() {
				_ = fr.ForeachSetting(func(s http2.Setting) error {
					settingsParts = append(settingsParts,
						fmt.Sprintf("%d:%d", s.ID, s.Val))
					return nil
				})
				report.H2Settings = strings.Join(settingsParts, ";")
				_ = framer.WriteSettingsAck()
			}

		case *http2.WindowUpdateFrame:
			if fr.StreamID == 0 {
				report.H2WindowUpdate = fr.Increment
			}

		case *http2.MetaHeadersFrame:
			var pseudos []string
			for _, field := range fr.Fields {
				if strings.HasPrefix(field.Name, ":") {
					pseudos = append(pseudos, field.Name)
				}
			}
			report.H2PseudoOrder = strings.Join(pseudos, ",")
			gotHeaders = true

		case *http2.PingFrame:
			if !fr.IsAck() {
				_ = framer.WritePing(true, fr.Data)
			}
		}
	}
}

func sendH2Response(conn net.Conn, report *FingerprintReport) {
	body, _ := json.MarshalIndent(report, "", "  ")

	framer := http2.NewFramer(conn, conn)
	var hpackBuf bytes.Buffer
	enc := hpack.NewEncoder(&hpackBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/json"})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-length", Value: strconv.Itoa(len(body))})

	_ = framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: hpackBuf.Bytes(),
		EndStream:     false,
		EndHeaders:    true,
	})
	_ = framer.WriteData(1, true, body)
}

func sendH1Response(conn net.Conn, report *FingerprintReport) {
	body, _ := json.MarshalIndent(report, "", "  ")
	br := bufio.NewReader(conn)
	_, _ = br.ReadString('\n')

	w := bufio.NewWriter(conn)
	fmt.Fprintf(w, "HTTP/1.1 200 OK\r\n")
	fmt.Fprintf(w, "Content-Type: application/json\r\n")
	fmt.Fprintf(w, "Content-Length: %d\r\n", len(body))
	fmt.Fprintf(w, "Connection: close\r\n")
	fmt.Fprintf(w, "\r\n")
	_, _ = w.Write(body)
	_ = w.Flush()
}

type replayConn struct {
	net.Conn
	reader io.Reader
}

func newReplayConn(conn net.Conn, initial []byte) *replayConn {
	return &replayConn{
		Conn:   conn,
		reader: io.MultiReader(bytes.NewReader(initial), conn),
	}
}

func (c *replayConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"tls-client-fpserver"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	leaf, _ := x509.ParseCertificate(certDER)

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}
