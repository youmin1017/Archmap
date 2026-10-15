package health

import (
	"context"
	"net/http"

	"archmap/api/internal/gen/api"
)

type checker interface {
	Check(ctx context.Context) (Status, error)
}

// Handler implements the generated api.StrictServerInterface for the health operations.
type Handler struct {
	svc checker
}

func NewHandler(svc checker) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	st, err := h.svc.Check(ctx)
	if err != nil {
		return api.GetHealthdefaultApplicationProblemPlusJSONResponse{
			StatusCode: http.StatusServiceUnavailable,
			Body: api.Problem{
				Title:  http.StatusText(http.StatusServiceUnavailable),
				Status: http.StatusServiceUnavailable,
				Detail: ptr("a dependency is unavailable"),
			},
		}, nil
	}
	return api.GetHealth200JSONResponse{Status: api.Ok, Version: st.Version}, nil
}

func ptr[T any](v T) *T { return &v }
