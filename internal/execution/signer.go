package execution

import (
	"errors"

	"github.com/valsalamakha/livepolybtc/internal/api"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// ErrSigningUnavailable is returned by an OrderSigner that cannot sign orders
// (e.g. no wallet key is configured). The executor treats it as a signal to
// fall back to simulation / refuse live submission.
var ErrSigningUnavailable = errors.New("order signing unavailable: a wallet private key and EIP-712 signer are required for live order submission")

// OrderSigner converts a logical order into the EIP-712 signed structure the
// CLOB requires. Implementations are pluggable so the EIP-712 signing
// component (which depends on a wallet key and an Ethereum signing library) can
// be supplied without coupling the rest of the engine to it.
type OrderSigner interface {
	// SignOrder builds and signs a CLOB order for the given logical order.
	SignOrder(o models.Order) (api.OrderRequest, error)
	// Available reports whether the signer can actually produce signed orders.
	Available() bool
	// Owner returns the API-key owner / maker address.
	Owner() string
}

// unsignedSigner is the default signer used when no wallet key is configured.
// It reports itself as unavailable so the executor runs in simulation mode.
type unsignedSigner struct {
	owner string
}

// NewUnsignedSigner returns an OrderSigner that cannot sign (simulation only).
func NewUnsignedSigner(owner string) OrderSigner { return &unsignedSigner{owner: owner} }

func (s *unsignedSigner) SignOrder(models.Order) (api.OrderRequest, error) {
	return api.OrderRequest{}, ErrSigningUnavailable
}

func (s *unsignedSigner) Available() bool { return false }

func (s *unsignedSigner) Owner() string { return s.owner }
