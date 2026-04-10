// Package handlers provides HTTP handler functions and shared response helpers
// for the Financial Compliance Ledger API.
package handlers

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard JSON error envelope returned by all API errors.
// Format: { "error": { "code": "STRING", "message": "human readable", "details": {} } }
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the error code, message, and optional details.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// RespondJSON writes a JSON response with the given status code and data.
// If data is nil, only the status code is written with no body.
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	if data == nil {
		w.WriteHeader(statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// RespondError writes a standard JSON error response without details.
func RespondError(w http.ResponseWriter, statusCode int, code, message string) {
	resp := ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
		},
	}
	RespondJSON(w, statusCode, resp)
}

// RespondErrorWithDetails writes a standard JSON error response with details.
// If details is nil, the details field is omitted from the response.
func RespondErrorWithDetails(w http.ResponseWriter, statusCode int, code, message string, details interface{}) {
	resp := ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
	RespondJSON(w, statusCode, resp)
}
