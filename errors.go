package knuckles

import "errors"

var (
  ErrFrontendAlreadyExists = errors.New("Frontend already exists")
  ErrBackendAlreadyExists  = errors.New("Backend already exists")
  ErrHostnameAlreadyExists = errors.New("Hostname already exists")
  ErrNoBackend             = errors.New("No backends")
  ErrNoHostname            = errors.New("No hostname")
  ErrDeadBackend           = errors.New("Dead backend")
  ErrAppAlreadyExists      = errors.New("Application already exists")
  ErrNoApp                 = errors.New("Application does not exist")
  ErrInvalidAction         = errors.New("Invalid action")
)
