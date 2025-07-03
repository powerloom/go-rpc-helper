package main

import (
	"context"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	rpchelper "github.com/powerloom/rpc-helper"
	"github.com/powerloom/rpc-helper/reporting"
)

// ExampleIntegration shows how to integrate the RPC helper with your existing codebase
func main() {
	// Example of how to replace the existing RPC client in your protocol-state-cacher

	// 1. Create RPC configuration with multiple nodes for failover
	config := rpchelper.NewRPCConfigFromURLs(
		[]string{
			"https://eth-mainnet.g.alchemy.com/v2/YOUR_API_KEY",
			"https://mainnet.infura.io/v3/YOUR_PROJECT_ID",
			"https://eth-mainnet.gateway.pokt.network/v1/YOUR_APP_ID",
		},
		[]string{
			"https://eth-mainnet.g.alchemy.com/v2/YOUR_ARCHIVE_KEY", // Archive node for historical data
		},
	)

	// 2. Customize retry settings if needed
	config.MaxRetries = 5
	config.RetryDelay = 200 * time.Millisecond
	config.MaxRetryDelay = 60 * time.Second
	config.RequestTimeout = 30 * time.Second

	// 3. Create and initialize RPC helper
	rpc := rpchelper.NewRPCHelper(config)
	ctx := context.Background()

	if err := rpc.Initialize(ctx); err != nil {
		log.Fatal("Failed to initialize RPC helper:", err)
	}
	defer rpc.Close()

	log.Println("RPC Helper initialized successfully")

	// 4. Example usage replacing existing functionality

	// Replace your existing BlockByNumber calls
	exampleBlockOperations(ctx, rpc)

	// Replace your existing FilterLogs calls
	exampleEventFiltering(ctx, rpc)

	// Replace your existing contract calls
	exampleContractCalls(ctx, rpc)

	// Health monitoring
	exampleHealthMonitoring(ctx, rpc)

	// Webhook alerting example
	exampleWebhookAlerting(ctx, rpc)
}

// exampleBlockOperations shows how to replace existing block operations
func exampleBlockOperations(ctx context.Context, rpc *rpchelper.RPCHelper) {
	log.Println("=== Block Operations ===")

	// Get latest block number (replaces your existing Client.BlockNumber calls)
	latestBlock, err := rpc.BlockNumber(ctx)
	if err != nil {
		log.Printf("Error getting latest block: %v", err)
		return
	}
	log.Printf("Latest block number: %d", latestBlock)

	// Get specific block (replaces your existing Client.BlockByNumber calls)
	targetBlock := big.NewInt(int64(latestBlock - 10)) // 10 blocks back
	block, err := rpc.BlockByNumber(ctx, targetBlock)
	if err != nil {
		log.Printf("Error getting block %s: %v", targetBlock.String(), err)
		return
	}
	log.Printf("Block %s hash: %s", targetBlock.String(), block.Hash().Hex())
	log.Printf("Block %s has %d transactions", targetBlock.String(), len(block.Transactions()))
}

// exampleEventFiltering shows how to replace existing log filtering
func exampleEventFiltering(ctx context.Context, rpc *rpchelper.RPCHelper) {
	log.Println("=== Event Filtering ===")

	// Example: Filter logs for a contract (replace your existing Client.FilterLogs calls)
	contractAddress := common.HexToAddress("0xA0b86a33E6441E5e4B90bbC44F16aa456D5CC54") // Example contract

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(18500000), // Example block range
		ToBlock:   big.NewInt(18500010),
		Addresses: []common.Address{contractAddress},
		Topics: [][]common.Hash{
			// Add specific event signatures here
			{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")}, // Transfer event signature
		},
	}

	logs, err := rpc.FilterLogs(ctx, query)
	if err != nil {
		log.Printf("Error filtering logs: %v", err)
		return
	}
	log.Printf("Found %d logs in the specified range", len(logs))

	// Process logs as you normally would
	for i, eventLog := range logs {
		log.Printf("Log %d: Block %d, TxHash %s", i, eventLog.BlockNumber, eventLog.TxHash.Hex())
		if i >= 2 { // Limit output for example
			break
		}
	}
}

// exampleContractCalls shows how to replace existing contract calls
func exampleContractCalls(ctx context.Context, rpc *rpchelper.RPCHelper) {
	log.Println("=== Contract Calls ===")

	// Example contract call (replace your existing contract instance calls)
	contractAddress := common.HexToAddress("0xA0b86a33E6441E5e4B90bbC44F16aa456D5CC54")

	// Example: Call a view function (like balanceOf, totalSupply, etc.)
	callData := common.Hex2Bytes("18160ddd") // totalSupply() function signature

	callMsg := ethereum.CallMsg{
		To:   &contractAddress,
		Data: callData,
	}

	result, err := rpc.CallContract(ctx, callMsg, nil) // nil = latest block
	if err != nil {
		log.Printf("Error calling contract: %v", err)
		return
	}

	// Convert result to big.Int (assuming totalSupply returns uint256)
	if len(result) == 32 {
		totalSupply := new(big.Int).SetBytes(result)
		log.Printf("Contract total supply: %s", totalSupply.String())
	}

	// Example: Historical contract call using archive nodes
	historicalBlock := big.NewInt(18000000)
	historicalResult, err := rpc.CallContractArchive(ctx, callMsg, historicalBlock)
	if err != nil {
		log.Printf("Error calling contract at historical block: %v", err)
	} else if len(historicalResult) == 32 {
		historicalSupply := new(big.Int).SetBytes(historicalResult)
		log.Printf("Historical total supply at block %s: %s", historicalBlock.String(), historicalSupply.String())
	}
}

// exampleHealthMonitoring shows how to monitor node health
func exampleHealthMonitoring(ctx context.Context, rpc *rpchelper.RPCHelper) {
	log.Println("=== Health Monitoring ===")

	// Check current healthy node count
	healthy, healthyArchive := rpc.GetHealthyNodeCount()
	log.Printf("Healthy nodes: %d regular, %d archive", healthy, healthyArchive)

	// Detailed health check
	checker := rpchelper.NewHealthChecker(rpc)
	regularErrors, archiveErrors := checker.CheckAllNodes(ctx)

	if len(regularErrors) == 0 {
		log.Println("All regular nodes are healthy")
	} else {
		log.Printf("Unhealthy regular nodes: %d", len(regularErrors))
		for url, err := range regularErrors {
			log.Printf("  - %s: %v", url, err)
		}
	}

	if len(archiveErrors) == 0 {
		log.Println("All archive nodes are healthy")
	} else {
		log.Printf("Unhealthy archive nodes: %d", len(archiveErrors))
		for url, err := range archiveErrors {
			log.Printf("  - %s: %v", url, err)
		}
	}
}

// Example of how you might modify your existing processor.go file
func exampleProcessorIntegration() {
	// In your existing processor.go, replace:
	//
	// OLD:
	// logs, err := Client.FilterLogs(context.Background(), filterQuery)
	//
	// NEW:
	// logs, err := rpcHelper.FilterLogs(context.Background(), filterQuery)
	//
	// The interface is the same, but now you get automatic retry and failover!

	log.Println("See comments above for integration example with existing processor.go")
}

// Example of how you might modify your existing contract.go file
func exampleContractIntegration() {
	// In your existing contract.go, replace:
	//
	// OLD:
	// block, err := Client.BlockByNumber(context.Background(), blockNum)
	//
	// NEW:
	// block, err := rpcHelper.BlockByNumber(context.Background(), blockNum)
	//
	// Again, same interface with enhanced reliability!

	log.Println("See comments above for integration example with existing contract.go")
}

// ConfigurationExample shows different ways to configure the RPC helper
func ConfigurationExample() {
	// Option 1: From environment variables (recommended for production)
	// You could read URLs from config files or environment variables

	// Option 2: Multiple providers for maximum reliability
	config := rpchelper.NewRPCConfigFromURLs(
		[]string{
			"https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY",
			"https://mainnet.infura.io/v3/YOUR_KEY",
			"https://rpc.ankr.com/eth",
			"https://eth-mainnet.gateway.pokt.network/v1/YOUR_APP_ID",
		},
		[]string{
			"https://eth-mainnet.g.alchemy.com/v2/YOUR_ARCHIVE_KEY",
		},
	)

	// Option 3: Custom retry settings for high-throughput applications
	config.MaxRetries = 10
	config.RetryDelay = 100 * time.Millisecond
	config.MaxRetryDelay = 120 * time.Second
	config.RequestTimeout = 60 * time.Second

	log.Printf("Configuration example created: %+v", config)
}

// exampleWebhookAlerting shows how to configure and use webhook alerting
func exampleWebhookAlerting(ctx context.Context, rpc *rpchelper.RPCHelper) {
	log.Println("=== Webhook Alerting ===")

	// Example 1: Creating RPC helper with webhook alerts configured
	log.Println("Example 1: RPC Helper with webhook alerts")

	alertConfig := &rpchelper.RPCConfig{
		Nodes: []rpchelper.NodeConfig{
			{URL: "https://eth-mainnet.g.alchemy.com/v2/YOUR_API_KEY"},
			{URL: "https://mainnet.infura.io/v3/YOUR_PROJECT_ID"},
		},
		ArchiveNodes: []rpchelper.NodeConfig{
			{URL: "https://eth-mainnet.g.alchemy.com/v2/YOUR_ARCHIVE_KEY"},
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

	// Create RPC helper with alerting (this would be in your actual application)
	alertRPC := rpchelper.NewRPCHelper(alertConfig)

	// Example 2: Manual alert sending
	log.Println("Example 2: Manual alert sending")

	// Send different types of alerts
	reporting.SendInfoAlert("example-app", "Application started successfully")
	reporting.SendWarningAlert("example-app", "High memory usage detected")
	reporting.SendCriticalAlert("example-app", "Database connection failed")

	// Send alert with custom timestamp
	reporting.SendFailureNotification("example-app", "Custom error occurred",
		time.Now().Format(time.RFC3339), "warning")

	// Example 3: Alerts are automatically sent when nodes fail
	log.Println("Example 3: Automatic alerts on node failures")
	log.Println("- Alerts are sent when nodes become unhealthy")
	log.Println("- Alerts are sent when nodes recover")
	log.Println("- Critical alerts are sent when all nodes fail")

	// Example 4: Monitoring alert channel
	log.Println("Example 4: Alert channel monitoring")

	// Check how many alerts are queued
	channelLen := len(reporting.RpcAlertsChannel)
	log.Printf("Current alerts in queue: %d", channelLen)

	// Example 5: Environment-based configuration
	log.Println("Example 5: Environment-based webhook configuration")
	log.Println("Set WEBHOOK_URL environment variable to configure alerts")
	log.Println("Set WEBHOOK_TIMEOUT environment variable to configure timeout")

	// In practice, you would get these from environment variables:
	// webhookURL := os.Getenv("WEBHOOK_URL")
	// if webhookURL != "" {
	//     config.WebhookConfig = &reporting.WebhookConfig{
	//         URL:     webhookURL,
	//         Timeout: 30 * time.Second,
	//         Retries: 3,
	//     }
	// }

	log.Println("Webhook alerting examples completed")

	// Clean up the example RPC helper
	alertRPC.Close()
}
