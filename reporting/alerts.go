package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// RpcAlert represents an alert message to be sent to a webhook
//
//	{
//	 "timestamp": "Example text",
//	 "process_name": "Example text",
//	 "error_msg": "Example text",
//	 "severity": "Example text"
//	}
type RpcAlert struct {
	ProcessName string `json:"process_name"`
	ErrorMsg    string `json:"error_msg"`
	Timestamp   string `json:"timestamp"`
	Severity    string `json:"severity"`
}

func (s RpcAlert) String() string {
	return fmt.Sprintf("ProcessName: %s, ErrorMsg: %s, Timestamp: %s, Severity: %s",
		s.ProcessName, s.ErrorMsg, s.Timestamp, s.Severity)
}

// These issues should not be common but best to allow high volumes
var RpcAlertsChannel = make(chan RpcAlert, 10000)

// WebhookConfig holds configuration for webhook alerts
type WebhookConfig struct {
	URL     string
	Timeout time.Duration
	Retries int
}

// AlertManager manages webhook alert sending
type AlertManager struct {
	config     WebhookConfig
	httpClient *http.Client
}

// NewAlertManager creates a new AlertManager with the given webhook configuration
func NewAlertManager(config WebhookConfig) *AlertManager {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Retries == 0 {
		config.Retries = 3
	}

	return &AlertManager{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// SendFailureNotification sends a failure notification to the alerts channel
func SendFailureNotification(processName, errorMsg, timestamp, severity string) {
	issue := RpcAlert{
		processName,
		errorMsg,
		timestamp,
		severity,
	}

	select {
	case RpcAlertsChannel <- issue:
		log.Debugln("Issue sent to channel: ", issue)
	default:
		log.Errorln("Issue channel is full, dropping issue: ", issue)
	}
}

// SendToWebhook sends an alert to the configured webhook endpoint
func (am *AlertManager) SendToWebhook(ctx context.Context, alert RpcAlert) error {
	if am.config.URL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	jsonData, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert to JSON: %w", err)
	}

	var lastErr error
	for i := 0; i < am.config.Retries; i++ {
		if i > 0 {
			log.Warnf("Retrying webhook send (attempt %d/%d)", i+1, am.config.Retries)
			time.Sleep(time.Duration(i) * time.Second) // Simple backoff
		}

		req, err := http.NewRequestWithContext(ctx, "POST", am.config.URL, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "go-rpc-helper/1.0")

		resp, err := am.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			log.Infof("Successfully sent alert to webhook: %s", alert.String())
			return nil
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	return fmt.Errorf("failed to send alert after %d retries: %w", am.config.Retries, lastErr)
}

// StartAlertProcessor starts a goroutine that processes alerts from the channel and sends them to webhook
func (am *AlertManager) StartAlertProcessor(ctx context.Context) {
	go func() {
		log.Info("Starting alert processor")
		for {
			select {
			case alert := <-RpcAlertsChannel:
				log.Debugf("Processing alert: %s", alert.String())
				if err := am.SendToWebhook(ctx, alert); err != nil {
					log.Errorf("Failed to send alert to webhook: %v", err)
				}
			case <-ctx.Done():
				log.Info("Alert processor stopped")
				return
			}
		}
	}()
}

// SendAlertWithTimestamp is a convenience function that sends an alert with the current timestamp
func SendAlertWithTimestamp(processName, errorMsg, severity string) {
	timestamp := time.Now().Format(time.RFC3339)
	SendFailureNotification(processName, errorMsg, timestamp, severity)
}

// SendCriticalAlert sends a critical severity alert
func SendCriticalAlert(processName, errorMsg string) {
	SendAlertWithTimestamp(processName, errorMsg, "critical")
}

// SendWarningAlert sends a warning severity alert
func SendWarningAlert(processName, errorMsg string) {
	SendAlertWithTimestamp(processName, errorMsg, "warning")
}

// SendInfoAlert sends an info severity alert
func SendInfoAlert(processName, errorMsg string) {
	SendAlertWithTimestamp(processName, errorMsg, "info")
}
