package handlers

import "encoding/json"

// SendMessageRequest is the expected JSON body for /sendMessage.
// @Description Request to send a message to one or more recipients
type SendMessageRequest struct {
	Message *SendMessageBody `json:"message"`
	Payment json.RawMessage  `json:"payment,omitempty"`
}

// SendMessageBody holds the message fields.
// @Description Message content and delivery details
type SendMessageBody struct {
	Recipient  json.RawMessage `json:"recipient"`
	Recipients json.RawMessage `json:"recipients,omitempty"`
	MessageBox string          `json:"messageBox"`
	MessageID  json.RawMessage `json:"messageId"`
	Body       json.RawMessage `json:"body"`
}

// ListMessagesRequest is the expected JSON body for /listMessages.
// @Description Request to list messages from a message box
type ListMessagesRequest struct {
	MessageBox string `json:"messageBox"`
}

// AcknowledgeMessageRequest is the expected JSON body for /acknowledgeMessage.
// @Description Request to acknowledge (delete) messages
type AcknowledgeMessageRequest struct {
	MessageIDs []string `json:"messageIds"`
}

// RegisterDeviceRequest is the expected JSON body for /registerDevice.
// @Description Request to register a device for push notifications
type RegisterDeviceRequest struct {
	FCMToken string  `json:"fcmToken"`
	DeviceID *string `json:"deviceId,omitempty"`
	Platform *string `json:"platform,omitempty"`
}

// SetPermissionRequest represents the request for setPermission.
// @Description Request to set a permission
type SetPermissionRequest struct {
	Sender       *string `json:"sender,omitempty" example:"03abc..."`
	MessageBox   string  `json:"messageBox" example:"inbox"`
	RecipientFee *int    `json:"recipientFee" example:"100"`
}
