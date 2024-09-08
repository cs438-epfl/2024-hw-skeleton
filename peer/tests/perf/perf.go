//go:build performance
// +build performance

package perf

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/registry/standard"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/perfchannel"
)

var peerFac peer.Factory = impl.NewPeer

var channelFac transport.Factory = perfchannel.NewTransport

var defaultRegistry = standard.NewRegistry()

type allocThresholds struct {
	name         string
	allocs       int64 // AllocsPerOp
	allocedBytes int64 // AllocedBytesPerOp
}

type speedThresholds struct {
	name string
	ns   time.Duration
}

// Report the execution time according to per performance threshold
func assessSpeed(t *testing.T, res testing.BenchmarkResult, thresholds []speedThresholds) {
	// retrieve nanoseconds per op
	ns := res.NsPerOp()
	// report 1 result (sub-test) per performance threshold
	for _, th := range thresholds {
		t.Run(th.name, func(t *testing.T) {
			require.Greater(t, res.T, 0*time.Second, "The benchmark execution failed, its result cannot be used")

			if ns > th.ns.Nanoseconds() {
				t.Errorf("%v > threshold %v", time.Duration(ns)*time.Nanosecond, th.ns)
			} else {
				t.Logf("%v <= threshold %v", time.Duration(ns)*time.Nanosecond, th.ns)
			}
		})
	}
}

// Report the allocation according to per performance threshold
func assessAllocs(t *testing.T, res testing.BenchmarkResult, thresholds []allocThresholds) {
	// retrieve nanoseconds per op
	allocedBytes := res.AllocedBytesPerOp()
	allocs := res.AllocsPerOp()

	// report 1 result (sub-test) per performance threshold
	for _, th := range thresholds {
		t.Run(th.name, func(t *testing.T) {
			require.Greater(t, res.T, 0*time.Second, "The benchmark execution failed, its result cannot be used")

			if allocedBytes > th.allocedBytes {
				t.Errorf("%d allocedBytes > threshold (%d)", allocedBytes, th.allocedBytes)
			} else {
				t.Logf("%d allocedBytes <= threshold (%d)", allocedBytes, th.allocedBytes)
			}
			if allocs > th.allocs {
				t.Errorf("%d allocs > threshold (%d)", allocs, th.allocs)
			} else {
				t.Logf("%d allocs <= threshold (%d)", allocs, th.allocs)
			}
		})
	}
}
