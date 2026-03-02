package main

import (
	"document-service/app/handlers"
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	docHandler, err := handlers.NewDocumentHandler()
	if err != nil {
		log.Fatalf("Failed to initialize document handler: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /documents/{docID}", docHandler.GetDocument)
	mux.HandleFunc("PUT /documents/{docID}", docHandler.UpdateDocument)
	mux.HandleFunc("DELETE /documents/{docID}", docHandler.DeleteDocument)
	mux.HandleFunc("POST /documents/{docID}/share", docHandler.ShareDocument)
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	log.Println("document-service listening on :8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}
