package models

// APIResponse provides a consistent envelope for API responses.
type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}
