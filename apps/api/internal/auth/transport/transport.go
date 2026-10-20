// Package transport implements the /auth and /me operations of api.StrictServerInterface.
// Responses that set cookies or redirect are small types implementing the generated
// Visit<Op>Response interfaces (ADR-061).
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"archmap/api/internal/auth"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/reqinfo"
)

const flowTTL = 10 * time.Minute

type Handler struct {
	svc *auth.Service
	log *slog.Logger
}

func NewHandler(svc *auth.Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func toMe(a authctx.Actor) api.Me {
	return api.Me{
		Id: a.ID, Email: a.Email, Name: a.Name, Role: api.Role(a.Role),
		Provider: a.Provider, PasswordMustChange: a.PasswordMustChange,
	}
}

func toSession(s auth.Session) api.Session {
	return api.Session{Me: toMe(s.Actor), CsrfToken: s.CSRFToken, ExpiresAt: s.ExpiresAt.UTC()}
}

func (h *Handler) token(ctx context.Context) string {
	return reqinfo.From(ctx).Cookie(h.svc.SessionCookie())
}

func (h *Handler) cookie(name, value string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value, Path: "/", Expires: expires, MaxAge: maxAge,
		HttpOnly: true, Secure: h.svc.CookieSecure(), SameSite: http.SameSiteLaxMode,
	}
}

func (h *Handler) sessionCookie(r auth.LoginResult) *http.Cookie {
	return h.cookie(h.svc.SessionCookie(), r.Token, r.Session.ExpiresAt, 0)
}

func (h *Handler) clear(name string) *http.Cookie { return h.cookie(name, "", time.Time{}, -1) }

// reply writes cookies, an optional Location and an optional JSON body.
type reply struct {
	status   int
	cookies  []*http.Cookie
	location string
	body     any
}

func (r reply) write(w http.ResponseWriter) error {
	for _, c := range r.cookies {
		http.SetCookie(w, c)
	}
	if r.location != "" {
		w.Header().Set("Location", r.location)
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.body == nil {
		w.WriteHeader(r.status)
		return nil
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(r.body); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.status)
	_, err := buf.WriteTo(w)
	return err
}

type (
	loginReply    struct{ reply }
	logoutReply   struct{ reply }
	startReply    struct{ reply }
	callbackReply struct{ reply }
)

func (r loginReply) VisitLoginResponse(w http.ResponseWriter) error             { return r.write(w) }
func (r logoutReply) VisitLogoutResponse(w http.ResponseWriter) error           { return r.write(w) }
func (r startReply) VisitStartAuthRedirectResponse(w http.ResponseWriter) error { return r.write(w) }
func (r callbackReply) VisitAuthCallbackResponse(w http.ResponseWriter) error   { return r.write(w) }

func (h *Handler) ListAuthProviders(context.Context, api.ListAuthProvidersRequestObject) (api.ListAuthProvidersResponseObject, error) {
	infos := h.svc.Providers()
	out := make([]api.AuthProvider, len(infos))
	for i, p := range infos {
		out[i] = api.AuthProvider{
			Id: p.ID, Name: p.DisplayName, Kind: api.AuthProviderKind(p.Kind),
			Restricted: p.Restricted, UsernameOnly: p.UsernameOnly,
		}
	}
	return api.ListAuthProviders200JSONResponse(api.AuthProviderList{Providers: out}), nil
}

func (h *Handler) Login(ctx context.Context, req api.LoginRequestObject) (api.LoginResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	cred := auth.Credentials{Username: req.Body.Username}
	if req.Body.Password != nil {
		cred.Password = *req.Body.Password
	}
	res, err := h.svc.Login(ctx, req.Body.Provider, cred, h.token(ctx))
	if err != nil {
		return nil, err
	}
	return loginReply{reply{status: http.StatusOK, cookies: []*http.Cookie{h.sessionCookie(res)}, body: toSession(res.Session)}}, nil
}

func (h *Handler) Logout(ctx context.Context, _ api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	if err := h.svc.Logout(ctx, h.token(ctx)); err != nil {
		return nil, err
	}
	return logoutReply{reply{status: http.StatusNoContent, cookies: []*http.Cookie{h.clear(h.svc.SessionCookie())}}}, nil
}

func (h *Handler) GetSession(ctx context.Context, _ api.GetSessionRequestObject) (api.GetSessionResponseObject, error) {
	s, err := h.svc.Current(ctx, h.token(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetSession200JSONResponse(toSession(s)), nil
}

func (*Handler) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	a, err := authctx.Require(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse(toMe(a)), nil
}

func (h *Handler) ChangeMyPassword(ctx context.Context, req api.ChangeMyPasswordRequestObject) (api.ChangeMyPasswordResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	if err := h.svc.ChangePassword(ctx, h.token(ctx), req.Body.CurrentPassword, req.Body.NewPassword); err != nil {
		return nil, err
	}
	return api.ChangeMyPassword204Response{}, nil
}

func (h *Handler) StartAuthRedirect(ctx context.Context, req api.StartAuthRedirectRequestObject) (api.StartAuthRedirectResponseObject, error) {
	returnTo := ""
	if req.Params.ReturnTo != nil {
		returnTo = *req.Params.ReturnTo
	}
	target, flowID, err := h.svc.BeginRedirect(ctx, req.Provider, returnTo)
	if err != nil {
		return nil, err
	}
	flow := h.cookie(h.svc.FlowCookie(), flowID, time.Time{}, int(flowTTL.Seconds()))
	return startReply{reply{status: http.StatusFound, cookies: []*http.Cookie{flow}, location: target}}, nil
}

// AuthCallback never answers problem+json: the browser is mid-navigation, so
// failures go back to the login page with a short error code.
func (h *Handler) AuthCallback(ctx context.Context, req api.AuthCallbackRequestObject) (api.AuthCallbackResponseObject, error) {
	params := url.Values{}
	for k, v := range map[string]*string{
		"code": req.Params.Code, "state": req.Params.State,
		"error": req.Params.Error, "error_description": req.Params.ErrorDescription,
	} {
		if v != nil {
			params.Set(k, *v)
		}
	}
	flowID := reqinfo.From(ctx).Cookie(h.svc.FlowCookie())
	res, returnTo, err := h.svc.Callback(ctx, req.Provider, flowID, params, h.token(ctx))
	cookies := []*http.Cookie{h.clear(h.svc.FlowCookie())}
	if err != nil {
		if apperr.KindOf(err) == 0 {
			h.log.ErrorContext(ctx, "sign-in callback failed", "provider", req.Provider, "error", err)
		}
		q := url.Values{"error": {errorCode(err)}}
		if returnTo != "/" {
			q.Set("return_to", returnTo)
		}
		return callbackReply{reply{status: http.StatusFound, cookies: cookies, location: "/login?" + q.Encode()}}, nil
	}
	cookies = append(cookies, h.sessionCookie(res))
	return callbackReply{reply{status: http.StatusFound, cookies: cookies, location: returnTo}}, nil
}

// errorCode is the login page's error vocabulary for redirect failures.
func errorCode(err error) string {
	switch apperr.KindOf(err) {
	case apperr.KindUnauthenticated:
		return "failed"
	case apperr.KindForbidden:
		return "denied"
	case apperr.KindTooManyRequests:
		return "rate_limited"
	case apperr.KindNotFound, apperr.KindInvalid:
		return "unknown_provider"
	}
	return "error"
}
