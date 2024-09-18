package impl

import (
	"errors"
	"log"
	"sync"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

// NewPeer creates a new peer. You can change the content and location of this
// function but you MUST NOT change its signature and package location.
func NewPeer(conf peer.Configuration) peer.Peer {
	n := &node{
		conf:         conf,
		routingTable: make(peer.RoutingTable),
	}
	// Initialize the routing table with an entry to itself
	n.routingTable[conf.Socket.GetAddress()] = conf.Socket.GetAddress()

	// Register the callback for ChatMessage
	conf.MessageRegistry.RegisterMessageCallback(types.ChatMessage{}, n.chatMessageCallback)

	return n
}

// node implements a peer to build a Peerster system
//
// - implements peer.Peer
type node struct {
	peer.Peer
	// You probably want to keep the peer.Configuration on this struct:
	conf     peer.Configuration
	socket   transport.ClosableSocket
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Add routing table and mutex for part 2
	routingTable peer.RoutingTable
	routingMutex sync.RWMutex
}

// Start implements peer.Service
func (n *node) Start() error {
	n.stopChan = make(chan struct{})
	n.wg.Add(1)

	go func() {
		defer n.wg.Done()
		for {
			select {
			case <-n.stopChan:
				return
			default:
				pkt, err := n.conf.Socket.Recv(time.Second * 1)
				if errors.Is(err, transport.TimeoutError(0)) {
					continue
				}
				if err != nil {
					// Handle error (log it, for example)
					log.Printf("Error receiving packet: %v", err)
					continue
				}

				// Process the packet
				err = n.processPacket(pkt)
				if err != nil {
					// Log the error
					log.Printf("Error processing packet: %v", err)
				}
			}

		}
	}()

	return nil
}

func (n *node) processPacket(pkt transport.Packet) error {
	if pkt.Header.Destination == n.conf.Socket.GetAddress() {
		// The packet is for this node
		return n.conf.MessageRegistry.ProcessPacket(pkt)
	} else {
		// Packet needs to be relayed
		pkt.Header.RelayedBy = n.conf.Socket.GetAddress()
		return n.conf.Socket.Send(pkt.Header.Destination, pkt, time.Second*5)
	}
}

// Stop implements peer.Service
func (n *node) Stop() error {
	// Signal the main loop to stop
	close(n.stopChan)

	// Wait for the goroutine to finish
	n.wg.Wait()

	// We can't close the socket directly, so we'll just log that we're stopping
	log.Println("Node stopped")

	return nil
}

// Unicast implements peer.Messaging
func (n *node) Unicast(dest string, msg transport.Message) error {
	header := transport.NewHeader(
		n.conf.Socket.GetAddress(),
		n.conf.Socket.GetAddress(), //same as source ?
		dest,
	)

	pkt := transport.Packet{
		Header: &header,
		Msg:    &msg,
	}

	const sendTimeout = 5 * time.Second //timeout, maybe add as parameter?

	return n.conf.Socket.Send(dest, pkt, sendTimeout)
}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	n.routingMutex.Lock()
	defer n.routingMutex.Unlock()

	for _, a := range addr {
		if a != n.conf.Socket.GetAddress() {
			n.routingTable[a] = a
		}
	}
}

// GetRoutingTable implements peer.Messaging
func (n *node) GetRoutingTable() peer.RoutingTable {
	n.routingMutex.RLock()
	defer n.routingMutex.RUnlock()

	copy := make(peer.RoutingTable)
	for k, v := range n.routingTable {
		copy[k] = v
	}
	return copy
}

// SetRoutingEntry implements peer.Messaging
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	n.routingMutex.Lock()
	defer n.routingMutex.Unlock()

	if relayAddr == "" {
		delete(n.routingTable, origin)
	} else {
		n.routingTable[origin] = relayAddr
	}
}

// chatMessageCallback handles incoming ChatMessage
func (n *node) chatMessageCallback(msg types.Message, pkt transport.Packet) error {
	chatMsg, ok := msg.(*types.ChatMessage)
	if !ok {
		return errors.New("invalid message type")
	}

	log.Printf("Received chat message: %s", chatMsg.String())
	log.Printf("From: %s", pkt.Header.Source)
	return nil
}
