package request

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"

	"github.com/inhere/apic/internal/model"
)

// NewClient builds an http.Client that never follows redirects and honors env TLS settings.
func NewClient(env model.Env) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: env.Insecure} //nolint:gosec // explicit user opt-in
	if env.CAFile != "" {
		pem, err := os.ReadFile(env.CAFile)
		if err != nil {
			return nil, model.Errorf(model.ExitLocal, "read ca_file: %v", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, model.Errorf(model.ExitLocal, "ca_file %s contains no valid certificate", env.CAFile)
		}
		cfg.RootCAs = pool
	}
	tr.TLSClientConfig = cfg
	return &http.Client{
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}
