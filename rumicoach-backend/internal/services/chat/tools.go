package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/notification"
	"github.com/rumi/rumi-be/internal/services/quote"
	"go.uber.org/zap"
)

// --- Tool Declarations (sent to Gemini in setup message) ---

var (
	toolSetWheelOfLifeCategories = map[string]interface{}{
		"name":        "set_wheel_of_life_categories",
		"description": "Call this tool as soon as you propose the initial categories to the user, and every time the categories are updated during discussion. This initialization is required before you can score any area. You do NOT need to wait for final confirmation to call this for visual updates; however, you will need to call complete_current_task() separately to move to the next stage.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"category_names": map[string]interface{}{
					"type":        "ARRAY",
					"items":       map[string]interface{}{"type": "STRING"},
					"description": "The FULL list of category names for the wheel, written in the USER'S PREFERRED LANGUAGE (translate them — never pass English names if the user speaks another language). The default areas (translate each one) are: ['Health & Energy', 'Relationships', 'Purpose & Career', 'Finances & Lifestyle', 'Wellbeing & Growth'] — in Portuguese: 'Saúde e Energia', 'Relacionamentos', 'Propósito e Carreira', 'Finanças e Lifestyle', 'Bem-estar e Crescimento' (keep 'lifestyle' as-is where it is commonly borrowed). If the user asks to add a new category, you MUST include ALL existing categories PLUS the new category in this array. If the user asks to remove a category, pass the full list WITHOUT it. CRITICAL: You MUST call this tool IMMEDIATELY in the same turn the user asks to add or remove a category. DO NOT postpone it.",
				},
			},
			"required": []string{"category_names"},
		},
	}

	toolUpdateWheelOfLife = map[string]interface{}{
		"name":        "update_wheel_of_life",
		"description": "Call this tool in the EXACT SAME turn as you provide your empathetic validation. Do not wait for a separate confirmation turn, and do not split the response into multiple turns. Call it immediately when the user shares their score. You MUST STOP GENERATING IMMEDIATELY and yield control to the system after calling this tool. Do NOT ask the next question in the same turn.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"category": map[string]interface{}{
					"type":        "STRING",
					"description": "The area name being updated, copied VERBATIM from the wheel's current areas (the names you created with set_wheel_of_life_categories, shown in EXISTING SCORES and in the area cue). It is in the user's language — e.g. 'Saúde' or 'Bem-estar e Crescimento', NOT the English label. It MUST match an existing area name exactly.",
				},
				"score": map[string]interface{}{
					"type":        "INTEGER",
					"description": "The score chosen by the user (1 to 10).",
				},
				"reasoning": map[string]interface{}{
					"type":        "STRING",
					"description": "What the user said sits behind this score, in their own words. WRITE IT TO THE USER, NOT ABOUT THEM: this text is shown back to them on their wheel, so address them directly in the second person and in their language ('Sentes que...', 'You feel that...'). NEVER write about them in the third person and never use their name ('O utilizador sente que...' and 'Filipa feels that...' are both wrong — they read like a stranger's case notes about them). Pass ONLY what they actually voiced; never summarize the coaching process itself and never invent a reason they did not give.",
				},
				"json_data": map[string]interface{}{
					"type":        "STRING",
					"description": "Optional fallback JSON string containing a list of categories to update.",
				},
			},
			"required": []string{"category", "score", "reasoning"},
		},
	}

	toolSaveSessionInsight = map[string]interface{}{
		"name":        "save_session_insight",
		"description": "Call this tool immediately when the user shares the key insight they take away at the end of the session (e.g. in response to 'What is the most important insight you take away from this conversation?'). This saves the insight to the session record for review and also records it as a session insight memory.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"insight": map[string]interface{}{
					"type":        "STRING",
					"description": "The key insight the user shared. CRITICAL: write it in the user's preferred language AND in the user's OWN FIRST-PERSON voice, as if they are saying it to themselves — the app displays it under 'My last insight', so it must read as their words ('Tudo depende apenas de mim — posso começar sem ter todas as respostas', 'I realized I keep waiting for permission'). Never write it in the second or third person, never use their name, and never describe the coaching process — capture THEIR realization in THEIR voice.",
				},
			},
			"required": []string{"insight"},
		},
	}

	toolCompleteCurrentTask = map[string]interface{}{
		"name":        "complete_current_task",
		"description": "Call this tool ONLY when you have naturally and fully completed the conversational goal of your current overarching task to move to the next logical stage of the session. AFTER calling this tool, you MUST STOP SPEAKING and wait for the system to provide new instructions. Do not repeat your last question or summarize the transition.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"current_state": map[string]interface{}{
					"type":        "STRING",
					"description": "The name of the current state/task you are completing (e.g., 'ONBOARDING_MEMORIES'). This ensures the transition is only executed if you are in the correct state.",
				},
			},
			"required": []string{"current_state"},
		},
	}

	toolSaveMemory = map[string]interface{}{
		"name":        "save_memory",
		"description": "Call this tool whenever you extract a significant new insight about the user during the conversation. Use this to remember facts, goals, preferences, or a-ha moments so you have 'Total Recall' in future sessions. Do NOT announce to the user that you are saving a memory. Simply continue the conversation naturally while calling this tool. CRITICAL: The memory text MUST be translated and formulated in the user's preferred language, not necessarily English.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"category": map[string]interface{}{
					"type":        "STRING",
					"description": "The category of the memory. Valid values: 'identity' (personality, essence), 'values' (what they value most, emotional levers), 'needs' (the gap between current state and desired state), 'context' (life stage, narrative details), 'obstacles' (what prevents goal achievement - mindset or skillset), 'insight' (a-ha moments, breakthroughs).",
				},
				"content": map[string]interface{}{
					"type":        "STRING",
					"description": "The specific fact or insight extracted from the conversation, written clearly and concisely in the user's preferred language. WRITE IT TO THE USER, NOT ABOUT THEM: memories are shown back to them on their memories screen, so address them directly in the second person ('Valorizas...', 'Percebeste que...', 'You value...'). NEVER write about them in the third person and never use their name ('A Filipa valoriza...' and 'O utilizador identifica...' are both wrong — they read like a stranger's case notes about them). EXCEPTION — category 'insight': insights are the user's own realizations and the app surfaces the latest one as 'My last insight', so write those in the user's FIRST person ('Percebi que...', 'Tudo depende apenas de mim...'), as if the user is saying it to themselves.",
				},
			},
			"required": []string{"category", "content"},
		},
	}

	toolUpdateEisenhowerMatrix = map[string]interface{}{
		"name":        "update_eisenhower_matrix",
		"description": "Use this tool to categorize the user's tasks into the Eisenhower Matrix quadrants (Urgent/Important) to help with overwhelm. Quadrant values MUST be: 'urgent_important', 'not_urgent_important', 'urgent_not_important', 'not_urgent_not_important'.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"json_data": map[string]interface{}{
					"type":        "STRING",
					"description": "A JSON string containing a flat array of tasks. Example: '[{\"task\": \"...\", \"quadrant\": \"urgent_important\", \"reasoning\": \"...\"}]'",
				},
			},
			"required": []string{"json_data"},
		},
	}

	toolInitEisenhowerMatrix = map[string]interface{}{
		"name":        "init_eisenhower_matrix",
		"description": "Call this tool to start the Eisenhower Matrix (Chaos Sorter) exercise when the user feels overwhelmed or has too many tasks. This will transition the conversation to a specialized task-sorting mode.",
	}

	toolDeleteEisenhowerMatrixTasks = map[string]interface{}{
		"name":        "delete_eisenhower_matrix_tasks",
		"description": "Use this tool to remove specific tasks from the Eisenhower Matrix if the user decides they are no longer relevant or were added by mistake.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"task_names": map[string]interface{}{
					"type":        "ARRAY",
					"items":       map[string]interface{}{"type": "STRING"},
					"description": "List of task names to remove from the matrix.",
				},
			},
			"required": []string{"task_names"},
		},
	}

	toolShowScreen = map[string]interface{}{
		"name":        "show_screen",
		"description": "Call this tool NATIVELY through your function-calling ability to navigate the user to a screen in the application — never write its name or a fake marker as text (e.g. writing '◆tasks' executes nothing and the user sees raw placeholder text). Only the data-less screens 'memories', 'session', 'tasks' (the user's daily plan board), 'journey', and 'profile' may be shown this way; call it only when the user asks to see one of them, or your active task instructions explicitly say to show it — never merely because the conversation touched on a related topic. NEVER use it for the Wheel of Life — that screen is shown only by calling set_wheel_of_life_categories, which populates it with the user's areas. IMPORTANT — THE VALUE IS NOT THE NAME: these are internal identifiers, and two of them differ from what the user sees on their screen. 'growth' is called the JOURNEY screen, and 'session' is the TALK tab. When you speak about either, use the name the user reads in the app, never the identifier you pass here.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"screen_name": map[string]interface{}{
					"type":        "STRING",
					"description": "The name of the screen to show. Valid values: 'memories', 'session', 'tasks', 'journey', 'profile'. These are identifiers, not the labels the user sees: 'growth' is the screen they know as their Journey, and 'session' is the tab labelled Talk. Pass the identifier; say the label.",
				},
			},
			"required": []string{"screen_name"},
		},
	}

	toolSaveIdealLifeVision = map[string]interface{}{
		"name":        "save_ideal_life_vision",
		"description": "Call this tool to save the user's detailed Ideal Life Vision and transition to the next phase. Write the vision in the SECOND person, addressing the user directly in their language ('You imagine a life where...'), never in the third person or with their name — it is shown back to them on screen.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"vision": map[string]interface{}{
					"type":        "STRING",
					"description": "A detailed summary of the user's 3-year ideal life vision.",
				},
			},
			"required": []string{"vision"},
		},
	}

	toolSaveActions = map[string]interface{}{
		"name":        "save_commitments",
		"description": "Call this tool to save the user's commitments and transition to the next phase. The commitments should be a list of concrete commitments that move the user forward in their focus area.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"commitments": map[string]interface{}{
					"type":        "STRING",
					"description": "A JSON string containing an array of commitments. CRITICAL: every 'title' MUST be written in the user's preferred language — titles are shown verbatim on the user's board, and an English title in a Portuguese user's app is a bug; the English titles in the example below show the FORMAT only, never the language. Weekdays for recurring commitments should be specified in the 'days' array where 1 = Monday, 2 = Tuesday, ..., 7 = Sunday. Dates for one-time commitments must be specified in the 'date' field in YYYY-MM-DD format. For recurring commitments ALWAYS set 'end_date' (YYYY-MM-DD): the day the habit stops, agreed with the user (e.g. two to four weeks out). A habit with no horizon runs forever and quietly becomes something the user is failing rather than finishing — a defined end gives them something to complete and revisit. Never set 'end_date' on a one-time commitment; its own date is its end. Example: '[{\"title\": \"Buy training shoes\", \"type\": \"one_time\", \"date\": \"2026-05-31\"}, {\"title\": \"Go to the gym\", \"type\": \"recurring\", \"days\": [1, 3, 5], \"end_date\": \"2026-06-30\"}]'",
				},
			},
			"required": []string{"commitments"},
		},
	}

	toolUpdateCommitmentPlan = map[string]interface{}{
		"name":        "update_commitment_plan",
		"description": "Call this tool to dynamically add or update commitments in the user's commitment plan as they are discussed. Do NOT wait for the final commitment to call this tool. For recurring commitments, specify the weekdays in the 'days' array where 1 = Monday, 2 = Tuesday, ..., 7 = Sunday. For one-time commitments, specify the date in the 'date' field in YYYY-MM-DD format.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"commitments": map[string]interface{}{
					"type":        "STRING",
					"description": "A JSON string containing the full array of current commitments. CRITICAL: every 'title' MUST be written in the user's preferred language — titles are shown verbatim on the user's board; the English titles in the example below show the FORMAT only, never the language. For recurring commitments ALWAYS set 'end_date' (YYYY-MM-DD): the day the habit stops, agreed with the user (e.g. two to four weeks out). A habit with no horizon runs forever and quietly becomes something the user is failing rather than finishing — a defined end gives them something to complete and revisit. Never set 'end_date' on a one-time commitment; its own date is its end. Example: '[{\"title\": \"Buy training shoes\", \"type\": \"one_time\", \"date\": \"2026-05-31\"}, {\"title\": \"Go to the gym\", \"type\": \"recurring\", \"days\": [1, 3, 5], \"end_date\": \"2026-06-30\"}]'",
				},
			},
			"required": []string{"commitments"},
		},
	}

	toolAddCommitment = map[string]interface{}{
		"name":        "add_commitment",
		"description": "Add one or more standalone commitments the user wants to track (e.g. when, during a check-in, the user asks to add something to their list). For recurring commitments specify the weekdays in 'days' (1 = Monday, ..., 7 = Sunday); for one-time commitments specify 'date' in YYYY-MM-DD format.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"commitments": map[string]interface{}{
					"type":        "STRING",
					"description": "A JSON string with an array of commitments to add. CRITICAL: every 'title' MUST be written in the user's preferred language — titles are shown verbatim on the user's board, and an English title in a Portuguese user's app is a bug; the English titles in the example below show the FORMAT only, never the language. For recurring commitments ALWAYS set 'end_date' (YYYY-MM-DD): the day the habit stops, agreed with the user (e.g. two to four weeks out). A habit with no horizon runs forever and quietly becomes something the user is failing rather than finishing — a defined end gives them something to complete and revisit. Never set 'end_date' on a one-time commitment; its own date is its end. Example: '[{\"title\": \"Drink more water\", \"type\": \"recurring\", \"days\": [1,2,3,4,5], \"end_date\": \"2026-07-31\"}, {\"title\": \"Call my mother\", \"type\": \"one_time\", \"date\": \"2026-07-01\"}]'",
				},
			},
			"required": []string{"commitments"},
		},
	}

	toolRemoveCommitment = map[string]interface{}{
		"name":        "remove_commitment",
		"description": "Remove a commitment from the user's board — when the user asks for one to be taken off ('remove that', 'pode retirar'), or when you saved one they never actually agreed to. Only commitments created during THIS session can be removed. Confirm the removal warmly and naturally, never mentioning any tool.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type":        "STRING",
					"description": "The commitment's title, as it appears on the user's board (the exact title you passed when saving it).",
				},
			},
			"required": []string{"title"},
		},
	}

	toolSaveTopValues = map[string]interface{}{
		"name":        "save_top_values",
		"description": "Call this in the Values session once the user has CHOSEN their top values (after reflecting the candidates back and asking which three matter most) — never before they have chosen, and never with values they did not name. The values appear on the user's screen the moment you save them and are carried into every future session. Safe to call again: a new call REPLACES the previous set.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"values": map[string]interface{}{
					"type":        "ARRAY",
					"items":       map[string]interface{}{"type": "STRING"},
					"description": "The user's chosen top values (usually three), each ONE short word or phrase in the user's preferred language, exactly as they chose them (e.g. [\"Crescimento\", \"Amor\", \"Família\"]). Never full sentences.",
				},
			},
			"required": []string{"values"},
		},
	}

	toolSaveFocus = map[string]interface{}{
		"name":        "save_focus",
		"description": "Call this tool ONLY during the METAPHOR phase of the onboarding session, AFTER the user has named the priority area they want to focus on AND explained why it matters to them. Pass the Wheel of Life area name to 'area' — the stored, translated area name in the user's language. Call it SILENTLY — do NOT output any spoken text in the same turn. It automatically advances the session to the closing reflection.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"area": map[string]interface{}{
					"type":        "STRING",
					"description": "The Wheel of Life area the user committed to work on (e.g., 'Health', 'Finances').",
				},
			},
			"required": []string{"area"},
		},
	}

	toolSaveVisionCommitment = map[string]interface{}{
		"name":        "save_vision_commitment",
		"description": "Call this tool SILENTLY, right after the user has actually named the one thing they could do right away to move closer to their priority area (Step 4 of the emotional closing) — do NOT announce that you are saving anything. CRITICAL: only call this AFTER the user has replied to that question, in the turn where you process their reply — never in the same turn where you ASK the question, and never with a guess, placeholder, or 'the user did not say' in place of their real answer; if they have not answered yet, do not call this tool at all. Write the commitment as ONE clean, concrete phrase that reads well on their screen — polish the WORDING, never the SUBSTANCE: keep exactly the step they chose, but drop speech fillers and hesitations ('maybe', 'I guess', 'um'), and never add steps, scope, dates, or details they did not say. If their answer was hedged between several options ('maybe eat better or exercise more'), do NOT save it yet — ask which one feels like their first step, and save the one they pick. Safe to call more than once: each call replaces the last, so correct yourself with another call if you captured the wrong thing.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"commitment": map[string]interface{}{
					"type":        "STRING",
					"description": "The one step the user chose, written as a single clean, concrete commitment phrase in their preferred language (e.g. 'Take a 10-minute walk every morning') — their substance, polished wording, no hesitation words or alternatives.",
				},
				"recurring": map[string]interface{}{
					"type":        "BOOLEAN",
					"description": "Set to true when the step is an ONGOING intention the user means to keep doing ('go to bed by 22h30', 'walk every morning', 'no phone at dinner') — it is then saved as a daily habit on their board. Leave it out (or false) for a genuinely one-off step done once ('book the appointment', 'send that message'). Saving an ongoing intention as a one-off makes it appear as a single task for today and then vanish, which is not what the user promised.",
				},
			},
			"required": []string{"commitment"},
		},
	}

	toolSaveIdentityReflection = map[string]interface{}{
		"name":        "save_identity_reflection",
		"description": "Call this tool SILENTLY at the end of the Identity session's synthesis (PHASE 10), right after the user has CONFIRMED your identity statement — never before their confirmation, and never with guesses in place of what they actually said. It captures the structured Identity Reflection the app shows on the session-end card and in future sessions. CRITICAL: every field must be written in the user's preferred language and in the user's OWN FIRST-PERSON voice, as if they are describing themselves ('Someone who handles everything alone' / 'Alguém que resolve tudo sozinha') — never about them in the third person, never their name. Safe to call again: a new call REPLACES the previous capture, so correct yourself if the user refines the statement.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"learned_identity": map[string]interface{}{
					"type":        "STRING",
					"description": "WHO THEY LEARNED TO BE — the identity built by their history, one short phrase (e.g. 'Someone who handles everything alone').",
				},
				"what_it_gave": map[string]interface{}{
					"type":        "STRING",
					"description": "WHAT THAT WAY OF BEING GAVE THEM — its protective function or gift, one short phrase (e.g. 'Independence and resilience').",
				},
				"what_it_costs": map[string]interface{}{
					"type":        "STRING",
					"description": "WHAT IT SOMETIMES COSTS THEM — the price they became aware of, one short phrase (e.g. 'Difficulty asking for help and letting others support me').",
				},
				"who_becoming": map[string]interface{}{
					"type":        "STRING",
					"description": "WHO THEY WANT TO KEEP BECOMING — the chosen identity, keeping the earlier positive quality (e.g. 'Independent enough to stand on my own, but secure enough to let others in').",
				},
				"qualities": map[string]interface{}{
					"type":        "ARRAY",
					"items":       map[string]interface{}{"type": "STRING"},
					"description": "The TWO or THREE qualities they chose to strengthen (e.g. ['Openness', 'Courage', 'Trust']), each a single word or very short phrase in their language.",
				},
				"evidence": map[string]interface{}{
					"type":        "STRING",
					"description": "Their next piece of evidence — the small concrete choice from PHASE 9, if they made one (e.g. 'Ask for help once this week instead of doing everything myself'). OMIT this field entirely if they chose observation over action; never invent one.",
				},
			},
			"required": []string{"learned_identity", "what_it_gave", "what_it_costs", "who_becoming", "qualities"},
		},
	}

	toolSaveAcceptanceReflection = map[string]interface{}{
		"name":        "save_acceptance_reflection",
		"description": "Call this tool SILENTLY at the end of the Acceptance session's synthesis (PHASE 9), right after the user has CONFIRMED your reflection ('You expected X, the reality is Y...') — never before their confirmation, and never with guesses in place of what they actually said. It captures the structured Acceptance Reflection the app shows on the session-end card. CRITICAL: every field must be written in the user's preferred language and in the user's OWN FIRST-PERSON voice ('I expected my partner to understand what I needed without me having to ask') — never about them in the third person, never their name. Safe to call again: a new call REPLACES the previous capture, so correct yourself if the user refines the reflection.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"expected": map[string]interface{}{
					"type":        "STRING",
					"description": "WHAT THEY EXPECTED — the expectation behind the situation, one short phrase (e.g. 'My partner would understand what I needed without me having to ask').",
				},
				"reality": map[string]interface{}{
					"type":        "STRING",
					"description": "WHAT IS TRUE RIGHT NOW — the reality as they recognized it, one short phrase (e.g. 'We communicate differently and some of my needs haven't been expressed clearly').",
				},
				"cannot_control": map[string]interface{}{
					"type":        "STRING",
					"description": "WHAT THEY CANNOT CONTROL — the part they recognized does not depend on them (e.g. 'How they interpret everything I say').",
				},
				"can_influence": map[string]interface{}{
					"type":        "STRING",
					"description": "WHAT THEY CAN INFLUENCE — the part still in their hands (e.g. 'How clearly I communicate and the boundaries I set').",
				},
				"choose_to_accept": map[string]interface{}{
					"type":        "STRING",
					"description": "WHAT THEY CHOOSE TO ACCEPT (e.g. 'I can't control another person's response').",
				},
				"where_i_act": map[string]interface{}{
					"type":        "STRING",
					"description": "WHERE THEY CHOOSE TO ACT (e.g. 'Say clearly what I need instead of waiting for it to be understood').",
				},
				"next_step": map[string]interface{}{
					"type":        "STRING",
					"description": "Their next step — the PHASE 8 commitment, if they made a concrete one (e.g. 'Have the conversation I've been postponing'). OMIT this field entirely if they chose to wait, observe, or do nothing for now; never invent one.",
				},
			},
			"required": []string{"expected", "reality", "cannot_control", "can_influence", "choose_to_accept", "where_i_act"},
		},
	}

	toolSaveProfileDetails = map[string]interface{}{
		"name":        "save_profile_details",
		"description": "Call this during the onboarding intro once the user has given you their country, date of birth and gender, to complete their registration. You may call it more than once as the details come in — pass whatever you have so far and call it again with the rest; already-saved values are never overwritten with empty ones. Call it SILENTLY: never announce that you are saving anything, and never read the values back as a list.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"country_code": map[string]interface{}{
					"type":        "STRING",
					"description": "The ISO 3166-1 alpha-2 code of the country the user lives in, uppercase (e.g. 'PT' for Portugal, 'BR' for Brazil, 'US' for the United States). ALWAYS convert the country the user names — in whatever language they said it — into this code. Never pass the spoken country name.",
				},
				"date_of_birth": map[string]interface{}{
					"type":        "STRING",
					"description": "The user's date of birth as 'YYYY-MM-DD'. Convert whatever format they say into this one (e.g. '3rd of May 1990' → '1990-05-03'). If they give only a year, do not guess the rest — ask for the full date instead.",
				},
				"gender": map[string]interface{}{
					"type":        "STRING",
					"description": "The user's gender, exactly 'male' or 'female' in lowercase (this drives grammatical agreement in gendered languages). If the user declines to say or does not identify with either, omit this field entirely rather than guessing.",
				},
			},
		},
	}

	toolSaveBehaviorPlan = map[string]interface{}{
		"name":        "save_behavior_plan",
		"description": "Call this at the end of the Behavior Change Protocol, once the user has designed a behavior WITH you: the smallest version of the behavior, why it matters, the identity it serves, and the trigger/context it attaches to. Also call it to UPDATE an existing plan (adjusting after a check-in, or changing its status): pass the same 'behavior' name and the fields to change. Call it silently — never announce that you are saving. All text arguments in the user's preferred language.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"behavior": map[string]interface{}{
					"type":        "STRING",
					"description": "The smallest sustainable version of the behavior, in the user's words (e.g. 'walk 10 minutes after lunch').",
				},
				"identity": map[string]interface{}{
					"type":        "STRING",
					"description": "The identity this behavior is a vote for, co-created with the user (e.g. 'someone who protects their health').",
				},
				"motive": map[string]interface{}{
					"type":        "STRING",
					"description": "Why this matters to the user and the values it serves, in their words.",
				},
				"trigger": map[string]interface{}{
					"type":        "STRING",
					"description": "The existing anchor the behavior attaches to (e.g. 'right after my morning coffee').",
				},
				"context": map[string]interface{}{
					"type":        "STRING",
					"description": "When / where / with whom / how they will remember.",
				},
				"frequency": map[string]interface{}{
					"type":        "STRING",
					"description": "Frequency in the user's words (e.g. 'every morning', '3 times a week').",
				},
				"days": map[string]interface{}{
					"type":        "STRING",
					"description": "JSON array of ISO weekdays the behavior happens (1 = Monday ... 7 = Sunday), e.g. '[1,3,5]'. Use [1,2,3,4,5,6,7] for daily.",
				},
				"obstacles": map[string]interface{}{
					"type":        "STRING",
					"description": "What could make this hard, as the user anticipated it.",
				},
				"plan_b": map[string]interface{}{
					"type":        "STRING",
					"description": "The fallback version for hard days, agreed with the user.",
				},
				"area": map[string]interface{}{
					"type":        "STRING",
					"description": "Optional: the Wheel of Life area this behavior serves, exactly as named on the user's wheel.",
				},
				"status": map[string]interface{}{
					"type":        "STRING",
					"description": "Optional, for updates only: 'active', 'parked' (user not ready now — never framed as failure), 'graduated' (behavior became part of who they are), or 'archived'.",
				},
			},
			"required": []string{"behavior"},
		},
	}

	toolLogBehaviorCheckin = map[string]interface{}{
		"name":        "log_behavior_checkin",
		"description": "Call this silently whenever the user reports on an existing behavior commitment — in a follow-up you initiated or spontaneously ('I managed to walk every day', 'I couldn't keep it up this week'). Records the datapoint on the plan. A 'missed' is information for adjusting the plan, NEVER a failure — never let the user conclude they failed.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"behavior": map[string]interface{}{
					"type":        "STRING",
					"description": "The behavior of the plan being checked in, matching the plan's name.",
				},
				"status": map[string]interface{}{
					"type":        "STRING",
					"description": "'kept' (they did it), 'partial' (some of it / plan B), or 'missed'.",
				},
				"note": map[string]interface{}{
					"type":        "STRING",
					"description": "What the user said about how it went — what helped, or what made it harder (their words, their language).",
				},
			},
			"required": []string{"behavior", "status"},
		},
	}

	toolTerminateSession = map[string]interface{}{
		"name":        "terminate_session",
		"description": "Call this tool in the EXACT SAME turn as you say your final closing sentence (the equivalent of 'See you soon.' in the user's language). Do not wait for a subsequent user message or a separate turn. You must call it immediately when providing the final outro response to stop the conversation loop.",
	}

	toolStartPlannedSession = map[string]interface{}{
		"name":        "start_planned_session",
		"description": "Call this when the user, during a daily check-in, agrees to begin the session that is planned for them today. Do NOT speak in the same turn — calling this hands the conversation over to the planned session. Only call it after the user has explicitly accepted starting the planned session.",
	}

	toolRestartSessionWithSummary = map[string]interface{}{
		"name":        "restart_session_with_summary",
		"description": "Call this tool to save the summary of the conversation before a connection restart. AFTER calling this tool, you MUST STOP SPEAKING and wait for the system to restart the connection.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"summary": map[string]interface{}{
					"type":        "STRING",
					"description": "A detailed summary of the entire conversation so far (user profile details, goals, feelings, state, categories scored, and progress). Do not lose any relevant information.",
				},
			},
			"required": []string{"summary"},
		},
	}

	toolRequestRecommendations = map[string]interface{}{
		"name":        "request_recommendations",
		"description": "Call this tool when the user asks for resource recommendations (such as books, articles, videos, podcasts, tools, etc.) or when you decide to send them. This triggers a grounded background search and emails the results directly to the user. Speak to the user explaining that you are compiling and sending them to their email.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"topic":        map[string]interface{}{"type": "STRING", "description": "The general theme of the recommendations (e.g. 'Time Management')"},
				"search_query": map[string]interface{}{"type": "STRING", "description": "A search engine query optimized to retrieve the requested resources (e.g. 'best books and videos on time management for working moms')"},
				"context":      map[string]interface{}{"type": "STRING", "description": "Details about the user's specific challenges, goals, or preferences to personalize the reasoning"},
			},
			"required": []string{"topic", "search_query", "context"},
		},
	}

	toolScheduleNotifications = map[string]interface{}{
		"name":        "schedule_notifications",
		"description": "Call this tool ONLY when instructed by a [SYSTEM COMMAND] at the end of the session to generate and schedule personalized notifications, and to pick the Journey screen's quote category and visual theme for the coming days. WHEN each message arrives is as much a coaching decision as what it says: place each one where it will actually help, in the user's own local time, around what you know is happening in their life. A message that arrives at the wrong moment — asking how something went before it has happened, or landing at 3am — does more harm than sending nothing. The server decides the delivery channel (messaging app or push) when each notification comes due.",
		"parameters": map[string]interface{}{
			"type": "OBJECT",
			"properties": map[string]interface{}{
				"quote_category": map[string]interface{}{
					"type":        "STRING",
					"enum":        quote.CategoryNames(),
					"description": "The quote category that best fits what the user needs in the coming days, based on this conversation and their current state. Daily quotes will be drawn from this category. Meanings: 'commitment' = taking the first step, momentum, doing instead of overthinking; 'growth' = learning, progress, becoming better over time; 'mindset' = perspective, self-talk, how thinking shapes experience; 'purpose' = meaning, direction, values, reconnecting with their why; 'resilience' = getting through setbacks, hard times, persistence; 'wisdom' = reflection, insight, seeing life with more depth.",
				},
				"theme": map[string]interface{}{
					"type":        "STRING",
					"enum":        models.JourneyThemes,
					"description": "The visual theme for the user's Journey screen over the coming days, matching the emotional tone of this conversation. Tones: 'lavender' = gentleness, self-care, softness after emotional work; 'fireplace' = warmth, comfort, feeling safe and held; 'mountain_lake' = clarity, stillness, gaining perspective; 'rain' = introspection, processing feelings, quiet reflection; 'sunset_beach' = calm optimism, gratitude, peaceful closure; 'waterfall' = energy, momentum, flow, a fresh start.",
				},
				"notifications": map[string]interface{}{
					"type": "ARRAY",
					"items": map[string]interface{}{
						"type": "OBJECT",
						"properties": map[string]interface{}{
							"title": map[string]interface{}{
								"type":        "STRING",
								"description": "A short, engaging title for the notification.",
							},
							"message": map[string]interface{}{
								"type":        "STRING",
								"description": "The body of the notification.",
							},
							"delay_hours": map[string]interface{}{
								"type":        "INTEGER",
								"description": "Hours from now, as a fallback when no particular moment matters. Prefer 'send_at' whenever the timing is part of the point.",
							},
							"send_at": map[string]interface{}{
								"type":        "STRING",
								"description": "When to send it, in the USER'S OWN local time, as 'YYYY-MM-DD HH:MM'. Use this whenever the moment matters — and it usually does. Think about what the user will be living through: a message before something they are dreading belongs the morning of, not the night before; a message asking how something went belongs AFTER it happened, never before. Never schedule anything between 22:00 and 07:00 their time. Their current local time is given in your context — count forward from that.",
							},
							"time_sensitive": map[string]interface{}{
								"type":        "BOOLEAN",
								"description": "True when this message only makes sense at the moment you chose — tied to something happening in their life (an exam, an interview, a difficult conversation). Time-sensitive messages are never shifted to make room for other messages; they are dropped instead, because arriving late is worse than not arriving.",
							},
						},
						"required": []string{"title", "message"},
					},
					"description": "List of personalized notifications to schedule.",
				},
			},
			"required": []string{"quote_category", "theme", "notifications"},
		},
	}
)

func ToolsForSession(sessionType api.SessionType, allowRestart bool, state *models.SessionState) []map[string]interface{} {
	var decls []map[string]interface{}

	// Runtime-infrastructure tools available on every connection, regardless of session/phase:
	//   - save_memory: Total Recall capture is part of the shared persona in every session.
	//   - request_recommendations: recommendations can be requested at any point.
	//   - schedule_notifications: the end-of-session notifications flow is injected on the live
	//     connection whenever the user disconnects — at any phase of any session — and an
	//     undeclared tool there fails with Gemini's "Invalid function call" (the same failure
	//     class as the restart-tool bug). The model never calls it spontaneously; only the
	//     shutdown prompt triggers it.
	// Everything else — including show_screen and save_session_insight — is owned by each
	// session's ToolNames(state), so the session is the single source of truth for its tools.
	decls = append(decls,
		toolSaveMemory,
		toolRequestRecommendations,
		toolScheduleNotifications,
	)

	if allowRestart {
		decls = append(decls, toolRestartSessionWithSummary)
	}

	// Task tools are scoped to the current phase so the model is only ever offered the tools
	// it legitimately needs right now — preventing it from calling a tool that belongs to a
	// different stage. For Onboarding the scope is bounded by the Wheel of Life restart points
	// (the only places the Gemini setup, and therefore the tool list, is refreshed), so each
	// phase must expose every tool reachable across its in-session transitions.
	if sess, ok := sessions.Get(sessionType); ok {
		// The session owns which tools each phase exposes (by name); we resolve those
		// names to the shared tool definitions here.
		st := sess.InitialState()
		if state != nil {
			st = *state
		}
		for _, name := range sess.ToolNames(st) {
			if def := toolByName[name]; def != nil {
				decls = append(decls, def)
			}
		}
	} else {
		decls = append(decls,
			toolCompleteCurrentTask,
			toolInitEisenhowerMatrix,
			toolUpdateEisenhowerMatrix,
			toolDeleteEisenhowerMatrixTasks,
			toolTerminateSession,
			toolShowScreen,
			toolSaveSessionInsight,
		)
	}

	return []map[string]interface{}{
		{
			"functionDeclarations": decls,
		},
	}
}

// toolByName resolves a tool name (as returned by the per-session flow packages) to
// its shared tool definition.
var toolByName = map[string]map[string]interface{}{
	"save_memory":                    toolSaveMemory,
	"show_screen":                    toolShowScreen,
	"save_profile_details":           toolSaveProfileDetails,
	"save_session_insight":           toolSaveSessionInsight,
	"request_recommendations":        toolRequestRecommendations,
	"schedule_notifications":         toolScheduleNotifications,
	"restart_session_with_summary":   toolRestartSessionWithSummary,
	"start_planned_session":          toolStartPlannedSession,
	"complete_current_task":          toolCompleteCurrentTask,
	"save_ideal_life_vision":         toolSaveIdealLifeVision,
	"set_wheel_of_life_categories":   toolSetWheelOfLifeCategories,
	"update_wheel_of_life":           toolUpdateWheelOfLife,
	"save_focus":                     toolSaveFocus,
	"save_vision_commitment":         toolSaveVisionCommitment,
	"save_identity_reflection":       toolSaveIdentityReflection,
	"save_acceptance_reflection":     toolSaveAcceptanceReflection,
	"save_commitments":               toolSaveActions,
	"update_commitment_plan":         toolUpdateCommitmentPlan,
	"add_commitment":                 toolAddCommitment,
	"remove_commitment":              toolRemoveCommitment,
	"save_top_values":                toolSaveTopValues,
	"save_behavior_plan":             toolSaveBehaviorPlan,
	"log_behavior_checkin":           toolLogBehaviorCheckin,
	"terminate_session":              toolTerminateSession,
	"init_eisenhower_matrix":         toolInitEisenhowerMatrix,
	"update_eisenhower_matrix":       toolUpdateEisenhowerMatrix,
	"delete_eisenhower_matrix_tasks": toolDeleteEisenhowerMatrixTasks,
}

func Tools() []map[string]interface{} {
	// Fallback/backward compatibility to return onboarding tools
	return ToolsForSession(api.SessionTypeOnboarding, true, nil)
}

// --- Tool Call Dispatcher ---

// SyncWheelVisuals sends the current Wheel of Life data to the client for rendering.
func (s *ChatSession) SyncWheelVisuals() {
	var exercise models.WheelOfLifeExercise
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at desc").Limit(1).Find(&exercise).Error; err != nil || exercise.ID == "" {
		return
	}

	var currentList []WheelOfLifeItem
	if exercise.Data != "" {
		json.Unmarshal([]byte(exercise.Data), &currentList) //nolint:errcheck
	}

	if len(currentList) > 0 {
		s.writeClientJSON(map[string]interface{}{
			"type": "wheel_of_life_update",
			"data": map[string]interface{}{
				"categories": currentList,
			},
		})
	}
}

// SyncEisenhowerMatrixVisuals sends the latest Eisenhower Matrix data to the client.
func (s *ChatSession) SyncEisenhowerMatrixVisuals() {
	var latestExercise models.EisenhowerMatrixExercise
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at desc").First(&latestExercise).Error; err != nil {
		return
	}
	var data EisenhowerMatrixData
	if err := json.Unmarshal([]byte(latestExercise.Data), &data); err != nil {
		return
	}
	s.writeClientJSON(map[string]interface{}{
		"type": "eisenhower_matrix_update",
		"data": data,
	})
}

// SyncCommitmentPlanVisuals sends the latest Commitment Plan (the plan commitments and the user's
// focus area) to the client.
func (s *ChatSession) SyncCommitmentPlanVisuals() {
	var commitments []models.Commitment
	database.DB.Where("user_id = ? AND origin = ?", s.UserID, models.CommitmentOriginPlan).Order("created_at asc").Find(&commitments) //nolint:errcheck
	area := ""
	if s.User != nil && s.User.FocusArea != nil {
		area = *s.User.FocusArea
	}
	s.writeClientJSON(map[string]interface{}{
		"type": "commitment_plan_update",
		"data": map[string]interface{}{
			"area":        area,
			"commitments": commitments,
		},
	})
}

// HandleToolCall processes function calls returned by Gemini and returns the tool response JSON to send back.
func (s *ChatSession) HandleToolCall(toolCall map[string]interface{}) {
	functionCalls, ok := toolCall["functionCalls"].([]interface{})
	if !ok {
		return
	}

	// Prioritize set_wheel_of_life_categories so it executes before update_wheel_of_life
	sort.SliceStable(functionCalls, func(i, j int) bool {
		callI, okI := functionCalls[i].(map[string]interface{})
		callJ, okJ := functionCalls[j].(map[string]interface{})
		if !okI || !okJ {
			return false
		}
		nameI, _ := callI["name"].(string)
		nameJ, _ := callJ["name"].(string)
		if nameI == "set_wheel_of_life_categories" && nameJ != "set_wheel_of_life_categories" {
			return true
		}
		return false
	})

	var toolResponses []map[string]interface{}

	executedCalls := make(map[string]struct {
		output string
		err    error
	})

	for _, fc := range functionCalls {
		call, ok := fc.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := call["name"].(string)
		args, _ := call["args"].(map[string]interface{})
		id, _ := call["id"].(string)

		s.logger.Info("Gemini → Server [Tool Call]",
			zap.String("name", name),
			zap.Any("args", args),
		)
		s.flushUserTranscript()
		argsJSON, _ := json.Marshal(args)
		s.logHistory("Tool Call", fmt.Sprintf("%s(%s)", name, string(argsJSON)))

		var output string
		var err error

		callKey := name + "_" + string(argsJSON)
		if prevResult, found := executedCalls[callKey]; found {
			s.logger.Info("Duplicate tool call detected in same turn, reusing previous response",
				zap.String("name", name),
			)
			output = prevResult.output
			err = prevResult.err
		} else {
			switch name {
			case "start_planned_session":
				output, err = s.handleStartPlannedSession(args)
			case "restart_session_with_summary":
				output, err = s.handleRestartSessionWithSummary(args)
			case "show_screen":
				output, err = s.handleShowScreen(args)
			case "save_memory":
				output, err = s.handleSaveMemory(args)
			case "save_profile_details":
				output, err = s.handleSaveProfileDetails(args)
			case "save_session_insight":
				output, err = s.handleSaveSessionInsight(args)
			case "request_recommendations":
				output, err = s.handleRequestRecommendations(args)
			case "complete_current_task":
				output, err = s.handleCompleteCurrentTask(args)
			case "save_ideal_life_vision":
				output, err = s.handleSaveIdealLifeVision(args)
			case "set_wheel_of_life_categories":
				output, err = s.handleSetWheelOfLifeCategories(args)
			case "update_wheel_of_life":
				output, err = s.handleUpdateWheelOfLife(args)
			case "save_focus":
				output, err = s.handleSaveFocus(args)
			case "save_vision_commitment":
				output, err = s.handleSaveVisionCommitment(args)
			case "save_identity_reflection":
				output, err = s.handleSaveIdentityReflection(args)
			case "save_acceptance_reflection":
				output, err = s.handleSaveAcceptanceReflection(args)
			case "save_commitments":
				output, err = s.handleSaveActions(args)
			case "update_commitment_plan":
				output, err = s.handleUpdateCommitmentPlan(args)
			case "add_commitment":
				output, err = s.handleAddCommitment(args)
			case "remove_commitment":
				output, err = s.handleRemoveCommitment(args)
			case "save_top_values":
				output, err = s.handleSaveTopValues(args)
			case "save_behavior_plan":
				output, err = s.handleSaveBehaviorPlan(args)
			case "log_behavior_checkin":
				output, err = s.handleLogBehaviorCheckin(args)
			case "terminate_session":
				output, err = s.handleTerminateSession(args)
			case "init_eisenhower_matrix":
				output, err = s.handleInitEisenhowerMatrix(args)
			case "update_eisenhower_matrix":
				output, err = s.handleUpdateEisenhowerMatrix(args)
			case "delete_eisenhower_matrix_tasks":
				output, err = s.handleDeleteEisenhowerMatrixTasks(args)
			case "schedule_notifications":
				output, err = s.handleScheduleNotifications(args)
			default:
				s.logger.Warn("Unknown tool call", zap.String("name", name))
				output = fmt.Sprintf(`{"status": "error", "message": "Unknown tool: %s"}`, name)
			}

			if err != nil {
				s.logger.Error("Tool call failed", zap.String("name", name), zap.Error(err))
				output = fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
			}

			executedCalls[callKey] = struct {
				output string
				err    error
			}{output: output, err: err}
		}

		s.logger.Info("Server → Gemini [Tool Response]",
			zap.String("name", name),
			zap.String("output", output),
		)
		s.logHistory("Tool Response", fmt.Sprintf("%s -> %s", name, output))

		var responseObj map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(output), &responseObj); unmarshalErr != nil {
			// Fallback if not valid JSON
			responseObj = map[string]interface{}{
				"result": output,
			}
		}

		toolResponse := map[string]interface{}{
			"id":       id,
			"name":     name,
			"response": responseObj,
		}

		isSilent := false
		if name == "complete_current_task" || name == "init_eisenhower_matrix" || name == "save_ideal_life_vision" || name == "save_commitments" || name == "terminate_session" || name == "save_session_insight" || name == "schedule_notifications" || name == "set_wheel_of_life_categories" || name == "save_memory" || name == "save_profile_details" || name == "delete_eisenhower_matrix_tasks" || name == "start_planned_session" {
			isSilent = true
		} else if name == "save_focus" || name == "save_vision_commitment" || name == "save_identity_reflection" || name == "save_acceptance_reflection" {
			isSilent = true
		} else if name == "update_wheel_of_life" {
			isSilent = true
		}

		if isSilent {
			toolResponse["scheduling"] = "SILENT"
		}

		toolResponses = append(toolResponses, toolResponse)
	}

	if len(toolResponses) == 0 {
		return
	}

	// Send tool response back to Gemini
	response := map[string]interface{}{
		"toolResponse": map[string]interface{}{
			"functionResponses": toolResponses,
		},
	}
	if err := s.writeGeminiJSONWithConn(s.GeminiWs, response); err != nil {
		s.logger.Error("Failed to send tool response to Gemini", zap.Error(err))
	}

	s.geminiMutex.Lock()
	isRestarting := s.pendingRestart
	isInSessionTransition := s.pendingInSessionTransition
	transitionDirective := s.inSessionTransitionDirective
	if isInSessionTransition {
		s.pendingInSessionTransition = false
		s.inSessionTransitionDirective = ""
	}
	s.geminiMutex.Unlock()

	if isRestarting {
		s.logger.Info("Forcing WebSocket close immediately to trigger pendingRestart since Gemini may not send TURN_COMPLETE")
		s.geminiMutex.Lock()
		if s.GeminiWs != nil {
			s.GeminiWs.Close()
		}
		s.geminiMutex.Unlock()
	} else if isInSessionTransition {
		// In-session transition (every transition except entering/leaving the Wheel of Life):
		// keep the live Gemini connection and inject the next task's full instructions, because
		// the existing system prompt still holds the previous task.
		nextInstructions := s.GetNextInstructions(s.User)
		combined := transitionDirective
		if nextInstructions != "" {
			combined = transitionDirective + "\n\n" + nextInstructions
		}

		s.geminiMutex.Lock()
		if s.turnSpokeText {
			// The model SPOKE in this same turn (the speak-then-call flows: the metaphor
			// save_focus acknowledgment, the emotional-closing acknowledgment). Injecting
			// right now barges in on the still-active generation: Gemini flags it
			// interrupted, the client discards the acknowledgment's unplayed audio, and
			// the user hears the sentence cut off mid-delivery (QA: "não terminou a
			// frase que estava a proferir"). A turn that produced speech always emits
			// TURN_COMPLETE, so defer the injection there — that path also inserts the
			// 1s separating silence so the two utterances don't glue.
			s.pendingTransitionPrompt = combined
			s.geminiMutex.Unlock()
			s.logger.Info("Deferring in-session transition to TURN_COMPLETE (model spoke this turn)")
		} else {
			// Silent tool turn (e.g. complete_current_task called with no acknowledgment):
			// ALSO defer to TURN_COMPLETE rather than injecting mid-turn. Injecting while
			// this turn's own generation cycle is still open produced a goodbye with a full
			// text transcript but ZERO seconds of actual audio (QA, confirmed via the
			// turnAudioDuration diagnostic — the user heard nothing at all, not even a cut-off
			// sentence). Deferring past a clean TURN_COMPLETE boundary is what let the exact
			// same ENDING_SESSION transition produce real audio on turns where the model spoke
			// first. A short fallback timer still guards against the original concern this
			// branch existed for — a silent tool call that never emits TURN_COMPLETE at all —
			// by forcing the injection anyway if TURN_COMPLETE hasn't consumed it in time.
			s.pendingTransitionPrompt = combined
			s.geminiMutex.Unlock()
			s.logger.Info("Deferring in-session transition to TURN_COMPLETE (silent tool turn)")
			go func(session *ChatSession, expected string) {
				time.Sleep(3 * time.Second)
				session.geminiMutex.Lock()
				stillPending := session.pendingTransitionPrompt == expected
				if stillPending {
					session.pendingTransitionPrompt = ""
				}
				session.geminiMutex.Unlock()
				if stillPending {
					session.logger.Warn("TURN_COMPLETE never arrived after a silent transition tool call; forcing injection")
					session.InjectPrompt(expected)
				}
			}(s, combined)
		}
	}

}

// transitionNeedsRestart reports whether advancing from one state to another must
// hard-restart the Gemini connection. We only restart around the Wheel of Life — the
// most complex task — i.e. when entering it or leaving it. Every other transition stays
// on the live connection (see scheduleTransition).
func (s *ChatSession) transitionNeedsRestart(from, to models.SessionState) bool {
	if sess, ok := sessions.Get(s.SessionType); ok {
		return sess.NeedsRestart(from, to)
	}
	return false
}

// scheduleTransition records how the session should advance after the current tool turn.
// Around the Wheel of Life it hard-restarts the Gemini connection (rebuilding the system
// prompt with the new task); otherwise it performs a lightweight in-session transition,
// injecting the next task's full instructions into the live session. `directive` is the
// short [SYSTEM ...] guidance describing the new stage.
// Caller must hold s.geminiMutex.
func (s *ChatSession) scheduleTransition(from, to models.SessionState, directive string) {
	if s.transitionNeedsRestart(from, to) {
		s.pendingRestart = true
		s.restartInstructions = directive
	} else {
		s.scheduleInSessionTransition(directive)
	}
}

// scheduleInSessionTransition marks a non-restart transition. The directive plus the
// next task's full instructions are injected into the live Gemini session right after
// the current tool response (see HandleToolCall), because the existing system prompt
// still holds the previous task. Caller must hold s.geminiMutex.
func (s *ChatSession) scheduleInSessionTransition(directive string) {
	s.pendingInSessionTransition = true
	s.inSessionTransitionDirective = directive
}

func (s *ChatSession) handleSetWheelOfLifeCategories(args map[string]interface{}) (string, error) {
	categoryNamesRaw, ok := args["category_names"].([]interface{})
	if !ok {
		return `{"status": "error", "message": "missing category_names"}`, nil
	}

	categories := make([]string, 0, len(categoryNamesRaw))
	for _, c := range categoryNamesRaw {
		if name, ok := c.(string); ok {
			categories = append(categories, name)
		}
	}

	// Load existing data to preserve scores
	existingMap := make(map[string]WheelOfLifeItem)
	var exercises []models.WheelOfLifeExercise
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at desc").Limit(1).Find(&exercises).Error; err == nil && len(exercises) > 0 && exercises[0].Data != "" {
		var existingList []WheelOfLifeItem
		if err := json.Unmarshal([]byte(exercises[0].Data), &existingList); err == nil {
			for _, item := range existingList {
				existingMap[item.Name] = item
			}
		}
	}

	// Update categories, preserving existing scores where possible
	freshCategories := make([]WheelOfLifeItem, 0, len(categories))
	for _, name := range categories {
		if existing, ok := existingMap[name]; ok {
			freshCategories = append(freshCategories, existing)
		} else {
			freshCategories = append(freshCategories, WheelOfLifeItem{
				Name:         name,
				CurrentScore: 0,
				Reasoning:    "Initial setup",
			})
		}
	}

	jsonData, err := json.Marshal(freshCategories)
	if err != nil {
		return "", fmt.Errorf("failed to marshal categories: %w", err)
	}
	strData := string(jsonData)

	// Save to WheelOfLifeExercise table
	var exercisesToSave []models.WheelOfLifeExercise
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at desc").Limit(1).Find(&exercisesToSave).Error; err == nil && len(exercisesToSave) > 0 {
		exerciseToSave := exercisesToSave[0]
		exerciseToSave.Data = strData
		database.DB.Save(&exerciseToSave)
	} else {
		newExercise := models.WheelOfLifeExercise{
			UserID:    s.UserID,
			SessionID: s.SessionDB.ID,
			Data:      strData,
		}
		database.DB.Create(&newExercise)
	}

	s.SyncWheelVisuals()
	s.NeedsDynamicTransitionPrompt = true
	s.wheelSetupThisTurn = true

	return `{"status": "success", "message": "Visuals initialized with saved data"}`, nil
}

func (s *ChatSession) handleUpdateWheelOfLife(args map[string]interface{}) (string, error) {
	var category string
	var score float64
	var reasoning string

	if cat, ok := args["category"].(string); ok {
		category = cat
	}
	if sc, ok := args["score"].(float64); ok {
		score = sc
	} else if scInt, ok := args["score"].(int); ok {
		score = float64(scInt)
	}
	if reas, ok := args["reasoning"].(string); ok {
		reasoning = reas
	}

	// Fallback to json_data if provided
	if jsonData, ok := args["json_data"].(string); ok && jsonData != "" {
		var updates []WheelOfLifeItem
		if err := json.Unmarshal([]byte(jsonData), &updates); err == nil && len(updates) > 0 {
			category = updates[0].Name
			score = updates[0].CurrentScore
			reasoning = updates[0].Reasoning
		}
	}

	if category == "" {
		return "", fmt.Errorf("missing category")
	}

	// Load existing full list
	var currentList []WheelOfLifeItem
	var exercises []models.WheelOfLifeExercise
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at desc").Limit(1).Find(&exercises).Error; err == nil && len(exercises) > 0 && exercises[0].Data != "" {
		json.Unmarshal([]byte(exercises[0].Data), &currentList) //nolint:errcheck
	}

	if len(currentList) == 0 {
		return `{"status": "error", "message": "no categories set yet; call set_wheel_of_life_categories first"}`, nil
	}

	matchedIdx := -1
	// First pass: try to find an exact match (case-insensitive)
	for i, item := range currentList {
		if strings.EqualFold(item.Name, category) {
			matchedIdx = i
			break
		}
	}

	// Second pass: if no exact match found, try alias matching
	if matchedIdx == -1 {
		for i, item := range currentList {
			match := false
			// Handle common variations to be extra robust (including back-compat for Relations/Relationships)
			if (strings.EqualFold(item.Name, "Relations") || strings.EqualFold(item.Name, "Relationships")) && (strings.EqualFold(category, "Relationships") || strings.EqualFold(category, "Relations") || strings.EqualFold(category, "Relationship")) {
				match = true
			} else if strings.EqualFold(item.Name, "Purpose") && (strings.EqualFold(category, "Career") || strings.EqualFold(category, "Job")) {
				match = true
			} else if strings.EqualFold(item.Name, "Money") && (strings.EqualFold(category, "Finances") || strings.EqualFold(category, "Finance")) {
				match = true
			} else if strings.EqualFold(item.Name, "Wellbeing & Growth") && (strings.EqualFold(category, "Wellbeing") || strings.EqualFold(category, "Growth") || strings.EqualFold(category, "Personal Growth") || strings.EqualFold(category, "Self-Care") || strings.EqualFold(category, "Inner Peace")) {
				match = true
			}

			if match {
				matchedIdx = i
				break
			}
		}
	}

	if matchedIdx == -1 {
		names := make([]string, len(currentList))
		for i, item := range currentList {
			names[i] = item.Name
		}
		return fmt.Sprintf(`{"status": "error", "message": "Area '%s' was not found on the wheel. The current areas are: %s. Call update_wheel_of_life again with the 'category' set to the EXACT area name from this list, copied verbatim in the user's language. Do NOT translate it or use the English label."}`, category, strings.Join(names, ", ")), nil
	}

	matched := &currentList[matchedIdx]
	if score < 1 || score > 10 {
		return fmt.Sprintf(`{"status": "error", "message": "Invalid score for %s: scores must be between 1 and 10. Please ask the user for a valid 1-10 score."}`, matched.Name), nil
	}

	// HARD-RULE guard (NO BARE-SCORE SAVES): the model sometimes saves in the very turn the
	// user gave only a number, fabricating the reasoning (QA: "1 3 também" was saved with an
	// invented "indicando insatisfação com a sua estabilidade..."). A genuine reasoning
	// answer is never just a few characters, so on an area's FIRST fill, reject when the
	// user's last utterance is too short to plausibly contain their "why" — once per area,
	// so a legitimately terse user still gets saved on the retry. Score revisions (area
	// already filled) are exempt: their reasoning was already captured.
	if matched.CurrentScore == 0 {
		recent := s.getRecentUserMessages(2)
		lastUser := ""
		if len(recent) > 0 {
			lastUser = strings.TrimSpace(recent[0])
		}
		// Reasoning sometimes arrives BEFORE the score ("...it's been a challenge because…"
		// then, asked for a number, just "a six"). That earlier message only counts as this
		// area's reasoning if it came after the previous area was saved — otherwise a long
		// message consumed by the PREVIOUS area would let a fabricated save through again.
		reasoningCameEarlier := s.userMsgsSinceWheelSave >= 2 && len(recent) > 1 &&
			utf8.RuneCountInString(strings.TrimSpace(recent[1])) >= bareScoreMinReasoningRunes
		if lastUser != "" && utf8.RuneCountInString(lastUser) < bareScoreMinReasoningRunes &&
			!reasoningCameEarlier &&
			!strings.EqualFold(s.bareScoreRejectedArea, matched.Name) {
			s.bareScoreRejectedArea = matched.Name
			s.logger.Warn("Rejecting bare-score wheel save; user utterance too short to contain reasoning",
				zap.String("area", matched.Name), zap.String("last_user", lastUser))
			return fmt.Sprintf(`{"status": "error", "message": "REJECTED for %s: the user's last words are only a score — they have NOT yet shared what sits behind it, and you may never invent their reasoning. Warmly validate the number and ask, in your own words, what is behind that score (what would make it a 10). ONLY after they have actually explained, call update_wheel_of_life again with their real words as the reasoning."}`, matched.Name), nil
		}
	}
	s.userMsgsSinceWheelSave = 0

	matched.CurrentScore = score
	matched.Reasoning = reasoning

	// 1. Get the pending category BEFORE saving any updates to the DB
	var pendingCat string
	if s.User != nil {
		pendingCat = s.getNextPendingCategory(s.User)
	}

	// Save merged full list
	mergedData, _ := json.Marshal(currentList)
	mergedStr := string(mergedData)

	// Save to WheelOfLifeExercise table
	var exercisesToSave []models.WheelOfLifeExercise
	if err := database.DB.Where("user_id = ?", s.UserID).Order("created_at desc").Limit(1).Find(&exercisesToSave).Error; err == nil && len(exercisesToSave) > 0 {
		exerciseToSave := exercisesToSave[0]
		exerciseToSave.Data = mergedStr
		database.DB.Save(&exerciseToSave)
	} else {
		newExercise := models.WheelOfLifeExercise{
			UserID:    s.UserID,
			SessionID: s.SessionDB.ID,
			Data:      mergedStr,
		}
		database.DB.Create(&newExercise)
	}

	s.SyncWheelVisuals()

	// Only advance to the next area if we just completed the current pending one. If the user is
	// updating a PAST area, stay where we are (do not inject a "next area" prompt).
	if s.User != nil {
		if strings.EqualFold(category, pendingCat) || pendingCat == "" {
			s.NeedsDynamicTransitionPrompt = true
		} else {
			s.logger.Info("Updated a past Wheel of Life area; not advancing to the next area", zap.String("category", category))
		}
	}

	return `{"status": "success", "message": "Wheel of Life updated"}`, nil
}

// trimStatePrefix strips the session-family prefix from a state name so the two halves
// of the opening arc compare on the phase alone ("VISION_EMOTIONAL_CLOSING" and the
// model's bare "EMOTIONAL_CLOSING" are the same intent).
func trimStatePrefix(state string) string {
	for _, prefix := range []string{"ONBOARDING_", "VISION_"} {
		if trimmed := strings.TrimPrefix(state, prefix); trimmed != state {
			return trimmed
		}
	}
	return state
}

func (s *ChatSession) handleCompleteCurrentTask(args map[string]interface{}) (string, error) {
	if s.User == nil {
		return "", fmt.Errorf("user not initialized")
	}

	providedState, _ := args["current_state"].(string)
	currentState := string(s.CurrentState())

	// The model regularly passes the un-prefixed phase name (e.g. "EMOTIONAL_CLOSING"
	// while the state is "ONBOARDING_EMOTIONAL_CLOSING"). QA showed the resulting
	// "ignored" derails the flow: the closing never completes and the model improvises.
	// Same phase name modulo the ONBOARDING_/VISION_ prefix is unambiguously the same intent.
	if providedState != "" && providedState != currentState &&
		trimStatePrefix(providedState) == trimStatePrefix(currentState) {
		s.logger.Info("Accepting complete_current_task state alias",
			zap.String("provided", providedState), zap.String("current", currentState))
		providedState = currentState
	}

	if providedState != "" && providedState != currentState {
		stateOrder := map[string]int{
			string(models.StateOnboardingIntro):        0,
			string(models.StateVisionIdealLife):        1,
			string(models.StateVisionWheelOfLife):      2,
			string(models.StateVisionMetaphor):         3,
			string(models.StateVisionEmotionalClosing): 4,
			string(models.StateVisionEndingSession):    5,
			string(models.StateEmotionalClosing):       6,
			string(models.StateEndingSession):          7,
			string(models.StateCheckin):                8,
		}

		providedOrder, hasProvided := stateOrder[providedState]
		currentOrder, hasCurrent := stateOrder[currentState]

		if hasProvided && hasCurrent && providedOrder < currentOrder {
			s.logger.Info("Allowing complete_current_task for already completed/progressed state",
				zap.String("provided", providedState),
				zap.String("current", currentState))
			return fmt.Sprintf(`{"status": "success", "message": "Task already completed. Current state is %s"}`, currentState), nil
		}

		s.logger.Warn("Ignoring complete_current_task: state mismatch",
			zap.String("provided", providedState),
			zap.String("current", currentState))
		return fmt.Sprintf(`{"status": "ignored", "message": "Task already completed or state mismatch. Current state is %s. If you are trying to complete the CURRENT task, call complete_current_task again NOW with current_state set to exactly '%s'."}`, currentState, currentState), nil
	}

	// The onboarding emotional closing is a four-step dialogue (insight → permission to
	// reflect → personalized synthesis → clarity check). The model's ingrained habit is to
	// call complete_current_task in the same turn it saves the insight, skipping the
	// synthesis that gives the closing its value — reject that once so the dialogue
	// happens. The retry always passes, so a degenerate conversation can still end.
	if currentState == string(models.StateVisionEmotionalClosing) {
		s.geminiMutex.Lock()
		tooEarly := s.closingInsightSaved && s.closingTurnsAfterInsight < 2 && !s.closingEarlyCompleteRejected
		if tooEarly {
			s.closingEarlyCompleteRejected = true
		}
		s.geminiMutex.Unlock()
		if tooEarly {
			s.logger.Warn("Rejecting early complete_current_task in emotional closing; synthesis dialogue not delivered yet")
			return `{"status": "error", "message": "REJECTED: the closing dialogue is NOT finished. You have not yet asked permission to share your reflection, delivered your personalized 3-5 sentence synthesis (a pattern, tension, or leverage point you noticed across the whole session — not a summary), asked whether it makes sense to them, invited a first concrete commitment, and asked whether they now have more clarity than when you started. Continue with steps 2-5 of your task instructions, and call complete_current_task ONLY after the user has answered the clarity question."}`, nil
		}

		// A question the model just asked must actually be answered before the closing may
		// end: QA caught it asking "do you feel you have more clarity...?" and completing
		// in the SAME turn, cutting the user off and jumping to the goodbye. If the newest
		// transcript entry is the model's own speech ending in a question mark, no answer
		// has arrived — reject once so a degenerate conversation can still end.
		if s.lastEntryIsAIQuestion() && !s.closingQuestionWaitRejected {
			s.closingQuestionWaitRejected = true
			s.logger.Warn("Rejecting complete_current_task in emotional closing; the model asked a question and did not wait for the answer")
			return `{"status": "error", "message": "REJECTED: you just asked the user a question in this same turn and have NOT waited for their answer. Stop generating NOW and let them speak. Only after they have actually answered may you acknowledge it in one short sentence and call complete_current_task."}`, nil
		}
	}

	if currentState == string(models.StateOnboardingIntro) {
		s.writeClientJSON(map[string]interface{}{
			"type": "show_screen",
			"data": map[string]string{
				"screen": "session",
			},
		})
	}

	var nextState models.SessionState
	handled := false
	// The session owns its own transition table.
	if sess, ok := sessions.Get(s.SessionType); ok {
		if t, owned := sess.NextOnCompleteTask(models.SessionState(currentState)); owned {
			if t.Blocked != "" {
				return t.Blocked, nil
			}
			nextState = t.Next
			handled = true
		}
	}
	if !handled {
		// Generic deep-coaching / check-in closing transitions for sessions without their
		// own transition table.
		switch currentState {
		case string(models.StateEmotionalClosing):
			nextState = models.StateEndingSession
		case string(models.StateEndingSession):
			nextState = models.StateCheckin
		case string(models.StateCheckin):
			return `{"status": "error", "message": "CRITICAL: You are already in the CHECKIN stage. There are no more stages to advance to. Stop generating tools."}`, nil
		default:
			return "", fmt.Errorf("unknown current state for transition: %s", currentState)
		}
	}

	nextStateStr := string(nextState)

	// Update DB
	if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
		Update("state", nextState).Error; err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	s.User.State = &nextStateStr
	s.logger.Info(fmt.Sprintf("[State change] %s -> %s", currentState, nextStateStr))

	directive := fmt.Sprintf("[SYSTEM: The session has transitioned to the %s stage. You MUST start from the very beginning of the new task instructions for this stage. Deliver the first step exactly as written. Do not mention the pause or summarize.]", nextStateStr)
	if nextState == models.StateEndingSession || nextState == models.StateVisionEndingSession {
		// Short pointer only — the full goodbye script, the exact "speak first, then call
		// terminate_session" rule, and the marker instruction all already live in the task
		// instructions this is paired with (endingSessionInstructions). Spelling the same
		// two-step instruction out again here (and a third time, until removed, in
		// InjectTransitionPrompt) stacked three near-identical copies of it into one prompt —
		// bloat that correlated with the model producing the full goodbye transcript but no
		// audio at all (QA).
		directive = "[SYSTEM: The session has transitioned to the ENDING_SESSION stage. Deliver the final goodbye exactly as written in the task instructions below, right now.]"
		// No summary emit here: the card is revealed ONLY at the goodbye's '◆▧' marker,
		// right after the script's announcement sentence has told the user it is coming
		// (QA: an unannounced early reveal — reading while Rumi was still speaking — was
		// rejected). handleTerminateSession remains the safety net if ◆▧ is dropped.
	}
	s.geminiMutex.Lock()
	s.scheduleTransition(models.SessionState(currentState), nextState, directive)
	s.geminiMutex.Unlock()
	return fmt.Sprintf(`{"status": "success", "message": "Moved to %s. You MUST stop generating now and yield control."}`, nextStateStr), nil
}

// normalizeMemoryCategory maps a model-supplied memory category to its canonical
// api.MemoryCategory value, tolerating case and surrounding whitespace. The second return
// value reports whether it is a real category at all.
func normalizeMemoryCategory(category string) (string, bool) {
	for _, valid := range []api.MemoryCategory{
		api.Identity, api.Values, api.Needs, api.Context, api.Obstacles, api.Insight,
	} {
		if strings.EqualFold(strings.TrimSpace(category), string(valid)) {
			return string(valid), true
		}
	}
	return "", false
}

// stripLeadingVocative removes the user's own first name used as an opening vocative in a
// memory ("Filipa, decides bloquear...") — memories are shown back to the user on their
// memories screen, where their own name reads as someone else talking about them. The
// persona forbids it, but the model slips it in as a second-person vocative (QA); this
// makes the rule hold regardless.
func stripLeadingVocative(content string, user *models.User) string {
	if user == nil || user.Name == nil {
		return content
	}
	first := strings.Fields(strings.TrimSpace(*user.Name))
	if len(first) == 0 {
		return content
	}
	trimmed := strings.TrimSpace(content)
	rest, found := strings.CutPrefix(trimmed, first[0]+",")
	if !found {
		return content
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return content
	}
	r := []rune(rest)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func (s *ChatSession) handleSaveMemory(args map[string]interface{}) (string, error) {
	category, okCat := args["category"].(string)
	content, okCon := args["content"].(string)

	if !okCat || !okCon || category == "" || content == "" {
		return "", fmt.Errorf("missing or invalid category or content parameter")
	}

	// The onboarding intro produces no memories worth keeping: everything it collects
	// (country, date of birth, gender) lives on the profile, and despite an explicit
	// prompt rule the model keeps duplicating the gender there as an identity memory
	// ("Identificas-te como feminino") that then pollutes the memories screen and every
	// later session's context (QA, twice). Server-side guard per house doctrine:
	// acknowledge the call, write nothing.
	if s.SessionType == api.SessionTypeOnboarding {
		s.logger.Info("Dropping save_memory during onboarding intro",
			zap.String("category", category), zap.String("content", content))
		return `{"status": "success", "message": "Memory saved"}`, nil
	}

	// Duplicate guard: at session start the model sometimes re-saves the entire Section
	// 1.1 memory block it was just shown — QA saw a burst of eleven save_memory calls
	// duplicating every existing memory, twice in one day. An identical content string
	// for this user is never new information; acknowledge and write nothing.
	var existingCount int64
	if err := database.DB.Model(&models.UserMemory{}).
		Where("user_id = ? AND content = ?", s.UserID, strings.TrimSpace(content)).
		Count(&existingCount).Error; err == nil && existingCount > 0 {
		s.logger.Info("Dropping duplicate save_memory", zap.String("content", content))
		return `{"status": "success", "message": "Memory saved"}`, nil
	}

	// The category set is closed (api.MemoryCategory). Nothing validated it before, so the
	// model's occasional invented categories were stored verbatim (QA: 'goals', 'vision').
	// That is not just noise: an insight filed under a made-up category silently disappears
	// from the session-summary panel and the profile's insight count, both of which query
	// category = 'insight'. Reject with the valid values listed so the model self-corrects,
	// the same way update_wheel_of_life reports an unknown area.
	normalized, valid := normalizeMemoryCategory(category)
	if !valid {
		s.logger.Warn("Rejecting save_memory with an unknown category", zap.String("category", category))
		return fmt.Sprintf(`{"status": "error", "message": "REJECTED: '%s' is not a memory category. Valid values are exactly: identity, values, needs, context, obstacles, insight. Call save_memory again with the closest valid category — an a-ha moment or breakthrough is 'insight', something they want or lack is 'needs', something they care about is 'values'."}`, category), nil
	}
	category = normalized
	content = stripLeadingVocative(content, s.User)

	memory := models.UserMemory{
		UserID:   s.UserID,
		Category: category,
		Content:  content,
	}

	if err := database.DB.Create(&memory).Error; err != nil {
		return "", fmt.Errorf("failed to save memory: %w", err)
	}
	// The content is deliberately not logged. A memory is the most intimate thing the
	// product holds ("you realised you were waiting for permission"), and a log line is
	// outside everything we built to govern it: it is not deleted when the user erases
	// their memories, it is not bound to their data region, and it is readable by anyone
	// with project access. The length is enough to debug a truncation or an empty save.
	s.logger.Info("Memory saved", zap.String("category", category), zap.Int("chars", len([]rune(content))))

	// The insight save is the structural anchor of the onboarding emotional closing: from
	// here the model must still deliver the permission question, the personalized
	// synthesis, and the clarity check before complete_current_task is accepted.
	if strings.EqualFold(category, "insight") && s.User != nil && s.User.State != nil &&
		*s.User.State == string(models.StateVisionEmotionalClosing) {
		s.geminiMutex.Lock()
		// Anchor on the FIRST insight only. The model sometimes files a second
		// insight-category memory mid-closing (QA: one during the commitment step);
		// resetting the turn counter then made the early-complete guard reject a
		// perfectly finished closing, forcing Rumi to re-ask the commitment and
		// clarity questions ("Pensei que estavas a ouvir-me").
		if !s.closingInsightSaved {
			s.closingInsightSaved = true
			s.closingTurnsAfterInsight = 0
		}
		s.geminiMutex.Unlock()
		// The session-summary panel is NOT emitted here: the model is only ONE sentence
		// into the permission question ("posso partilhar...?") at this point, and the
		// user has not yet agreed. The reveal is instead driven by the '◆▦' marker the
		// model outputs at the start of Step 3's synthesis — a turn that structurally
		// cannot begin before the user has answered yes (see scheduleScreenReveal).
	}

	return `{"status": "success", "message": "Memory saved"}`, nil
}

func (s *ChatSession) handleSaveSessionInsight(args map[string]interface{}) (string, error) {
	insight, ok := args["insight"].(string)
	if !ok || insight == "" {
		return "", fmt.Errorf("missing or empty insight parameter")
	}

	// 1. Save to the session record's insight field in DB
	s.SessionDB.UserSessionInsight = &insight
	if err := database.DB.Save(&s.SessionDB).Error; err != nil {
		s.logger.Error("failed to save session insight in DB", zap.Error(err), zap.String("session_id", s.SessionDB.ID))
	} else {
		s.logger.Info("Saved session insight in DB", zap.String("session_id", s.SessionDB.ID), zap.String("insight", insight))
	}

	// 2. Also save to user memories as an insight so it's loaded in future sessions
	memory := models.UserMemory{
		UserID:   s.UserID,
		Category: "insight",
		Content:  insight,
	}
	if err := database.DB.Create(&memory).Error; err != nil {
		s.logger.Error("failed to save session insight as user memory", zap.Error(err))
	} else {
		s.logger.Info("Session insight duplicated as user memory insight successfully")
	}

	// The reminder matters: in single-prompt sessions the model has repeatedly spoken
	// the goodbye and then never called terminate_session — the session hung open, no
	// session_terminated reached the client, and the synthesis card was never shown
	// (QA, twice in one test round).
	return `{"status": "success", "message": "Session insight saved successfully. Reminder: when you reach the goodbye, you MUST call terminate_session in the SAME turn as the goodbye — a goodbye without terminate_session leaves the session hanging and the user never sees their summary card."}`, nil
}

// handleSaveIdentityReflection captures the Identity session's structured synthesis
// (Filipa's Identity Reflection Card). REPLACES, never accumulates: a re-call for the
// same session (the model correcting itself after the user refines the statement)
// deletes this session's prior row first, mirroring save_vision_commitment. The row
// feeds the session-end card (buildSessionSummary) and future sessions' context
// (FormatIdentityReflectionContext).
func (s *ChatSession) handleSaveIdentityReflection(args map[string]interface{}) (string, error) {
	str := func(key string) string {
		v, _ := args[key].(string)
		return strings.TrimSpace(v)
	}
	learned := str("learned_identity")
	becoming := str("who_becoming")
	if learned == "" || becoming == "" {
		return "", fmt.Errorf("missing learned_identity or who_becoming")
	}

	var qualities models.StringSlice
	if raw, ok := args["qualities"].([]interface{}); ok {
		for _, q := range raw {
			if qs, ok := q.(string); ok && strings.TrimSpace(qs) != "" {
				qualities = append(qualities, strings.TrimSpace(qs))
			}
		}
	}
	if len(qualities) == 0 {
		return "", fmt.Errorf("qualities must list the two or three qualities the user chose")
	}

	reflection := models.IdentityReflection{
		UserID:          s.UserID,
		SessionID:       s.SessionDB.ID,
		LearnedIdentity: learned,
		WhatItGave:      str("what_it_gave"),
		WhatItCosts:     str("what_it_costs"),
		WhoBecoming:     becoming,
		Qualities:       qualities,
		Evidence:        str("evidence"),
	}
	if err := database.DB.Where("user_id = ? AND session_id = ?", s.UserID, s.SessionDB.ID).
		Delete(&models.IdentityReflection{}).Error; err != nil {
		return "", fmt.Errorf("failed to replace identity reflection: %w", err)
	}
	if err := database.DB.Create(&reflection).Error; err != nil {
		return "", fmt.Errorf("failed to save identity reflection: %w", err)
	}
	s.logger.Info("Identity reflection saved", zap.String("session_id", s.SessionDB.ID))
	return `{"status": "success", "message": "Identity reflection saved. It will appear on the session-end card — continue the conversation naturally without mentioning any tool."}`, nil
}

// handleSaveAcceptanceReflection captures the Acceptance session's structured synthesis
// (Filipa's Acceptance Reflection Card). REPLACES, never accumulates: a re-call for the
// same session (the model correcting itself after the user refines the reflection)
// deletes this session's prior row first, mirroring save_identity_reflection. The row
// feeds the session-end card (buildSessionSummary).
func (s *ChatSession) handleSaveAcceptanceReflection(args map[string]interface{}) (string, error) {
	str := func(key string) string {
		v, _ := args[key].(string)
		return strings.TrimSpace(v)
	}
	expected := str("expected")
	reality := str("reality")
	if expected == "" || reality == "" {
		return "", fmt.Errorf("missing expected or reality")
	}

	reflection := models.AcceptanceReflection{
		UserID:         s.UserID,
		SessionID:      s.SessionDB.ID,
		Expected:       expected,
		Reality:        reality,
		CannotControl:  str("cannot_control"),
		CanInfluence:   str("can_influence"),
		ChooseToAccept: str("choose_to_accept"),
		WhereIAct:      str("where_i_act"),
		NextStep:       str("next_step"),
	}
	if err := database.DB.Where("user_id = ? AND session_id = ?", s.UserID, s.SessionDB.ID).
		Delete(&models.AcceptanceReflection{}).Error; err != nil {
		return "", fmt.Errorf("failed to replace acceptance reflection: %w", err)
	}
	if err := database.DB.Create(&reflection).Error; err != nil {
		return "", fmt.Errorf("failed to save acceptance reflection: %w", err)
	}
	s.logger.Info("Acceptance reflection saved", zap.String("session_id", s.SessionDB.ID))
	return `{"status": "success", "message": "Acceptance reflection saved. It will appear on the session-end card — continue the conversation naturally without mentioning any tool."}`, nil
}

func (s *ChatSession) handleUpdateEisenhowerMatrix(args map[string]interface{}) (string, error) {
	jsonData, ok := args["json_data"].(string)
	if !ok {
		return "", fmt.Errorf("missing json_data")
	}

	var newItems []EisenhowerMatrixItem
	if err := json.Unmarshal([]byte(jsonData), &newItems); err != nil {
		return "", fmt.Errorf("invalid json_data format; expected array of tasks: %w", err)
	}

	// 1. Load existing exercise
	var exercise models.EisenhowerMatrixExercise
	var existingData EisenhowerMatrixData
	err := database.DB.Where("user_id = ? AND session_id = ?", s.UserID, s.SessionDB.ID).First(&exercise).Error
	if err == nil && exercise.Data != "" {
		json.Unmarshal([]byte(exercise.Data), &existingData) //nolint:errcheck
	}

	// 2. Create a map of all tasks (existing + new) to merge by name
	taskMap := make(map[string]EisenhowerMatrixItem)

	// Add existing tasks first
	allExisting := append(existingData.UrgentImportant, existingData.NotUrgentImportant...)
	allExisting = append(allExisting, existingData.UrgentNotImportant...)
	allExisting = append(allExisting, existingData.NotUrgentNotImportant...)
	for _, item := range allExisting {
		taskMap[item.Task] = item
	}

	// Overwrite/Add with new items
	for _, item := range newItems {
		taskMap[item.Task] = item
	}

	// 3. Re-group into quadrants
	var mergedData EisenhowerMatrixData
	for _, item := range taskMap {
		switch item.Quadrant {
		case "urgent_important":
			mergedData.UrgentImportant = append(mergedData.UrgentImportant, item)
		case "not_urgent_important":
			mergedData.NotUrgentImportant = append(mergedData.NotUrgentImportant, item)
		case "urgent_not_important":
			mergedData.UrgentNotImportant = append(mergedData.UrgentNotImportant, item)
		case "not_urgent_not_important":
			mergedData.NotUrgentNotImportant = append(mergedData.NotUrgentNotImportant, item)
		}
	}

	mergedJSON, _ := json.Marshal(mergedData)
	strJSON := string(mergedJSON)

	if err == nil {
		exercise.Data = strJSON
		if err := database.DB.Save(&exercise).Error; err != nil {
			return "", fmt.Errorf("failed to save eisenhower matrix: %w", err)
		}
	} else {
		exercise = models.EisenhowerMatrixExercise{
			UserID:    s.UserID,
			SessionID: s.SessionDB.ID,
			Data:      strJSON,
		}
		if err := database.DB.Create(&exercise).Error; err != nil {
			return "", fmt.Errorf("failed to create eisenhower matrix exercise: %w", err)
		}
	}

	s.SyncEisenhowerMatrixVisuals()
	return `{"status": "success", "message": "Eisenhower Matrix updated (merged)"}`, nil
}

func (s *ChatSession) handleDeleteEisenhowerMatrixTasks(args map[string]interface{}) (string, error) {
	taskNamesRaw, ok := args["task_names"].([]interface{})
	if !ok {
		return "", fmt.Errorf("missing task_names")
	}

	// 1. Load existing exercise
	var exercise models.EisenhowerMatrixExercise
	if err := database.DB.Where("user_id = ? AND session_id = ?", s.UserID, s.SessionDB.ID).First(&exercise).Error; err != nil {
		return `{"status": "error", "message": "No active Eisenhower Matrix exercise found to delete from"}`, nil
	}

	var currentData EisenhowerMatrixData
	if exercise.Data != "" {
		json.Unmarshal([]byte(exercise.Data), &currentData) //nolint:errcheck
	}

	// 2. Create a map for easy filtering
	taskMap := make(map[string]EisenhowerMatrixItem)
	allCurrent := append(currentData.UrgentImportant, currentData.NotUrgentImportant...)
	allCurrent = append(allCurrent, currentData.UrgentNotImportant...)
	allCurrent = append(allCurrent, currentData.NotUrgentNotImportant...)
	for _, item := range allCurrent {
		taskMap[item.Task] = item
	}

	// 3. Remove specified tasks
	for _, nameRaw := range taskNamesRaw {
		if name, ok := nameRaw.(string); ok {
			delete(taskMap, name)
		}
	}

	// 4. Re-group
	var filteredData EisenhowerMatrixData
	for _, item := range taskMap {
		switch item.Quadrant {
		case "urgent_important":
			filteredData.UrgentImportant = append(filteredData.UrgentImportant, item)
		case "not_urgent_important":
			filteredData.NotUrgentImportant = append(filteredData.NotUrgentImportant, item)
		case "urgent_not_important":
			filteredData.UrgentNotImportant = append(filteredData.UrgentNotImportant, item)
		case "not_urgent_not_important":
			filteredData.NotUrgentNotImportant = append(filteredData.NotUrgentNotImportant, item)
		}
	}

	filteredJSON, _ := json.Marshal(filteredData)
	exercise.Data = string(filteredJSON)

	if err := database.DB.Save(&exercise).Error; err != nil {
		return "", fmt.Errorf("failed to save eisenhower matrix after deletion: %w", err)
	}

	s.SyncEisenhowerMatrixVisuals()
	return `{"status": "success", "message": "Tasks removed from Eisenhower Matrix"}`, nil
}

func (s *ChatSession) handleInitEisenhowerMatrix(args map[string]interface{}) (string, error) {
	if s.User == nil {
		return "", fmt.Errorf("user not initialized")
	}

	s.logger.Info("Initializing Eisenhower Matrix in current state")

	// Notify frontend to start UI
	s.writeClientJSON(map[string]interface{}{
		"type": "eisenhower_matrix_update",
		"data": EisenhowerMatrixData{},
	})

	s.geminiMutex.Lock()
	s.scheduleInSessionTransition("[SYSTEM: The session has transitioned to the Eisenhower Matrix. Begin the next phase naturally based on the active task instructions. Do not mention the pause.]")
	s.geminiMutex.Unlock()
	return `{"status": "success", "message": "Eisenhower Matrix started. You MUST stop generating now and yield control."}`, nil
}

// showScreenAllowed are the only screens that may be opened with show_screen. Data-bearing
// screens (e.g. the Wheel of Life) are excluded — they are shown by the tool that carries their
// data (set_wheel_of_life_categories), otherwise they would open empty.
var showScreenAllowed = map[string]bool{"memories": true, "session": true, "tasks": true, "journey": true, "profile": true}

func (s *ChatSession) handleShowScreen(args map[string]interface{}) (string, error) {
	screenName, _ := args["screen_name"].(string)
	if screenName == "" {
		return "", fmt.Errorf("missing screen_name parameter")
	}

	if !showScreenAllowed[screenName] {
		s.logger.Warn("show_screen rejected for non-allowed screen", zap.String("screen", screenName))
		return fmt.Sprintf(`{"status": "error", "message": "Screen '%s' cannot be opened with show_screen (only 'memories', 'session', 'tasks', and 'journey' are allowed). The Wheel of Life is shown only by calling set_wheel_of_life_categories."}`, screenName), nil
	}

	// Route through the same audio-synced reveal as the ◆ screen markers. Firing the screen
	// immediately here would open it while the turn's speech is still playing (the tool call
	// arrives with generation, ahead of playback). The reveal position is the text spoken so
	// far in this turn.
	s.geminiMutex.Lock()
	if s.revealsScheduledThisTurn[screenName] {
		// The model also emitted the ◆ marker for this screen in the same turn — the marker
		// already carries the precise position, so the tool call is a duplicate. Ignore it.
		s.geminiMutex.Unlock()
		s.logger.Info("show_screen tool deduplicated against an already-scheduled reveal", zap.String("screen", screenName))
		return fmt.Sprintf(`{"status": "success", "message": "Screen '%s' shown"}`, screenName), nil
	}
	s.geminiMutex.Unlock()

	s.logger.Info("show_screen tool scheduling audio-synced reveal", zap.String("screen", screenName))
	s.queueShowScreen(screenName, s.accumulatedText)
	return fmt.Sprintf(`{"status": "success", "message": "Screen '%s' shown"}`, screenName), nil
}

func (s *ChatSession) handleSaveIdealLifeVision(args map[string]interface{}) (string, error) {
	vision, ok := args["vision"].(string)
	if !ok || vision == "" {
		return "", fmt.Errorf("missing or empty vision parameter")
	}

	if s.User == nil || s.User.State == nil {
		return "", fmt.Errorf("user state not initialized")
	}

	// The exploration phase mandates two-to-three follow-up questions, but the model saved
	// the vision right after the user's very first answer — however rich — skipping the
	// deepening entirely (QA: "não me perguntou nem aquilo que eu iria sentir, nem com quem
	// é que eu estaria... perdemos um follow-up muito rico"). One user turn in this state
	// means no follow-up was ever answered. Reject once; a resumed session (whose counter
	// restarted) still advances after a single extra follow-up.
	s.geminiMutex.Lock()
	tooShallow := s.visionUserTurns < 2 && !s.visionEarlySaveRejected
	if tooShallow {
		s.visionEarlySaveRejected = true
	}
	s.geminiMutex.Unlock()
	if tooShallow {
		s.logger.Warn("Rejecting save_ideal_life_vision before any follow-up was answered",
			zap.Int("visionUserTurns", s.visionUserTurns))
		return `{"status": "error", "message": "REJECTED: the exploration is not finished — the user has only answered your opening question, and this phase REQUIRES two or three genuine follow-up questions before saving. Warmly ask ONE open follow-up about their vision now (for example who is with them in that life, what a morning there feels like, or which emotions define it), then WAIT for their answer. Only after they have explored further may you call save_ideal_life_vision with everything they shared."}`, nil
	}

	// The prompt orders a FRESH summary of what the user described in THIS session, but the
	// model sometimes copies the pre-filled 'Ideal Life Vision' from the profile verbatim
	// (QA: a re-onboarding run saved the old vision word-for-word, losing everything the
	// user had just shared). Language-agnostic check: normalized equality with the stored
	// value. Reject-once so a degenerate retry can still advance the session.
	if s.User.IdealLifeVision != nil && !s.visionCopyRejected {
		norm := func(t string) string { return strings.Join(strings.Fields(t), " ") }
		if norm(vision) == norm(*s.User.IdealLifeVision) {
			s.visionCopyRejected = true
			s.logger.Warn("Rejecting save_ideal_life_vision: vision copied verbatim from profile")
			return `{"status": "error", "message": "REJECTED: the 'vision' you passed is a word-for-word copy of the pre-filled Ideal Life Vision from the user's profile. That text is from the PAST — you must write a FRESH, concise summary of the ideal life the user actually described in THIS conversation (their own words and feelings from today), in the second person. Call save_ideal_life_vision again NOW with that new summary."}`, nil
		}
	}

	now := time.Now()
	// Update DB
	if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
		Updates(map[string]interface{}{
			"ideal_life_vision":        vision,
			"ideal_life_vision_set_at": now,
			"state":                    string(models.StateVisionWheelOfLife),
		}).Error; err != nil {
		return "", fmt.Errorf("failed to save ideal life vision and state: %w", err)
	}

	nextStateStr := string(models.StateVisionWheelOfLife)
	s.User.IdealLifeVision = &vision
	s.User.IdealLifeVisionSetAt = &now
	s.User.State = &nextStateStr
	s.logger.Info("[State change] ONBOARDING_IDEAL_LIFE_VISION -> ONBOARDING_WHEEL_OF_LIFE")

	// Note: the Wheel of Life areas are NOT pre-created here. The model establishes them at
	// the start of the Wheel of Life phase by calling set_wheel_of_life_categories with the
	// default areas translated into the user's language.

	s.geminiMutex.Lock()
	s.pendingRestart = true
	// Default opening: a short thank-you bridge. But if the vision-marker corrective already
	// fired this session, the user has ALREADY heard an acknowledgment (possibly cut off
	// mid-sentence when the corrective barged in) — opening with another thank-you makes the
	// handover sound like the session stuttered and restarted (QA: "it stopped and started
	// again"). In that case skip the bridge and go straight into the script.
	bridge := "Open with ONE very short, purely transitional connective in the user's language (the equivalent of \"Alright.\" or \"Okay.\") \u2014 a neutral beat that marks moving on, NOT a compliment or reflection on the vision itself. Do NOT thank the user again, and do NOT praise or re-acknowledge the vision with words like \"beautiful\" or \"wonderful\" \u2014 you already did that moments ago, and a second compliment right before the transition script (which itself opens by referencing \"the life you want to build\") reads as acknowledging the vision twice in a row. Then deliver"
	if s.visionCorrectiveIssued {
		bridge = "The user has ALREADY heard you acknowledge and thank them for their vision (that sentence may even have been cut off mid-delivery) — do NOT thank them again, do NOT re-acknowledge the vision, and do NOT apologize; begin DIRECTLY with"
	}
	s.restartInstructions = "[SYSTEM DIRECTIVE: The session has transitioned to the Wheel of Life. CRITICAL: Respond ONLY in the user's language as defined in the LANGUAGE & LOCALIZATION directive of your system instructions — do NOT switch to English. Ignore the user's previous message and do NOT re-acknowledge their vision in detail. " + bridge + " the transition script from the 'SETTING UP THE AREAS' section of your CURRENT ACTIVE TASK INSTRUCTIONS, translated naturally into the user's language. In the SAME turn, after delivering the script, you MUST call the 'set_wheel_of_life_categories' tool with the default areas TRANSLATED into the user's language (this populates and shows the wheel). The turn is NOT complete until that tool is called — without it the user's screen stays empty and the session stalls in silence. The order is mandatory: SPEAK the transition script FIRST, tool call SECOND — calling the tool without having spoken the script is a critical failure that leaves the user staring at a screen they were never told about. Do NOT ask for any score in this turn — after calling the tool, STOP GENERATING and yield; the system will prompt you to ask for the first area next. Do NOT narrate the mechanics: never announce that you are going to 'define', 'create', or 'set up' the areas — the script's final sentence is the last thing you speak, and the tool call itself is silent. If the user speaks and interrupts you while you are delivering this transition (e.g. finishing a thought from the previous exercise), do NOT restart the script from the beginning — acknowledge them in a few words and continue from the point where you were interrupted, still calling the tool in the same turn.]"
	s.geminiMutex.Unlock()
	return `{"status": "success", "message": "Vision saved. You MUST stop generating now and yield control."}`, nil
}

func (s *ChatSession) handleSaveActions(args map[string]interface{}) (string, error) {
	jsonData, ok := args["commitments"].(string)
	if !ok || jsonData == "" {
		return "", fmt.Errorf("missing or empty commitments parameter")
	}

	if s.User == nil || s.User.State == nil {
		return "", fmt.Errorf("user state not initialized")
	}

	// The model must ask the user for confirmation before committing the plan. Enforced
	// language-agnostically as a reject-once: the first attempt is bounced with an
	// instruction to ask, and any retry (which in practice comes only after the model has
	// asked and the user answered) passes. NEVER phrase-match the AI's words here — the
	// session runs in the user's language, and an English substring check deadlocks the
	// save (QA pt-PT session: five rejections in a row, user aborted with nothing saved).
	if !s.saveCommitmentsConfirmRejected {
		s.saveCommitmentsConfirmRejected = true
		s.logger.Warn("Rejecting first save_commitments call; confirmation question required")
		return `{"status": "error", "message": "You are NOT allowed to call 'save_commitments' yet. First ask the user, out loud in their language, whether they are ready to save these commitments and move to the session wrap-up, then yield. Once the user confirms, call save_commitments again and it will succeed."}`, nil
	}

	incoming, err := parseIncomingActions(jsonData)
	if err != nil {
		return "", fmt.Errorf("invalid commitments JSON: %w", err)
	}

	// Replace the user's plan (origin = plan commitments) with the provided set.
	if err := s.replacePlanCommitments(incoming); err != nil {
		return "", fmt.Errorf("failed to save commitments: %w", err)
	}

	// Advance onboarding state.
	if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
		Update("state", string(models.StateEmotionalClosing)).Error; err != nil {
		return "", fmt.Errorf("failed to update state: %w", err)
	}

	// Sync with DailyJourney snapshot for today
	s.SyncDailyJourneyCommitments()

	nextStateStr := string(models.StateEmotionalClosing)
	s.User.State = &nextStateStr
	s.logger.Info("[State change] -> EMOTIONAL_CLOSING")

	// Notify client to return to default session visualizer screen
	s.writeClientJSON(map[string]interface{}{
		"type": "show_screen",
		"data": map[string]string{
			"screen": "session",
		},
	})

	s.geminiMutex.Lock()
	s.scheduleInSessionTransition("[SYSTEM: The session has transitioned to the EMOTIONAL_CLOSING phase. Begin the next phase naturally based on the active task instructions. Do not mention the pause.]")
	s.geminiMutex.Unlock()
	return `{"status": "success", "message": "Commitments saved, moved to EMOTIONAL_CLOSING. You MUST stop generating now and yield control."}`, nil
}

func (s *ChatSession) handleUpdateCommitmentPlan(args map[string]interface{}) (string, error) {
	jsonData, ok := args["commitments"].(string)
	if !ok || jsonData == "" {
		return "", fmt.Errorf("missing or empty commitments parameter")
	}

	if s.User == nil || s.User.State == nil {
		return "", fmt.Errorf("user state not initialized")
	}

	incoming, err := parseIncomingActions(jsonData)
	if err != nil {
		return "", fmt.Errorf("invalid commitments JSON: %w", err)
	}

	// Replace the user's plan (origin = plan commitments) with the provided set.
	if err := s.replacePlanCommitments(incoming); err != nil {
		return "", fmt.Errorf("failed to save draft commitments: %w", err)
	}

	s.SyncCommitmentPlanVisuals()

	// Sync with DailyJourney snapshot for today
	s.SyncDailyJourneyCommitments()

	return `{"status": "success", "message": "Commitment plan updated visually"}`, nil
}

// handleAddCommitment adds one or more standalone (manual) commitments the user wants to track.
// Used by the check-in session when the user asks to add something to their list.
//
// Idempotent within a session: an incoming item whose normalized title matches a
// commitment already created during this session is silently skipped. The model
// sometimes saves the same step twice (a retry, or the "immediate movement" step
// echoing the main step) and the duplicate landed on the user's board and the
// session-end card twice (QA).
func (s *ChatSession) handleAddCommitment(args map[string]interface{}) (string, error) {
	jsonData, ok := args["commitments"].(string)
	if !ok || jsonData == "" {
		return "", fmt.Errorf("missing or empty commitments parameter")
	}
	incoming, err := parseIncomingActions(jsonData)
	if err != nil {
		return "", fmt.Errorf("invalid commitments JSON: %w", err)
	}
	existing := s.sessionCommitmentTitles()
	added := 0
	for _, it := range incoming {
		key := normalizeCommitmentTitle(it.Title)
		if existing[key] {
			continue
		}
		t := models.Commitment{
			ID:      it.ID,
			UserID:  s.UserID,
			Origin:  models.CommitmentOriginManual,
			Title:   it.Title,
			Type:    it.Type,
			Days:    models.IntSlice(it.Days),
			Date:    it.Date,
			EndedAt: it.endedAt(time.Now()),
			Done:    it.Done,
		}
		if err := database.DB.Create(&t).Error; err != nil {
			return "", fmt.Errorf("failed to add commitment: %w", err)
		}
		existing[key] = true
		added++
	}
	if added == 0 {
		return `{"status": "success", "message": "This commitment was already saved during this session — it is already on the user's board. Do NOT save it again; refer to the existing one."}`, nil
	}
	s.SyncDailyJourneyCommitments()
	// Data-bearing reveal: the user watches the commitment appear on the in-session board.
	s.emitSessionTasksPanel()
	return `{"status": "success", "message": "Commitment(s) added. The user can now SEE this commitment on their screen — you may refer to it naturally."}`, nil
}

// handleRemoveCommitment takes a commitment off the user's board, scoped to commitments
// created during THIS session: the tool exists for "remove that" moments — chiefly a
// commitment the model saved that the user never actually agreed to (QA: an invented
// commitment sat on the board, the user said "pode retirar", and there was no way to
// comply). Session-scoping keeps it from touching the user's wider plan.
func (s *ChatSession) handleRemoveCommitment(args map[string]interface{}) (string, error) {
	title, _ := args["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("missing or empty title parameter")
	}
	if s.SessionDB.StartTime.IsZero() {
		return `{"status": "error", "message": "No commitment with that title was created in this session."}`, nil
	}
	var rows []models.Commitment
	if err := database.DB.Where("user_id = ? AND created_at >= ?",
		s.UserID, s.SessionDB.StartTime).Find(&rows).Error; err != nil {
		return "", fmt.Errorf("failed to look up commitments: %w", err)
	}
	want := normalizeCommitmentTitle(title)
	for _, r := range rows {
		if normalizeCommitmentTitle(r.Title) == want {
			if err := database.DB.Delete(&models.Commitment{}, "id = ?", r.ID).Error; err != nil {
				return "", fmt.Errorf("failed to remove commitment: %w", err)
			}
			s.SyncDailyJourneyCommitments()
			s.emitSessionTasksPanel()
			return `{"status": "success", "message": "Commitment removed from the user's board. Confirm it naturally without mentioning any tool."}`, nil
		}
	}
	return `{"status": "error", "message": "No commitment with that title was created in this session. Pass the title exactly as it was saved."}`, nil
}

// handleSaveTopValues persists the Values session's outcome — the user's chosen top
// values — on the user row (they are durable personal data every later session leans
// on), mirrors them as a values memory for the memories screen, and shows them on the
// user's screen (session_values_update). REPLACES, never accumulates.
func (s *ChatSession) handleSaveTopValues(args map[string]interface{}) (string, error) {
	var values models.StringSlice
	if raw, ok := args["values"].([]interface{}); ok {
		for _, v := range raw {
			if vs, ok := v.(string); ok && strings.TrimSpace(vs) != "" {
				values = append(values, strings.TrimSpace(vs))
			}
		}
	}
	if len(values) == 0 {
		return "", fmt.Errorf("values must list the values the user chose")
	}

	if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
		Update("top_values", values).Error; err != nil {
		return "", fmt.Errorf("failed to save top values: %w", err)
	}
	if s.User != nil {
		s.User.TopValues = values
	}

	// The memories screen keeps a trace too — a short list, not a sentence, so it reads
	// as the user's own compass. Content is model-provided values in the user's language.
	memory := models.UserMemory{
		UserID:   s.UserID,
		Category: "values",
		Content:  strings.Join(values, " · "),
	}
	if err := database.DB.Create(&memory).Error; err != nil {
		s.logger.Error("failed to mirror top values as memory", zap.Error(err))
	}

	s.writeClientJSON(map[string]interface{}{
		"type": "session_values_update",
		"data": map[string]interface{}{"values": []string(values)},
	})
	return `{"status": "success", "message": "Top values saved. The user can now SEE their values on the screen — you may refer to them naturally."}`, nil
}

// sessionCommitmentTitles returns the normalized titles of every commitment created
// during this session, for duplicate suppression across the commitment-writing tools.
func (s *ChatSession) sessionCommitmentTitles() map[string]bool {
	titles := make(map[string]bool)
	if s.SessionDB.StartTime.IsZero() {
		return titles
	}
	var rows []models.Commitment
	if err := database.DB.Where("user_id = ? AND created_at >= ?",
		s.UserID, s.SessionDB.StartTime).Find(&rows).Error; err != nil {
		return titles
	}
	for _, r := range rows {
		titles[normalizeCommitmentTitle(r.Title)] = true
	}
	return titles
}

// incomingCommitment is the JSON shape coaching sessions pass to the commitment-writing tools.
type incomingCommitment struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Type    string  `json:"type"`
	Days    []int   `json:"days"`
	Date    *string `json:"date"`
	EndDate *string `json:"end_date"`
	Done    bool    `json:"done"`
}

// endedAt resolves the horizon a recurring commitment runs until. A habit with no end
// runs forever, which is rarely what a coach and user actually agree to — so sessions
// are asked to set one. Ignored for one-time commitments (their date IS their end), and
// a malformed or past date is dropped rather than silently ending the habit on creation.
func (it incomingCommitment) endedAt(now time.Time) *time.Time {
	if it.Type != "recurring" || it.EndDate == nil || strings.TrimSpace(*it.EndDate) == "" {
		return nil
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(*it.EndDate))
	if err != nil || !end.After(now) {
		return nil
	}
	return &end
}

func parseIncomingActions(jsonData string) ([]incomingCommitment, error) {
	var commitments []incomingCommitment
	if err := json.Unmarshal([]byte(jsonData), &commitments); err != nil {
		return nil, err
	}
	return commitments, nil
}

// replacePlanCommitments replaces the user's commitment plan (their origin=plan commitments) with the
// provided full set, generating ids for commitments that don't carry one.
//
// Items duplicating a commitment already created during this session under another
// origin are skipped: the delete above only clears plan rows, so when the model routes
// the same step through add_commitment (manual) and then update_commitment_plan, the
// second write used to leave two rows with the same title on the board (QA).
func (s *ChatSession) replacePlanCommitments(incoming []incomingCommitment) error {
	if err := database.DB.Where("user_id = ? AND origin = ?", s.UserID, models.CommitmentOriginPlan).
		Delete(&models.Commitment{}).Error; err != nil {
		return err
	}
	existing := s.sessionCommitmentTitles()
	for _, it := range incoming {
		if existing[normalizeCommitmentTitle(it.Title)] {
			continue
		}
		t := models.Commitment{
			ID:      it.ID,
			UserID:  s.UserID,
			Origin:  models.CommitmentOriginPlan,
			Title:   it.Title,
			Type:    it.Type,
			Days:    models.IntSlice(it.Days),
			Date:    it.Date,
			EndedAt: it.endedAt(time.Now()),
			Done:    it.Done,
		}
		if err := database.DB.Create(&t).Error; err != nil {
			return err
		}
	}
	return nil
}

// SyncDailyJourneyCommitments notifies the client that the user's master commitments have changed.
func (s *ChatSession) SyncDailyJourneyCommitments() {
	// Every commitment-writing path funnels through here, so this is the one place to tell the
	// client the board changed.
	s.writeClientJSON(map[string]interface{}{
		"type": "tasks_updated",
	})
}

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStringPtrs(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (s *ChatSession) handleTerminateSession(args map[string]interface{}) (string, error) {
	// A model-initiated terminate with no goodbye spoken in the same turn hangs up on
	// the user mid-conversation (QA: "I have to go, but I'll come back" → instant
	// disconnect, no farewell, no "see you soon"). Bounce it ONCE with instructions to
	// speak first; the retry always passes so the session can never soft-lock. The
	// backend's own auto-terminate at ENDING TURN_COMPLETE passes nil args (the goodbye
	// was already spoken in that flow) and is exempt, as is the post-disconnect path.
	// Also exempt once the session insight is saved: the closing is underway, the
	// goodbye often lands in the turn BEFORE the terminate call (Gemini splits them),
	// and bouncing a legitimate end-of-session terminate left the whole close hanging —
	// no session_terminated, no card (QA: the Values session died exactly this way).
	closingUnderway := s.SessionDB.UserSessionInsight != nil && *s.SessionDB.UserSessionInsight != ""
	if args != nil && !s.clientGone && !s.terminateNoSpeechBounced && !closingUnderway &&
		strings.TrimSpace(s.accumulatedText) == "" && strings.TrimSpace(s.accumulatedPartsText) == "" {
		s.terminateNoSpeechBounced = true
		s.logger.Warn("terminate_session called with no goodbye spoken this turn; bouncing once")
		return `{"status": "error", "message": "You called terminate_session WITHOUT having spoken any goodbye in this turn — the session would cut off abruptly, mid-conversation, with no farewell. FIRST speak a short, warm goodbye in the user's language, ending with the equivalent of 'See you soon.', and THEN call terminate_session again in that SAME turn."}`, nil
	}

	s.logger.Info("Terminating coaching session")

	// Final catch-all, WITH the next-session card: for a single-prompt session (Movement)
	// this is the only reveal point there is, so it must carry everything. For a staged
	// session (Vision) the '◆▧' marker in the goodbye's bridge sentence normally already
	// added the card by now; this only does anything if that marker somehow never fired.
	s.emitAndPersistSessionSummary(true)

	if s.User != nil {
		// Sessions return the user to the CHECKIN resting state, where the journey endpoint
		// decides the next deep session from session history. The onboarding intro is the
		// exception: it hands over to Vision, so a user who declines to continue right now
		// is parked at the start of Vision instead. Sending them to CHECKIN would strand
		// them — their next session would be resolved from history, and the intro is far
		// too short to register as a completed session.
		nextState := models.StateCheckin
		if s.SessionType == api.SessionTypeOnboarding {
			nextState = models.StateVisionIdealLife
		}
		nextStateStr := string(nextState)
		if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
			Update("state", nextStateStr).Error; err != nil {
			s.logger.Error("Failed to update user state on session termination", zap.Error(err))
		} else {
			s.User.State = &nextStateStr
			s.logger.Info("[State change on termination]", zap.String("new_state", nextStateStr))
		}
	}

	// Where the client should land once it closes the session. The intro has no synthesis
	// screen to show (nothing was explored yet), so a user who declined the first exercise
	// would otherwise be left staring at the session screen — send them to the growth
	// screen, where the exercise waits for them. Full sessions keep their summary panel.
	if s.SessionType == api.SessionTypeOnboarding {
		s.writeClientJSON(map[string]interface{}{
			"type": "show_screen",
			// "at" distinguishes this from the tour's ◆▥ reveal, which navigates as it is
			// spoken: here the client must stay on the session screen until the goodbye has
			// finished playing, then land on growth as it tears the session down.
			"data": map[string]string{"screen": "growth", "at": "session_end"},
		})
	}

	// Flag session for termination after the current stream chunk finishes processing
	s.pendingShutdown = true
	return `{"status": "success", "message": "Session terminated. Stop generating tools and content."}`, nil
}

func (s *ChatSession) handleSaveFocus(args map[string]interface{}) (string, error) {
	area, ok := args["area"].(string)
	if !ok || area == "" {
		return "", fmt.Errorf("missing or empty area parameter")
	}

	if s.User == nil || s.User.State == nil {
		return "", fmt.Errorf("user state not initialized")
	}

	if s.User.State != nil && *s.User.State != string(models.StateVisionMetaphor) {
		stateOrder := map[string]int{
			string(models.StateVisionMetaphor):   0,
			string(models.StateEmotionalClosing): 1,
			string(models.StateEndingSession):    2,
			string(models.StateCheckin):          3,
		}
		currentOrder, hasCurrent := stateOrder[*s.User.State]
		if hasCurrent && currentOrder >= 1 {
			s.logger.Info("Ignore save_focus: already progressed beyond METAPHOR", zap.String("state", *s.User.State))
			return fmt.Sprintf(`{"status": "success", "message": "Focus area already saved. Current state is %s"}`, *s.User.State), nil
		}
	}

	// Update User State and Focus Area (onboarding metaphor → onboarding emotional closing)
	if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
		Updates(map[string]interface{}{
			"state":      string(models.StateVisionEmotionalClosing),
			"focus_area": area,
		}).Error; err != nil {
		return "", fmt.Errorf("failed to update user state and focus area: %w", err)
	}

	nextStateStr := string(models.StateVisionEmotionalClosing)
	s.User.State = &nextStateStr
	s.User.FocusArea = &area
	s.logger.Info("[State change] ONBOARDING_METAPHOR -> ONBOARDING_EMOTIONAL_CLOSING and FocusArea saved", zap.String("focus_area", area))

	s.geminiMutex.Lock()
	s.scheduleInSessionTransition("[SYSTEM: The session has transitioned to the EMOTIONAL_CLOSING phase. The user just committed to their priority area and shared why it matters — this is a meaningful moment, and it must NOT feel like the subject was changed abruptly. If you have NOT yet verbally responded to the reasoning they gave (including answering any question they asked you, e.g. 'it is the area with the lowest score, right?'), open with ONE short, warm sentence doing exactly that — their words must never be met with silence. If you already acknowledged their reasoning, do NOT stack a second validation. Then bridge gently into the active task instructions (the final insight question). Do not mention the pause.]")
	s.geminiMutex.Unlock()
	return `{"status": "success", "message": "Focus area saved, moved to EMOTIONAL_CLOSING. You MUST stop generating now and yield control."}`, nil
}

// handleSaveVisionCommitment persists the single first-step commitment the user names at
// Step 4 of the Vision closing (the "what's one thing you could do right away" question) —
// see the toolSaveVisionCommitment description. Origin is "plan": this is the seed of the
// user's coaching plan, and the Movement session's update_commitment_plan naturally
// supersedes it once the structured plan is built. Unlike add_commitment (the check-in
// session's board tool), this does NOT call emitSessionTasksPanel — there is no in-session
// board here, only the quiet closing reflection; the commitment surfaces later via
// session_summary and the Journey screen.
//
// Idempotent per session (REPLACES, never accumulates): the model sometimes jumps ahead of
// the user's actual answer and calls this before they have replied (QA: it invented "the
// user did not name a concrete action" as the commitment before the user had said a word).
// The prompt now tells the model to correct itself with a second call once it has the real
// answer — deleting this session's prior plan-origin capture before inserting the new one
// means that correction always wins instead of leaving both rows, or the fabricated one, in
// the summary the user actually sees.
func (s *ChatSession) handleSaveVisionCommitment(args map[string]interface{}) (string, error) {
	commitment, ok := args["commitment"].(string)
	commitment = strings.TrimSpace(commitment)
	if !ok || commitment == "" {
		return "", fmt.Errorf("missing or empty commitment parameter")
	}

	if !s.SessionDB.StartTime.IsZero() {
		database.DB.Where("user_id = ? AND origin = ? AND created_at >= ?",
			s.UserID, models.CommitmentOriginPlan, s.SessionDB.StartTime).Delete(&models.Commitment{})
	}

	now := time.Now().In(s.Location)
	today := now.Format("2006-01-02")
	t := models.Commitment{
		UserID: s.UserID,
		Origin: models.CommitmentOriginPlan,
		Title:  commitment,
		Type:   "one_time",
		Date:   &today,
	}
	// An ongoing intention ("go to bed by 22h30") is a daily habit, not a one-off for
	// today — saved as one_time it shows up once and vanishes, which is not what the
	// user promised (QA). Daily for two weeks: a defined horizon the next session can
	// revisit, consistent with the recurring-commitment rule everywhere else.
	if recurring, _ := args["recurring"].(bool); recurring {
		end := now.AddDate(0, 0, 14)
		t.Type = "recurring"
		t.Days = models.IntSlice{1, 2, 3, 4, 5, 6, 7}
		t.Date = nil
		t.EndedAt = &end
	}
	if err := database.DB.Create(&t).Error; err != nil {
		return "", fmt.Errorf("failed to save commitment: %w", err)
	}
	s.SyncDailyJourneyCommitments()
	return `{"status": "success", "message": "Commitment saved. If you have not yet received the user's actual answer, call this tool AGAIN once you have it — this call replaces the previous one."}`, nil
}

func (s *ChatSession) handleRestartSessionWithSummary(args map[string]interface{}) (string, error) {
	summary, ok := args["summary"].(string)
	if !ok || summary == "" {
		return "", fmt.Errorf("missing or empty summary parameter")
	}

	s.geminiMutex.Lock()
	lastRestart := s.lastRestartAt
	if !lastRestart.IsZero() && time.Since(lastRestart) < 2*time.Minute {
		s.logger.Warn("Ignoring restart_session_with_summary: restarted too recently", zap.Duration("elapsed", time.Since(lastRestart)))
		s.geminiMutex.Unlock()
		return `{"status": "error", "message": "A connection restart was performed recently. Do NOT call restart_session_with_summary again. Please proceed with the conversation normally based on the summary."}`, nil
	}

	s.conversationSummary = summary
	s.pendingRestart = true
	s.latestSessionHandle = "" // Don't use previous sessionID
	if s.GeminiWs != nil {
		s.logger.Info("Closing GeminiWs to force connection restart loop")
		s.GeminiWs.Close()
	}
	s.geminiMutex.Unlock()

	// Clear DB session handle to avoid resuming stale session
	if s.User != nil {
		database.DB.Model(s.User).Updates(map[string]interface{}{
			"latest_session_handle":    nil,
			"latest_session_handle_at": nil,
		})
	}

	return `{"status": "success", "message": "Summary saved, restarting connection..."}`, nil
}

func (s *ChatSession) handleRequestRecommendations(args map[string]interface{}) (string, error) {
	topic, _ := args["topic"].(string)
	searchQuery, _ := args["search_query"].(string)
	contextStr, _ := args["context"].(string)

	if topic == "" || searchQuery == "" || contextStr == "" {
		return `{"status": "error", "message": "Missing required arguments: topic, search_query, and context are all required"}`, nil
	}

	// 1. Instantly check if user has a registered email address.
	if s.User == nil || s.User.Email == nil || *s.User.Email == "" {
		return `{"status": "error", "message": "User does not have a registered email address. Please ask the user to provide their email address so you can send the recommendations."}`, nil
	}

	email := *s.User.Email
	name := "there"
	if s.User.Name != nil && *s.User.Name != "" {
		fields := strings.Fields(*s.User.Name)
		if len(fields) > 0 {
			name = fields[0]
		}
	}

	locale := "en-US"
	if s.User.PreferredLanguage != nil && *s.User.PreferredLanguage != "" {
		locale = *s.User.PreferredLanguage
	}

	// 2. Spawn a background goroutine to perform search grounding and send the email
	go func(userID, sessionID, topic, searchQuery, contextStr, email, name, locale string) {
		s.logger.Info("Starting background recommendations search and compilation",
			zap.String("topic", topic),
			zap.String("search_query", searchQuery),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		recs, err := GenerateRecommendations(ctx, topic, searchQuery, contextStr, userID, sessionID)
		if err != nil {
			s.logger.Error("Failed to generate grounded recommendations in background", zap.Error(err))
			return
		}

		if len(recs) == 0 {
			s.logger.Warn("Background search returned zero recommendations")
			return
		}

		// Save recommendations to database
		if err := database.DB.Create(&recs).Error; err != nil {
			s.logger.Error("Failed to save recommendations to database", zap.Error(err))
		} else {
			s.logger.Info("Saved recommendations to database successfully", zap.Int("count", len(recs)))
		}

		// Map models.Recommendation to notification.RecommendationItem
		notificationItems := make([]notification.RecommendationItem, len(recs))
		for i, r := range recs {
			notificationItems[i] = notification.RecommendationItem{
				Title:       r.Title,
				Type:        r.Type,
				Author:      r.Author,
				URL:         r.URL,
				Description: r.Description,
			}
		}

		// Send email using notification service
		if notification.GlobalNotificationService == nil {
			s.logger.Error("Notification service is not initialized, cannot send recommendations email")
			return
		}

		err = notification.GlobalNotificationService.SendRecommendationsEmail(email, name, notificationItems, locale)
		if err != nil {
			s.logger.Error("Failed to send recommendations email in background", zap.Error(err))
		} else {
			s.logger.Info("Successfully sent recommendations email in background", zap.String("user_id", s.UserID))
		}
	}(s.UserID, s.SessionDB.ID, topic, searchQuery, contextStr, email, name, locale)

	return `{"status": "success", "message": "Recommendations compilation started"}`, nil
}

func (s *ChatSession) handleScheduleNotifications(args map[string]interface{}) (string, error) {
	s.saveGrowthFocus(args)

	// Tolerant by design, like the commitment tools: the model sometimes serializes the
	// array as a JSON STRING instead of passing a native array (QA: two sessions ended
	// with zero notifications scheduled because of exactly this). Unwrap it.
	if str, isStr := args["notifications"].(string); isStr && strings.TrimSpace(str) != "" {
		var parsed []interface{}
		if err := json.Unmarshal([]byte(str), &parsed); err == nil {
			args["notifications"] = parsed
		}
	}

	notificationsRaw, ok := args["notifications"].([]interface{})
	if ok && len(notificationsRaw) == 0 {
		// An empty list is a legitimate answer, not a failure: when a session is abandoned
		// before the user says anything there is genuinely nothing personal to schedule, and
		// the model correctly passed none (QA). Returning an error there punished the right
		// call and invited it to invent generic messages instead. The quote category and
		// theme were already stored by saveGrowthFocus above.
		s.logger.Info("schedule_notifications called with no notifications; nothing to schedule")
		return `{"status": "success", "message": "No notifications scheduled."}`, nil
	}
	if !ok {
		return `{"status": "error", "message": "missing or empty notifications array"}`, nil
	}

	var savedCount int
	for _, nRaw := range notificationsRaw {
		nMap, ok := nRaw.(map[string]interface{})
		if !ok {
			continue
		}

		title, _ := nMap["title"].(string)
		message, _ := nMap["message"].(string)
		delayHoursFloat, _ := nMap["delay_hours"].(float64)
		delayHours := int(delayHoursFloat)
		sendAt, _ := nMap["send_at"].(string)
		timeSensitive, _ := nMap["time_sensitive"].(bool)

		if title == "" || message == "" {
			continue
		}

		// An absolute local time is what the model should be using: "the morning of the
		// exam" is a real moment, and hours-from-now cannot express it reliably — the
		// model has no exact anchor for when the session ended. Fall back to delay_hours
		// only when no moment was given.
		scheduledAt := resolveNotificationTime(sendAt, delayHours, s.Location)
		if scheduledAt.IsZero() {
			continue
		}

		notification := models.Notification{
			UserID:        s.UserID,
			SessionID:     s.SessionDB.ID,
			Title:         title,
			Message:       message,
			DelayHours:    delayHours,
			ScheduledAt:   scheduledAt,
			TimeSensitive: timeSensitive,
		}

		if err := database.DB.Create(&notification).Error; err != nil {
			s.logger.Error("Failed to save notification to database", zap.Error(err))
			continue
		}
		savedCount++
	}

	s.logger.Info("Saved notifications", zap.Int("count", savedCount))

	// Signal to Run() that notifications have been scheduled
	select {
	case <-s.notificationsDone:
		// already closed
	default:
		close(s.notificationsDone)
	}

	return fmt.Sprintf(`{"status": "success", "message": "Scheduled %d notifications"}`, savedCount), nil
}

// saveGrowthFocus persists the model's end-of-session picks for the coming
// days' Journey screen: the quote category the daily quote is filtered by and
// the visual theme the frontend renders (GET /journey). Values are
// normalized and invalid ones dropped — the picks are best-effort and must
// never fail the notifications flow. Column-scoped update: a full-row user
// Save here would clobber concurrent balance debits.
func (s *ChatSession) saveGrowthFocus(args map[string]interface{}) {
	updates := map[string]interface{}{}
	if raw, ok := args["quote_category"].(string); ok && raw != "" {
		if cat := quote.NormalizeCategory(raw); cat != "" {
			updates["journey_quote_category"] = cat
		} else {
			s.logger.Warn("Ignoring unknown quote category from schedule_notifications", zap.String("quote_category", raw))
		}
	}
	if raw, ok := args["theme"].(string); ok && raw != "" {
		if theme := models.NormalizeJourneyTheme(raw); theme != "" {
			updates["journey_theme"] = theme
		} else {
			s.logger.Warn("Ignoring unknown journey theme from schedule_notifications", zap.String("theme", raw))
		}
	}
	if len(updates) == 0 {
		return
	}
	if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).Updates(updates).Error; err != nil {
		s.logger.Error("Failed to save journey focus", zap.Error(err))
		return
	}
	s.logger.Info("Saved journey focus", zap.Any("updates", updates))
}

// handleSaveProfileDetails completes the registration details the onboarding intro
// collects — country, date of birth and gender. It is tolerant by design: the model may
// call it once per answer as the conversation goes, so each field is applied only when
// present and valid, and a bad value is reported back specifically so the model can ask
// again rather than silently dropping the answer.
// isAgeDerivedBirthDate reports whether a birth date carries today's exact day and month —
// the signature of an age subtracted from today's date rather than a birthday the user
// actually gave. See the caller for why this is worth catching.
func isAgeDerivedBirthDate(dob, now time.Time) bool {
	return dob.Day() == now.Day() && dob.Month() == now.Month()
}

// tz is the session's timezone, falling back to UTC. The WS route always supplies one
// (GetTimezoneLocation defaults to UTC), but a directly-constructed session — as in tests —
// leaves it nil, and time.Time.In(nil) panics.
func (s *ChatSession) tz() *time.Location {
	if s.Location == nil {
		return time.UTC
	}
	return s.Location
}

func (s *ChatSession) handleSaveProfileDetails(args map[string]interface{}) (string, error) {
	if s.User == nil {
		return "", fmt.Errorf("user not loaded")
	}

	updates := map[string]interface{}{}
	var problems []string

	if raw, ok := args["country_code"].(string); ok && strings.TrimSpace(raw) != "" {
		code := strings.ToUpper(strings.TrimSpace(raw))
		if len(code) != 2 {
			problems = append(problems, fmt.Sprintf("country_code %q is not a 2-letter ISO 3166-1 alpha-2 code (e.g. 'PT')", raw))
		} else {
			updates["country"] = code
		}
	}

	if raw, ok := args["date_of_birth"].(string); ok && strings.TrimSpace(raw) != "" {
		dob, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("date_of_birth %q is not in YYYY-MM-DD format", raw))
		case dob.After(time.Now()):
			problems = append(problems, "date_of_birth is in the future; ask the user again")
		case time.Since(dob) > 120*365*24*time.Hour:
			problems = append(problems, "date_of_birth is implausibly long ago; ask the user again")
		case !s.dobTodayRejected && isAgeDerivedBirthDate(dob, time.Now().In(s.tz())):
			// Fingerprint of an age silently converted into a date: the model asks "how old
			// are you?", hears a number, and subtracts it from today — leaving today's day and
			// month attached to a guessed year (QA: "34" became 1992-08-02 on 2 August). The
			// day and month are pure invention, and the user sees them on their profile.
			// Rejected once per session, so a user genuinely born on this day-and-month can
			// simply confirm and have it saved on the retry.
			s.dobTodayRejected = true
			s.logger.Warn("Rejecting a date of birth whose day and month are today's; likely derived from an age",
				zap.String("date_of_birth", raw))
			problems = append(problems, "date_of_birth falls on today's exact day and month, which is what happens when an AGE is subtracted from today's date instead of the user actually giving their birthday. Do NOT guess it. Ask the user warmly for the day and month they were born, and only save once they have told you — unless they really were born on this day and month, in which case save it again as it is")
		default:
			updates["date_of_birth"] = dob
		}
	}

	if raw, ok := args["gender"].(string); ok && strings.TrimSpace(raw) != "" {
		gender := strings.ToLower(strings.TrimSpace(raw))
		if gender != "male" && gender != "female" {
			problems = append(problems, fmt.Sprintf("gender %q must be exactly 'male' or 'female'", raw))
		} else {
			updates["gender"] = gender
		}
	}

	if len(updates) > 0 {
		// Targeted column updates, never a full-row Save: balance_seconds is owned by the
		// balance ledger and a full Save here would clobber a concurrent debit.
		if err := database.DB.Model(&models.User{}).Where("id = ?", s.UserID).
			Updates(updates).Error; err != nil {
			s.logger.Error("failed to save profile details", zap.Error(err))
			return "", fmt.Errorf("could not save the profile details")
		}
		// Keep the in-memory user in sync so the rest of the turn (and the persona's
		// gendered agreement) sees the new values without a reload.
		if v, ok := updates["country"].(string); ok {
			s.User.Country = &v
		}
		if v, ok := updates["date_of_birth"].(time.Time); ok {
			s.User.DateOfBirth = &v
		}
		if v, ok := updates["gender"].(string); ok {
			s.User.Gender = &v
		}

		var dobStr *string
		if s.User.DateOfBirth != nil {
			formattedDob := s.User.DateOfBirth.Format("2006-01-02")
			dobStr = &formattedDob
		}

		s.writeClientJSON(map[string]interface{}{
			"type": "onboarding",
			"data": map[string]interface{}{
				"country":     s.User.Country,
				"dateOfBirth": dobStr,
				"gender":      s.User.Gender,
			},
		})

		s.logger.Info("Saved onboarding profile details", zap.Int("fields", len(updates)))
	}

	if len(problems) > 0 {
		return fmt.Sprintf(`{"status": "error", "message": "Some details were not saved: %s. Ask the user again for those, then call save_profile_details with the corrected values. Do NOT mention this error to the user."}`,
			strings.Join(problems, "; ")), nil
	}

	missing := missingProfileFields(s.User)
	if len(missing) > 0 {
		return fmt.Sprintf(`{"status": "success", "message": "Saved. Still missing: %s. Continue the conversation and ask for those naturally."}`,
			strings.Join(missing, ", ")), nil
	}
	return `{"status": "success", "message": "All registration details are complete. Continue to the privacy explanation."}`, nil
}

// missingProfileFields lists the registration details still absent, so the tool result
// can tell the model exactly what is left instead of it having to track that itself.
func missingProfileFields(u *models.User) []string {
	var missing []string
	if u.Country == nil || *u.Country == "" {
		missing = append(missing, "country")
	}
	if u.DateOfBirth == nil {
		missing = append(missing, "date of birth")
	}
	if u.Gender == nil || *u.Gender == "" {
		missing = append(missing, "gender")
	}
	return missing
}

// notificationQuietStart/End bound the hours a scheduled message may arrive. A message
// that wakes someone at 3am undoes far more than it delivers, and the model cannot be
// relied on to never produce one.
const (
	notificationQuietStart = 22 // 22:00
	notificationQuietEnd   = 7  // 07:00
)

// resolveNotificationTime turns what the model asked for into an absolute instant.
// sendAt is "YYYY-MM-DD HH:MM" in the user's local time and wins when present;
// delayHours is the fallback. loc is the user's timezone (nil = UTC).
//
// Returns the zero time when neither is usable, which drops the notification — better
// than guessing a moment for a message whose whole value is arriving at the right one.
func resolveNotificationTime(sendAt string, delayHours int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now()

	if sendAt = strings.TrimSpace(sendAt); sendAt != "" {
		// Accept the space form the schema asks for and the ISO form models tend to emit.
		for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", time.RFC3339} {
			parsed, err := time.ParseInLocation(layout, sendAt, loc)
			if err != nil {
				continue
			}
			// A moment already gone is not a schedule. Dropping beats sending "good luck
			// tomorrow" an hour after the thing happened.
			if parsed.After(now) {
				return shiftOutOfQuietHours(parsed, loc)
			}
			return time.Time{}
		}
	}

	if delayHours > 0 {
		return shiftOutOfQuietHours(now.Add(time.Duration(delayHours)*time.Hour), loc)
	}
	return time.Time{}
}

// shiftOutOfQuietHours moves a scheduled instant to the next 07:00 local time when it
// falls in the night. Deliberately forward only: waking someone is worse than being late.
func shiftOutOfQuietHours(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	hour := local.Hour()
	if hour >= notificationQuietStart {
		next := local.AddDate(0, 0, 1)
		return time.Date(next.Year(), next.Month(), next.Day(), notificationQuietEnd, 0, 0, 0, loc)
	}
	if hour < notificationQuietEnd {
		return time.Date(local.Year(), local.Month(), local.Day(), notificationQuietEnd, 0, 0, 0, loc)
	}
	return t
}
