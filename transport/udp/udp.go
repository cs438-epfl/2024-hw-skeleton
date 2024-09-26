package udp

import (
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"go.dedis.ch/cs438/transport"
)

// It is advised to define a constant (max) size all relevant byte buffers, e.g:
const bufSize = 65000

// NewUDP returns a new udp transport implementation.
func NewUDP() transport.Transport {
	return &UDP{}
}

// UDP implements a transport layer using UDP
//
// - transport.Transport
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

	// Return new Socket instance
	return &Socket{
		conn:    conn,
		address: n.address,
		ins:     make([]transport.Packet, 0),
		outs:    make([]transport.Packet, 0),
		mu:      &sync.Mutex{},
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
	mu      *sync.Mutex
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

	// Set the write
	if timeout > 0 {
		timeouErr := s.conn.SetWriteDeadline(time.Now().Add(timeout))
		if timeouErr != nil {
			log.Printf("Error setting write deadline: %v", timeouErr)
			return transport.TimeoutError(timeout)
		}
		//Unlimited Timeout
	} else {
		timeouErr := s.conn.SetWriteDeadline(time.Time{})
		if timeouErr != nil {
			log.Printf("Error setting write deadline: %v", timeouErr)
			return transport.TimeoutError(timeout)
		}
	}

	// Send the packet
	_, err = s.conn.WriteToUDP(data, udpAddr)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return transport.TimeoutError(timeout)
		}
		return err
	}

	// Store the sent packet in outs
	s.mu.Lock()
	s.outs = append(s.outs, pkt)
	s.mu.Unlock()

	return nil
}

// Recv implements transport.Socket. It blocks until a packet is received, or
// the timeout is reached. In the case the timeout is reached, return a
// TimeoutErr.
func (s *Socket) Recv(timeout time.Duration) (transport.Packet, error) {
	buf := make([]byte, bufSize)

	// Set the read deadline
	if timeout > 0 {
		timeoutErr := s.conn.SetReadDeadline(time.Now().Add(timeout))
		if timeoutErr != nil {
			log.Printf("Error setting write deadline: %v", timeoutErr)
			return transport.Packet{}, timeoutErr
		}
		//Unlimited Timeout
	} else {
		timeoutErr := s.conn.SetReadDeadline(time.Time{})
		if timeoutErr != nil {
			log.Printf("Error setting write deadline: %v", timeoutErr)
			return transport.Packet{}, timeoutErr
		}
	}

	// Receive the packet
	n, _, err := s.conn.ReadFromUDP(buf)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Printf("Recv timeout: %v", timeout)
			return transport.Packet{}, transport.TimeoutError(timeout)
		}
		log.Printf("Recv error: %v", err)
		return transport.Packet{}, err
	}

	// Unmarshal the packet
	var pkt transport.Packet

	err = pkt.Unmarshal(buf[:n])
	if err != nil {
		log.Printf("Unmarshal error: %v", err)
		return transport.Packet{}, err
	}

	// Store the received packet in ins
	s.mu.Lock()
	s.ins = append(s.ins, pkt)
	s.mu.Unlock()

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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ins
}

// GetOuts implements transport.Socket
func (s *Socket) GetOuts() []transport.Packet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outs
}
