package dto

// ErrorResponse is the JSON shape returned by every handler on failure.
type ErrorResponse struct {
	Error string `json:"error"`
}
