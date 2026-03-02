package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/openfga/go-sdk/client"
)

type Document struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type DocumentHandler struct {
	fgaClient *client.OpenFgaClient
	mu        sync.RWMutex
	store     map[string]Document
}

func NewDocumentHandler() (*DocumentHandler, error) {
	apiURL := os.Getenv("OPENFGA_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	storeID := os.Getenv("OPENFGA_STORE_ID")

	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:  apiURL,
		StoreId: storeID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create fga client: %w", err)
	}

	return &DocumentHandler{
		fgaClient: fgaClient,
		store:     make(map[string]Document),
	}, nil
}

func (h *DocumentHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")

	h.mu.RLock()
	doc, ok := h.store[docID]
	h.mu.RUnlock()

	if !ok {
		http.Error(w, `{"error":"document not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (h *DocumentHandler) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	h.store[docID] = Document{ID: docID, Content: body.Content}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"document": docID,
		"status":   "updated",
	})
}

func (h *DocumentHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")

	h.mu.Lock()
	delete(h.store, docID)
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"document": docID,
		"status":   "deleted",
	})
}

func (h *DocumentHandler) ShareDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")
	var body struct {
		User     string `json:"user"`
		Relation string `json:"relation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.User == "" || body.Relation == "" {
		http.Error(w, `{"error":"body must include user and relation"}`, http.StatusBadRequest)
		return
	}

	// Write tuple to OpenFGA
	_, err := h.fgaClient.Write(r.Context()).
		Body(client.ClientWriteRequest{
			Writes: []client.ClientTupleKey{
				{
					User:     "user:" + body.User,
					Relation: body.Relation,
					Object:   "document:" + docID,
				},
			},
		}).
		Execute()

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to write fga tuple: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"document":    docID,
		"shared_with": body.User,
		"as":          body.Relation,
		"status":      "ok",
	})
}
