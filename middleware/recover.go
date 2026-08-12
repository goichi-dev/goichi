package middleware

import (
	"github.com/goichi-dev/goichi/core"
	"log"
	"runtime/debug"
)

// Recover turns a panic in a later handler into a 500 response and a logged
// stack trace. It is installed by default; fasthttp does not recover panics on
// its own, so without it an escaping panic terminates the process.
func Recover() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] %v\n%s", r, debug.Stack())
					err = c.Status(core.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
				}
			}()
			return next(c)
		}
	}
}
