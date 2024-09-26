package disrupted

import (
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/channel"
)

var chanFac transport.Factory = channel.NewTransport

// Test Loss Socket - drop rate 0%, 10%, 60%, 100%
func Test_Disrupted_Loss(t *testing.T) {

	// mapping of the drop rate to the number of received packets, with the set seed above. Those values might change if
	// the seed is different.
	receivedPackets := map[float64]int{
		0:   10,
		0.1: 9,
		0.6: 4,
		1:   0,
	}

	for _, dropRate := range []float64{0, 0.1, 0.6, 1.0} {
		net := NewDisrupted(chanFac(), WithLossSocket(dropRate))
		net.SetRandomGenSeed(7165897632553559653)

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
				log.Info().Msgf("Packet timeout with drop rate %f", dropRate)
			}
			time.Sleep(time.Millisecond)
		}

		require.Equal(t, 10, len(sock1.GetOuts()))
		require.Equal(t, receivedPackets[dropRate], len(sock2.GetIns()))
		require.NoError(t, sock1.Close())
		require.NoError(t, sock2.Close())
	}

}
