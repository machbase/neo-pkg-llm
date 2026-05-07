package main

import (
	"io"
	"net/http"

	"neo-pkg-llm/logger"
)

// proxyMachbase forwards POST /db/tql to machbase-neo without auth.
func (inst *Instance) proxyMachbase(w http.ResponseWriter, r *http.Request) {
	resp, err := inst.mc.Forward(
		r.Context(),
		r.Method,
		r.URL.Path,
		r.URL.RawQuery,
		r.Body,
		r.Header.Get("Content-Type"),
	)
	if err != nil {
		logger.Errorf("[Instance:%s] proxy /db/tql failed: %v", inst.name, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyResponse(w, resp)
}

// proxyMachbaseWeb forwards GET/POST /web/* to machbase-neo with JWT auth.
func (inst *Instance) proxyMachbaseWeb(w http.ResponseWriter, r *http.Request) {
	resp, err := inst.mc.Forward(
		r.Context(),
		r.Method,
		r.URL.Path,
		r.URL.RawQuery,
		r.Body,
		r.Header.Get("Content-Type"),
	)
	if err != nil {
		logger.Errorf("[Instance:%s] proxy %s failed: %v", inst.name, r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyResponse(w, resp)
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}
