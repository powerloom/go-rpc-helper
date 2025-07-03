package rpchelper

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	log "github.com/sirupsen/logrus"

	"github.com/powerloom/rpc-helper/reporting"
)

// NodeConfig represents configuration for a single RPC node
type NodeConfig struct {
	URL string `json:"url"`
}

// RPCConfig represents the configuration for the RPC helper
type RPCConfig struct {
	Nodes          []NodeConfig             `json:"nodes"`
	ArchiveNodes   []NodeConfig             `json:"archive_nodes,omitempty"`
	MaxRetries     int                      `json:"max_retries"`
	RetryDelay     time.Duration            `json:"retry_delay"`
	MaxRetryDelay  time.Duration            `json:"max_retry_delay"`
	RequestTimeout time.Duration            `json:"request_timeout"`
	WebhookConfig  *reporting.WebhookConfig `json:"webhook_config,omitempty"`
}

// RPCNode represents an RPC node with its client and metadata
type RPCNode struct {
	URL       string
	EthClient *ethclient.Client
	RPCClient *rpc.Client
	IsHealthy bool
	LastError error
	LastUsed  time.Time
}

// RPCHelper is the main RPC client wrapper
type RPCHelper struct {
	config         *RPCConfig
	nodes          []*RPCNode
	archiveNodes   []*RPCNode
	currentNodeIdx int
	nodeMutex      sync.RWMutex
	logger         *log.Logger
	initialized    bool
	alertManager   *reporting.AlertManager
}

// RPCException represents an RPC error with detailed information
type RPCException struct {
	Request         interface{} `json:"request"`
	Response        interface{} `json:"response"`
	UnderlyingError error       `json:"underlying_error"`
	ExtraInfo       string      `json:"extra_info"`
	NodeURL         string      `json:"node_url"`
}

func (e *RPCException) Error() string {
	return fmt.Sprintf("RPC Error: %s | Node: %s | Underlying: %v",
		e.ExtraInfo, e.NodeURL, e.UnderlyingError)
}

// NewRPCHelper creates a new RPC helper instance
func NewRPCHelper(config *RPCConfig) *RPCHelper {
	rpcHelper := &RPCHelper{
		config:         config,
		nodes:          make([]*RPCNode, 0),
		archiveNodes:   make([]*RPCNode, 0),
		currentNodeIdx: 0,
		logger:         log.New(),
		initialized:    false,
	}

	// Initialize AlertManager if webhook configuration is provided
	if config.WebhookConfig != nil {
		rpcHelper.alertManager = reporting.NewAlertManager(*config.WebhookConfig)
	}

	return rpcHelper
}

// Initialize sets up the RPC clients for all configured nodes
func (r *RPCHelper) Initialize(ctx context.Context) error {
	r.nodeMutex.Lock()
	defer r.nodeMutex.Unlock()

	if r.initialized {
		return nil
	}

	// Start alert processor if AlertManager is configured
	if r.alertManager != nil {
		r.alertManager.StartAlertProcessor(ctx)
	}

	// Initialize regular nodes
	for _, nodeConfig := range r.config.Nodes {
		node, err := r.createNode(ctx, nodeConfig.URL)
		if err != nil {
			r.logger.Warnf("Failed to initialize node %s: %v", nodeConfig.URL, err)
			// Add node anyway for potential retry later
			node = &RPCNode{
				URL:       nodeConfig.URL,
				IsHealthy: false,
				LastError: err,
			}
		}
		r.nodes = append(r.nodes, node)
	}

	// Initialize archive nodes
	for _, nodeConfig := range r.config.ArchiveNodes {
		node, err := r.createNode(ctx, nodeConfig.URL)
		if err != nil {
			r.logger.Warnf("Failed to initialize archive node %s: %v", nodeConfig.URL, err)
			node = &RPCNode{
				URL:       nodeConfig.URL,
				IsHealthy: false,
				LastError: err,
			}
		}
		r.archiveNodes = append(r.archiveNodes, node)
	}

	if len(r.nodes) == 0 {
		if r.alertManager != nil {
			reporting.SendCriticalAlert("rpc-helper", "No RPC nodes available after initialization")
		}
		return fmt.Errorf("no RPC nodes available")
	}

	r.initialized = true
	r.logger.Infof("RPC helper initialized with %d nodes and %d archive nodes",
		len(r.nodes), len(r.archiveNodes))
	return nil
}

// createNode creates an RPC node with clients
func (r *RPCHelper) createNode(ctx context.Context, url string) (*RPCNode, error) {
	// Create HTTP client with insecure TLS config (skip certificate verification)
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: r.config.RequestTimeout,
	}

	// Create RPC client with custom HTTP client
	rpcClient, err := rpc.DialOptions(ctx, url, rpc.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RPC: %w", err)
	}

	// Create eth client
	ethClient := ethclient.NewClient(rpcClient)

	// Test connection
	_, err = ethClient.BlockNumber(ctx)
	if err != nil {
		rpcClient.Close()
		return nil, fmt.Errorf("failed to test connection: %w", err)
	}

	return &RPCNode{
		URL:       url,
		EthClient: ethClient,
		RPCClient: rpcClient,
		IsHealthy: true,
		LastUsed:  time.Now(),
	}, nil
}

// getCurrentNode returns the current healthy node, with fallback logic
// Always tries the first node first, then tries subsequent nodes in order
// Periodically retries the primary node even when marked unhealthy
func (r *RPCHelper) getCurrentNode(useArchive bool) (*RPCNode, error) {
	r.nodeMutex.Lock()
	defer r.nodeMutex.Unlock()

	nodes := r.nodes
	if useArchive && len(r.archiveNodes) > 0 {
		nodes = r.archiveNodes
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}

	primaryNode := nodes[0]

	// Always try the first node (primary) if it's healthy
	if primaryNode.IsHealthy {
		r.currentNodeIdx = 0
		return primaryNode, nil
	}

	// If primary node is unhealthy, occasionally retry it (every 10 requests)
	r.currentNodeIdx++
	if r.currentNodeIdx%10 == 0 {
		r.logger.Infof("Periodic retry attempt for primary node %s", primaryNode.URL)
		return primaryNode, nil
	}

	// Otherwise, try subsequent nodes in order
	for i := 1; i < len(nodes); i++ {
		if nodes[i].IsHealthy {
			return nodes[i], nil
		}
	}

	// If all nodes are unhealthy, still return the first node to attempt retry
	return primaryNode, nil
}

// switchToNode marks a node as healthy and updates its usage timestamp
func (r *RPCHelper) switchToNode(nodeURL string, useArchive bool) {
	r.nodeMutex.Lock()
	defer r.nodeMutex.Unlock()

	nodes := r.nodes
	if useArchive && len(r.archiveNodes) > 0 {
		nodes = r.archiveNodes
	}

	for _, node := range nodes {
		if node.URL == nodeURL {
			wasUnhealthy := !node.IsHealthy
			node.IsHealthy = true
			node.LastUsed = time.Now()
			r.logger.Infof("Node %s is now healthy and active", nodeURL)

			// Send alert for node recovery if it was previously unhealthy
			if wasUnhealthy && r.alertManager != nil {
				nodeType := "regular"
				if useArchive {
					nodeType = "archive"
				}
				reporting.SendInfoAlert("rpc-helper",
					fmt.Sprintf("Node %s (%s) has recovered and is now healthy", nodeURL, nodeType))
			}

			return
		}
	}
}

// markNodeUnhealthy marks a node as unhealthy
func (r *RPCHelper) markNodeUnhealthy(nodeURL string, err error) {
	r.nodeMutex.Lock()
	defer r.nodeMutex.Unlock()

	for _, node := range r.nodes {
		if node.URL == nodeURL {
			wasHealthy := node.IsHealthy
			node.IsHealthy = false
			node.LastError = err
			r.logger.Warnf("Marked node unhealthy: %s, error: %v", nodeURL, err)

			// Send alert for node failure if it was previously healthy
			if wasHealthy && r.alertManager != nil {
				reporting.SendWarningAlert("rpc-helper",
					fmt.Sprintf("Full node %s has become unhealthy: %v", nodeURL, err))
			}

			return
		}
	}

	for _, node := range r.archiveNodes {
		if node.URL == nodeURL {
			wasHealthy := node.IsHealthy
			node.IsHealthy = false
			node.LastError = err
			r.logger.Warnf("Marked archive node unhealthy: %s, error: %v", nodeURL, err)

			// Send alert for archive node failure if it was previously healthy
			if wasHealthy && r.alertManager != nil {
				reporting.SendWarningAlert("rpc-helper",
					fmt.Sprintf("Archive node %s has become unhealthy: %v", nodeURL, err))
			}

			return
		}
	}
}

// executeWithRetryAndFailover executes a function with retry logic and node failover
func (r *RPCHelper) executeWithRetryAndFailover(ctx context.Context, operation func(*RPCNode) (interface{}, error), useArchive bool) (interface{}, error) {
	var lastErr error
	var lastResult interface{}
	nodes := r.nodes
	if useArchive && len(r.archiveNodes) > 0 {
		nodes = r.archiveNodes
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}

	// Try each node with retries
	for nodeAttempt := 0; nodeAttempt < len(nodes); nodeAttempt++ {
		node, err := r.getCurrentNode(useArchive)
		if err != nil {
			return nil, err
		}

		// Create backoff strategy for this node
		backoffStrategy := backoff.NewExponentialBackOff()
		backoffStrategy.MaxElapsedTime = r.config.MaxRetryDelay
		backoffStrategy.InitialInterval = r.config.RetryDelay

		// Retry operation on current node
		retryOperation := func() error {
			result, err := operation(node)
			if err != nil {
				lastErr = err
				return err
			}
			lastResult = result
			return nil
		}

		retryCtx, cancel := context.WithTimeout(ctx, r.config.RequestTimeout)
		err = backoff.Retry(retryOperation, backoff.WithContext(backoffStrategy, retryCtx))
		cancel()

		if err == nil {
			// Success! Mark node as healthy and update last used time
			r.switchToNode(node.URL, useArchive)
			return lastResult, nil
		}

		// Mark current node as unhealthy and try next
		r.markNodeUnhealthy(node.URL, err)
		r.logger.Warnf("Node %s failed after retries: %v", node.URL, err)
		lastErr = err
	}

	// Check if all nodes are unhealthy and log critical error
	healthyNodes, healthyArchiveNodes := r.GetHealthyNodeCount()
	if healthyNodes == 0 && (!useArchive || healthyArchiveNodes == 0) {
		nodeType := "regular"
		if useArchive {
			nodeType = "archive"
		}

		// Send critical alert for all nodes being unhealthy
		if r.alertManager != nil {
			reporting.SendCriticalAlert("rpc-helper",
				fmt.Sprintf("All %s nodes are unhealthy! This is a critical issue.", nodeType))
		}

		r.logger.Errorf("All %s nodes are unhealthy! This is a critical issue.", nodeType)
	}

	return nil, fmt.Errorf("all nodes failed, last error: %w", lastErr)
}

// BlockByNumber retrieves a block by number
func (r *RPCHelper) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.BlockByNumber(ctx, number)
	}, false)

	if err != nil {
		return nil, err
	}
	return result.(*types.Block), nil
}

// BlockNumber returns the current block number
func (r *RPCHelper) BlockNumber(ctx context.Context) (uint64, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.BlockNumber(ctx)
	}, false)

	if err != nil {
		return 0, err
	}
	return result.(uint64), nil
}

// HeaderByNumber returns a block header by number
func (r *RPCHelper) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.HeaderByNumber(ctx, number)
	}, false)

	if err != nil {
		return nil, err
	}
	return result.(*types.Header), nil
}

// FilterLogs executes a filter query
func (r *RPCHelper) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.FilterLogs(ctx, query)
	}, false)

	if err != nil {
		return nil, err
	}
	return result.([]types.Log), nil
}

// TransactionByHash returns the transaction for the given hash
func (r *RPCHelper) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		tx, isPending, err := node.EthClient.TransactionByHash(ctx, hash)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"tx":        tx,
			"isPending": isPending,
		}, nil
	}, false)

	if err != nil {
		return nil, false, err
	}

	resultMap := result.(map[string]interface{})
	return resultMap["tx"].(*types.Transaction), resultMap["isPending"].(bool), nil
}

// TransactionReceipt returns the receipt of a transaction by transaction hash
func (r *RPCHelper) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.TransactionReceipt(ctx, txHash)
	}, false)

	if err != nil {
		return nil, err
	}
	return result.(*types.Receipt), nil
}

// CallContract executes a message call transaction
func (r *RPCHelper) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.CallContract(ctx, msg, blockNumber)
	}, false)

	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// CallContractArchive executes a message call transaction using archive nodes
func (r *RPCHelper) CallContractArchive(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.CallContract(ctx, msg, blockNumber)
	}, true)

	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// JSONRPCCall makes a raw JSON-RPC call
func (r *RPCHelper) JSONRPCCall(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	result, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		var response json.RawMessage
		err := node.RPCClient.CallContext(ctx, &response, method, params...)
		return response, err
	}, false)

	if err != nil {
		return nil, err
	}
	return result.(json.RawMessage), nil
}

// BatchJSONRPCCall makes batch JSON-RPC calls
func (r *RPCHelper) BatchJSONRPCCall(ctx context.Context, requests []rpc.BatchElem) error {
	_, err := r.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return nil, node.RPCClient.BatchCallContext(ctx, requests)
	}, false)

	return err
}

// GetHealthyNodeCount returns the number of healthy nodes
func (r *RPCHelper) GetHealthyNodeCount() (int, int) {
	r.nodeMutex.RLock()
	defer r.nodeMutex.RUnlock()

	healthyNodes := 0
	for _, node := range r.nodes {
		if node.IsHealthy {
			healthyNodes++
		}
	}

	healthyArchiveNodes := 0
	for _, node := range r.archiveNodes {
		if node.IsHealthy {
			healthyArchiveNodes++
		}
	}

	return healthyNodes, healthyArchiveNodes
}

// Close closes all RPC connections
func (r *RPCHelper) Close() {
	r.nodeMutex.Lock()
	defer r.nodeMutex.Unlock()

	for _, node := range r.nodes {
		if node.RPCClient != nil {
			node.RPCClient.Close()
		}
	}

	for _, node := range r.archiveNodes {
		if node.RPCClient != nil {
			node.RPCClient.Close()
		}
	}

	r.logger.Info("RPC helper closed")
}

// ContractBackend implements bind.ContractBackend interface using RPC helper
type ContractBackend struct {
	rpcHelper *RPCHelper
}

// NewContractBackend creates a new ContractBackend that uses the RPC helper
func (r *RPCHelper) NewContractBackend() *ContractBackend {
	return &ContractBackend{
		rpcHelper: r,
	}
}

// CallContract implements bind.ContractCaller interface
func (cb *ContractBackend) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	// Use regular call for non-archive data
	return cb.rpcHelper.CallContract(ctx, call, blockNumber)
}

// PendingCallContract implements bind.ContractCaller interface
func (cb *ContractBackend) PendingCallContract(ctx context.Context, call ethereum.CallMsg) ([]byte, error) {
	// Call with nil block number for pending
	return cb.rpcHelper.CallContract(ctx, call, nil)
}

// CodeAt implements bind.ContractCaller interface
func (cb *ContractBackend) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	result, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.CodeAt(ctx, account, blockNumber)
	}, false)
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// HeaderByNumber implements bind.ContractCaller interface
func (cb *ContractBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	result, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.HeaderByNumber(ctx, number)
	}, false)
	if err != nil {
		return nil, err
	}
	return result.(*types.Header), nil
}

// PendingCodeAt implements bind.ContractCaller interface
func (cb *ContractBackend) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	result, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.PendingCodeAt(ctx, account)
	}, false)
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// PendingNonceAt implements bind.ContractTransactor interface
func (cb *ContractBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	result, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.PendingNonceAt(ctx, account)
	}, false)
	if err != nil {
		return 0, err
	}
	return result.(uint64), nil
}

// SuggestGasPrice implements bind.ContractTransactor interface
func (cb *ContractBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	result, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.SuggestGasPrice(ctx)
	}, false)
	if err != nil {
		return nil, err
	}
	return result.(*big.Int), nil
}

// SuggestGasTipCap implements bind.ContractTransactor interface
func (cb *ContractBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	result, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.SuggestGasTipCap(ctx)
	}, false)
	if err != nil {
		return nil, err
	}
	return result.(*big.Int), nil
}

// EstimateGas implements bind.ContractTransactor interface
func (cb *ContractBackend) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	result, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return node.EthClient.EstimateGas(ctx, call)
	}, false)
	if err != nil {
		return 0, err
	}
	return result.(uint64), nil
}

// SendTransaction implements bind.ContractTransactor interface
func (cb *ContractBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	_, err := cb.rpcHelper.executeWithRetryAndFailover(ctx, func(node *RPCNode) (interface{}, error) {
		return nil, node.EthClient.SendTransaction(ctx, tx)
	}, false)
	return err
}

// FilterLogs implements bind.ContractFilterer interface
func (cb *ContractBackend) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return cb.rpcHelper.FilterLogs(ctx, query)
}

// SubscribeFilterLogs implements bind.ContractFilterer interface
func (cb *ContractBackend) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	// Use the first available healthy node for subscription
	node, err := cb.rpcHelper.getCurrentNode(false)
	if err != nil {
		return nil, &RPCException{
			Request:         query,
			UnderlyingError: err,
			ExtraInfo:       "failed to get healthy node for subscription",
		}
	}

	return node.EthClient.SubscribeFilterLogs(ctx, query, ch)
}
