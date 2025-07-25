## Examples Package Tests

This directory contains integration tests for the `go-rpc-helper` library.

### `example_integration_test.go`

This file provides an example of how to write an integration test for the `go-rpc-helper` library. It demonstrates how to use a mock HTTP server to simulate an Ethereum RPC endpoint, allowing for self-contained and reliable testing.

#### Test Strategy

The `TestExampleIntegrationWithMockServer` test uses `net/http/httptest` to create a mock server that responds to `eth_blockNumber` RPC calls. This approach allows us to:

1.  **Test Initialization:** Verify that the `rpchelper.Initialize` function works correctly with a responsive mock server.
2.  **Test Basic RPC Calls:** Confirm that the `rpc.BlockNumber` method can successfully make a call to the mock endpoint and parse the response.
3.  **Avoid External Dependencies:** The test does not require a live Ethereum node or API keys, making it fast, reliable, and able to run in any environment.

#### Running the Tests

To run the tests for this package, navigate to the `go-rpc-helper` directory and run:

```sh
go test -v ./examples/...
```
