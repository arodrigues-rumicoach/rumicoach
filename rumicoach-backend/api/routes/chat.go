package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/handlers"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/internal/services/chat"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// wsAuthProtocol is the sentinel WebSocket subprotocol that precedes the access
// token in the Sec-WebSocket-Protocol header. The server echoes only this sentinel
// back in the handshake, never the token.
const wsAuthProtocol = "rumi-auth"

var upgrader = websocket.Upgrader{
	// Echo back only the sentinel subprotocol (the client also offers the token as a
	// second protocol value; that one is never reflected).
	Subprotocols: []string{wsAuthProtocol},
	CheckOrigin:  checkOrigin,
}

// checkOrigin restricts which browser origins may open the socket. Native app clients
// send no Origin header and are always allowed. Enforcement is opt-in: it activates
// only once WS_ALLOWED_ORIGINS is configured, so environments that haven't set it keep
// the previous permissive behavior rather than silently breaking web clients. When set,
// the allowlist is WS_ALLOWED_ORIGINS plus FRONTEND_URL.
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // native (non-browser) client — no Origin header
	}

	configured := strings.TrimSpace(config.AppConfig.WSAllowedOrigins)
	if configured == "" {
		return true // enforcement not enabled for this environment
	}

	allowed := strings.Split(configured, ",")
	if config.AppConfig.FrontendURL != "" {
		allowed = append(allowed, config.AppConfig.FrontendURL)
	}
	for _, a := range allowed {
		if strings.EqualFold(origin, strings.TrimSpace(a)) {
			return true
		}
	}
	return false
}

// wsToken extracts the access token from the Sec-WebSocket-Protocol header
// (["rumi-auth", "<token>"]). It falls back to the legacy ?token= query parameter so
// already-distributed clients keep working during rollout; new clients must use the
// header, which — unlike the URL — is not written to access logs or browser history.
func wsToken(r *http.Request) string {
	protocols := websocket.Subprotocols(r)
	for i, p := range protocols {
		if p == wsAuthProtocol && i+1 < len(protocols) {
			return protocols[i+1]
		}
	}
	return r.URL.Query().Get("token")
}

// tooLowToStart reports whether a user must be refused a session for want of
// balance. The introductory sessions — the onboarding intro and the Vision session
// it hands over to — are free until they have produced what they exist to produce,
// decided server-side and never from the client-supplied session_type.
//
// free is the result of balance.FreeSessionAvailable for this user.
func tooLowToStart(user *models.User, free bool) bool {
	return !free && user.BalanceSeconds < balance.MinimumStartSeconds
}

// wsCloseInsufficientBalance is the close code the socket is shut with when a
// session is refused for balance. 4402 is in the private-use range (4000-4999) and
// echoes the HTTP 402 this used to be.
const wsCloseInsufficientBalance = 4402

// refuseForBalance tells an already-upgraded client why it is not getting a session
// and hangs up.
//
// This runs *after* the upgrade, which looks wrong for what is conceptually a
// pre-flight rejection, and is the entire point. A handshake failed with an HTTP
// status is invisible to every WebSocket client there is — the browser and React
// Native APIs both surface it as an untyped error event with no status on it — so
// refusing before the upgrade meant the app could only show "connection failed"
// unless it duplicated this rule against GET /me and pre-empted the call. It did,
// and the two copies drifted, which is what put a paywall in front of users who
// were still being onboarded. Refusing over the socket puts the reason somewhere
// the client can actually read, so the rule can live here alone.
//
// The message frame carries the reason; the close code repeats it, so a client that
// is torn down before it reads the frame still learns why from onclose.
func refuseForBalance(ws *websocket.Conn, logger *zap.Logger, userID string) {
	logger.Info("Session refused: insufficient balance", zap.String("userID", userID))

	if err := ws.WriteJSON(map[string]string{
		"type":    "error",
		"code":    string(apierror.CodeInsufficientBalance),
		"message": "Not enough session minutes to start a session",
	}); err != nil {
		logger.Warn("Could not write balance refusal", zap.Error(err), zap.String("userID", userID))
	}
	ws.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(wsCloseInsufficientBalance, string(apierror.CodeInsufficientBalance)))
	ws.Close()
}

func RegisterChatRoutes(router chi.Router, logger *zap.Logger) {
	router.Get("/ws/chat", ChatWebsocketEndpoint(logger))
}

func ChatWebsocketEndpoint(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := wsToken(r)
		if token == "" {
			apierror.Write(w, http.StatusUnauthorized, apierror.CodeAuthTokenMissing, "Authentication token missing")
			return
		}

		claims, err := auth.VerifyToken(token)
		if err != nil {
			apierror.Write(w, http.StatusUnauthorized, apierror.CodeUnauthenticated, "Authentication failed")
			return
		}

		if !auth.RegionAllowed(claims) {
			apierror.Write(w, http.StatusForbidden, apierror.CodeWrongRegion, "Wrong region: this account's data is stored in another region")
			return
		}

		userID := claims.Subject

		clientWs, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error("WebSocket Upgrade Error", zap.Error(err))
			return
		}

		// Balance pre-flight: users whose introductory sessions are finished and who
		// have less than a full minute left are refused over the socket, immediately
		// after the upgrade and before any session is created — see refuseForBalance
		// for why the upgrade has to happen first. BALANCE_ENFORCED remains as an
		// escape hatch, but it now defaults on — the products it was waiting for exist.
		//
		// This asks the account-level question (is the opening pair unfinished?) rather
		// than the per-session one (is THIS session free?), because the session type is
		// resolved inside the chat service, from users.state, and does not exist yet
		// here. The client-supplied session_type is never consulted — a client could
		// name a free type at will.
		//
		// The two questions agree: resolveSessionType routes an unfinished opening pair
		// to onboarding/Vision, so a user this lets through cannot land on a billable
		// type. Where they could diverge this is the more permissive one, which is the
		// right direction for a pre-flight — the debit at session end is decided by
		// balance.FreeSessionAvailable, on the resolved type.
		//
		// This is the only enforcement there is. The app used to mirror the rule off
		// GET /me to pre-empt the call; it no longer does, and nothing here may assume
		// any client-side check ran.
		if config.AppConfig.BalanceEnforced {
			var user models.User
			err := database.DB.Select("balance_seconds").Where("id = ?", userID).First(&user).Error
			free, freeErr := balance.OpeningPairUnfinished(r.Context(), userID)
			switch {
			case err != nil || freeErr != nil:
				// Fail-open on DB errors: a balance-check hiccup must not lock
				// users out. The debit at session end still records usage.
				logger.Error("Balance check failed, allowing session",
					zap.Error(errors.Join(err, freeErr)), zap.String("userID", userID))
			case tooLowToStart(&user, free):
				refuseForBalance(clientWs, logger, userID)
				return
			}
		}

		sessionType := api.SessionType(r.URL.Query().Get("session_type"))

		logger.Info("WebSocket connection established", zap.String("userID", userID), zap.String("sessionType", string(sessionType)))

		loc := handlers.GetTimezoneLocation(r)

		// Delegate to the chat session manager
		session := chat.NewChatSession(userID, sessionType, clientWs, loc, logger)
		go session.Run()
	}
}
