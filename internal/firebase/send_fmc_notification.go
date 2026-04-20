package firebase

import (
	"context"
	"fmt"
	"time"

	"firebase.google.com/go/v4/messaging"
	"github.com/bsv-blockchain/go-message-box-server/pkg/db"
	"github.com/bsv-blockchain/go-message-box-server/internal/logger"
)

var DEVICE_SEND_MESSAGE_TIMEOUT = 5 * time.Second

// FCMPayload contains the notification data to send.
type FCMPayload struct {
	Title      string
	MessageID  string
	Originator string
}

// SendFCMNotificationResult contains the result of a send operation.
type SendFCMNotificationResult struct {
	Success bool
	Error   string
}

// SendFCMNotification pushes notification to all registered devices for a recipient.
// Looks up FCM tokens from device_registrations table and sends to all active devices.
func SendFCMNotification(database *db.DB, recipient string, payload FCMPayload) *SendFCMNotificationResult {
	if !IsEnabled() {
		return &SendFCMNotificationResult{Success: false, Error: "FCM not configured"}
	}

	logger.Log("[DEBUG] Attempting to send FCM notification to", "recipient", recipient)
	logger.Log("[DEBUG] Payload", "payload", payload)

	devices, err := database.ListActiveDevices(recipient)
	if err != nil {
		logger.Error("[FCM] Failed to get devices", "error", err, "recipient", recipient)
		return &SendFCMNotificationResult{Success: false, Error: fmt.Sprintf("failed to get devices: %v", err)}
	}

	if len(devices) == 0 {
		logger.Log("[FCM] No active devices found", "recipient", recipient)
		return &SendFCMNotificationResult{Success: false, Error: "No registered devices found for recipient"}
	}

	logger.Log("[FCM] Sending notifications", "recipient", recipient, "deviceCount", len(devices))

	var successCount, failureCount int

	for _, device := range devices {
		msg := buildMessage(device.FCMToken, payload)

		ctx, cancel := context.WithTimeout(context.Background(), DEVICE_SEND_MESSAGE_TIMEOUT)
		_, err := Client().Send(ctx, msg)
		cancel()

		if err != nil {
			logger.Error("[FCM] Failed to send", "error", err, "tokenSuffix", lastN(device.FCMToken, 10))
			failureCount++

			// we only mark devices as disabled when token is invalid
			if isInvalidTokenError(err) {
				logger.Log("[FCM] Deactivating invalid token", "tokenSuffix", lastN(device.FCMToken, 10))
				if err := database.DeactivateDevice(device.FCMToken); err != nil {
					logger.Error("[FCM] Failed to deactivate device", "error", err)
				}
			}
			continue
		}

		successCount++
		logger.Log("[FCM] Notification sent", "tokenSuffix", lastN(device.FCMToken, 10))

		if err := database.UpdateDeviceLastUsed(device.FCMToken); err != nil {
			logger.Error("[FCM] Failed to update last_used", "error", err)
		}
	}

	logger.Log("[FCM] Send complete", "success", successCount, "failed", failureCount)

	// if not a single device received a notification consider it a fail
	// otherwise we consider it a success
	if successCount == 0 {
		return &SendFCMNotificationResult{Success: false, Error: fmt.Sprintf("failed to send to all %d registered devices", len(devices))}
	}

	return &SendFCMNotificationResult{Success: true}
}

func buildMessage(token string, payload FCMPayload) *messaging.Message {
	return &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: payload.Title,
			Body:  payload.MessageID,
		},
		// Android configuration for headless service
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Data: map[string]string{
				"messageId":  payload.MessageID,
				"originator": payload.Originator,
			},
		},
		// iOs configuration for mutable content and Notification Service Extension
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-push-type": "alert", // required for iOS 13+
				"apns-priority":  "10",    // deliver immediately
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					MutableContent: true,
					Alert: &messaging.ApsAlert{ // include an alert so NSE can modify it
						Title: payload.Title,
						Body:  payload.MessageID,
					},
				},
				CustomData: map[string]interface{}{
					"messageId":  payload.MessageID,
					"originator": payload.Originator,
				},
			},
		},
	}
}

func isInvalidTokenError(err error) bool {
	return messaging.IsUnregistered(err) || messaging.IsInvalidArgument(err)
}

// lastN returns the last n characters of a string (for logging tokens safely)
// UTF-8 compatible.
func lastN(s string, n int) string {
	if n <= 0 {
		return ""
	}

	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[len(r)-n:])
}
