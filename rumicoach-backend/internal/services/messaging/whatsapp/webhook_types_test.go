package whatsapp

import (
	"encoding/json"
	"testing"
)

// Golden payload shaped after Meta's webhook documentation: one text message,
// one voice note, and a delivery status in a single envelope.
const goldenWebhookPayload = `{
  "object": "whatsapp_business_account",
  "entry": [{
    "id": "102290129340398",
    "changes": [{
      "field": "messages",
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {"display_phone_number": "15550783881", "phone_number_id": "106540352242922"},
        "contacts": [{"profile": {"name": "Maria"}, "wa_id": "351912345678"}],
        "messages": [
          {"from": "351912345678", "id": "wamid.HBgLM==", "timestamp": "1749416383", "type": "text", "text": {"body": "RUMI-7K3M2X"}},
          {"from": "351912345678", "id": "wamid.AUDIO1==", "timestamp": "1749416401", "type": "audio", "audio": {"id": "media-123", "mime_type": "audio/ogg; codecs=opus", "voice": true}}
        ],
        "statuses": [{"id": "wamid.OUT1==", "status": "delivered", "recipient_id": "351912345678"}]
      }
    }]
  }]
}`

func TestWebhookEnvelopeParsing(t *testing.T) {
	var envelope WebhookEnvelope
	if err := json.Unmarshal([]byte(goldenWebhookPayload), &envelope); err != nil {
		t.Fatalf("failed to parse golden payload: %v", err)
	}

	if envelope.Object != "whatsapp_business_account" {
		t.Errorf("object = %q", envelope.Object)
	}
	if len(envelope.Entry) != 1 || len(envelope.Entry[0].Changes) != 1 {
		t.Fatalf("unexpected entry/changes shape")
	}
	value := envelope.Entry[0].Changes[0].Value
	if envelope.Entry[0].Changes[0].Field != "messages" {
		t.Errorf("field = %q", envelope.Entry[0].Changes[0].Field)
	}
	if len(value.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(value.Messages))
	}

	text := value.Messages[0]
	if text.Type != "text" || text.From != "351912345678" || text.ID != "wamid.HBgLM==" {
		t.Errorf("unexpected text message: %+v", text)
	}
	if text.Text == nil || text.Text.Body != "RUMI-7K3M2X" {
		t.Errorf("text body not parsed: %+v", text.Text)
	}

	audio := value.Messages[1]
	if audio.Type != "audio" || audio.Audio == nil {
		t.Fatalf("audio message not parsed: %+v", audio)
	}
	if audio.Audio.ID != "media-123" || !audio.Audio.Voice || audio.Audio.MimeType != "audio/ogg; codecs=opus" {
		t.Errorf("unexpected audio fields: %+v", audio.Audio)
	}

	if len(value.Statuses) != 1 || value.Statuses[0].Status != "delivered" {
		t.Errorf("statuses not parsed: %+v", value.Statuses)
	}
}
