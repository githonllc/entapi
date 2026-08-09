package entapi

import (
	"net/http"
	"strings"
)

// ColonPath rewrites whole path segments with non-empty placeholder names from
// {name} form to :name form, preserving every other segment and separator.
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

// Bind returns r.Handler adapted to receive path parameters from a third-party
// router, or r.Handler itself when the route has no placeholders. The
// func(string) string signature matches gin.Context.Param and echo.Context.Param
// exactly. chi and fiber each need a one-line closure -- over chi.URLParam, and
// over fiber's Params, whose defaultValue variadic makes it func(string,
// ...string) string rather than the plain one.
// A mount-time constant closure, such as func(string) string { return actorID },
// is how one pins a route to a fixed id.
func (r Route) Bind(get func(string) string) http.Handler {
	var names []string
	for _, segment := range strings.Split(r.Path, "/") {
		if name, ok := routePlaceholder(segment); ok {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return r.Handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, name := range names {
			req.SetPathValue(name, get(name))
		}
		r.Handler.ServeHTTP(w, req)
	})
}

func routePlaceholder(segment string) (string, bool) {
	if len(segment) < 3 || segment[0] != '{' || segment[len(segment)-1] != '}' {
		return "", false
	}
	name := segment[1 : len(segment)-1]
	return name, !strings.ContainsAny(name, "{}")
}
