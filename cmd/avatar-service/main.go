package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/fieldservice-avatar-pipeline/internal/fieldservice"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	baseURL := os.Getenv("INFRAI_BASE_URL")
	httpClient := &http.Client{Timeout: 30 * time.Second}
	client := fieldservice.NewInfraiClient(baseURL, key, httpClient)
	workflow := fieldservice.NewAvatarWorkflow(client, httpClient)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /avatars", avatarHandler(workflow))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("avatar service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func avatarHandler(workflow *fieldservice.AvatarWorkflow) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart request")
			return
		}
		file, header, err := r.FormFile("photo")
		if err != nil {
			writeError(w, http.StatusBadRequest, "photo is required")
			return
		}
		defer file.Close()
		image, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read photo")
			return
		}
		if len(image) > 8<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "photo exceeds 8 MiB")
			return
		}
		result, err := workflow.Process(r.Context(), fieldservice.AvatarRequest{
			UserID: r.FormValue("user_id"), WorkOrderID: r.FormValue("work_order_id"),
			DispatchStatus: r.FormValue("dispatch_status"), TechnicianFollowUp: r.FormValue("technician_follow_up"),
			Filename: header.Filename, Image: image,
		})
		if err != nil {
			var apiErr *fieldservice.InfraiError
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				writeError(w, apiErr.HTTPStatus, apiErr.Error())
				return
			}
			if errors.As(err, &apiErr) {
				writeError(w, http.StatusBadGateway, "avatar processing failed")
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(result)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
