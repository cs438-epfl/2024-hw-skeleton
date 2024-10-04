package disrupted

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/transport"
)

// Test GenericModifier Socket using duplicator - insertion rate 0%, 10%, 60%, 100%
func Test_Disrupted_Duplicator(t *testing.T) {

	// mapping of the drop rate to the number of received packets, with the set seed above. Those values might change if
	// the seed is different.
	receivedPackets := map[float64]int{
		0:   10,
		0.1: 11,
		0.6: 15,
		1:   20,
	}

	for _, insertRate := range []float64{0, 0.1, 0.6, 1.0} {
		net := NewDisrupted(chanFac(), WithDuplicator(insertRate))
		net.SetRandomGenSeed(7165897632553559652)

		sock1, err := net.CreateSocket("127.0.0.1:0")
		require.NoError(t, err)
		sock2, err := net.CreateSocket("127.0.0.1:0")
		require.NoError(t, err)

		// > n1 send 100 packets to n2
		for i := 0; i < 10; i++ {
			pkt := z.GetRandomPkt(t)
			err = sock1.Send(sock2.GetAddress(), pkt, 0)
			require.NoError(t, err)
			_, err = sock2.Recv(time.Second)
			if err != nil {
				log.Info().Msgf("Packet timeout with drop rate %f", insertRate)
			}
			time.Sleep(time.Millisecond)
		}
		// Flush all remaining messages
		for err == nil {
			_, err = sock2.Recv(time.Second)
		}

		require.Equal(t, 10, len(sock1.GetOuts()))
		require.Equal(t, receivedPackets[insertRate], len(sock2.GetIns()))
		require.NoError(t, sock1.Close())
		require.NoError(t, sock2.Close())
	}

}

// Tests the packet Duplicator function (as a PacketModifier function)
func Test_Packet_Duplicator(t *testing.T) {
	testChan := make(chan transport.Packet)
	pkt := z.GetRandomPkt(t)
	pktCopy := pkt.Copy()
	n := NewDisrupted(nil) // Emulate an underlying DisruptedLayer

	go duplicator()(pkt, testChan, n.randGen)
	p := <-testChan

	// pointer comparison, should be false as the address of the duplicated
	//packet should be in a different part of memory
	require.False(t, pktCopy == p)
	require.Equal(t, *pktCopy.Header, *pkt.Header)
	require.Equal(t, *pktCopy.Msg, *pkt.Msg)
	require.Equal(t, *pkt.Header, *p.Header)
	require.Equal(t, *pkt.Msg, *p.Msg)

}

// Tests the Source Spoofer function
func Test_Source_Spoofer(t *testing.T) {
	testChan := make(chan transport.Packet)
	pkt := z.GetRandomPkt(t)
	pktCopy := pkt.Copy()
	n := NewDisrupted(nil) // Emulate an underlying DisruptedLayer

	go sourceSpoofer("testString")(pkt, testChan, n.randGen)
	p := <-testChan
	require.Equal(t, *pktCopy.Header, *pkt.Header)
	require.Equal(t, *pktCopy.Msg, *pkt.Msg)
	require.Equal(t, p.Header.Source, "testString")
	require.Equal(t, p.Header.PacketID, pkt.Header.PacketID)
	require.Equal(t, p.Header.Destination, pkt.Header.Destination)
	require.Equal(t, p.Header.RelayedBy, pkt.Header.RelayedBy)
	require.Equal(t, p.Header.Timestamp, pkt.Header.Timestamp)
	require.Equal(t, p.Msg.Payload, pkt.Msg.Payload)
	require.Equal(t, p.Msg.Type, pkt.Msg.Type)
}

// Tests the PacketID Randomizer function
func Test_PacketID_Randomizer(t *testing.T) {

	testChan := make(chan transport.Packet)
	pkt := z.GetRandomPkt(t)
	pktCopy := pkt.Copy()
	n := NewDisrupted(nil) // Emulate an underlying DisruptedLayer

	go packetIDRandomizer()(pkt, testChan, n.randGen)
	p := <-testChan
	require.Equal(t, *pktCopy.Header, *pkt.Header)
	require.Equal(t, *pktCopy.Msg, *pkt.Msg)
	require.Equal(t, p.Header.Source, pkt.Header.Source)
	require.NotEqual(t, p.Header.PacketID, pkt.Header.PacketID)
	require.Equal(t, p.Header.Destination, pkt.Header.Destination)
	require.Equal(t, p.Header.RelayedBy, pkt.Header.RelayedBy)
	require.Equal(t, p.Header.Timestamp, pkt.Header.Timestamp)
	require.Equal(t, p.Msg.Payload, pkt.Msg.Payload)
	require.Equal(t, p.Msg.Type, pkt.Msg.Type)
}

// Tests the Payload Randomizer function
func Test_Payload_Randomizer(t *testing.T) {

	net := NewDisrupted(nil) // Emulate an underlying DisruptedLayer
	net.SetRandomGenSeed(495957731692)

	testChan := make(chan transport.Packet)
	pkt := z.GetRandomPkt(t)
	pktCopy := pkt.Copy()

	go payloadRandomizer()(pkt, testChan, net.randGen)
	p := <-testChan

	require.Equal(t, *pktCopy.Header, *pkt.Header)
	require.Equal(t, *pktCopy.Msg, *pkt.Msg)
	require.Equal(t, p.Header.Source, pkt.Header.Source)
	require.Equal(t, p.Header.PacketID, pkt.Header.PacketID)
	require.Equal(t, p.Header.Destination, pkt.Header.Destination)
	require.Equal(t, p.Header.RelayedBy, pkt.Header.RelayedBy)
	require.Equal(t, p.Header.Timestamp, pkt.Header.Timestamp)
	require.NotEqual(t, p.Msg.Payload, pkt.Msg.Payload)
	require.Equal(t, p.Msg.Type, pkt.Msg.Type)

	// testing the value of the payload, should be fixed given the seed
	var payload int
	err := json.Unmarshal(p.Msg.Payload, &payload)
	require.NoError(t, err)
	require.Equal(t, payload, 500352282)
}
