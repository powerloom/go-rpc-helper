package reporting

import (
	"bytes"
	"crypto/tls"
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

var reportingClient *ReportingService

type ReportingService struct {
	url    string
	client *http.Client
}

func InitializeReportingService(url string, timeout time.Duration) {
	reportingClient = &ReportingService{
		url: url,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
	go StartListening()
}

func StartListening() {
	log.Info("Starting alert processor")
	for issue := range RpcAlertsChannel {
		SendFailureNotificationDirect(issue)
	}
}

// SendFailureNotificationDirect sends the alert directly using the working pattern
func SendFailureNotificationDirect(issue RpcAlert) {
	if reportingClient == nil {
		log.Warn("Reporting client not initialized")
		return
	}

	jsonData, err := json.Marshal(issue)
	if err != nil {
		log.Errorln("Unable to marshal notification: ", issue)
		return
	}

	req, err := http.NewRequest("POST", reportingClient.url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Errorln("Error creating request: ", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := reportingClient.client.Do(req)
	if err != nil {
		log.Errorf("Error sending request for issue %s: %s\n", issue.String(), err)
		return
	}
	defer resp.Body.Close()

	// Here you can handle response or further actions
	log.Debugln("Reporting service response status: ", resp.Status)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Infof("Successfully sent alert: %s", issue.String())
	} else {
		log.Errorf("Failed to send alert, status: %d", resp.StatusCode)
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

	log.Debugf("Sending failure notification: %s", issue.String())

	select {
	case RpcAlertsChannel <- issue:
		log.Debugln("Issue sent to channel: ", issue)
	default:
		log.Errorln("Issue channel is full, dropping issue: ", issue)
	}
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
