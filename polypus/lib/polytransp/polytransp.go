package polytransp

// Package polytransp provides a channel-based transport implementation that
// supports Polypus.

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.dedis.ch/cs438/polypus/lib/polylib"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/dela/core"
	"golang.org/x/xerrors"
)

// You can specify a destination file for the log by specifying the LOG_FILE
// environment variable. For example by running:
//	LOG_FILE=logs.json go run .

var counter uint32 // initialized by default to 0

// Polypus APIs will start at this port. There is one for each node.
const basePolyPort = 4000

// we use an artificial delay when sending messages
const artificialDelay = time.Millisecond * 300

// NewTransport returns a channel-based Polypus transport service.
func NewTransport() *Transport {
	var out io.Writer
	var err error

	logFile := os.Getenv("LOG_FILE")

	if logFile != "" {
		out, err = os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic("failed to open file: " + err.Error())
		}
	}

	return &Transport{
		incomings: make(map[string]chan transport.Packet),
		polylog:   out,
	}
}

// Transport is a simple transport implementation using channels
//
// - implements transport.Transport
// - implements transport.ClosableSocket
type Transport struct {
	sync.RWMutex
	incomings map[string]chan transport.Packet
	polyNodes []polyNode
	polylog   io.Writer
}

// CreateSocket implements transport.Transport
func (t *Transport) CreateSocket(address string) (transport.ClosableSocket, error) {
	inWatch := core.NewWatcher()
	outWatch := core.NewWatcher()

	t.Lock()
	port := atomic.AddUint32(&counter, 1)
	if strings.HasSuffix(address, ":0") {
		address = address[:len(address)-2]
		address = fmt.Sprintf("%s:%d", address, port)
	}
	t.incomings[address] = make(chan transport.Packet, 100)
	polyAddr := fmt.Sprintf("localhost:%d", basePolyPort+port)
	polyID := getID(int(port))

	t.Unlock()

	proxy := polylib.Start(polyAddr, inWatch, outWatch, t.polylog)

	t.polyNodes = append(t.polyNodes, polyNode{
		id:    polyID,
		addr:  address,
		proxy: proxy,
	})

	return &Socket{
		Transport: t,
		myAddr:    address,

		ins:  packets{},
		outs: packets{},

		inWatch:  inWatch,
		outWatch: outWatch,
	}, nil
}

// MustCreate returns a socket and panic if something goes wrong. This function
// is mostly usefull in tests.
func (t *Transport) MustCreate(address string) transport.ClosableSocket {
	socket, err := t.CreateSocket(address)
	if err != nil {
		panic("failed to create socket: " + err.Error())
	}

	return socket
}

// GetConfig returns the Polypus configuration
func (t *Transport) GetConfig() string {
	out := new(strings.Builder)
	out.WriteString("{\"nodes\": [\n")

	nodeCfgs := make([]string, len(t.polyNodes))
	for i, n := range t.polyNodes {
		nodeCfgs[i] = "\t" + fmt.Sprintf(`{"id": "%s", "addr": "%s", "proxy": "http://%s"}`, n.id, n.addr, n.proxy)
	}

	out.WriteString(strings.Join(nodeCfgs, ",\n"))

	out.WriteString("\n]}")

	return out.String()
}

type polyNode struct {
	id    string
	addr  string
	proxy string
}

// Socket provide a network layer using channels.
//
// - implements transport.Socket
type Socket struct {
	*Transport
	myAddr string

	ins  packets
	outs packets

	inWatch  *core.Watcher
	outWatch *core.Watcher
}

// Close implements transport.Socket
func (s *Socket) Close() error {
	s.Lock()
	defer s.Unlock()

	delete(s.incomings, s.myAddr)

	return nil
}

func tryMarshal(pkt transport.Packet) string {
	buf, err := pkt.Marshal()
	if err != nil {
		return "ERROR: marshal: " + err.Error()
	}
	return string(buf)
}

// Send implements transport.Socket.
func (s *Socket) Send(dest string, pkt transport.Packet, timeout time.Duration) error {
	s.RLock()
	to, ok := s.incomings[dest]

	if !ok {
		s.RUnlock()
		return xerrors.Errorf("%s is not listening", dest)
	}
	s.RUnlock()

	s.outWatch.Notify(polylib.Event{
		Address: dest,
		ID:      pkt.Header.PacketID,
		Msg:     tryMarshal(pkt),
	})

	if timeout == 0 {
		timeout = math.MaxInt64
	}

	time.Sleep(artificialDelay)

	select {
	case to <- pkt.Copy():
	case <-time.After(timeout):
		return transport.TimeoutError(timeout)
	}

	s.outs.add(pkt)

	return nil
}

// Recv implements transport.Socket.
func (s *Socket) Recv(timeout time.Duration) (transport.Packet, error) {
	s.RLock()
	myChan := s.incomings[s.myAddr]
	s.RUnlock()

	select {
	case <-time.After(timeout):
		return transport.Packet{}, transport.TimeoutError(timeout)
	case pkt := <-myChan:
		s.ins.add(pkt)
		s.inWatch.Notify(polylib.Event{Address: pkt.Header.RelayedBy, ID: pkt.Header.PacketID})
		return pkt, nil
	}
}

// GetAddress implements transport.Socket.
func (s *Socket) GetAddress() string {
	return s.myAddr
}

// GetIns implements transport.Socket
func (s *Socket) GetIns() []transport.Packet {
	return s.ins.getAll()
}

// GetOuts implements transport.Socket
func (s *Socket) GetOuts() []transport.Packet {
	return s.outs.getAll()
}

type packets struct {
	sync.Mutex
	data []transport.Packet
}

func (p *packets) add(pkt transport.Packet) {
	p.Lock()
	defer p.Unlock()

	p.data = append(p.data, pkt.Copy())
}

func (p *packets) getAll() []transport.Packet {
	p.Lock()
	defer p.Unlock()

	res := make([]transport.Packet, len(p.data))

	for i, pkt := range p.data {
		res[i] = pkt.Copy()
	}

	return res
}

// getID return a string of form AA to ZZ
func getID(i int) string {
	if i < 0 || i > 675 {
		return "UNDEFINED"
	}

	firstLetter := byte('A') + byte(i/26)
	secondLetter := byte('A') + byte(i%26)

	return string(firstLetter) + string(secondLetter)
}
