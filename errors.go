package knuckles

import "errors"

var (
  ErrFrontendAlreadyExists = errors.New("Frontend already exists")
  ErrInvalidFrontend       = errors.New("Invalid frontend")
  ErrBackendAlreadyExists  = errors.New("Backend already exists")
  ErrHostnameAlreadyExists = errors.New("Hostname already exists")
  ErrNoBackend             = errors.New("No backends")
  ErrDeadBackend           = errors.New("Dead backend")
  ErrAppAlreadyExists      = errors.New("Application already exists")
  ErrNoApp                 = errors.New("Application does not exist")
)
