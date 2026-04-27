// Package handlers contains HTTP request handlers for API endpoints.
package handlers

import (
	"encoding/json"
	"net/http"
	"sort"

	"go-proxy/internal/config"
)

// ModelsHandler handles /v1/models requests.
type ModelsHandler struct {
	config *config.Config
}

// Model represents an OpenAI-compatible model object.
type Model struct {
	ID         string        `json:"id"`
	Object     string        `json:"object"`
	Created    int64         `json:"created"`
	OwnedBy    string        `json:"owned_by"`
	Permission []interface{} `json:"permission"`
	Root       string        `json:"root"`
	Parent     *string       `json:"parent"`
}

// ModelsResponse represents the response for /v1/models.
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// NewModelsHandler creates a new models handler.
func NewModelsHandler(cfg *config.Config) *ModelsHandler {
	return &ModelsHandler{
		config: cfg,
	}
}

// HandleModels handles GET /v1/models.
func (h *ModelsHandler) HandleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Build model list from config
	var models []Model
	for modelID := range h.config.Models {
		model := Model{
			ID:         modelID,
			Object:     "model",
			Created:    0, // We don't track creation time
			OwnedBy:    "opencode-go",
			Permission: []interface{}{},
			Root:       modelID,
			Parent:     nil,
		}
		models = append(models, model)
	}

	// Sort models by ID for consistent output
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	response := ModelsResponse{
		Object: "list",
		Data:   models,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
