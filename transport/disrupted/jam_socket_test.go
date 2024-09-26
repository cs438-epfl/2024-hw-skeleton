package disrupted

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
)

func Test_Jam(t *testing.T) {

	// Create a jammed network, with 5-packets and infinite timeout jams
	net := NewDisrupted(chanFac(), WithJam(0, 5))
	net.SetRandomGenSeed(29117685307434355)

	sock1, err := net.CreateSocket("127.0.0.1:0")
	require.NoError(t, err)
	sock2, err := net.CreateSocket("127.0.0.1:0")
	require.NoError(t, err)

	// Run twice to check the epoch change works
	for j := 0; j < 2; j++ {
		// Send 4 packets (1 less than jam threshold)
		for i := 0; i < 4; i++ {
			pkt := z.GetRandomPkt(t)
			err = sock1.Send(sock2.GetAddress(), pkt, 0)
			require.NoError(t, err)

			// Try to receive
			_, err := sock2.Recv(500 * time.Millisecond)

			// Expect a timeout error (packets jammed)
			require.Error(t, err)
		}

		// Try to receive
		_, err = sock2.Recv(500 * time.Millisecond)

		// Expect a timeout error (packets jammed)
		require.Error(t, err)

		// Send 1 random packets (reach jam threshold)
		pkt := z.GetRandomPkt(t)
		err = sock1.Send(sock2.GetAddress(), pkt, 0)
		require.NoError(t, err)

		// Receive all un-jammed packets
		for i := 0; i < 5; i++ {
			_, err = sock2.Recv(0)
			require.NoError(t, err)
		}

		// Wait
		time.Sleep(time.Second)

		// Check there are as many sent and received messages (5 then 10)
		require.Equal(t, 5*(j+1), len(sock1.GetOuts()))
		require.Equal(t, 5*(j+1), len(sock2.GetIns()))
	}

	// Close the sockets
	require.NoError(t, sock1.Close())
	require.NoError(t, sock2.Close())
}

func Test_Jam_Timeout(t *testing.T) {

	// Create a jammed network, with 5-packets and 500ms timeout jams
	net := NewDisrupted(chanFac(), WithJam(500*time.Millisecond, 5))
	net.SetRandomGenSeed(29117685307434355)

	sock1, err := net.CreateSocket("127.0.0.1:0")
	require.NoError(t, err)
	sock2, err := net.CreateSocket("127.0.0.1:0")
	require.NoError(t, err)

	// Send 4 packets (1 less than jam threshold)
	for i := 0; i < 4; i++ {
		pkt := z.GetRandomPkt(t)
		err = sock1.Send(sock2.GetAddress(), pkt, 0)
		require.NoError(t, err)

		// Try to receive
		_, err := sock2.Recv(50 * time.Millisecond)
		// Expect a timeout error (packets jammed)
		require.Error(t, err)
	}
	// Wait for jam timeout
	time.Sleep(time.Second)

	// Receive all un-jammed packets due to timeout
	for i := 0; i < 4; i++ {
		_, err = sock2.Recv(0)
		require.NoError(t, err)
	}

	// Check there are as many sent and received messages
	require.Equal(t, 4, len(sock1.GetOuts()))
	require.Equal(t, 4, len(sock2.GetIns()))
}
