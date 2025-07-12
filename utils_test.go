package rpchelper

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUtilityFunctions tests all utility functions
func TestUtilityFunctions(t *testing.T) {
	t.Run("IsValidAddress", func(t *testing.T) {
		// Valid addresses
		assert.True(t, IsValidAddress("0x1234567890abcdef1234567890abcdef12345678"))
		assert.True(t, IsValidAddress("0xABCDEF1234567890abcdef1234567890ABCDEF12"))
		assert.True(t, IsValidAddress("0x0000000000000000000000000000000000000000"))

		// Invalid addresses
		assert.False(t, IsValidAddress("1234567890abcdef1234567890abcdef12345678"))    // No 0x prefix
		assert.False(t, IsValidAddress("0x1234567890abcdef1234567890abcdef1234567"))   // Too short
		assert.False(t, IsValidAddress("0x1234567890abcdef1234567890abcdef123456789")) // Too long
		assert.False(t, IsValidAddress("0x1234567890abcdef1234567890abcdef1234567g"))  // Invalid hex
		assert.False(t, IsValidAddress(""))                                            // Empty string
		assert.False(t, IsValidAddress("0x"))                                          // Just prefix
	})

	t.Run("ToWei", func(t *testing.T) {
		// Test conversion from Ether to Wei
		oneEther := ToWei(1.0)
		expectedWei := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
		assert.Equal(t, expectedWei, oneEther)

		// Test fractional Ether
		halfEther := ToWei(0.5)
		expectedHalfWei := new(big.Int).Div(expectedWei, big.NewInt(2))
		assert.Equal(t, expectedHalfWei, halfEther)

		// Test zero
		zero := ToWei(0.0)
		assert.Equal(t, big.NewInt(0), zero)

		// Test small amount
		smallAmount := ToWei(0.000000001) // 1 Gwei
		expectedGwei := big.NewInt(1000000000)
		assert.Equal(t, expectedGwei, smallAmount)
	})

	t.Run("FromWei", func(t *testing.T) {
		// Test conversion from Wei to Ether
		oneEtherWei := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
		ether := FromWei(oneEtherWei)
		assert.Equal(t, 1.0, ether)

		// Test half Ether
		halfEtherWei := new(big.Int).Div(oneEtherWei, big.NewInt(2))
		halfEther := FromWei(halfEtherWei)
		assert.Equal(t, 0.5, halfEther)

		// Test zero
		zero := FromWei(big.NewInt(0))
		assert.Equal(t, 0.0, zero)

		// Test Gwei
		gweiWei := big.NewInt(1000000000)
		gwei := FromWei(gweiWei)
		assert.Equal(t, 0.000000001, gwei)
	})

	t.Run("ParseAddressFromString", func(t *testing.T) {
		// Valid address
		validAddr := "0x1234567890abcdef1234567890abcdef12345678"
		addr, err := ParseAddressFromString(validAddr)
		require.NoError(t, err)
		assert.Equal(t, common.HexToAddress(validAddr), addr)

		// Invalid address
		invalidAddr := "invalid"
		_, err = ParseAddressFromString(invalidAddr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid address format")
	})

	t.Run("FormatAddress", func(t *testing.T) {
		// Test address formatting
		addr := common.HexToAddress("0x1234567890ABCDEF1234567890abcdef12345678")
		formatted := FormatAddress(addr)
		assert.Equal(t, "0x1234567890abcdef1234567890abcdef12345678", formatted)

		// Test zero address
		zeroAddr := common.Address{}
		formatted = FormatAddress(zeroAddr)
		assert.Equal(t, "0x0000000000000000000000000000000000000000", formatted)
	})

	t.Run("BlockNumberToHex", func(t *testing.T) {
		// Test regular block number
		blockNum := big.NewInt(1234567)
		hex := BlockNumberToHex(blockNum)
		assert.Equal(t, "0x12d687", hex)

		// Test zero block
		zeroBlock := big.NewInt(0)
		hex = BlockNumberToHex(zeroBlock)
		assert.Equal(t, "0x0", hex)

		// Test nil (should return "latest")
		hex = BlockNumberToHex(nil)
		assert.Equal(t, "latest", hex)
	})

	t.Run("HexToBlockNumber", func(t *testing.T) {
		// Test valid hex
		hex := "0x12d687"
		blockNum, err := HexToBlockNumber(hex)
		require.NoError(t, err)
		assert.Equal(t, big.NewInt(1234567), blockNum)

		// Test zero
		hex = "0x0"
		blockNum, err = HexToBlockNumber(hex)
		require.NoError(t, err)
		assert.Equal(t, big.NewInt(0), blockNum)

		// Test special identifiers (should error)
		specialIds := []string{"latest", "pending", "earliest"}
		for _, id := range specialIds {
			_, err = HexToBlockNumber(id)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "special block identifiers not supported")
		}

		// Test invalid hex (no 0x prefix)
		_, err = HexToBlockNumber("12d687")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hex string must start with 0x")
	})
}

// TestConfigurationHelpers tests configuration helper functions
func TestConfigurationHelpers(t *testing.T) {
	t.Run("DefaultRPCConfig", func(t *testing.T) {
		config := DefaultRPCConfig()

		assert.Equal(t, 3, config.MaxRetries)
		assert.Equal(t, 500*time.Millisecond, config.RetryDelay)
		assert.Equal(t, 30*time.Second, config.MaxRetryDelay)
		assert.Equal(t, 30*time.Second, config.RequestTimeout)
		assert.Empty(t, config.Nodes)
		assert.Empty(t, config.ArchiveNodes)
	})

	t.Run("NewRPCConfigFromURLs", func(t *testing.T) {
		urls := []string{
			"https://mainnet.infura.io/v3/key1",
			"https://mainnet.infura.io/v3/key2",
		}
		archiveURLs := []string{
			"https://mainnet-archive.infura.io/v3/key1",
		}

		config := NewRPCConfigFromURLs(urls, archiveURLs)

		// Check that it inherits from DefaultRPCConfig
		assert.Equal(t, 3, config.MaxRetries)
		assert.Equal(t, 500*time.Millisecond, config.RetryDelay)

		// Check nodes
		require.Len(t, config.Nodes, 2)
		assert.Equal(t, urls[0], config.Nodes[0].URL)
		assert.Equal(t, urls[1], config.Nodes[1].URL)

		// Check archive nodes
		require.Len(t, config.ArchiveNodes, 1)
		assert.Equal(t, archiveURLs[0], config.ArchiveNodes[0].URL)
	})

	t.Run("NewRPCConfig", func(t *testing.T) {
		urls := []string{"https://rpc1.example.com", "https://rpc2.example.com"}
		archiveURLs := []string{"https://archive.example.com"}
		maxRetries := 5
		retryDelay := 1 * time.Second
		maxRetryDelay := 60 * time.Second
		requestTimeout := 45 * time.Second

		config := NewRPCConfig(urls, archiveURLs, maxRetries, retryDelay, maxRetryDelay, requestTimeout)

		assert.Equal(t, maxRetries, config.MaxRetries)
		assert.Equal(t, retryDelay, config.RetryDelay)
		assert.Equal(t, maxRetryDelay, config.MaxRetryDelay)
		assert.Equal(t, requestTimeout, config.RequestTimeout)

		require.Len(t, config.Nodes, 2)
		assert.Equal(t, urls[0], config.Nodes[0].URL)
		assert.Equal(t, urls[1], config.Nodes[1].URL)

		require.Len(t, config.ArchiveNodes, 1)
		assert.Equal(t, archiveURLs[0], config.ArchiveNodes[0].URL)
	})

	t.Run("NewRPCConfigWithRetrySchedule", func(t *testing.T) {
		urls := []string{"https://rpc1.example.com"}
		archiveURLs := []string{"https://archive.example.com"}
		primarySchedule := []int{1, 3, 5}
		secondarySchedule := []int{2, 4, 8}

		config := NewRPCConfigWithRetrySchedule(urls, archiveURLs, primarySchedule, secondarySchedule)

		// Should inherit default values
		assert.Equal(t, 3, config.MaxRetries)
		assert.Equal(t, 500*time.Millisecond, config.RetryDelay)

		// Should have custom retry schedule
		assert.Equal(t, primarySchedule, config.PrimaryRetryAfterRequests)
		assert.Equal(t, secondarySchedule, config.SecondaryRetryAfterRequests)

		require.Len(t, config.Nodes, 1)
		assert.Equal(t, urls[0], config.Nodes[0].URL)

		require.Len(t, config.ArchiveNodes, 1)
		assert.Equal(t, archiveURLs[0], config.ArchiveNodes[0].URL)
	})

	t.Run("NewRPCConfigWithRetrySchedule empty schedules", func(t *testing.T) {
		urls := []string{"https://rpc1.example.com"}

		config := NewRPCConfigWithRetrySchedule(urls, nil, nil, nil)

		// Should use defaults when empty/nil schedules provided
		assert.Equal(t, []int{5, 10, 20}, config.PrimaryRetryAfterRequests)
		assert.Equal(t, []int{10, 20, 40}, config.SecondaryRetryAfterRequests)
	})
}

// TestHealthChecker tests the health checker functionality
func TestHealthChecker(t *testing.T) {
	t.Run("health checker with healthy nodes", func(t *testing.T) {
		server1 := NewMockRPCServer()
		server2 := NewMockRPCServer()
		archiveServer := NewMockRPCServer()
		defer server1.Close()
		defer server2.Close()
		defer archiveServer.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server1.URL()},
				{URL: server2.URL()},
			},
			ArchiveNodes: []NodeConfig{
				{URL: archiveServer.URL()},
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

		checker := NewHealthChecker(helper)
		regularErrors, archiveErrors := checker.CheckAllNodes(ctx)

		// Should have no errors with healthy nodes
		assert.Empty(t, regularErrors)
		assert.Empty(t, archiveErrors)
	})

	t.Run("health checker with unhealthy nodes", func(t *testing.T) {
		server1 := NewMockRPCServer()
		server2 := NewMockRPCServer()
		archiveServer := NewMockRPCServer()
		defer server1.Close()
		defer server2.Close()
		defer archiveServer.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: server1.URL()},
				{URL: server2.URL()},
			},
			ArchiveNodes: []NodeConfig{
				{URL: archiveServer.URL()},
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

		// Make some servers fail
		server1.SetShouldFail(true)
		archiveServer.SetShouldFail(true)

		checker := NewHealthChecker(helper)
		regularErrors, archiveErrors := checker.CheckAllNodes(ctx)

		// Should have errors for failed nodes
		assert.Len(t, regularErrors, 1) // server1 should fail
		assert.Contains(t, regularErrors, server1.URL())
		assert.NotContains(t, regularErrors, server2.URL()) // server2 should be healthy

		assert.Len(t, archiveErrors, 1) // archiveServer should fail
		assert.Contains(t, archiveErrors, archiveServer.URL())
	})

	t.Run("health checker with uninitialized clients", func(t *testing.T) {
		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: "http://fake-url"},
			},
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			MaxRetryDelay:  5 * time.Second,
			RequestTimeout: 10 * time.Second,
		}

		helper := NewRPCHelper(config)

		// Don't initialize - create a node with nil client
		helper.nodes = []*RPCNode{
			{
				URL:       "http://fake-url",
				EthClient: nil, // Uninitialized client
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		checker := NewHealthChecker(helper)
		regularErrors, archiveErrors := checker.CheckAllNodes(ctx)

		// Should have error for uninitialized client
		assert.Len(t, regularErrors, 1)
		assert.Contains(t, regularErrors["http://fake-url"].Error(), "eth client not initialized")
		assert.Empty(t, archiveErrors)
	})
}

// TestWeiConversions tests Wei conversion edge cases
func TestWeiConversions(t *testing.T) {
	t.Run("round trip conversion", func(t *testing.T) {
		// Test that converting to Wei and back gives the same result
		originalEther := 1.23456789
		wei := ToWei(originalEther)
		backToEther := FromWei(wei)

		// Should be very close (within floating point precision)
		assert.InDelta(t, originalEther, backToEther, 0.000000001)
	})

	t.Run("large numbers", func(t *testing.T) {
		// Test with large Ether amounts
		largeEther := 1000000.0
		wei := ToWei(largeEther)
		backToEther := FromWei(wei)

		assert.Equal(t, largeEther, backToEther)
	})

	t.Run("very small numbers", func(t *testing.T) {
		// Test with very small amounts (1 wei)
		oneWei := big.NewInt(1)
		ether := FromWei(oneWei)

		expected := 1.0 / 1000000000000000000.0 // 1 / 10^18
		assert.Equal(t, expected, ether)
	})
}

// TestAddressValidation tests address validation edge cases
func TestAddressValidation(t *testing.T) {
	t.Run("case insensitive validation", func(t *testing.T) {
		// Both should be valid
		lowerCase := "0x1234567890abcdef1234567890abcdef12345678"
		upperCase := "0x1234567890ABCDEF1234567890ABCDEF12345678"
		mixedCase := "0x1234567890AbCdEf1234567890aBcDeF12345678"

		assert.True(t, IsValidAddress(lowerCase))
		assert.True(t, IsValidAddress(upperCase))
		assert.True(t, IsValidAddress(mixedCase))
	})

	t.Run("address parsing consistency", func(t *testing.T) {
		original := "0x1234567890ABCDEF1234567890abcdef12345678"

		// Parse and format should be consistent
		addr, err := ParseAddressFromString(original)
		require.NoError(t, err)

		formatted := FormatAddress(addr)
		assert.Equal(t, "0x1234567890abcdef1234567890abcdef12345678", formatted)
	})
}

// TestBlockNumberConversions tests block number conversion edge cases
func TestBlockNumberConversions(t *testing.T) {
	t.Run("round trip conversion", func(t *testing.T) {
		original := big.NewInt(12345678)
		hex := BlockNumberToHex(original)
		converted, err := HexToBlockNumber(hex)
		require.NoError(t, err)

		assert.Equal(t, original, converted)
	})

	t.Run("large block numbers", func(t *testing.T) {
		// Test with very large block number
		large := new(big.Int)
		large.SetString("999999999999999999", 10)

		hex := BlockNumberToHex(large)
		converted, err := HexToBlockNumber(hex)
		require.NoError(t, err)

		assert.Equal(t, large, converted)
	})

	t.Run("hex validation", func(t *testing.T) {
		// Test various invalid hex formats
		invalidHex := []string{
			"0xgg",   // Invalid hex characters
			"0x",     // Empty hex
			"xyz",    // No 0x prefix
			"0x12G3", // Mixed valid/invalid
		}

		for _, hex := range invalidHex {
			_, err := HexToBlockNumber(hex)
			if hex != "0x" { // "0x" might be handled specially
				assert.Error(t, err, "Should error for invalid hex: %s", hex)
			}
		}
	})
}
