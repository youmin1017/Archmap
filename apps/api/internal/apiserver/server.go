// Package apiserver composes the per-feature transport handlers into the single
// api.StrictServerInterface generated from contracts/openapi.yaml.
package apiserver

import (
	authtransport "archmap/api/internal/auth/transport"
	changesettransport "archmap/api/internal/changeset/transport"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/health"
	layerruletransport "archmap/api/internal/layerrule/transport"
	modeltransport "archmap/api/internal/model/transport"
	settingstransport "archmap/api/internal/settings/transport"
	usertransport "archmap/api/internal/user/transport"
	viewtransport "archmap/api/internal/view/transport"
)

// Aliases give each embedded handler a distinct field name.
type (
	authHandler      = authtransport.Handler
	healthHandler    = health.Handler
	modelHandler     = modeltransport.Handler
	changesetHandler = changesettransport.Handler
	userHandler      = usertransport.Handler
	groupHandler     = usertransport.GroupHandler
	viewHandler      = viewtransport.Handler
	layerRuleHandler = layerruletransport.Handler
	settingsHandler  = settingstransport.Handler
)

type Server struct {
	*authHandler
	*healthHandler
	*modelHandler
	*changesetHandler
	*userHandler
	*groupHandler
	*viewHandler
	*layerRuleHandler
	*settingsHandler
}

func New(a *authtransport.Handler, h *health.Handler, m *modeltransport.Handler, c *changesettransport.Handler, u *usertransport.Handler, g *usertransport.GroupHandler, v *viewtransport.Handler, lr *layerruletransport.Handler, st *settingstransport.Handler) *Server {
	return &Server{authHandler: a, healthHandler: h, modelHandler: m, changesetHandler: c, userHandler: u, groupHandler: g, viewHandler: v, layerRuleHandler: lr, settingsHandler: st}
}

var _ api.StrictServerInterface = (*Server)(nil)
