package rpchelper

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// DefaultRPCConfig returns a default configuration for the RPC helper
func DefaultRPCConfig() *RPCConfig {
	return &RPCConfig{
		MaxRetries:     3,
		RetryDelay:     500 * time.Millisecond,
		MaxRetryDelay:  30 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
}

// NewRPCConfigFromURLs creates an RPC config from a list of URLs
func NewRPCConfigFromURLs(urls []string, archiveURLs []string) *RPCConfig {
	config := DefaultRPCConfig()

	for _, url := range urls {
		config.Nodes = append(config.Nodes, NodeConfig{URL: url})
	}

	for _, url := range archiveURLs {
		config.ArchiveNodes = append(config.ArchiveNodes, NodeConfig{URL: url})
	}

	return config
}

// HealthChecker provides utilities for checking node health
type HealthChecker struct {
	rpc *RPCHelper
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(rpcHelper *RPCHelper) *HealthChecker {
	return &HealthChecker{rpc: rpcHelper}
}

// CheckAllNodes checks the health of all nodes
func (h *HealthChecker) CheckAllNodes(ctx context.Context) (map[string]error, map[string]error) {
	regularErrors := make(map[string]error)
	archiveErrors := make(map[string]error)

	h.rpc.nodeMutex.RLock()
	defer h.rpc.nodeMutex.RUnlock()

	// Check regular nodes
	for _, node := range h.rpc.nodes {
		if node.EthClient != nil {
			_, err := node.EthClient.BlockNumber(ctx)
			if err != nil {
				regularErrors[node.URL] = err
			}
		} else {
			regularErrors[node.URL] = fmt.Errorf("eth client not initialized")
		}
	}

	// Check archive nodes
	for _, node := range h.rpc.archiveNodes {
		if node.EthClient != nil {
			_, err := node.EthClient.BlockNumber(ctx)
			if err != nil {
				archiveErrors[node.URL] = err
			}
		} else {
			archiveErrors[node.URL] = fmt.Errorf("eth client not initialized")
		}
	}

	return regularErrors, archiveErrors
}

// Utility functions

// IsValidAddress checks if a string is a valid Ethereum address
func IsValidAddress(addr string) bool {
	if !strings.HasPrefix(addr, "0x") {
		return false
	}

	address := strings.TrimPrefix(addr, "0x")
	if len(address) != 40 {
		return false
	}

	_, err := hex.DecodeString(address)
	return err == nil
}

// ToWei converts Ether to Wei
func ToWei(ether float64) *big.Int {
	weiPerEther := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	etherBig := new(big.Float).SetFloat64(ether)
	weiBig := new(big.Float).Mul(etherBig, new(big.Float).SetInt(weiPerEther))
	wei, _ := weiBig.Int(nil)
	return wei
}

// FromWei converts Wei to Ether
func FromWei(wei *big.Int) float64 {
	weiPerEther := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	weiFloat := new(big.Float).SetInt(wei)
	etherFloat := new(big.Float).Quo(weiFloat, new(big.Float).SetInt(weiPerEther))
	ether, _ := etherFloat.Float64()
	return ether
}

// ParseAddressFromString safely parses an address string
func ParseAddressFromString(addr string) (common.Address, error) {
	if !IsValidAddress(addr) {
		return common.Address{}, fmt.Errorf("invalid address format: %s", addr)
	}
	return common.HexToAddress(addr), nil
}

// FormatAddress formats an address to lowercase hex string
func FormatAddress(addr common.Address) string {
	return strings.ToLower(addr.Hex())
}

// BlockNumberToHex converts a block number to hex string for RPC calls
func BlockNumberToHex(blockNumber *big.Int) string {
	if blockNumber == nil {
		return "latest"
	}
	return fmt.Sprintf("0x%x", blockNumber)
}

// HexToBlockNumber converts a hex string to big.Int block number
func HexToBlockNumber(hex string) (*big.Int, error) {
	if hex == "latest" || hex == "pending" || hex == "earliest" {
		return nil, fmt.Errorf("special block identifiers not supported: %s", hex)
	}

	if !strings.HasPrefix(hex, "0x") {
		return nil, fmt.Errorf("hex string must start with 0x")
	}

	blockNum := new(big.Int)
	blockNum.SetString(hex[2:], 16)
	return blockNum, nil
}
