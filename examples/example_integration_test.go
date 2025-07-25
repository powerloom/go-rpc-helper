package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	rpchelper "github.com/powerloom/go-rpc-helper"
	"github.com/stretchr/testify/assert"
)

func TestExampleIntegrationWithMockServer(t *testing.T) {
	// Create a mock server to simulate an Ethereum RPC endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Handle the eth_blockNumber method used for health checks and the actual test
		if reqBody["method"] == "eth_blockNumber" {
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      reqBody["id"],
				"result":  "0x1b4", // Example block number
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			// Respond with an error for any other method
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      reqBody["id"],
				"error": map[string]interface{}{
					"code":    -32601,
					"message": "method not found",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	// Use the mock server's URL for the RPC configuration
	config := rpchelper.NewRPCConfigFromURLs(
		[]string{server.URL},
		[]string{server.URL}, // Using the same for archive for simplicity
	)

	rpc := rpchelper.NewRPCHelper(config)
	ctx := context.Background()

	// Initialize the RPC helper
	err := rpc.Initialize(ctx)
	assert.NoError(t, err, "Initialize should not return an error with a working mock server")
	defer rpc.Close()

	// Call the BlockNumber method, which should now succeed
	blockNumber, err := rpc.BlockNumber(ctx)
	assert.NoError(t, err, "BlockNumber should not return an error")
	assert.Equal(t, uint64(0x1b4), blockNumber, "BlockNumber should return the expected value from the mock server")
}
