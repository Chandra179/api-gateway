// Package greeting is an example vertical module of the modular monolith. It
// owns its handler, its domain logic, and (in a fuller system) its persistence,
// exposing a small public surface through the module boundary.
package greeting

import (
	"fmt"
	"net/http"
	"strings"
)

// Handler serves the protected greeting endpoint behind the rate limiter.
type Handler struct{}

// New builds a greeting Handler.
func New() *Handler { return &Handler{} }

// ServeHTTP renders a greeting. It reads nothing sensitive; it exists to give
// the gateway protected traffic to forward once the rate-limit check passes.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "world"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "hello, %s\n", name)
}
