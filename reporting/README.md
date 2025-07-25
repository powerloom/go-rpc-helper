## Reporting Package Tests

This directory contains the unit tests for the `reporting` package.

### `alerts_test.go`

This file tests the alerting functionality in `alerts.go`.

#### Test Strategy

The tests use a mock HTTP server (`httptest`) to simulate a webhook endpoint. This allows us to test the following without making actual network calls:

1.  **Alert Formatting:** Verifies that the `RpcAlert` struct is correctly marshaled into JSON.
2.  **Request Generation:** Ensures that the HTTP request to the webhook is created with the correct method, headers, and body.
3.  **Asynchronous Sending:** Confirms that the alerting functions, which run in a separate goroutine, correctly send the data to the mock server.

#### Running the Tests

To run the tests for this package, navigate to the `go-rpc-helper` directory and run:

```sh
go test -v ./reporting/...
```
