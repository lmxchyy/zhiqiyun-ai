package providerexecution

import (
	"errors"
	"fmt"
	"time"
)

type Status string

const (
	Prepared   Status = "prepared"
	Submitting Status = "submitting"
	Submitted  Status = "submitted"
	Processing Status = "processing"
	Succeeded  Status = "succeeded"
	Failed     Status = "failed"
	Unknown    Status = "unknown"
)

var ErrIllegalTransition = errors.New("illegal provider execution state transition")
var ErrUnknownResubmitBlocked = errors.New("automatic resubmission is blocked for unknown execution")
var ErrProviderStillProcessing = errors.New("provider execution is still processing")
var ErrProviderExecutionFailed = errors.New("provider execution failed")

type Execution struct {
	ID                                                          int64
	TaskID                                                      string
	Provider                                                    string
	ProviderChannel                                             string
	ProviderModel                                               string
	Capability                                                  string
	Attempt                                                     int
	Status                                                      Status
	RequestFingerprint                                          string
	ProviderRequestID                                           *string
	SubmittedAt, ProcessingAt, SucceededAt, FailedAt, UnknownAt *time.Time
	LastCheckedAt, NextCheckAt                                  *time.Time
	ErrorCode, ErrorClass, LastError                            *string
	CreatedAt, UpdatedAt                                        time.Time
}

func CanTransition(from, to Status) bool {
	if from == to {
		return true
	}
	switch from {
	case Prepared:
		return to == Submitting || to == Failed
	case Submitting:
		return to == Submitted || to == Failed || to == Unknown
	case Submitted:
		return to == Processing || to == Succeeded || to == Failed || to == Unknown
	case Processing:
		return to == Succeeded || to == Failed || to == Unknown
	case Unknown:
		return to == Submitted || to == Processing || to == Failed
	default:
		return false
	}
}
func ValidateTransition(from, to Status) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	}
	return nil
}
