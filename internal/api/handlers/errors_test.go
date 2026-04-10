package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondJSON_SetsContentTypeAndStatus(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	handlers.RespondJSON(w, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "value", body["key"])
}

func TestRespondJSON_StatusCreated(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]int{"id": 42}

	handlers.RespondJSON(w, http.StatusCreated, data)

	assert.Equal(t, http.StatusCreated, w.Code)

	var body map[string]int
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, 42, body["id"])
}

func TestRespondJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()

	handlers.RespondJSON(w, http.StatusNoContent, nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestRespondError_BasicError(t *testing.T) {
	w := httptest.NewRecorder()

	handlers.RespondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid input")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	errObj, ok := body["error"].(map[string]interface{})
	require.True(t, ok, "response must contain 'error' object")

	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.Equal(t, "invalid input", errObj["message"])
}

func TestRespondError_NotFound(t *testing.T) {
	w := httptest.NewRecorder()

	handlers.RespondError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
	assert.Equal(t, "resource not found", errObj["message"])
}

func TestRespondError_InternalServerError(t *testing.T) {
	w := httptest.NewRecorder()

	handlers.RespondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "something went wrong")

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
	assert.Equal(t, "something went wrong", errObj["message"])
}

func TestRespondErrorWithDetails_IncludesDetailsField(t *testing.T) {
	w := httptest.NewRecorder()

	details := map[string]interface{}{
		"field":  "amount",
		"reason": "must be positive",
	}

	handlers.RespondErrorWithDetails(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "validation failed", details)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.Equal(t, "validation failed", errObj["message"])

	detailsObj, ok := errObj["details"].(map[string]interface{})
	require.True(t, ok, "error must contain 'details' object")
	assert.Equal(t, "amount", detailsObj["field"])
	assert.Equal(t, "must be positive", detailsObj["reason"])
}

func TestRespondErrorWithDetails_NilDetails(t *testing.T) {
	w := httptest.NewRecorder()

	handlers.RespondErrorWithDetails(w, http.StatusBadRequest, "BAD_REQUEST", "bad request", nil)

	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "BAD_REQUEST", errObj["code"])
	// details should be omitted or empty when nil
	_, hasDetails := errObj["details"]
	assert.False(t, hasDetails, "details should be omitted when nil")
}
