package connector

import (
	"context"
	"io"
	"net/http"
)

// IncomingMessage is the platform-neutral message consumed by connector workers.
// Platform SDK payloads must be translated into this type at the adapter boundary.
type IncomingMessage struct {
	Platform          string
	ExternalMessageID string
	ExternalChatID    string
	ExternalUserID    string
	ExternalUnionID   string
	ExternalTenantKey string
	ExternalName      string
	ChatType          string
	MessageType       string
	Text              string
	MentionedBot      bool
	MentionOpenIDs    []string
}

type MessageTarget struct {
	ChatID string
}

type OutgoingMessage struct {
	Text     string
	Card     map[string]any
	Image    io.Reader
	File     io.Reader
	FileName string
	MIMEType string
}

type EventRequest struct {
	Body    []byte
	Headers http.Header
}

type ParsedEvent struct {
	Challenge string
	Message   *IncomingMessage
	EventID   string
}

type SendResult struct {
	ExternalMessageID string
}

// PlatformConnector is the stable boundary implemented by Feishu now and by
// DingTalk/WeCom adapters in later phases.
type PlatformConnector interface {
	VerifyEvent(context.Context, EventRequest) ([]byte, error)
	ParseEvent(context.Context, []byte) (ParsedEvent, error)
	SendText(context.Context, MessageTarget, OutgoingMessage) (SendResult, error)
	SendImage(context.Context, MessageTarget, OutgoingMessage) (SendResult, error)
	SendFile(context.Context, MessageTarget, OutgoingMessage) (SendResult, error)
	SendCard(context.Context, MessageTarget, OutgoingMessage) (SendResult, error)
}
