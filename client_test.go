package rpchelper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRPCServer represents a mock RPC server for testing
type MockRPCServer struct {
	server     *httptest.Server
	mu         sync.RWMutex
	responses  map[string]interface{}
	callCount  int
	shouldFail bool
	failCount  int
	delay      time.Duration
}

// NewMockRPCServer creates a new mock RPC server
func NewMockRPCServer() *MockRPCServer {
	mock := &MockRPCServer{
		responses: make(map[string]interface{}),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handler))
	mock.setupDefaultResponses()
	return mock
}

// URL returns the server URL
func (m *MockRPCServer) URL() string {
	return m.server.URL
}

// Close closes the server
func (m *MockRPCServer) Close() {
	m.server.Close()
}

// SetShouldFail sets whether the server should fail requests
func (m *MockRPCServer) SetShouldFail(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
	if !fail {
		m.failCount = 0
	}
}

// SetFailCount sets how many requests should fail before succeeding
func (m *MockRPCServer) SetFailCount(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failCount = count
	m.shouldFail = count > 0
}

// SetDelay sets a delay for all responses
func (m *MockRPCServer) SetDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
}

// GetCallCount returns the number of calls made to the server
func (m *MockRPCServer) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

// SetResponse sets a custom response for a method
func (m *MockRPCServer) SetResponse(method string, response interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[method] = response
}

// handler handles HTTP requests
func (m *MockRPCServer) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.callCount++
	shouldFail := m.shouldFail
	delay := m.delay
	if m.failCount > 0 {
		m.failCount--
		if m.failCount == 0 {
			m.shouldFail = false
		}
	}
	m.mu.Unlock()

	// Apply delay if set
	if delay > 0 {
		time.Sleep(delay)
	}

	if shouldFail {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Check if it's a batch request (starts with '[')
	if len(body) > 0 && body[0] == '[' {
		// Handle batch request
		var batchReq []struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
			ID     interface{}   `json:"id"`
		}

		if err := json.Unmarshal(body, &batchReq); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		var batchResp []map[string]interface{}
		for _, req := range batchReq {
			m.mu.RLock()
			response, exists := m.responses[req.Method]
			m.mu.RUnlock()

			if !exists {
				response = "0x1" // Default response
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"result":  response,
				"id":      req.ID,
			}
			batchResp = append(batchResp, resp)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(batchResp)
	} else {
		// Handle single request
		var req struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
			ID     interface{}   `json:"id"`
		}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		m.mu.RLock()
		response, exists := m.responses[req.Method]
		m.mu.RUnlock()

		if !exists {
			response = "0x1" // Default response
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"result":  response,
			"id":      req.ID,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// setupDefaultResponses sets up default responses for common RPC methods
func (m *MockRPCServer) setupDefaultResponses() {
	m.responses["eth_blockNumber"] = "0x1234567"
	m.responses["eth_getBlockByNumber"] = map[string]interface{}{
		"number":           "0x1234567",
		"hash":             "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"parentHash":       "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"timestamp":        "0x12345678",
		"gasLimit":         "0x1c9c380",
		"gasUsed":          "0x0",
		"miner":            "0x1234567890abcdef1234567890abcdef12345678",
		"difficulty":       "0x1",
		"totalDifficulty":  "0x1",
		"size":             "0x220",
		"transactions":     []interface{}{},
		"uncles":           []interface{}{},
		"transactionsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		"stateRoot":        "0x6350d0454245fb410fc0fb93f6648c5b9047a6081441e36f0ff3ab259c9a47f0",
		"receiptsRoot":     "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		"sha3Uncles":       "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		"logsBloom":        "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		"extraData":        "0x",
		"nonce":            "0x0000000000000000",
		"mixHash":          "0x0000000000000000000000000000000000000000000000000000000000000000",
	}
	m.responses["eth_getTransactionByHash"] = map[string]interface{}{
		"hash":             "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"nonce":            "0x1",
		"blockHash":        "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"blockNumber":      "0x1234567",
		"transactionIndex": "0x0",
		"from":             "0x1234567890abcdef1234567890abcdef12345678",
		"to":               "0x1234567890abcdef1234567890abcdef12345678",
		"value":            "0x1",
		"gas":              "0x5208",
		"gasPrice":         "0x1",
		"input":            "0x",
		"r":                "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"s":                "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"v":                "0x1c",
		"type":             "0x0",
	}
	m.responses["eth_getTransactionReceipt"] = map[string]interface{}{
		"transactionHash":   "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"transactionIndex":  "0x0",
		"blockHash":         "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"blockNumber":       "0x1234567",
		"from":              "0x1234567890abcdef1234567890abcdef12345678",
		"to":                "0x1234567890abcdef1234567890abcdef12345678",
		"cumulativeGasUsed": "0x5208",
		"gasUsed":           "0x5208",
		"contractAddress":   nil,
		"logs":              []interface{}{},
		"status":            "0x1",
		"logsBloom":         "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		"type":              "0x0",
	}
	m.responses["eth_getLogs"] = []interface{}{}
	m.responses["eth_call"] = "0x"
	m.responses["eth_estimateGas"] = "0x5208"
	m.responses["eth_gasPrice"] = "0x1"
	m.responses["eth_maxPriorityFeePerGas"] = "0x1"
	m.responses["eth_getCode"] = "0x"
	m.responses["eth_getTransactionCount"] = "0x1"
	m.responses["eth_sendRawTransaction"] = "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
}

// TestRPCHelperInitialization tests the initialization of RPCHelper
func TestRPCHelperInitialization(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		server := NewMockRPCServer()
		defer server.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server.URL()},
			},
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			MaxRetryDelay:  5 * time.Second,
			RequestTimeout: 10 * time.Second,
		}

		helper := NewRPCHelper(config)
		require.NotNil(t, helper)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)

		assert.True(t, helper.initialized)
		assert.Len(t, helper.nodes, 1)
		assert.True(t, helper.nodes[0].IsHealthy)

		helper.Close()
	})

	t.Run("initialization with archive nodes", func(t *testing.T) {
		server1 := NewMockRPCServer()
		server2 := NewMockRPCServer()
		defer server1.Close()
		defer server2.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server1.URL()},
			},
			ArchiveNodes: []NodeConfig{
				{URL: server2.URL()},
			},
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			MaxRetryDelay:  5 * time.Second,
			RequestTimeout: 10 * time.Second,
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)

		assert.Len(t, helper.nodes, 1)
		assert.Len(t, helper.archiveNodes, 1)

		helper.Close()
	})

	t.Run("initialization with failed nodes", func(t *testing.T) {
		server1 := NewMockRPCServer()
		server2 := NewMockRPCServer()
		defer server1.Close()
		defer server2.Close()

		// Make server2 fail
		server2.SetShouldFail(true)

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server1.URL()},
				{URL: server2.URL()},
			},
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			MaxRetryDelay:  5 * time.Second,
			RequestTimeout: 10 * time.Second,
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)

		// Should have only one healthy node
		assert.Len(t, helper.nodes, 1)
		assert.Equal(t, server1.URL(), helper.nodes[0].URL)

		helper.Close()
	})

	t.Run("initialization with no nodes", func(t *testing.T) {
		config := &RPCConfig{
			Nodes:          []NodeConfig{},
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			MaxRetryDelay:  5 * time.Second,
			RequestTimeout: 10 * time.Second,
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no nodes configured")
	})

	t.Run("initialization with all nodes failing", func(t *testing.T) {
		server := NewMockRPCServer()
		defer server.Close()
		server.SetShouldFail(true)

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server.URL()},
			},
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			MaxRetryDelay:  5 * time.Second,
			RequestTimeout: 10 * time.Second,
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no RPC nodes available")
	})
}

// TestRPCHelperBasicFunctionality tests basic RPC operations
func TestRPCHelperBasicFunctionality(t *testing.T) {
	server := NewMockRPCServer()
	defer server.Close()

	config := &RPCConfig{
		Nodes: []NodeConfig{
			{URL: server.URL()},
		},
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		MaxRetryDelay:  5 * time.Second,
		RequestTimeout: 10 * time.Second,
	}

	helper := NewRPCHelper(config)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := helper.Initialize(ctx)
	require.NoError(t, err)
	defer helper.Close()

	t.Run("BlockNumber", func(t *testing.T) {
		blockNumber, err := helper.BlockNumber(ctx)
		require.NoError(t, err)
		assert.Greater(t, blockNumber, uint64(0))
	})

	t.Run("BlockByNumber", func(t *testing.T) {
		blockNumber := big.NewInt(1)
		block, err := helper.BlockByNumber(ctx, blockNumber)
		require.NoError(t, err)
		assert.NotNil(t, block)
		assert.NotNil(t, block.Number())
	})

	t.Run("HeaderByNumber", func(t *testing.T) {
		blockNumber := big.NewInt(1)
		header, err := helper.HeaderByNumber(ctx, blockNumber)
		require.NoError(t, err)
		assert.NotNil(t, header)
		assert.NotNil(t, header.Number)
	})

	t.Run("TransactionByHash", func(t *testing.T) {
		hash := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
		tx, isPending, err := helper.TransactionByHash(ctx, hash)
		require.NoError(t, err)
		assert.NotNil(t, tx)
		assert.False(t, isPending)
	})

	t.Run("TransactionReceipt", func(t *testing.T) {
		hash := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
		receipt, err := helper.TransactionReceipt(ctx, hash)
		require.NoError(t, err)
		assert.NotNil(t, receipt)
		assert.Equal(t, hash, receipt.TxHash)
	})

	t.Run("FilterLogs", func(t *testing.T) {
		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(1),
			ToBlock:   big.NewInt(2),
		}
		logs, err := helper.FilterLogs(ctx, query)
		require.NoError(t, err)
		assert.NotNil(t, logs)
	})

	t.Run("CallContract", func(t *testing.T) {
		msg := ethereum.CallMsg{
			To:   &common.Address{},
			Data: []byte{},
		}
		result, err := helper.CallContract(ctx, msg, big.NewInt(1))
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("CallContractArchive", func(t *testing.T) {
		// Add archive node
		archiveServer := NewMockRPCServer()
		defer archiveServer.Close()

		helper.config.ArchiveNodes = []NodeConfig{{URL: archiveServer.URL()}}
		archiveNode, err := helper.createNode(ctx, archiveServer.URL())
		require.NoError(t, err)
		helper.archiveNodes = append(helper.archiveNodes, archiveNode)

		msg := ethereum.CallMsg{
			To:   &common.Address{},
			Data: []byte{},
		}
		result, err := helper.CallContractArchive(ctx, msg, big.NewInt(1))
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("JSONRPCCall", func(t *testing.T) {
		result, err := helper.JSONRPCCall(ctx, "eth_blockNumber")
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("BatchJSONRPCCall", func(t *testing.T) {
		// Test with empty batch - should not panic
		var requests []rpc.BatchElem
		err := helper.BatchJSONRPCCall(ctx, requests)
		// This might error due to empty batch, but shouldn't panic
		_ = err
	})
}

// TestRPCHelperErrorHandling tests error handling scenarios
func TestRPCHelperErrorHandling(t *testing.T) {
	t.Run("operation with failed server", func(t *testing.T) {
		server := NewMockRPCServer()
		defer server.Close()
		server.SetShouldFail(true)

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server.URL()},
			},
			MaxRetries:     2,
			RetryDelay:     50 * time.Millisecond,
			MaxRetryDelay:  1 * time.Second,
			RequestTimeout: 5 * time.Second,
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Initialize with working server
		server.SetShouldFail(false)
		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Now make server fail
		server.SetShouldFail(true)

		_, err = helper.BlockNumber(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "all available nodes failed")
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := NewMockRPCServer()
		defer server.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server.URL()},
			},
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			MaxRetryDelay:  5 * time.Second,
			RequestTimeout: 10 * time.Second,
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Create a context that will be cancelled immediately
		cancelCtx, cancelFunc := context.WithCancel(context.Background())
		cancelFunc()

		_, err = helper.BlockNumber(cancelCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})

	t.Run("timeout handling", func(t *testing.T) {
		server := NewMockRPCServer()
		defer server.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server.URL()},
			},
			MaxRetries:     1,
			RetryDelay:     10 * time.Millisecond,
			MaxRetryDelay:  100 * time.Millisecond,
			RequestTimeout: 50 * time.Millisecond, // Will be shorter than server delay
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Initialize first (without delay)
		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Set a delay longer than the timeout after initialization
		server.SetDelay(100 * time.Millisecond)

		_, err = helper.BlockNumber(ctx)
		require.Error(t, err)
		// Should timeout because server delay (100ms) > request timeout (50ms)
	})
}

// TestRPCHelperHealthCheck tests health checking functionality
func TestRPCHelperHealthCheck(t *testing.T) {
	server1 := NewMockRPCServer()
	server2 := NewMockRPCServer()
	defer server1.Close()
	defer server2.Close()

	config := &RPCConfig{
		Nodes: []NodeConfig{
			{URL: server1.URL()},
			{URL: server2.URL()},
		},
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		MaxRetryDelay:  5 * time.Second,
		RequestTimeout: 10 * time.Second,
	}

	helper := NewRPCHelper(config)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := helper.Initialize(ctx)
	require.NoError(t, err)
	defer helper.Close()

	t.Run("get healthy node count", func(t *testing.T) {
		healthy, archiveHealthy := helper.GetHealthyNodeCount()
		assert.Equal(t, 2, healthy)
		assert.Equal(t, 0, archiveHealthy)
	})

	t.Run("health checker", func(t *testing.T) {
		checker := NewHealthChecker(helper)
		regularErrors, archiveErrors := checker.CheckAllNodes(ctx)

		assert.Len(t, regularErrors, 0) // No errors expected
		assert.Len(t, archiveErrors, 0) // No archive nodes
	})

	t.Run("mark node unhealthy", func(t *testing.T) {
		// Mark first node as unhealthy
		helper.markNodeUnhealthy(server1.URL(), fmt.Errorf("test error"))

		healthy, _ := helper.GetHealthyNodeCount()
		assert.Equal(t, 1, healthy)

		// Verify the node is marked unhealthy
		assert.False(t, helper.nodes[0].IsHealthy)
		assert.Equal(t, 1, helper.nodes[0].FailureCount)
	})

	t.Run("mark node healthy", func(t *testing.T) {
		// Mark first node as healthy again
		helper.markNodeHealthy(server1.URL(), false)

		healthy, _ := helper.GetHealthyNodeCount()
		assert.Equal(t, 2, healthy)

		// Verify the node is marked healthy
		assert.True(t, helper.nodes[0].IsHealthy)
		assert.Equal(t, 0, helper.nodes[0].FailureCount)
	})
}

// TestRPCHelperContractBackend tests the ContractBackend functionality
func TestRPCHelperContractBackend(t *testing.T) {
	server := NewMockRPCServer()
	defer server.Close()

	config := &RPCConfig{
		Nodes: []NodeConfig{
			{URL: server.URL()},
		},
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		MaxRetryDelay:  5 * time.Second,
		RequestTimeout: 10 * time.Second,
	}

	helper := NewRPCHelper(config)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := helper.Initialize(ctx)
	require.NoError(t, err)
	defer helper.Close()

	backend := helper.NewContractBackend()
	require.NotNil(t, backend)

	t.Run("CallContract", func(t *testing.T) {
		msg := ethereum.CallMsg{
			To:   &common.Address{},
			Data: []byte{},
		}
		result, err := backend.CallContract(ctx, msg, big.NewInt(1))
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("PendingCallContract", func(t *testing.T) {
		msg := ethereum.CallMsg{
			To:   &common.Address{},
			Data: []byte{},
		}
		result, err := backend.PendingCallContract(ctx, msg)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("CodeAt", func(t *testing.T) {
		addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
		code, err := backend.CodeAt(ctx, addr, big.NewInt(1))
		require.NoError(t, err)
		assert.NotNil(t, code)
	})

	t.Run("HeaderByNumber", func(t *testing.T) {
		header, err := backend.HeaderByNumber(ctx, big.NewInt(1))
		require.NoError(t, err)
		assert.NotNil(t, header)
	})

	t.Run("PendingCodeAt", func(t *testing.T) {
		addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
		code, err := backend.PendingCodeAt(ctx, addr)
		require.NoError(t, err)
		assert.NotNil(t, code)
	})

	t.Run("PendingNonceAt", func(t *testing.T) {
		addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
		nonce, err := backend.PendingNonceAt(ctx, addr)
		require.NoError(t, err)
		assert.Greater(t, nonce, uint64(0))
	})

	t.Run("SuggestGasPrice", func(t *testing.T) {
		gasPrice, err := backend.SuggestGasPrice(ctx)
		require.NoError(t, err)
		assert.NotNil(t, gasPrice)
		assert.True(t, gasPrice.Cmp(big.NewInt(0)) > 0)
	})

	t.Run("SuggestGasTipCap", func(t *testing.T) {
		gasTip, err := backend.SuggestGasTipCap(ctx)
		require.NoError(t, err)
		assert.NotNil(t, gasTip)
		assert.True(t, gasTip.Cmp(big.NewInt(0)) > 0)
	})

	t.Run("EstimateGas", func(t *testing.T) {
		msg := ethereum.CallMsg{
			To:   &common.Address{},
			Data: []byte{},
		}
		gas, err := backend.EstimateGas(ctx, msg)
		require.NoError(t, err)
		assert.Greater(t, gas, uint64(0))
	})

	t.Run("FilterLogs", func(t *testing.T) {
		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(1),
			ToBlock:   big.NewInt(2),
		}
		logs, err := backend.FilterLogs(ctx, query)
		require.NoError(t, err)
		assert.NotNil(t, logs)
	})

	t.Run("SubscribeFilterLogs", func(t *testing.T) {
		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(1),
			ToBlock:   big.NewInt(2),
		}
		ch := make(chan types.Log, 1)
		sub, err := backend.SubscribeFilterLogs(ctx, query, ch)
		if err != nil {
			// Subscription might fail with mock server, that's ok
			assert.Contains(t, err.Error(), "notifications not supported")
		} else {
			assert.NotNil(t, sub)
			sub.Unsubscribe()
		}
		close(ch)
	})
}

// TestConfigDefaults tests configuration defaults
func TestConfigDefaults(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		config := DefaultRPCConfig()
		assert.Equal(t, 3, config.MaxRetries)
		assert.Equal(t, 500*time.Millisecond, config.RetryDelay)
		assert.Equal(t, 30*time.Second, config.MaxRetryDelay)
		assert.Equal(t, 30*time.Second, config.RequestTimeout)
	})

	t.Run("config from URLs", func(t *testing.T) {
		urls := []string{"http://localhost:8545", "http://localhost:8546"}
		archiveURLs := []string{"http://localhost:8547"}

		config := NewRPCConfigFromURLs(urls, archiveURLs)
		assert.Len(t, config.Nodes, 2)
		assert.Len(t, config.ArchiveNodes, 1)
		assert.Equal(t, "http://localhost:8545", config.Nodes[0].URL)
		assert.Equal(t, "http://localhost:8547", config.ArchiveNodes[0].URL)
	})

	t.Run("config with retry schedule", func(t *testing.T) {
		urls := []string{"http://localhost:8545"}
		primarySchedule := []int{3, 6, 12}
		secondarySchedule := []int{6, 12, 24}

		config := NewRPCConfigWithRetrySchedule(urls, nil, primarySchedule, secondarySchedule)
		assert.Equal(t, primarySchedule, config.PrimaryRetryAfterRequests)
		assert.Equal(t, secondarySchedule, config.SecondaryRetryAfterRequests)
	})

	t.Run("set config defaults", func(t *testing.T) {
		config := &RPCConfig{}
		setConfigDefaults(config)

		assert.Equal(t, []int{5, 10, 20}, config.PrimaryRetryAfterRequests)
		assert.Equal(t, []int{10, 20, 40}, config.SecondaryRetryAfterRequests)
	})
}
