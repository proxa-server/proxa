package plugin

import "errors"

// ErrRegistryClosed is returned by Register after Close was called.
var ErrRegistryClosed = errors.New("plugin: registry is closed")

// ErrHookAlreadyRegistered is returned by Register when a Hook with
// the same Name is already in the Registry.
var ErrHookAlreadyRegistered = errors.New("plugin: hook with this name is already registered")
