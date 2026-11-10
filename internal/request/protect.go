package request

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// CheckProtect enforces allow_methods and protected-environment write confirmation.
// tty must be true only when stdin and stderr are both terminals. prompt may be nil,
// which counts as a refusal.
func CheckProtect(method string, env model.Env, auth model.AuthSpec, yes, tty bool, prompt func(msg string) bool) error {
	method = strings.ToUpper(strings.TrimSpace(method))
	if len(auth.AllowMethods) > 0 {
		allowed := false
		for _, m := range auth.AllowMethods {
			if strings.ToUpper(strings.TrimSpace(m)) == method {
				allowed = true
				break
			}
		}
		if !allowed {
			return model.Errorf(model.ExitProtected, "method %s is not allowed by this auth (allow_methods)", method)
		}
	}
	if !env.Protect || slices.Contains([]string{http.MethodGet, http.MethodHead, http.MethodOptions}, method) || yes {
		return nil
	}
	if tty && os.Getenv("APIC_NO_PROMPT") == "" && prompt != nil {
		if prompt(fmt.Sprintf("%s on a protected environment, continue?", method)) {
			return nil
		}
	}
	return model.Errorf(model.ExitProtected, "%s on a protected environment requires --yes", method)
}
