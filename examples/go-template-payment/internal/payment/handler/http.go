package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	paymentservice "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/payment/service"
)

// RegisterHTTP registers payment-owned HTTP handlers on the service HTTP listener.
func RegisterHTTP(mux *http.ServeMux, service *paymentservice.Service) {
	mux.HandleFunc("/internal/payments/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			id = "pay-1001"
		}
		payment, err := service.GetPayment(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"source":  "payment-http",
			"payment": payment,
		})
	})
}
