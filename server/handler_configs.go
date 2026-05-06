package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// configsResp is the standard API response format for configs/instances endpoints.
type configsResp struct {
	Success bool   `json:"success"`
	Reason  string `json:"reason"`
	Elapse  string `json:"elapse"`
	Data    any    `json:"data"`
}

func writeConfigsResp(w http.ResponseWriter, status int, success bool, reason string, elapsed time.Duration, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(configsResp{
		Success: success,
		Reason:  reason,
		Elapse:  elapsed.String(),
		Data:    data,
	})
}
