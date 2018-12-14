package crash

import (
	"net/http"
	"os"
	"runtime/debug"
	"time"
)

func RecoverLogger() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if r := recover(); r != nil {
					logPanicAsJSON(os.Stderr, r, time.Now(), debug.Stack())
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
