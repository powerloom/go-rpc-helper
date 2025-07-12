package rpchelper

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNodeManagement tests comprehensive node management functionality
func TestNodeManagement(t *testing.T) {
	t.Run("primary node prioritization", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},   // Primary node
				{URL: secondary.URL()}, // Secondary node
			},
			MaxRetries:                  3,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2, 4, 6},
			SecondaryRetryAfterRequests: []int{3, 6, 9},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Test that primary node is selected first
		node, err := helper.getCurrentNode(false)
		require.NoError(t, err)
		assert.Equal(t, primary.URL(), node.URL)

		// Make multiple requests to ensure primary is consistently selected
		for i := 0; i < 5; i++ {
			_, err := helper.BlockNumber(ctx)
			require.NoError(t, err)
		}

		// Verify primary was used (should have higher call count)
		assert.Greater(t, primary.GetCallCount(), secondary.GetCallCount())
	})

	t.Run("failover to secondary on primary failure", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2, 4, 6},
			SecondaryRetryAfterRequests: []int{3, 6, 9},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make primary fail
		primary.SetShouldFail(true)

		// Make a request - should failover to secondary
		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err)

		// Verify primary was marked unhealthy
		assert.False(t, helper.nodes[0].IsHealthy)
		assert.True(t, helper.nodes[1].IsHealthy)

		// Verify secondary was used
		assert.Greater(t, secondary.GetCallCount(), 0)
	})

	t.Run("primary node recovery", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2, 4, 6}, // Retry after 2, 4, 6 requests
			SecondaryRetryAfterRequests: []int{3, 6, 9},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make primary fail initially
		primary.SetShouldFail(true)

		// Make a request to mark primary as unhealthy
		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err)

		// Verify primary is unhealthy
		assert.False(t, helper.nodes[0].IsHealthy)
		assert.Equal(t, 1, helper.nodes[0].FailureCount)

		// Fix primary server
		primary.SetShouldFail(false)

		// Make enough requests to trigger primary recovery (2 requests for first retry)
		for i := 0; i < 3; i++ {
			_, err = helper.BlockNumber(ctx)
			require.NoError(t, err)
		}

		// Verify primary node recovered
		assert.True(t, helper.nodes[0].IsHealthy)
		assert.Equal(t, 0, helper.nodes[0].FailureCount)
		assert.Equal(t, 0, helper.nodes[0].SkipCount)
	})

	t.Run("secondary node retry logic", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary1 := NewMockRPCServer()
		secondary2 := NewMockRPCServer()
		defer primary.Close()
		defer secondary1.Close()
		defer secondary2.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary1.URL()},
				{URL: secondary2.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2, 4, 6},
			SecondaryRetryAfterRequests: []int{3, 6, 9}, // Retry after 3, 6, 9 requests
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make all nodes fail initially
		primary.SetShouldFail(true)
		secondary1.SetShouldFail(true)
		secondary2.SetShouldFail(true)

		// Make requests to mark all nodes as unhealthy
		_, err = helper.BlockNumber(ctx)
		// Expect error since all nodes are failing
		assert.Error(t, err)

		// Verify all nodes are unhealthy
		assert.False(t, helper.nodes[0].IsHealthy)
		assert.False(t, helper.nodes[1].IsHealthy)
		assert.False(t, helper.nodes[2].IsHealthy)

		// Fix secondary1
		secondary1.SetShouldFail(false)

		// Make enough requests to trigger secondary1 recovery (3 requests for first retry)
		for i := 0; i < 5; i++ {
			_, err = helper.BlockNumber(ctx)
			if err == nil {
				break // Success - secondary1 recovered
			}
		}

		// Verify secondary1 recovered
		assert.True(t, helper.nodes[1].IsHealthy)
		assert.Equal(t, 0, helper.nodes[1].FailureCount)
	})

	t.Run("skip count increment and reset", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{3}, // Retry after 3 requests
			SecondaryRetryAfterRequests: []int{4}, // Retry after 4 requests
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make primary fail
		primary.SetShouldFail(true)

		// Make a request to mark primary as unhealthy
		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err)

		// Verify primary is unhealthy
		assert.False(t, helper.nodes[0].IsHealthy)

		// Make requests to increment skip count
		for i := 0; i < 2; i++ {
			_, err = helper.BlockNumber(ctx)
			require.NoError(t, err)
		}

		// Check skip count is incremented (should be 3 now)
		// Initial failure request increments to 1 on fallback to secondary, then 2 loop iterations increment to 3
		assert.Equal(t, 3, helper.nodes[0].SkipCount)

		// Fix primary
		primary.SetShouldFail(false)

		// Make one more request to trigger retry (skip count reaches 4, then gets reset to 0 on retry)
		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err)

		// Verify primary recovered and skip count reset
		assert.True(t, helper.nodes[0].IsHealthy)
		assert.Equal(t, 0, helper.nodes[0].SkipCount)
		assert.Equal(t, 0, helper.nodes[0].FailureCount)
	})

	t.Run("minimum retry time enforcement", func(t *testing.T) {
		// Test that MinRetryTime controls when nodes are considered for recovery
		// but primary is still returned as fallback even during cooldown
		primary := NewMockRPCServer()
		defer primary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{1}, // Retry after 1 request
			SecondaryRetryAfterRequests: []int{1},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make primary fail
		primary.SetShouldFail(true)

		// Make a request to mark primary as unhealthy
		_, err = helper.BlockNumber(ctx)
		assert.Error(t, err)

		// Record when primary was marked unhealthy
		failTime := helper.nodes[0].LastFailTime

		// Make a request immediately (primary still failing, should use as fallback)
		_, err = helper.BlockNumber(ctx)
		assert.Error(t, err) // Should still fail because primary is still set to fail

		// Fix primary
		primary.SetShouldFail(false)

		// Now retry should work
		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err)

		// Verify primary recovered
		assert.True(t, helper.nodes[0].IsHealthy)
		assert.True(t, helper.nodes[0].LastFailTime.After(failTime) || helper.nodes[0].LastFailTime.IsZero())
	})

	t.Run("all nodes unhealthy with primary fallback", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2, 4, 6},
			SecondaryRetryAfterRequests: []int{3, 6, 9},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make all nodes fail
		primary.SetShouldFail(true)
		secondary.SetShouldFail(true)

		// Make requests - should error because nodes are unhealthy but system shouldn't crash
		for i := 0; i < 3; i++ {
			_, err = helper.BlockNumber(ctx)
			assert.Error(t, err) // Should error but not panic
		}

		// Verify getCurrentNode returns primary as fallback when all nodes are unhealthy
		node, err := helper.getCurrentNode(false)
		require.NoError(t, err)
		assert.Equal(t, primary.URL(), node.URL) // Should return primary as fallback

		// Verify health counts - all nodes should be unhealthy
		healthy, _ := helper.GetHealthyNodeCount()
		assert.Equal(t, 0, healthy)
	})

	t.Run("primary recovery from all nodes unhealthy", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2}, // Retry after 2 requests
			SecondaryRetryAfterRequests: []int{3},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make all nodes fail
		primary.SetShouldFail(true)
		secondary.SetShouldFail(true)

		// Make requests to mark all nodes unhealthy
		for i := 0; i < 3; i++ {
			_, err = helper.BlockNumber(ctx)
			assert.Error(t, err)
		}

		// Verify all nodes are unhealthy
		healthy, _ := helper.GetHealthyNodeCount()
		assert.Equal(t, 0, healthy)

		// Fix the primary node
		primary.SetShouldFail(false)

		// Make requests to trigger primary recovery
		for i := 0; i < 3; i++ {
			_, err = helper.BlockNumber(ctx)
			if err == nil {
				break // Primary recovered
			}
		}

		// Verify primary recovered
		assert.True(t, helper.nodes[0].IsHealthy)
		assert.Equal(t, 0, helper.nodes[0].FailureCount)
		assert.Equal(t, 0, helper.nodes[0].SkipCount)

		// Secondary should still be unhealthy
		assert.False(t, helper.nodes[1].IsHealthy)

		// Verify system continues to work with recovered primary
		for i := 0; i < 3; i++ {
			_, err = helper.BlockNumber(ctx)
			require.NoError(t, err)
		}
	})

	t.Run("both nodes recover independently", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{1}, // Quick recovery
			SecondaryRetryAfterRequests: []int{1},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make both nodes fail
		primary.SetShouldFail(true)
		secondary.SetShouldFail(true)

		// Make requests to mark both unhealthy
		_, err = helper.BlockNumber(ctx)
		assert.Error(t, err)

		// Both should be unhealthy
		healthy, _ := helper.GetHealthyNodeCount()
		assert.Equal(t, 0, healthy)

		// Fix both nodes
		primary.SetShouldFail(false)
		secondary.SetShouldFail(false)

		// Make requests - primary should recover first
		for i := 0; i < 3; i++ {
			_, err = helper.BlockNumber(ctx)
			if err == nil {
				break
			}
		}

		// Primary should be healthy
		assert.True(t, helper.nodes[0].IsHealthy)

		// Continue making requests - system should work normally
		for i := 0; i < 5; i++ {
			_, err = helper.BlockNumber(ctx)
			require.NoError(t, err)
		}

		// At least primary should be healthy
		healthy, _ = helper.GetHealthyNodeCount()
		assert.GreaterOrEqual(t, healthy, 1)
	})

	t.Run("concurrent node access", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2, 4, 6},
			SecondaryRetryAfterRequests: []int{3, 6, 9},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Run concurrent requests
		var wg sync.WaitGroup
		errors := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := helper.BlockNumber(ctx)
				if err != nil {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Check for any errors
		var errorCount int
		for err := range errors {
			if err != nil {
				errorCount++
				t.Logf("Concurrent request error: %v", err)
			}
		}

		// Should have minimal errors with healthy nodes
		assert.Less(t, errorCount, 5, "Too many concurrent request errors")
	})

	t.Run("failure count escalation", func(t *testing.T) {
		primary := NewMockRPCServer()
		defer primary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{1, 2, 4}, // Escalating retry delays
			SecondaryRetryAfterRequests: []int{2, 4, 8},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make primary fail
		primary.SetShouldFail(true)

		// Cause multiple failures to increase failure count
		for i := 0; i < 3; i++ {
			_, err = helper.BlockNumber(ctx)
			assert.Error(t, err)
		}

		// Check failure count increased
		assert.Equal(t, 3, helper.nodes[0].FailureCount)
		assert.False(t, helper.nodes[0].IsHealthy)

		// Fix primary
		primary.SetShouldFail(false)

		// The retry schedule should now use 4 requests (third failure level)
		// Make requests to trigger retry
		for i := 0; i < 6; i++ {
			_, err = helper.BlockNumber(ctx)
			if err == nil {
				break // Success - node recovered
			}
		}

		// Verify node recovered
		assert.True(t, helper.nodes[0].IsHealthy)
		assert.Equal(t, 0, helper.nodes[0].FailureCount)
	})

	t.Run("archive node handling", func(t *testing.T) {
		regularNode := NewMockRPCServer()
		archiveNode1 := NewMockRPCServer()
		archiveNode2 := NewMockRPCServer()
		defer regularNode.Close()
		defer archiveNode1.Close()
		defer archiveNode2.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: regularNode.URL()},
			},
			ArchiveNodes: []NodeConfig{
				{URL: archiveNode1.URL()},
				{URL: archiveNode2.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2, 4, 6},
			SecondaryRetryAfterRequests: []int{3, 6, 9},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Test archive node selection
		node, err := helper.getCurrentNode(true) // Use archive nodes
		require.NoError(t, err)
		assert.Equal(t, archiveNode1.URL(), node.URL) // Should select first archive node

		// Test archive node failover
		archiveNode1.SetShouldFail(true)
		msg := ethereum.CallMsg{
			To:   &common.Address{},
			Data: []byte{},
		}
		_, err = helper.CallContractArchive(ctx, msg, nil)
		require.NoError(t, err)

		// Verify archive node was marked unhealthy and secondary archive node was used
		assert.False(t, helper.archiveNodes[0].IsHealthy)
		assert.Greater(t, archiveNode2.GetCallCount(), 0)
	})
}

// TestRetryScheduleCalculation tests the retry schedule calculation logic
func TestRetryScheduleCalculation(t *testing.T) {
	config := &RPCConfig{
		PrimaryRetryAfterRequests:   []int{2, 4, 8},
		SecondaryRetryAfterRequests: []int{3, 6, 12},
	}

	helper := NewRPCHelper(config)

	t.Run("primary retry schedule", func(t *testing.T) {
		// Test different failure counts
		assert.Equal(t, 2, helper.calculateRequiredSkips(1, true))  // First failure
		assert.Equal(t, 4, helper.calculateRequiredSkips(2, true))  // Second failure
		assert.Equal(t, 8, helper.calculateRequiredSkips(3, true))  // Third failure
		assert.Equal(t, 8, helper.calculateRequiredSkips(4, true))  // Beyond schedule - use last value
		assert.Equal(t, 8, helper.calculateRequiredSkips(10, true)) // Way beyond schedule
	})

	t.Run("secondary retry schedule", func(t *testing.T) {
		// Test different failure counts
		assert.Equal(t, 3, helper.calculateRequiredSkips(1, false))  // First failure
		assert.Equal(t, 6, helper.calculateRequiredSkips(2, false))  // Second failure
		assert.Equal(t, 12, helper.calculateRequiredSkips(3, false)) // Third failure
		assert.Equal(t, 12, helper.calculateRequiredSkips(4, false)) // Beyond schedule - use last value
		assert.Equal(t, 12, helper.calculateRequiredSkips(10, false))
	})

	t.Run("empty retry schedule fallback", func(t *testing.T) {
		emptyConfig := &RPCConfig{
			PrimaryRetryAfterRequests:   []int{},
			SecondaryRetryAfterRequests: []int{},
		}
		emptyHelper := NewRPCHelper(emptyConfig)

		// Won't use fallback values of 20/40 because we force the default values
		assert.Equal(t, 5, emptyHelper.calculateRequiredSkips(1, true))   // Primary fallback
		assert.Equal(t, 10, emptyHelper.calculateRequiredSkips(1, false)) // Secondary fallback
	})
}

// TestShouldRetryNode tests the shouldRetryNode logic
func TestShouldRetryNode(t *testing.T) {
	config := &RPCConfig{
		PrimaryRetryAfterRequests:   []int{2, 4, 8},
		SecondaryRetryAfterRequests: []int{3, 6, 12},
	}

	helper := NewRPCHelper(config)

	t.Run("skip count requirements", func(t *testing.T) {
		node := &RPCNode{
			FailureCount: 1,
			SkipCount:    1, // Less than required (2)
		}

		// Should not retry due to insufficient skip count
		assert.False(t, helper.shouldRetryNode(node, true))

		// Should retry with sufficient skip count
		node.SkipCount = 2
		assert.True(t, helper.shouldRetryNode(node, true))
	})

	t.Run("primary vs secondary differences", func(t *testing.T) {
		primaryNode := &RPCNode{
			FailureCount: 1,
			SkipCount:    2, // Meets primary requirement
		}

		secondaryNode := &RPCNode{
			FailureCount: 1,
			SkipCount:    2, // Less than secondary requirement (3)
		}

		// Primary should be ready to retry
		assert.True(t, helper.shouldRetryNode(primaryNode, true))

		// Secondary should not be ready yet
		assert.False(t, helper.shouldRetryNode(secondaryNode, false))

		// Secondary should be ready with higher skip count
		secondaryNode.SkipCount = 3
		assert.True(t, helper.shouldRetryNode(secondaryNode, false))
	})

	t.Run("escalating failure counts", func(t *testing.T) {
		node := &RPCNode{
			FailureCount: 2, // Second failure
			SkipCount:    3, // Less than required (4 for second failure)
		}

		// Should not retry yet
		assert.False(t, helper.shouldRetryNode(node, true))

		// Should retry with sufficient skip count for second failure
		node.SkipCount = 4
		assert.True(t, helper.shouldRetryNode(node, true))
	})
}

// TestGetCurrentNodeEdgeCases tests edge cases and potential bugs in getCurrentNode
func TestGetCurrentNodeEdgeCases(t *testing.T) {
	t.Run("skip count and recovery behavior", func(t *testing.T) {
		primary := NewMockRPCServer()
		secondary := NewMockRPCServer()
		defer primary.Close()
		defer secondary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
				{URL: secondary.URL()},
			},
			MaxRetries:                  3,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{2}, // Primary needs 2 skips to retry
			SecondaryRetryAfterRequests: []int{3}, // Secondary needs 3 skips to retry
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// PHASE 1: Test that primary is used when healthy
		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err)
		assert.Greater(t, primary.GetCallCount(), 0, "Primary should be used when healthy")

		// PHASE 2: Make primary fail, test skip count increment behavior
		primary.SetShouldFail(true)
		secondary.SetShouldFail(false)

		// First failure - primary gets marked unhealthy, secondary takes over
		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err) // Should succeed using secondary

		assert.False(t, helper.nodes[0].IsHealthy, "Primary should be unhealthy")
		assert.True(t, helper.nodes[1].IsHealthy, "Secondary should be healthy")
		// Primary skip count will be 1 because getCurrentNode was called during failover
		assert.Equal(t, 1, helper.nodes[0].SkipCount, "Primary skip count should be 1 after failover")

		// PHASE 3: Make secondary fail too, test skip count accumulation
		secondary.SetShouldFail(true)

		// Test skip count increment by calling getCurrentNode directly

		// Check initial state after both nodes are unhealthy
		t.Logf("Initial state - Primary SkipCount=%d, Secondary SkipCount=%d",
			helper.nodes[0].SkipCount, helper.nodes[1].SkipCount)

		// Call getCurrentNode and see what happens
		node, err := helper.getCurrentNode(false)
		require.NoError(t, err)

		t.Logf("After 1st getCurrentNode call - Returned: %s, Primary SkipCount=%d, Secondary SkipCount=%d",
			node.URL, helper.nodes[0].SkipCount, helper.nodes[1].SkipCount)

		// Call getCurrentNode again
		node, err = helper.getCurrentNode(false)
		require.NoError(t, err)

		t.Logf("After 2nd getCurrentNode call - Returned: %s, Primary SkipCount=%d, Secondary SkipCount=%d",
			node.URL, helper.nodes[0].SkipCount, helper.nodes[1].SkipCount)

		// Call getCurrentNode again
		node, err = helper.getCurrentNode(false)
		require.NoError(t, err)

		t.Logf("After 3rd getCurrentNode call - Returned: %s, Primary SkipCount=%d, Secondary SkipCount=%d",
			node.URL, helper.nodes[0].SkipCount, helper.nodes[1].SkipCount)

		// PHASE 4: Fix primary and verify system continues to work
		primary.SetShouldFail(false)
		secondary.SetShouldFail(false) // Also fix secondary to avoid interference

		// Log state before the call
		t.Logf("Before BlockNumber call - Primary Healthy=%v, SkipCount=%d, FailureCount=%d",
			helper.nodes[0].IsHealthy, helper.nodes[0].SkipCount, helper.nodes[0].FailureCount)

		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err, "Should succeed (using healthy secondary)")

		// Log state after the call
		t.Logf("After BlockNumber call - Primary Healthy=%v, SkipCount=%d, FailureCount=%d",
			helper.nodes[0].IsHealthy, helper.nodes[0].SkipCount, helper.nodes[0].FailureCount)

		// The primary should still be unhealthy because it wasn't used (secondary was healthy)
		// But skip count should have incremented because getCurrentNode was called
		assert.False(t, helper.nodes[0].IsHealthy, "Primary should still be unhealthy (secondary was used)")
		assert.Equal(t, 2, helper.nodes[0].SkipCount, "Primary skip count should be 2 (ready for retry)")
		assert.Equal(t, 1, helper.nodes[0].FailureCount, "Primary failure count should remain 1")

		// PHASE 5: Force primary recovery by making secondary unhealthy
		secondary.SetShouldFail(true)

		_, err = helper.BlockNumber(ctx)
		require.NoError(t, err, "Should succeed with recovered primary")

		// Now primary should be healthy
		assert.True(t, helper.nodes[0].IsHealthy, "Primary should be healthy after forced recovery")
		assert.Equal(t, 0, helper.nodes[0].SkipCount, "Primary skip count should be 0 after recovery")
		assert.Equal(t, 0, helper.nodes[0].FailureCount, "Primary failure count should be reset")
	})

	t.Run("single node fallback behavior", func(t *testing.T) {
		primary := NewMockRPCServer()
		defer primary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{1}, // Need 1 skip
			SecondaryRetryAfterRequests: []int{1},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make primary fail
		primary.SetShouldFail(true)

		// Make request to mark primary unhealthy
		_, err = helper.BlockNumber(ctx)
		assert.Error(t, err)

		// Try to get current node - should return primary as fallback
		node, err := helper.getCurrentNode(false)
		require.NoError(t, err)
		assert.Equal(t, primary.URL(), node.URL) // Should return primary as fallback

		// Fix the primary
		primary.SetShouldFail(false)

		// Now should be able to get the node for retry
		_, err = helper.getCurrentNode(false)
		require.NoError(t, err) // Should succeed now
	})

	t.Run("executeWithRetryAndFailover maxAttempts logic", func(t *testing.T) {
		primary := NewMockRPCServer()
		defer primary.Close()

		config := &RPCConfig{
			Nodes: []NodeConfig{
				{URL: primary.URL()},
			},
			MaxRetries:                  2,
			RetryDelay:                  50 * time.Millisecond,
			MaxRetryDelay:               1 * time.Second,
			RequestTimeout:              5 * time.Second,
			PrimaryRetryAfterRequests:   []int{1},
			SecondaryRetryAfterRequests: []int{1},
		}

		helper := NewRPCHelper(config)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := helper.Initialize(ctx)
		require.NoError(t, err)
		defer helper.Close()

		// Make primary fail
		primary.SetShouldFail(true)

		// Make request - should succeed because getCurrentNode returns primary as fallback
		// but the actual RPC call will fail since primary is set to fail
		_, err = helper.BlockNumber(ctx)
		assert.Error(t, err)

		// The error should be from the actual RPC call, not from getCurrentNode
		assert.NotContains(t, err.Error(), "no nodes with valid connections available")
	})
}
