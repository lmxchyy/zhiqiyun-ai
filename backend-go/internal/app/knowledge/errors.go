package knowledge

import "errors"

var (
	ErrNotFound   = errors.New("knowledge resource not found")
	ErrForbidden  = errors.New("knowledge permission denied")
	ErrConflict   = errors.New("knowledge resource conflict")
	ErrValidation = errors.New("knowledge validation failed")
)
