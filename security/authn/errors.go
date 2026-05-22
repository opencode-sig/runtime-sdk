package authn

import "errors"

var (
	ErrMissingCredential = errors.New("missing credential")
	ErrUnauthenticated   = errors.New("unauthenticated")
	ErrPermissionDenied  = errors.New("permission denied")
	ErrUnavailable       = errors.New("auth unavailable")
)

func IsMissingCredential(err error) bool {
	return errors.Is(err, ErrMissingCredential)
}

func IsUnauthenticated(err error) bool {
	return errors.Is(err, ErrUnauthenticated)
}

func IsPermissionDenied(err error) bool {
	return errors.Is(err, ErrPermissionDenied)
}

func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable)
}
