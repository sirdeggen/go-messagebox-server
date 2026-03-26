package handlers

import "encoding/json"

// SendMessageRequest is the expected JSON body for /sendMessage.
// @Description Request to send a message to one or more recipients
type SendMessageRequest struct {
	Message *SendMessageBody `json:"message"`
	Payment *Payment         `json:"payment,omitempty"`
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

// Payment represents the payment transaction data for paid message delivery.
// Contains an Atomic BEEF transaction and output mappings for internalization.
// @Description Payment transaction for message delivery fees
type Payment struct {
	Tx             []byte          `json:"tx"` // Atomic BEEF (BRC-95) encoded transaction
	Outputs        []PaymentOutput `json:"outputs"`
	Description    string          `json:"description,omitempty"`
	Labels         []string        `json:"labels,omitempty"`
	SeekPermission *bool           `json:"seekPermission,omitempty"`
}

// PaymentOutput represents a single output in the payment transaction.
// Specifies how an output should be internalized (as wallet payment or basket insertion).
// @Description Single output mapping for payment internalization
type PaymentOutput struct {
	OutputIndex         uint32               `json:"outputIndex"`
	Protocol            string               `json:"protocol"`                      // "wallet payment" or "basket insertion"
	PaymentRemittance   *PaymentRemittance   `json:"paymentRemittance,omitempty"`   // Required for "wallet payment" protocol
	InsertionRemittance *InsertionRemittance `json:"insertionRemittance,omitempty"` // Required for "basket insertion" protocol
}

// PaymentRemittance contains derivation info for the "wallet payment" protocol.
// Used to derive the key that can unlock the payment output.
// @Description Derivation info for wallet payment outputs
type PaymentRemittance struct {
	DerivationPrefix   string          `json:"derivationPrefix"`  // Base64-encoded derivation prefix
	DerivationSuffix   string          `json:"derivationSuffix"`  // Base64-encoded derivation suffix
	SenderIdentityKey  string          `json:"senderIdentityKey"` // public key hex
	CustomInstructions json.RawMessage `json:"customInstructions,omitempty"`
}

// InsertionRemittance contains basket info for the "basket insertion" protocol.
// Used to insert an output into a specific basket for later retrieval.
// @Description Basket insertion info for output storage
type InsertionRemittance struct {
	Basket             string          `json:"basket"`
	CustomInstructions json.RawMessage `json:"customInstructions,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
}
