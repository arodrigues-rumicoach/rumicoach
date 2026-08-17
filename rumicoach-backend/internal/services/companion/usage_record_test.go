package companion

import (
	"context"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/aiusage"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/internal/services/messaging"
)

// The token totals for a turn must survive every way the message log can be
// erased, so they are accumulated at the source: every Gemini call in a turn
// folds its usageMetadata in. Recording only the final response — what the code
// used to do — undercounted every turn that used a tool.
func TestUsageAccumulation(t *testing.T) {
	var u Usage

	first := &Response{}
	first.UsageMetadata.PromptTokenCount = 100
	first.UsageMetadata.CandidatesTokenCount = 20
	first.UsageMetadata.TotalTokenCount = 150 // thinking tokens included
	u.AddResponse(first)

	// A model/endpoint that omits totalTokenCount must not zero the total.
	second := &Response{}
	second.UsageMetadata.PromptTokenCount = 200
	second.UsageMetadata.CandidatesTokenCount = 30
	u.AddResponse(second)

	u.Add(Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}) // e.g. transcription

	if u.InputTokens != 310 || u.OutputTokens != 55 || u.TotalTokens != 395 {
		t.Errorf("accumulated usage = %+v, want {310 55 395}", u)
	}
}

// Every outbound send leaves its spend in the cost ledger, and the row must survive
// the message being purged — ai_usage_records is never touched by any content erasure,
// while channel_messages can go three different ways.
func TestRecordOutboundWritesUsageRecord(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	long := time.Now().Add(-48 * time.Hour)
	addIntegration(t, "i1", &long)
	var integration models.Integration
	if err := database.DB.First(&integration, "id = 'i1'").Error; err != nil {
		t.Fatalf("load integration: %v", err)
	}

	s.recordOutbound(&integration, models.ChannelMessageTypeText, "olá", nil,
		models.ChannelMessageSent, 7, Spend{Chat: Usage{InputTokens: 40, OutputTokens: 12, TotalTokens: 60}})

	var cost models.AIUsageRecord
	if err := database.DB.First(&cost).Error; err != nil {
		t.Fatalf("no AI cost record written: %v", err)
	}
	if cost.UserID != "u1" || cost.Kind != models.AIUsageMessage {
		t.Errorf("cost identity = %s/%s, want u1/message", cost.UserID, cost.Kind)
	}
	if cost.InputTokens != 40 || cost.OutputTokens != 12 || cost.TotalTokens != 60 {
		t.Errorf("cost tokens = %d/%d/%d, want 40/12/60", cost.InputTokens, cost.OutputTokens, cost.TotalTokens)
	}
	if cost.RefID == nil {
		t.Error("cost record should reference the outbound message it metered")
	}

	// The reference is informational only: purging the message must leave the ledger intact.
	if err := database.DB.Where("1 = 1").Delete(&models.ChannelMessage{}).Error; err != nil {
		t.Fatalf("purge messages: %v", err)
	}
	var count int64
	database.DB.Model(&models.AIUsageRecord{}).Count(&count)
	if count != 1 {
		t.Errorf("cost records after message purge = %d, want 1", count)
	}
}

// recordUsage is what the inbound path calls at each terminal state; the row it
// writes is the durable trace that the turn happened and what it cost.
func TestRecordUsageInboundRow(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	long := time.Now().Add(-48 * time.Hour)
	addIntegration(t, "i1", &long)
	var integration models.Integration
	if err := database.DB.First(&integration, "id = 'i1'").Error; err != nil {
		t.Fatalf("load integration: %v", err)
	}

	msgID := "m-in-1"
	s.recordUsage(&integration, &msgID,
		Spend{
			Chat: Usage{InputTokens: 500, OutputTokens: 80, TotalTokens: 640},
			STT:  Usage{InputTokens: 300, OutputTokens: 10, TotalTokens: 310},
		})

	// The turn's spend, in the cost ledger, pointing back at the message it paid for.
	var cost models.AIUsageRecord
	if err := database.DB.First(&cost).Error; err != nil {
		t.Fatalf("no AI cost record written: %v", err)
	}
	if cost.TotalTokens != 640 {
		t.Errorf("cost total tokens = %d, want 640", cost.TotalTokens)
	}
	if cost.RefType == nil || *cost.RefType != models.AIUsageRefMessage ||
		cost.RefID == nil || *cost.RefID != msgID {
		t.Errorf("cost reference = %v/%v, want channel_message/%s", cost.RefType, cost.RefID, msgID)
	}

	// The whole point of the split: transcription runs on its own model at its own
	// price, so its tokens are recorded apart from the chat turn's rather than added
	// in. Merged, the ledger could only price the sum as chat.
	if cost.STTModel != transcribeModel() {
		t.Errorf("STT model = %q, want %q", cost.STTModel, transcribeModel())
	}
	if cost.STTInputTokens != 300 || cost.STTOutputTokens != 10 || cost.STTTotalTokens != 310 {
		t.Errorf("STT tokens = %d/%d/%d, want 300/10/310",
			cost.STTInputTokens, cost.STTOutputTokens, cost.STTTotalTokens)
	}
	// The chat columns must carry only the chat turn — transcription must not have
	// leaked into them.
	if cost.InputTokens != 500 || cost.OutputTokens != 80 {
		t.Errorf("chat tokens = %d/%d, want 500/80 (STT leaked in?)", cost.InputTokens, cost.OutputTokens)
	}
	// Unused groups stay empty rather than naming a model that earned nothing.
	if cost.TTSModel != "" || cost.TTSTotalTokens != 0 {
		t.Errorf("TTS group should be empty on a text reply, got %q/%d", cost.TTSModel, cost.TTSTotalTokens)
	}
}

// A voice note costs three models. The cost has to be the sum of all three priced
// separately — the bug this replaced charged the whole turn at the chat rate, which
// understates synthesis ($20.00 per million output tokens against $9.00) and
// overstates nothing to compensate.
func TestRecordUsagePricesEachModelSeparately(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	long := time.Now().Add(-48 * time.Hour)
	addIntegration(t, "i1", &long)
	var integration models.Integration
	if err := database.DB.First(&integration, "id = 'i1'").Error; err != nil {
		t.Fatalf("load integration: %v", err)
	}

	msgID := "m-voice-1"
	spend := Spend{
		Chat: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
		STT:  Usage{InputTokens: 1_000_000},
		TTS:  Usage{OutputTokens: 1_000_000},
	}
	s.recordUsage(&integration, &msgID, spend)

	var cost models.AIUsageRecord
	if err := database.DB.First(&cost).Error; err != nil {
		t.Fatalf("no AI cost record written: %v", err)
	}
	if cost.CostMicros == nil {
		t.Fatal("cost not computed; are all three models priced in aiusage/prices.go?")
	}

	// A million tokens of each, so each rate lands in the total as itself. Derived
	// from the price table rather than written out, because the rates move — the
	// launch price of the current chat model expires 2026-12-31 — and this test is
	// about the three models being priced separately, not about what they cost.
	chat, okChat := aiusage.PriceFor(chatModel())
	stt, okSTT := aiusage.PriceFor(transcribeModel())
	tts, okTTS := aiusage.PriceFor(ttsModel())
	if !okChat || !okSTT || !okTTS {
		t.Fatalf("a default model is unpriced in aiusage/prices.go: chat=%v stt=%v tts=%v",
			okChat, okSTT, okTTS)
	}

	want := chat.InputTextMicrosPerMTok + chat.OutputTextMicrosPerMTok +
		stt.InputTextMicrosPerMTok + tts.OutputTextMicrosPerMTok
	if *cost.CostMicros != want {
		t.Errorf("cost = %d micro-dollars, want %d", *cost.CostMicros, want)
	}

	// Priced as one model, the transcription's input and the synthesis's output would
	// both be charged at the chat model's rates — the understatement this test exists
	// to prevent. Synthesis output is the expensive half ($20.00 per million against
	// the chat model's few dollars), so the single-model figure is always the lower one.
	singleModel := 2*chat.InputTextMicrosPerMTok + 2*chat.OutputTextMicrosPerMTok
	if *cost.CostMicros <= singleModel {
		t.Errorf("cost %d is at or below the single-model figure %d; the split is not being priced",
			*cost.CostMicros, singleModel)
	}
}

// Only a reply is charged. What we send on our own initiative — the daily nudge, a
// re-engagement template, the daily-cap notice — is our idea, and billing someone for
// a message they did not ask for is indefensible. deliverReply tells the two apart by
// whether it was handed the user's message.
func TestOnlyRepliesAreCharged(t *testing.T) {
	setupNotificationTestDB(t)
	s := newToolService()

	recent := time.Now().Add(-time.Hour)
	addIntegration(t, "i1", &recent)
	var integration models.Integration
	if err := database.DB.First(&integration, "id = 'i1'").Error; err != nil {
		t.Fatalf("load integration: %v", err)
	}
	user := models.User{ID: "u1"}

	// Our own initiative: no inbound message behind it.
	s.deliverReply(t.Context(), mockChannel{}, &integration, &user, "a nudge", nil, Spend{})
	if n := balanceTxCount(t); n != 0 {
		t.Errorf("a proactive message produced %d charges, want 0", n)
	}

	// A reply to something the user sent.
	inbound := &models.ChannelMessage{ID: "m-in-1", Type: models.ChannelMessageTypeText}
	s.deliverReply(t.Context(), mockChannel{}, &integration, &user, "an answer", inbound, Spend{})

	var txs []models.BalanceTransaction
	database.DB.Find(&txs)
	if len(txs) != 1 {
		t.Fatalf("a reply produced %d charges, want 1", len(txs))
	}
	if txs[0].Type != models.BalanceTxMessageUsage || txs[0].AmountSeconds != -balance.MessageCostSeconds {
		t.Errorf("charge = %s/%d, want message_usage/%d",
			txs[0].Type, txs[0].AmountSeconds, -balance.MessageCostSeconds)
	}
	// Keyed to the message being answered, so a redelivered inbound that slipped past
	// the webhook dedupe cannot be charged twice.
	if txs[0].ReferenceID == nil || *txs[0].ReferenceID != "message:m-in-1" {
		t.Errorf("reference = %v, want message:m-in-1", txs[0].ReferenceID)
	}

	// Answering the same inbound message again is already paid for.
	s.deliverReply(t.Context(), mockChannel{}, &integration, &user, "again", inbound, Spend{})
	if n := balanceTxCount(t); n != 1 {
		t.Errorf("answering the same message twice produced %d charges, want 1", n)
	}
}

// mockChannel accepts every send. The charging rule is about WHICH message we sent,
// not whether the provider liked it, so the transport is stubbed out.
type mockChannel struct{}

func (mockChannel) Provider() string { return "telegram" }
func (mockChannel) SendText(context.Context, messaging.Address, string) (string, error) {
	return "provider-msg-1", nil
}
func (mockChannel) SendAudio(context.Context, messaging.Address, []byte, string, bool) (string, error) {
	return "provider-msg-1", nil
}
func (mockChannel) SendTemplate(context.Context, messaging.Address, messaging.TemplateMessage) (string, error) {
	return "provider-msg-1", nil
}
func (mockChannel) DownloadMedia(context.Context, string) ([]byte, string, error) {
	return nil, "", nil
}

func balanceTxCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	database.DB.Model(&models.BalanceTransaction{}).Count(&n)
	return n
}
