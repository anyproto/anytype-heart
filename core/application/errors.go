package application

import "errors"

// TODO Remove Failed prefix, it's an error already
var (
	ErrFailedToStartApplication = errors.New("failed to run node")
	ErrFailedToStopApplication  = errors.New("failed to stop running node")
	ErrFailedToCreateLocalRepo  = errors.New("failed to create local repo")
	ErrFailedToWriteConfig      = errors.New("failed to write config")
	ErrSetDetails               = errors.New("failed to set details")
)
