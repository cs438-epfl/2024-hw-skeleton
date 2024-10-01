package impl

import (
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

// Global logger function
func logger(format string, v ...interface{}) {
	if os.Getenv("GLOG") != "no" {
		log.Printf(format, v...)
	}
}

// NewPeer creates a new peer. You can change the content and location of this
// function but you MUST NOT change its signature and package location.
func NewPeer(conf peer.Configuration) peer.Peer {

	// Create a new node
	// Initialize everything inside the node (including the routing table and its mutex)
	n := &node{
		conf: conf,
		asyncRoutingTable: AsyncRoutingTable{
			routingTable: make(peer.RoutingTable),
			mutex:        sync.RWMutex{},
		},
	}
	// Initialize the routing table with an entry to itself
	n.asyncRoutingTable.routingTable[conf.Socket.GetAddress()] = conf.Socket.GetAddress()

	// Register the callback for ChatMessage
	conf.MessageRegistry.RegisterMessageCallback(types.ChatMessage{}, n.chatMessageCallback)

	return n
}

// Custom Struct for handling both the routing table and its associated mutex
// (less confusing than putting everything inside the node struct)
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
	asyncRoutingTable AsyncRoutingTable
}

// Start implements peer.Service
func (n *node) Start() error {
	n.stopChan = make(chan struct{})
	n.wg.Add(1)

	// Go routine that handles the receiving of packets
	// And sending packets to other node using the aux function processPacket
	go func() {
		defer n.wg.Done()
		for {
			select {
			case <-n.stopChan:
				return
			default:
				//Receive Packet
				pkt, err := n.conf.Socket.Recv(time.Second * 1)
				if errors.Is(err, transport.TimeoutError(0)) {
					continue
				}
				if err != nil {
					// Handle error (log it, for example, might need something else later on)
					logger("Error receiving packet: %v", err)
					continue
				}

				//Aux function to either relay packet or give it to current node if dest
				err = n.processPacket(pkt)

				if err != nil {
					// Log the error
					logger("Error receiving packet: %v", err)
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
	logger("Node stopped")

	return nil
}

// processPacket is an aux function that processes a packet received by the node
func (n *node) processPacket(pkt transport.Packet) error {
	// Check if the packet is for this node
	if pkt.Header.Destination == n.conf.Socket.GetAddress() {
		// The packet is for this node
		return n.conf.MessageRegistry.ProcessPacket(pkt)
	}

	//Else Packet needs to be relayed
	n.asyncRoutingTable.mutex.RLock()
	nextHop, known := n.asyncRoutingTable.routingTable[pkt.Header.Destination]
	n.asyncRoutingTable.mutex.RUnlock()

	if !known {
		return errors.New("unknown destination for relay")
	}

	//Create new Header, maybe try to modify the pointer directly ?
	newHeader := transport.NewHeader(
		pkt.Header.Source,
		n.conf.Socket.GetAddress(), //modify relayedBy field to the current node
		pkt.Header.Destination,
	)
	//Create new packet with modified Header
	pkt = transport.Packet{
		Header: &newHeader,
		Msg:    pkt.Msg,
	}
	return n.conf.Socket.Send(nextHop, pkt, time.Second*5)

}

func (n *node) Unicast(dest string, msg transport.Message) error {
	//Read routing table from struct, with R/W protection
	n.asyncRoutingTable.mutex.RLock()
	nextHop, known := n.asyncRoutingTable.routingTable[dest]
	n.asyncRoutingTable.mutex.RUnlock()

	if !known {
		return errors.New("unknown destination")
	}
	//Make header, relay same as source for unicast
	header := transport.NewHeader(
		n.conf.Socket.GetAddress(),
		n.conf.Socket.GetAddress(), // same as source?
		dest,
	)

	//Create Packet
	pkt := transport.Packet{
		Header: &header,
		Msg:    &msg,
	}

	const sendTimeout = 5 * time.Second // timeout, maybe add as parameter?

	// Check if the destination is the next hop
	if nextHop == dest {
		return n.conf.Socket.Send(dest, pkt, sendTimeout)
	}

	//Else Relay through the next hop
	return n.conf.Socket.Send(nextHop, pkt, sendTimeout)

}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	n.asyncRoutingTable.mutex.Lock()
	defer n.asyncRoutingTable.mutex.Unlock()

	//Add all peers from Routing Table
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

	//Copy routing table while R/W locked
	routingTableCopy := make(peer.RoutingTable)
	for k, v := range n.asyncRoutingTable.routingTable {
		routingTableCopy[k] = v
	}
	return routingTableCopy
}

// SetRoutingEntry implements peer.M
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	n.asyncRoutingTable.mutex.Lock()
	defer n.asyncRoutingTable.mutex.Unlock()

	//Sets ROuting table Entries with relayAddr Param, R/W locked
	if relayAddr == "" {
		delete(n.asyncRoutingTable.routingTable, origin)
	} else {
		n.asyncRoutingTable.routingTable[origin] = relayAddr
	}
}

// chatMessageCallback handles incoming ChatMessage
func (n *node) chatMessageCallback(msg types.Message, pkt transport.Packet) error {
	//First variable is chat msg, second is boolean
	chatMsg, ok := msg.(*types.ChatMessage)
	if !ok {
		return errors.New("invalid message type")
	}
	logger("Received chat message: %s", chatMsg.Message)
	return nil
}
