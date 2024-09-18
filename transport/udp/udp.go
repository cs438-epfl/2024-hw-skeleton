package udp

import (
	"net"
	"sync"
	"time"

	"go.dedis.ch/cs438/transport"
)

// It is advised to define a constant (max) size for all relevant byte buffers, e.g:
const bufSize = 65000

// NewUDP returns a new udp transport implementation.
func NewUDP() transport.Transport {
	return &UDP{}
}

// UDP implements a transport layer using UDP
//
// - implements transport.Transport
type UDP struct {
	address string
	conn    *net.UDPConn
}

// CreateSocket implements transport.Transport
func (n *UDP) CreateSocket(address string) (transport.ClosableSocket, error) {
	// Resolve the UDP address
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	// Create the UDP connection
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	// Set the UDP struct fields
	n.address = conn.LocalAddr().String()
	n.conn = conn

	// Return the new Socket instance
	return &Socket{
		conn:    conn,
		address: n.address,
		ins:     make([]transport.Packet, 0),
		outs:    make([]transport.Packet, 0),
	}, nil
}

// Socket implements a network socket using UDP.
//
// - implements transport.Socket
// - implements transport.ClosableSocket
type Socket struct {
	conn    *net.UDPConn
	address string
	ins     []transport.Packet
	outs    []transport.Packet

	mu *sync.Mutex //added mutex to protect ins and outs

}

// Close implements transport.Socket. It returns an error if already closed.
func (s *Socket) Close() error {
	return s.conn.Close()
}

// Send implements transport.Socket
func (s *Socket) Send(dest string, pkt transport.Packet, timeout time.Duration) error {
	// Resolve the destination address
	udpAddr, err := net.ResolveUDPAddr("udp", dest)
	if err != nil {
		return err
	}

	// Marshal the packet to bytes
	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	// Set the write deadline
	if timeout > 0 {
		s.conn.SetWriteDeadline(time.Now().Add(timeout))
	} else {
		s.conn.SetWriteDeadline(time.Time{})
	}

	// Send the packet
	_, err = s.conn.WriteToUDP(data, udpAddr)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return transport.TimeoutError(timeout)
		}
		return err
	}

	// Store the sent packet
	s.outs = append(s.outs, pkt)

	return nil
}

// Recv implements transport.Socket. It blocks until a packet is received, or
// the timeout is reached. In the case the timeout is reached, return a
// TimeoutErr.
func (s *Socket) Recv(timeout time.Duration) (transport.Packet, error) {
	buf := make([]byte, bufSize)

	// Set the read deadline
	if timeout > 0 {
		s.conn.SetReadDeadline(time.Now().Add(timeout))
	} else {
		s.conn.SetDeadline(time.Time{})
	}

	// Receive the packet
	n, _, err := s.conn.ReadFromUDP(buf)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return transport.Packet{}, transport.TimeoutError(timeout)
		}
		return transport.Packet{}, err
	}

	// Unmarshal the packet
	var pkt transport.Packet
	err = pkt.Unmarshal(buf[:n])
	if err != nil {
		return transport.Packet{}, err
	}

	// Store the received packet
	//s.mu.Lock()
	s.ins = append(s.ins, pkt)
	//s.mu.Unlock()

	return pkt, nil
}

// GetAddress implements transport.Socket. It returns the address assigned. Can
// be useful in the case one provided a :0 address, which makes the system use a
// random free port.
func (s *Socket) GetAddress() string {
	return s.address
}

// GetIns implements transport.Socket
func (s *Socket) GetIns() []transport.Packet {
	return s.ins
}

// GetOuts implements transport.Socket
func (s *Socket) GetOuts() []transport.Packet {
	return s.outs
}
