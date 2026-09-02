package xapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Grok is x.com's built-in assistant. A chat runs against a conversation created
// via the CreateGrokConversation GraphQL op, then messages are exchanged over the
// REST add_response endpoint, which replies with newline-delimited JSON chunks.

const (
	grokAddResponseURL  = "https://grok.x.com/2/grok/add_response.json"
	grokAddResponsePath = "/2/grok/add_response.json"
)

// GrokMessage is one turn in a Grok chat. Role is "user" or "assistant".
type GrokMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GrokRateLimit reports that Grok declined the request because the account hit its
// usage quota (a Premium upsell). When set, Message holds the upsell text.
type GrokRateLimit struct {
	IsRateLimited bool   `json:"is_rate_limited"`
	Message       string `json:"message"`
	UsageLimit    string `json:"usage_limit,omitempty"`
}

// GrokChatResponse carries Grok's combined reply plus the running message history.
type GrokChatResponse struct {
	ConversationID string         `json:"conversation_id"`
	Message        string         `json:"message"`
	Messages       []GrokMessage  `json:"messages"`
	WebResults     []any          `json:"web_results,omitempty"`
	RateLimit      *GrokRateLimit `json:"rate_limit,omitempty"`
}

// GrokChat exchanges messages with Grok. An empty conversationID creates a new
// conversation first. It returns the combined assistant reply and the appended
// history; a quota block surfaces in RateLimit rather than as an error.
func (c *XClient) GrokChat(messages []GrokMessage, conversationID string, returnSearchResults, returnCitations bool) (*GrokChatResponse, error) {
	if conversationID == "" {
		cid, err := c.createGrokConversation()
		if err != nil {
			return nil, err
		}
		conversationID = cid
	}
	payload := map[string]any{
		"responses":            grokResponses(messages),
		"systemPromptName":     "",
		"grokModelOptionId":    "grok-3-latest",
		"modelMode":            "MODEL_MODE_FAST",
		"conversationId":       conversationID,
		"returnSearchResults":  returnSearchResults,
		"returnCitations":      returnCitations,
		"promptMetadata":       map[string]any{"promptSource": "NATURAL", "action": "INPUT"},
		"imageGenerationCount": 4,
		"requestFeatures":      map[string]any{"eagerTweets": true, "serverHistory": true},
		"enableSideBySide":     true,
		"toolOverrides":        map[string]any{},
		"modelConfigOverride":  map[string]any{},
		"isTemporaryChat":      false,
	}
	body, err := c.callJSONRaw("GrokAddResponse", grokAddResponseURL, grokAddResponsePath, "text/plain;charset=UTF-8", payload)
	if err != nil {
		return nil, err
	}
	return parseGrokResponse(body, conversationID, messages), nil
}

// createGrokConversation opens a new Grok conversation and returns its id.
func (c *XClient) createGrokConversation() (string, error) {
	payload, err := c.call("CreateGrokConversation", map[string]any{})
	if err != nil {
		return "", err
	}
	cid := asString(dig(payload, "data", "create_grok_conversation", "conversation_id"))
	if cid == "" {
		return "", fmt.Errorf("CreateGrokConversation: no conversation_id in response")
	}
	return cid, nil
}

// grokResponses converts the chat history to Grok's wire format (sender 1=user,
// 2=assistant); user turns carry empty prompt-source and attachment fields.
func grokResponses(messages []GrokMessage) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		r := map[string]any{"message": m.Content, "sender": grokSender(m.Role)}
		if m.Role != "assistant" {
			r["promptSource"] = ""
			r["fileAttachments"] = []any{}
		}
		out = append(out, r)
	}
	return out
}

func grokSender(role string) int {
	if role == "assistant" {
		return 2
	}
	return 1
}

// parseGrokResponse splits the newline-delimited reply into chunks and combines
// their result.message fields, or surfaces a quota block from the first chunk.
func parseGrokResponse(body []byte, conversationID string, history []GrokMessage) *GrokChatResponse {
	chunks := grokChunks(body)
	resp := &GrokChatResponse{ConversationID: conversationID, Messages: history}
	if len(chunks) == 0 {
		return resp
	}
	for _, ch := range chunks {
		if rl := grokRateLimit(asMap(ch["result"])); rl != nil {
			resp.Message = rl.Message
			resp.RateLimit = rl
			resp.Messages = append(history, GrokMessage{Role: "assistant", Content: rl.Message})
			return resp
		}
	}
	var sb strings.Builder
	for _, ch := range chunks {
		rt := asMap(ch["result"])
		// Skip reasoning chunks (isThinking) so the reply holds only the answer.
		if !asBool(rt["isThinking"]) {
			sb.WriteString(asString(rt["message"]))
		}
		if resp.WebResults == nil {
			if wr, ok := rt["webResults"].([]any); ok {
				resp.WebResults = wr
			}
		}
	}
	resp.Message = sb.String()
	resp.Messages = append(history, GrokMessage{Role: "assistant", Content: sb.String()})
	return resp
}

// grokChunks parses each non-empty newline-delimited line as a JSON object,
// skipping any line that does not decode.
func grokChunks(body []byte) []map[string]any {
	var chunks []map[string]any
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if json.Unmarshal(line, &m) == nil {
			chunks = append(chunks, m)
		}
	}
	return chunks
}

// grokRateLimit returns a rate-limit reply when result.responseType is "limiter",
// otherwise nil.
func grokRateLimit(rt map[string]any) *GrokRateLimit {
	if asString(rt["responseType"]) != "limiter" {
		return nil
	}
	rl := &GrokRateLimit{IsRateLimited: true, Message: asString(rt["message"])}
	if up := asMap(rt["upsell"]); up != nil {
		rl.UsageLimit = asString(up["usageLimit"])
	}
	return rl
}
