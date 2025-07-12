# RPC Helper - Go Ethereum RPC Client Wrapper

A robust, production-ready Ethereum RPC client wrapper with automatic failover, retry logic, and comprehensive error handling.

## Features

- **Multiple Node Support**: Configure multiple RPC endpoints for high availability
- **Automatic Failover**: Seamlessly switches to backup nodes when primary fails
- **Smart Retry Logic**: Exponential backoff with configurable retry attempts
- **Health Monitoring**: Track node health and automatically recover failed nodes
- **Archive Node Support**: Separate configuration for archive nodes for historical data
- **Comprehensive Logging**: Detailed logging for debugging and monitoring
- **Thread Safe**: Concurrent access support with proper mutex handling
- **Production Ready**: Built for high-throughput applications

## Installation

```bash
go get github.com/powerloom/go-rpc-helper
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "math/big"
    "time"

    rpchelper "github.com/powerloom/go-rpc-helper"
)

func main() {
    // Create configuration
    config := rpchelper.NewRPCConfigFromURLs(
        []string{
            "https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY",
            "https://mainnet.infura.io/v3/YOUR_KEY",
        },
        []string{
            "https://eth-mainnet.g.alchemy.com/v2/YOUR_ARCHIVE_KEY",
        },
    )

    // Create RPC helper
    rpc := rpchelper.NewRPCHelper(config)
    
    // Initialize
    ctx := context.Background()
    if err := rpc.Initialize(ctx); err != nil {
        log.Fatal("Failed to initialize RPC helper:", err)
    }
    defer rpc.Close()

    // Get latest block number
    blockNumber, err := rpc.BlockNumber(ctx)
    if err != nil {
        log.Fatal("Failed to get block number:", err)
    }
    
    log.Printf("Latest block: %d", blockNumber)
    
    // Get block details
    block, err := rpc.BlockByNumber(ctx, big.NewInt(int64(blockNumber)))
    if err != nil {
        log.Fatal("Failed to get block:", err)
    }
    
    log.Printf("Block hash: %s", block.Hash().Hex())
}
```

## Configuration

### Basic Configuration

```go
config := &rpchelper.RPCConfig{
    Nodes: []rpchelper.NodeConfig{
        {URL: "https://eth-mainnet.alchemyapi.io/v2/YOUR_KEY"},
        {URL: "https://mainnet.infura.io/v3/YOUR_KEY"},
    },
    MaxRetries:     3,
    RetryDelay:     500 * time.Millisecond,
    MaxRetryDelay:  30 * time.Second,
    RequestTimeout: 30 * time.Second,
}
```

### Using Default Configuration

```go
config := rpchelper.DefaultRPCConfig()
config.Nodes = []rpchelper.NodeConfig{
    {URL: "https://your-rpc-endpoint.com"},
}
```

### With Archive Nodes

```go
config := rpchelper.NewRPCConfigFromURLs(
    []string{"https://mainnet-rpc.com"},      // Regular nodes
    []string{"https://archive-rpc.com"},      // Archive nodes
)
```

## API Reference

### Core Methods

#### Block Operations
```go
// Get latest block number
blockNum, err := rpc.BlockNumber(ctx)

// Get block by number
block, err := rpc.BlockByNumber(ctx, big.NewInt(12345))

// Get block by number (latest)
block, err := rpc.BlockByNumber(ctx, nil)
```

#### Transaction Operations
```go
// Get transaction by hash
tx, isPending, err := rpc.TransactionByHash(ctx, txHash)

// Get transaction receipt
receipt, err := rpc.TransactionReceipt(ctx, txHash)
```

#### Contract Calls
```go
// Execute contract call
result, err := rpc.CallContract(ctx, callMsg, blockNumber)

// Execute contract call on archive node
result, err := rpc.CallContractArchive(ctx, callMsg, blockNumber)
```

#### Event Filtering
```go
// Filter logs
logs, err := rpc.FilterLogs(ctx, filterQuery)
```

#### Raw JSON-RPC
```go
// Make raw JSON-RPC call
response, err := rpc.JSONRPCCall(ctx, "eth_getBalance", address, "latest")

// Batch JSON-RPC calls
requests := []rpc.BatchElem{
    {Method: "eth_getBalance", Args: []interface{}{address1, "latest"}},
    {Method: "eth_getBalance", Args: []interface{}{address2, "latest"}},
}
err := rpc.BatchJSONRPCCall(ctx, requests)
```

### Utility Functions

```go
// Validate Ethereum address
isValid := rpchelper.IsValidAddress("0x742d35Cc6634C0532925a3b8D6cC6C2")

// Convert between Wei and Ether
wei := rpchelper.ToWei(1.5) // 1.5 ETH to Wei
ether := rpchelper.FromWei(big.NewInt(1000000000000000000)) // Wei to ETH

// Parse address safely
addr, err := rpchelper.ParseAddressFromString("0x742d35Cc6634C0532925a3b8D6cC6C2")

// Format address
formatted := rpchelper.FormatAddress(addr) // lowercase hex
```

### Health Monitoring

```go
checker := rpchelper.NewHealthChecker(rpc)
regularErrors, archiveErrors := checker.CheckAllNodes(ctx)

// Check healthy node count
healthy, healthyArchive := rpc.GetHealthyNodeCount()
```

## Webhook Alerting

The RPC helper includes built-in webhook alerting to notify you when nodes fail or recover.

### Configuration

```go
config := &rpchelper.RPCConfig{
    Nodes: []rpchelper.NodeConfig{
        {URL: "https://your-primary-rpc.com"},
        {URL: "https://your-backup-rpc.com"},
    },
    MaxRetries:     3,
    RetryDelay:     time.Second,
    MaxRetryDelay:  time.Minute,
    RequestTimeout: 30 * time.Second,
    
    // Configure webhook alerts
    WebhookConfig: &reporting.WebhookConfig{
        URL:     "https://your-webhook-endpoint.com/alerts",
        Timeout: 30 * time.Second,
        Retries: 3,
    },
}

rpc := rpchelper.NewRPCHelper(config)
```

### Automatic Alerts

When configured, the RPC helper automatically sends alerts for:

- **Node failures**: When individual nodes become unhealthy
- **Node recovery**: When previously unhealthy nodes recover
- **Critical failures**: When all nodes are unhealthy
- **Initialization errors**: When nodes fail to initialize

### Manual Alerts

You can also send custom alerts:

```go
import "github.com/powerloom/go-rpc-helper/reporting"

// Send different severity levels
reporting.SendInfoAlert("myapp", "Application started successfully")
reporting.SendWarningAlert("myapp", "High memory usage detected")
reporting.SendCriticalAlert("myapp", "Database connection failed")

// Send alert with custom timestamp
reporting.SendFailureNotification("myapp", "Custom error occurred", 
    time.Now().Format(time.RFC3339), "warning")
```

### Alert Format

Alerts are sent as JSON to your webhook endpoint:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "process_name": "rpc-helper",
  "error_msg": "Node https://rpc.example.com has become unhealthy: connection timeout",
  "severity": "warning"
}
```

### Environment Configuration

```bash
# Set webhook URL via environment variable
export WEBHOOK_URL="https://your-webhook-endpoint.com/alerts"
export WEBHOOK_TIMEOUT="30s"
```

```go
// Configure from environment
webhookURL := os.Getenv("WEBHOOK_URL")
if webhookURL != "" {
    config.WebhookConfig = &reporting.WebhookConfig{
        URL:     webhookURL,
        Timeout: 30 * time.Second,
        Retries: 3,
    }
}
```

### Alert Management

```go
// Check current alert queue
queueLength := len(reporting.RpcAlertsChannel)
log.Printf("Alerts in queue: %d", queueLength)

// The alert processor runs in background automatically
// and sends alerts to your webhook with retry logic
```

## Integration with Existing Code

Replace your existing RPC client initialization:

### Before (using ethclient directly)
```go
client, err := ethclient.Dial("https://mainnet.infura.io/v3/YOUR_KEY")
if err != nil {
    log.Fatal(err)
}

// Get block
block, err := client.BlockByNumber(ctx, nil)
```

### After (using rpc-helper)
```go
config := rpchelper.NewRPCConfigFromURLs([]string{
    "https://mainnet.infura.io/v3/YOUR_KEY",
    "https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY", // Backup
}, nil)

rpc := rpchelper.NewRPCHelper(config)
if err := rpc.Initialize(ctx); err != nil {
    log.Fatal(err)
}
defer rpc.Close()

// Get block with automatic retry and failover
block, err := rpc.BlockByNumber(ctx, nil)
```

## Error Handling

The package provides detailed error information:

```go
block, err := rpc.BlockByNumber(ctx, nil)
if err != nil {
    if rpcErr, ok := err.(*rpchelper.RPCException); ok {
        log.Printf("RPC Error: %s", rpcErr.ExtraInfo)
        log.Printf("Node: %s", rpcErr.NodeURL)
        log.Printf("Underlying: %v", rpcErr.UnderlyingError)
    } else {
        log.Printf("Other error: %v", err)
    }
}
```

## Advanced Usage

### Custom Retry Configuration

```go
config := &rpchelper.RPCConfig{
    Nodes: []rpchelper.NodeConfig{{URL: "https://your-rpc.com"}},
    MaxRetries:     5,                    // Retry up to 5 times
    RetryDelay:     100 * time.Millisecond, // Start with 100ms delay
    MaxRetryDelay:  60 * time.Second,     // Max delay between retries
    RequestTimeout: 45 * time.Second,     // Timeout per request
}
```

### Monitoring Node Health

```go
// Periodically check node health
ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()

go func() {
    for range ticker.C {
        checker := rpchelper.NewHealthChecker(rpc)
        regularErrors, archiveErrors := checker.CheckAllNodes(ctx)
        
        if len(regularErrors) > 0 {
            log.Printf("Unhealthy regular nodes: %v", regularErrors)
        }
        if len(archiveErrors) > 0 {
            log.Printf("Unhealthy archive nodes: %v", archiveErrors)
        }
    }
}()
```

## Testing

The RPC helper includes comprehensive tests covering node management, failover logic, retry mechanisms, and error handling.

### Running Tests

```bash
# Run all tests
go test -v

# Run tests with coverage
go test -v -cover

# Run specific test suites
go test -v -run TestNodeManagement
go test -v -run TestRetryScheduleCalculation
go test -v -run TestGetCurrentNodeEdgeCases

# Run tests with race detection
go test -v -race

# Run tests with timeout
go test -v -timeout 30s
```

### Test Categories

#### Node Management Tests
```bash
# Test primary node prioritization and failover
go test -v -run "TestNodeManagement/primary_node_prioritization"
go test -v -run "TestNodeManagement/failover_to_secondary"
go test -v -run "TestNodeManagement/primary_node_recovery"
```

#### Retry Logic Tests
```bash
# Test retry schedule calculation
go test -v -run "TestRetryScheduleCalculation"
go test -v -run "TestShouldRetryNode"
```

#### Edge Case Tests
```bash
# Test complex scenarios and edge cases
go test -v -run "TestGetCurrentNodeEdgeCases"
```

### Mock Server Testing

The tests use a built-in mock RPC server that simulates various failure scenarios:

```go
// Example test setup
primary := NewMockRPCServer()
secondary := NewMockRPCServer()
defer primary.Close()
defer secondary.Close()

// Simulate node failure
primary.SetShouldFail(true)

// Test failover behavior
_, err := helper.BlockNumber(ctx)
```

### Test Configuration

Tests use configurable retry schedules for different scenarios:

```go
config := &RPCConfig{
    Nodes: []NodeConfig{
        {URL: primary.URL()},
        {URL: secondary.URL()},
    },
    MaxRetries:                  3,
    RetryDelay:                  50 * time.Millisecond,
    MaxRetryDelay:               1 * time.Second,
    RequestTimeout:              5 * time.Second,
    PrimaryRetryAfterRequests:   []int{2, 4, 6},    // Retry after 2, 4, 6 requests
    SecondaryRetryAfterRequests: []int{3, 6, 9},    // Retry after 3, 6, 9 requests
}
```

### Writing Custom Tests

When writing tests for your application using the RPC helper:

```go
func TestMyApplication(t *testing.T) {
    // Create test configuration
    config := rpchelper.DefaultRPCConfig()
    config.Nodes = []rpchelper.NodeConfig{
        {URL: "http://localhost:8545"}, // Your test RPC endpoint
    }
    
    // Create RPC helper
    rpc := rpchelper.NewRPCHelper(config)
    ctx := context.Background()
    
    err := rpc.Initialize(ctx)
    require.NoError(t, err)
    defer rpc.Close()
    
    // Test your application logic
    result, err := rpc.BlockNumber(ctx)
    assert.NoError(t, err)
    assert.Greater(t, result, uint64(0))
}
```

### Continuous Integration

Example GitHub Actions workflow:

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    - uses: actions/setup-go@v2
      with:
        go-version: 1.21
    
    - name: Run tests
      run: |
        go test -v -race -cover ./...
        
    - name: Run benchmarks
      run: |
        go test -bench=. -benchmem
```

## Best Practices

1. **Always use context with timeout** for operations
2. **Configure multiple RPC endpoints** for high availability
3. **Monitor node health** periodically in production
4. **Use archive nodes** only when historical data is needed
5. **Handle errors gracefully** and implement appropriate fallbacks
6. **Close the RPC helper** when shutting down your application

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details. 