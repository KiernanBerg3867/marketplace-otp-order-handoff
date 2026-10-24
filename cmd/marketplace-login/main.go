package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/infrai-examples/marketplace-otp-handoff/internal/infrai"
	"github.com/infrai-examples/marketplace-otp-handoff/internal/marketplace"
)

type loginRequest struct {
	OrderID   string `json:"order_id"`
	Phone     string `json:"phone"`
	Code      string `json:"code"`
	AttemptID string `json:"attempt_id"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	handoff := marketplace.NewHandoff(infrai.NewSMSOTP(key), []marketplace.Order{{
		ID: "order-42", SellerID: "seller-7", BuyerPhone: "+14155550100",
		SellerAssets: []string{"condition-report.pdf", "pickup-photo.jpg"},
		BuyerUpdates: []string{"Seller packed the order"}, HandoffStatus: "awaiting_phone_verification",
	}})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/code", func(w http.ResponseWriter, r *http.Request) {
		var input loginRequest
		if err := decode(r, &input); err != nil || input.OrderID == "" || input.Phone == "" || input.AttemptID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_id, phone, and attempt_id are required"})
			return
		}
		if err := handoff.SendBuyerCode(r.Context(), input.OrderID, input.Phone, input.AttemptID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"handoff_status": "code_sent"})
	})
	mux.HandleFunc("POST /login/verify", func(w http.ResponseWriter, r *http.Request) {
		var input loginRequest
		if err := decode(r, &input); err != nil || input.OrderID == "" || input.Phone == "" || input.Code == "" || input.AttemptID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_id, phone, code, and attempt_id are required"})
			return
		}
		order, err := handoff.VerifyAndRelease(r.Context(), input.OrderID, input.Phone, input.Code, input.AttemptID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
	})

	log.Println("marketplace login listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func decode(r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, marketplace.ErrOrderNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, marketplace.ErrPhoneMismatch) || errors.Is(err, marketplace.ErrCodeRejected) {
		status = http.StatusUnprocessableEntity
	} else {
		var apiErr *infrai.APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			status = apiErr.HTTPStatus
		}
	}
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(err.Error())})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
