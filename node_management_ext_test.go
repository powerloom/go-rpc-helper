package rpchelper_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	rpchelper "github.com/powerloom/go-rpc-helper"
	"github.com/stretchr/testify/assert"
)

// ControllableMockServer is a mock server that can be manipulated at runtime.

type ControllableMockServer struct {
	server  *httptest.Server
	isDown  atomic.Bool
	latency time.Duration
}

func NewControllableMockServer() *ControllableMockServer {
	m := &ControllableMockServer{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.isDown.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		time.Sleep(m.latency)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": "0x1"})
	}))
	return m
}

func (m *ControllableMockServer) URL() string {
	return m.server.URL
}

func (m *ControllableMockServer) SetDown(down bool) {
	m.isDown.Store(down)
}

func (m *ControllableMockServer) Close() {
	m.server.Close()
}

func TestRPCSwitchingWithFailingMock(t *testing.T) {
	// Create two controllable mock servers
	primaryNode := NewControllableMockServer()
	defer primaryNode.Close()
	secondaryNode := NewControllableMockServer()
	defer secondaryNode.Close()

	// Configure the RPC helper with the mock servers
	config := rpchelper.NewRPCConfigFromURLs(
		[]string{primaryNode.URL(), secondaryNode.URL()},
		[]string{},
	)
	config.MaxRetries = 1
	config.RetryDelay = 100 * time.Millisecond

	rpc := rpchelper.NewRPCHelper(config)
	ctx := context.Background()
	err := rpc.Initialize(ctx)
	assert.NoError(t, err)
	defer rpc.Close()

	// Initially, both nodes are healthy. The primary node should be used.
	_, err = rpc.BlockNumber(ctx)
	assert.NoError(t, err)

	// Now, take down the primary node
	primaryNode.SetDown(true)

	// The next call should fail over to the secondary node
	_, err = rpc.BlockNumber(ctx)
	assert.NoError(t, err)

	// Bring the primary node back up
	primaryNode.SetDown(false)

	// The next call should use the primary node again
	_, err = rpc.BlockNumber(ctx)
	assert.NoError(t, err)
}

func TestRPCWithRealEndpoints(t *testing.T) {
	rpcURLs := os.Getenv("RPC_URLS_TEST")
	if rpcURLs == "" {
		t.Skip("Skipping test with real RPC endpoints: RPC_URLS_TEST environment variable not set")
	}

	urls := strings.Split(rpcURLs, ",")
	config := rpchelper.NewRPCConfigFromURLs(urls, []string{})
	rpc := rpchelper.NewRPCHelper(config)
	ctx := context.Background()
	err := rpc.Initialize(ctx)
	assert.NoError(t, err)
	defer rpc.Close()

	_, err = rpc.BlockNumber(ctx)
	assert.NoError(t, err)
}
