package reporting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSendFailureNotification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var alert RpcAlert
		err := json.NewDecoder(r.Body).Decode(&alert)
		assert.NoError(t, err)
		assert.Equal(t, "test-process", alert.ProcessName)
		assert.Equal(t, "test-error", alert.ErrorMsg)
		assert.Equal(t, "critical", alert.Severity)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	InitializeReportingService(server.URL, 5*time.Second)

	SendFailureNotification("test-process", "test-error", time.Now().Format(time.RFC3339), "critical")

	// Allow time for the async function to execute
	time.Sleep(100 * time.Millisecond)
}

func TestSendAlertWithTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var alert RpcAlert
		err := json.NewDecoder(r.Body).Decode(&alert)
		assert.NoError(t, err)
		assert.Equal(t, "test-process", alert.ProcessName)
		assert.Equal(t, "test-error", alert.ErrorMsg)
		assert.Equal(t, "warning", alert.Severity)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	InitializeReportingService(server.URL, 5*time.Second)

	SendWarningAlert("test-process", "test-error")

	// Allow time for the async function to execute
	time.Sleep(100 * time.Millisecond)
}
