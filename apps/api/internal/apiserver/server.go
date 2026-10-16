// Package apiserver composes the per-feature transport handlers into the single
// api.StrictServerInterface generated from contracts/openapi.yaml.
package apiserver

import (
	changesettransport "archmap/api/internal/changeset/transport"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/health"
	layerruletransport "archmap/api/internal/layerrule/transport"
	modeltransport "archmap/api/internal/model/transport"
	usertransport "archmap/api/internal/user/transport"
	viewtransport "archmap/api/internal/view/transport"
)

// Aliases give each embedded handler a distinct field name.
type (
	healthHandler    = health.Handler
	modelHandler     = modeltransport.Handler
	changesetHandler = changesettransport.Handler
	userHandler      = usertransport.Handler
	viewHandler      = viewtransport.Handler
	layerRuleHandler = layerruletransport.Handler
)

type Server struct {
	*healthHandler
	*modelHandler
	*changesetHandler
	*userHandler
	*viewHandler
	*layerRuleHandler
}

func New(h *health.Handler, m *modeltransport.Handler, c *changesettransport.Handler, u *usertransport.Handler, v *viewtransport.Handler, lr *layerruletransport.Handler) *Server {
	return &Server{healthHandler: h, modelHandler: m, changesetHandler: c, userHandler: u, viewHandler: v, layerRuleHandler: lr}
}

var _ api.StrictServerInterface = (*Server)(nil)
