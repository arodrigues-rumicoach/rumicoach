package companion

import (
	"context"
	"fmt"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
)

// The companion tool set covers what a user actually does by message between sessions:
// remember something about them, and act on their commitments. The declarations mirror
// the live engine's (chat/tools.go) but are a separate copy — the live tool table and
// dispatcher are coupled to the Gemini Live WebSocket and must not be imported here.
// Deep exercises (Wheel of Life, the coaching sessions) stay in the app by design.
var companionTools = []Tool{{
	FunctionDeclarations: append([]map[string]any{{
		"name":        "save_memory",
		"description": "Call this tool whenever you extract a significant new insight about the user during the conversation. Use this to remember facts, goals, preferences, or a-ha moments so you have 'Total Recall' in future sessions. Do NOT announce to the user that you are saving a memory. Simply continue the conversation naturally while calling this tool. CRITICAL: The memory text MUST be translated and formulated in the user's preferred language, not necessarily English.",
		"parameters": map[string]any{
			"type": "OBJECT",
			"properties": map[string]any{
				"category": map[string]any{
					"type":        "STRING",
					"description": "The category of the memory. Valid values: 'identity' (personality, essence), 'values' (what they value most, emotional levers), 'needs' (the gap between current state and desired state), 'context' (life stage, narrative details), 'obstacles' (what prevents goal achievement - mindset or skillset), 'insight' (a-ha moments, breakthroughs).",
				},
				"content": map[string]any{
					"type":        "STRING",
					"description": "The specific fact or insight extracted from the conversation, written clearly and concisely in the user's preferred language.",
				},
			},
			"required": []string{"category", "content"},
		},
	}}, commitmentToolDeclarations...),
}}

// executeTool runs one function call and returns the response payload to feed
// back to the model.
func (s *Service) executeTool(ctx context.Context, userID string, call *FunctionCall) map[string]any {
	switch call.Name {
	case "complete_commitment":
		return s.completeCommitment(userID, call.Args)
	case "add_commitment":
		return s.addCommitment(userID, call.Args)
	case "end_commitment":
		return s.endCommitment(userID, call.Args)
	case "save_memory":
		category, _ := call.Args["category"].(string)
		content, _ := call.Args["content"].(string)
		if content == "" {
			return map[string]any{"status": "error", "message": "content is required"}
		}
		if category == "" {
			category = "context"
		}
		memory := models.UserMemory{UserID: userID, Category: category, Content: content}
		if err := database.DB.Create(&memory).Error; err != nil {
			s.logger.Error("companion: failed to save memory", zap.Error(err), zap.String("userID", userID))
			return map[string]any{"status": "error", "message": "failed to save memory"}
		}
		s.logger.Info("companion: memory saved", zap.String("userID", userID), zap.String("category", category))
		return map[string]any{"status": "success"}
	default:
		return map[string]any{"status": "error", "message": fmt.Sprintf("unknown tool %q", call.Name)}
	}
}
