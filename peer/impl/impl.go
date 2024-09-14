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
	// here you must return a struct that implements the peer.Peer functions.
	// Therefore, you are free to rename and change it as you want.
	return &node{}
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
	// First, unmarshal the message
	var msg types.Message
	// Define a constant for the timeout duration
	const sendTimeout = 5 * time.Second // You can adjust this value as needed

	err := n.conf.MessageRegistry.UnmarshalMessage(pkt.Msg, msg)
	if err != nil {
		return err
	}

	if pkt.Header.Destination == n.conf.Socket.GetAddress() {
		// The packet is for this node
		err = n.conf.MessageRegistry.ProcessPacket(pkt)
		if err != nil {
			return err
		}
	} else {
		// Packet needs to be relayed
		pkt.Header.RelayedBy = n.conf.Socket.GetAddress()
		err := n.conf.Socket.Send(pkt.Header.Destination, pkt, sendTimeout)
		if err != nil {
			return err
		}
	}

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

// Unicast implements peer.Messaging
func (n *node) Unicast(dest string, msg transport.Message) error {
	panic("to be implemented in HW0")
}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	panic("to be implemented in HW0")
}

// GetRoutingTable implements peer.Messaging
func (n *node) GetRoutingTable() peer.RoutingTable {
	panic("to be implemented in HW0")
}

// SetRoutingEntry implements peer.Messaging
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	panic("to be implemented in HW0")
}
