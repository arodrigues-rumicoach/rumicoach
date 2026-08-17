package whatsapp

// WebhookEnvelope is the payload Meta POSTs to the webhook. Only the fields
// the companion channel consumes are declared; unknown message types are
// detected via Message.Type and answered with a polite fallback.
type WebhookEnvelope struct {
	Object string  `json:"object"` // "whatsapp_business_account"
	Entry  []Entry `json:"entry"`
}

type Entry struct {
	ID      string   `json:"id"`
	Changes []Change `json:"changes"`
}

type Change struct {
	Field string      `json:"field"` // "messages"
	Value ChangeValue `json:"value"`
}

type ChangeValue struct {
	MessagingProduct string    `json:"messaging_product"`
	Metadata         Metadata  `json:"metadata"`
	Contacts         []Contact `json:"contacts"`
	Messages         []Message `json:"messages"`
	Statuses         []Status  `json:"statuses"`
}

type Metadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type Contact struct {
	WaID    string `json:"wa_id"` // sender phone in E.164 digits
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}

type Message struct {
	From      string `json:"from"` // sender phone in E.164 digits
	ID        string `json:"id"`   // "wamid.…" — the dedupe key
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"` // text|audio|image|sticker|...
	Text      *struct {
		Body string `json:"body"`
	} `json:"text,omitempty"`
	Audio *struct {
		ID       string `json:"id"`
		MimeType string `json:"mime_type"`
		Voice    bool   `json:"voice"`
	} `json:"audio,omitempty"`
}

// Status is a delivery receipt (sent/delivered/read). Acked and dropped in v1.
type Status struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	RecipientID string `json:"recipient_id"`
}
