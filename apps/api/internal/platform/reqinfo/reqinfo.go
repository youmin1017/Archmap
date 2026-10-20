// Package reqinfo carries transport facts that services need (client IP, user
// agent, the raw cookies the auth feature reads) through context.Context, so
// handlers generated from the spec can reach them without importing Echo.
// The httpx middleware fills it in for every request.
package reqinfo

import "context"

type Info struct {
	IP        string
	UserAgent string
	// Cookies holds the request's cookies by name (first value wins).
	Cookies map[string]string
}

func (i Info) Cookie(name string) string { return i.Cookies[name] }

type ctxKey struct{}

func With(ctx context.Context, i Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, i)
}

// From returns the request info, or the zero value outside an HTTP request.
func From(ctx context.Context) Info {
	i, _ := ctx.Value(ctxKey{}).(Info)
	return i
}
