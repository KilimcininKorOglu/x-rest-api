package apiv2

import "x-rest-api/internal/xapi"

// DMEventObject renders one direct message into a v2 dm_event of type
// MessageCreate. created_at is passed through as-is, because the DM timestamp is
// not in the tweet created_at format.
func DMEventObject(m xapi.DirectMessage) map[string]any {
	out := map[string]any{
		"id":         m.ID,
		"event_type": "MessageCreate",
		"text":       m.Text,
		"sender_id":  m.SenderID,
	}
	if m.ConversationID != "" {
		out["dm_conversation_id"] = m.ConversationID
	}
	if m.CreatedAt != "" {
		out["created_at"] = m.CreatedAt
	}
	return out
}

// DMEvents flattens an inbox's conversations into a v2 dm_events list, newest
// conversations first as the inbox returns them.
func DMEvents(inbox *xapi.Inbox) []map[string]any {
	out := []map[string]any{}
	for _, conv := range inbox.Conversations {
		for _, m := range conv.Messages {
			out = append(out, DMEventObject(m))
		}
	}
	return out
}
