// Package token provides token counting utilities using tiktoken encoding.
package token

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
)

// Counter handles token counting for text and message arrays.
type Counter struct {
	tiktoken *tiktoken.Tiktoken
}

// NewCounter creates a new token counter with cl100k_base encoding.
func NewCounter() (*Counter, error) {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("failed to get encoding: %w", err)
	}
	return &Counter{tiktoken: enc}, nil
}

// CountTokens counts tokens in a string.
func (c *Counter) CountTokens(text string) (int, error) {
	tokens := c.tiktoken.Encode(text, nil, nil)
	return len(tokens), nil
}

// MessageContent represents a single message in a conversation.
type MessageContent struct {
	Role    string
	Content string
}

// CountMessages counts tokens in a message array.
// Estimates tokens for system prompt + messages with formatting overhead.
func (c *Counter) CountMessages(system string, messages []MessageContent) (int, error) {
	// Base tokens for message formatting
	total := 3 // Start token

	if system != "" {
		sysTokens, err := c.CountTokens(system)
		if err != nil {
			return 0, err
		}
		total += sysTokens + 5 // System prompt overhead
	}

	for _, msg := range messages {
		msgTokens, err := c.CountTokens(msg.Content)
		if err != nil {
			return 0, err
		}
		total += msgTokens + 5 // Per-message overhead
	}

	return total, nil
}
