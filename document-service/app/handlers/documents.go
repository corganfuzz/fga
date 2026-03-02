package handlers

import (
	"encoding/json"
	"net/http"
)

func GetDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"document": docID,
		"content":  "Content of " + docID,
	})
}

func UpdateDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"document": docID,
		"payload":  body,
		"status":   "updated",
	})
}

func DeleteDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"document": docID,
		"status":   "deleted",
	})
}

func ShareDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docID")
	var body struct {
		User     string `json:"user"`
		Relation string `json:"relation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.User == "" || body.Relation == "" {
		http.Error(w, `{"error":"body must include user and relation"}`, http.StatusBadRequest)
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
