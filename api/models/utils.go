package models

import "time"

// NewHealthResponse returns a HealthResponse initialized with the given status and version.
func NewHealthResponse(status, version string) HealthResponse {
	return HealthResponse{
		Status:    status,
		Version:   version,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
