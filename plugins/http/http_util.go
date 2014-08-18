package http

import "net/http"

func CustomHeadersHandler(h http.Handler, header http.Header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, vals := range header {
			for _, val := range vals {
				w.Header().Add(key, val)
			}
		}
		h.ServeHTTP(w, r)
	})
}
