package disrupted

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
)

// Tests that a fixed delay is applied when receiving packets
func Test_Delay(t *testing.T) {

	for _, delay := range []time.Duration{1 * time.Millisecond, 100 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second} { // in ms
		net := NewDisrupted(chanFac(), WithFixedDelay(delay))
		net.SetRandomGenSeed(29117685307434355)

		sock1, err := net.CreateSocket("127.0.0.1:0")
		require.NoError(t, err)
		sock2, err := net.CreateSocket("127.0.0.1:0")
		require.NoError(t, err)

		experimentalDelay := time.Duration(0)
		// > n1 send 100 packets to n2
		for i := 0; i < 10; i++ {
			pkt := z.GetRandomPkt(t)
			t0 := time.Now()
			err = sock1.Send(sock2.GetAddress(), pkt, 0)
			require.NoError(t, err)
			_, err := sock2.Recv(0)
			require.NoError(t, err)
			experimentalDelay += time.Since(t0)
		}

		time.Sleep(time.Millisecond)
		require.Equal(t, sock1.GetOuts(), sock2.GetIns())
		require.Equal(t, len(sock1.GetOuts()), 10)

		// In order to test the delays, which heavily depend on network conditions, we chose to fix a bound to evaluate
		// the empirical delay. It should be greater than the theoretical delay.
		experimentalDelay /= 10
		require.Greater(t, experimentalDelay, delay)

		require.NoError(t, sock1.Close())
		require.NoError(t, sock2.Close())
	}

}

// Test that the exponential delay returns the correct value, given a constant seed
func Test_Delay_Functions(t *testing.T) {

	net := NewDisrupted(nil)
	net.SetRandomGenSeed(30510822542570081)

	// FixedDelay
	require.Equal(t, FixedDelay(time.Second)(time.Now(), net.randGen), time.Second)

	// ExponentialDelay
	mean := time.Second * 3 // 3s
	experimentalMean := time.Duration(0)
	for i := 0; i < 1000; i++ {
		experimentalMean += ExponentialDelay(mean)(time.Now(), net.randGen)
	}

	// the Exponential Delay should have a fixed value, since the random generator is seeded
	experimentalMean /= 1000
	require.Equal(t, int64(3112778554), experimentalMean.Nanoseconds())

	// SineDelay
	amp := 5.0
	freq := 10.0
	period := time.Second / 10
	t0 := time.Now()
	require.Equal(t, SineDelay(amp, freq)(t0, net.randGen), SineDelay(amp, freq)(t0.Add(period), net.randGen))
}
