package entapi

import (
	"net/http"
	"strings"
)

// ColonPath rewrites whole path segments with non-empty placeholder names from
// {name} form to :name form, preserving every other segment and separator. It
// is the spelling half of composing an [Endpoint] into a router that wants
// :name placeholders; [Endpoint.Bind] is the reading half.
func ColonPath(p string) string {
	segments := strings.Split(p, "/")
	changed := false
	for i, segment := range segments {
		if name, ok := routePlaceholder(segment); ok {
			segments[i] = ":" + name
			changed = true
		}
	}
	if !changed {
		return p
	}
	return strings.Join(segments, "/")
}

// Bind returns e.Handler adapted to receive path parameters from a third-party
// router, or e.Handler itself when the endpoint has no placeholders. The
// placeholder names come from e.Path, so nothing here hardcodes "id". The
// func(string) string signature matches gin.Context.Param and echo.Context.Param
// exactly. chi and fiber each need a one-line closure -- over chi.URLParam, and
// over fiber's Params, whose defaultValue variadic makes it func(string,
// ...string) string rather than the plain one.
// A mount-time constant closure, such as func(string) string { return actorID },
// is how one pins an endpoint to a fixed id.
func (e Endpoint) Bind(get func(string) string) http.Handler {
	var names []string
	for _, segment := range strings.Split(e.Path, "/") {
		if name, ok := routePlaceholder(segment); ok {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return e.Handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, name := range names {
			req.SetPathValue(name, get(name))
		}
		e.Handler.ServeHTTP(w, req)
	})
}

func routePlaceholder(segment string) (string, bool) {
	if len(segment) < 3 || segment[0] != '{' || segment[len(segment)-1] != '}' {
		return "", false
	}
	name := segment[1 : len(segment)-1]
	return name, !strings.ContainsAny(name, "{}")
}
