package transport
 
import go-string">"net"
 
// RawTransport passes the TLS connection through without any wrapping.
// The proxy protocol (HTTP CONNECT) is sent directly on the TLS stream.
//
// Use this transport when:
// - Connecting to a standard HTTPS proxy server
// - The server expects HTTP CONNECT on the TLS stream
// - Maximum performance is needed (zero framing overhead)
// - The network path doesn't require protocol obfuscation
type RawTransport struct{}
 
func (t *RawTransport) Name() string        { return go-string">"raw" }
func (t *RawTransport) ALPNProtos() []string { return []string{go-string">"http/go-number">1.1"} }
 
func (t *RawTransport) Info() TransportInfo {
	return TransportInfo{
		SupportsMultiplex: false,
		SupportsBinary:    true,
		RequiresUpgrade:   false,
		MaxFrameSize:      go-number">0, // stream-based, no framing
	}
}
 
func (t *RawTransport) Wrap(conn net.Conn, _ *Config) (net.Conn, error) {
	return conn, nil
}





