package main

import (
	"document-service/app/handlers"
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /documents/{docID}", handlers.GetDocument)
	mux.HandleFunc("PUT /documents/{docID}", handlers.UpdateDocument)
	mux.HandleFunc("DELETE /documents/{docID}", handlers.DeleteDocument)
	mux.HandleFunc("POST /documents/{docID}/share", handlers.ShareDocument)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	log.Println("document-service listening on :8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}
