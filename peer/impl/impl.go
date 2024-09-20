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

	// Initialize the async routing table
	asyncRoutingTable := AsyncRoutingTable{
		routingTable: make(peer.RoutingTable),
	}

	log.Printf("Creating new peer with address: %s", conf.Socket.GetAddress())

	n := &node{
		conf:              conf,
		asyncRoutingTable: asyncRoutingTable,
	}
	// Initialize the routing table with an entry to itself
	n.asyncRoutingTable.routingTable[conf.Socket.GetAddress()] = conf.Socket.GetAddress()

	// Register the callback for ChatMessage
	conf.MessageRegistry.RegisterMessageCallback(types.ChatMessage{}, n.chatMessageCallback)

	return n
}

type AsyncRoutingTable struct {
	routingTable peer.RoutingTable
	mutex        sync.RWMutex
}

// node implements a peer to build a Peerster system
//
// - implements peer.Peer
type node struct {
	peer.Peer
	// You probably want to keep the peer.Configuration on this struct:
	conf     peer.Configuration
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Add routing table and mutex for part 2
	routingTable      peer.RoutingTable
	routingMutex      sync.RWMutex
	asyncRoutingTable AsyncRoutingTable
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

func (n *node) processPacket(pkt transport.Packet) error {
	log.Printf("Processing packet relayed by: %+v from %s", pkt.Header.RelayedBy, n.conf.Socket.GetAddress())
	if pkt.Header.Destination == n.conf.Socket.GetAddress() {
		// The packet is for this node
		//pkt.Header.RelayedBy = n.conf.Socket.GetAddress()
		log.Printf("Packet is for this node: %s", n.conf.Socket.GetAddress())
		return n.conf.MessageRegistry.ProcessPacket(pkt)
	} else {
		// Packet needs to be relayed
		n.asyncRoutingTable.mutex.RLock()
		nextHop, known := n.asyncRoutingTable.routingTable[pkt.Header.Destination]
		n.asyncRoutingTable.mutex.RUnlock()

		if !known {
			log.Printf("Unknown destination for relay: %s", pkt.Header.Destination)
			return errors.New("unknown destination for relay")
		}

		log.Printf("Relaying packet to next hop: %s", nextHop)
		pkt.Header.RelayedBy = n.conf.Socket.GetAddress()
		return n.conf.Socket.Send(nextHop, pkt, time.Second*5)
	}
}

func (n *node) Unicast(dest string, msg transport.Message) error {
	log.Printf("Unicasting message to: %s", dest)
	n.asyncRoutingTable.mutex.RLock()
	nextHop, known := n.asyncRoutingTable.routingTable[dest]
	n.asyncRoutingTable.mutex.RUnlock()

	if !known {
		log.Printf("Unknown destination: %s", dest)
		return errors.New("unknown destination")
	}

	log.Printf("Routing table entry for %s: %s", dest, nextHop)

	header := transport.NewHeader(
		n.conf.Socket.GetAddress(),
		n.conf.Socket.GetAddress(), // same as source?
		dest,
	)

	pkt := transport.Packet{
		Header: &header,
		Msg:    &msg,
	}

	const sendTimeout = 5 * time.Second // timeout, maybe add as parameter?

	log.Printf("Sending packet: %+v to next hop: %s", pkt, nextHop)
	if nextHop == dest {
		// Directly send to the destination
		//pkt.Header.RelayedBy = n.conf.Socket.GetAddress() // Set RelayedBy to current node's address

		return n.conf.Socket.Send(dest, pkt, sendTimeout)
	} else {
		// Relay through the next hop
		pkt.Header.RelayedBy = n.conf.Socket.GetAddress() // Set RelayedBy to current node's address
		return n.conf.Socket.Send(nextHop, pkt, sendTimeout)
	}
}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	n.asyncRoutingTable.mutex.Lock()
	defer n.asyncRoutingTable.mutex.Unlock()

	for _, a := range addr {
		if a != n.conf.Socket.GetAddress() {
			n.asyncRoutingTable.routingTable[a] = a
		}
	}
}

// GetRoutingTable implements peer.Messaging
func (n *node) GetRoutingTable() peer.RoutingTable {
	n.asyncRoutingTable.mutex.RLock()
	defer n.asyncRoutingTable.mutex.RUnlock()

	copy := make(peer.RoutingTable)
	for k, v := range n.asyncRoutingTable.routingTable {
		copy[k] = v
	}
	return copy
}

// SetRoutingEntry implements peer.M
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	n.asyncRoutingTable.mutex.Lock()
	defer n.asyncRoutingTable.mutex.Unlock()

	if relayAddr == "" {
		log.Printf("Deleting routing entry for origin: %s", origin)
		delete(n.asyncRoutingTable.routingTable, origin)
	} else {
		log.Printf("Setting routing entry: origin=%s, relayAddr=%s", origin, relayAddr)
		n.asyncRoutingTable.routingTable[origin] = relayAddr
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
