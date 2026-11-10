package model

import "errors"

// ErrNoRefresh is returned by an authorizer whose credentials cannot be refreshed
// (static tokens, basic, apikey); the caller then keeps the original 401 response.
var ErrNoRefresh = errors.New("auth: credentials cannot be refreshed")
