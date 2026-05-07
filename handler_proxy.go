package main

import (
	"io"
	"net/http"
)

// proxyMachbase godoc
// POST /db/tql
// 프론트엔드 요청을 machbase-neo 로 중계한다.
func (inst *Instance) proxyMachbase(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	resp, err := inst.mc.Forward(
		r.Context(),
		r.Method,
		path,
		r.URL.RawQuery,
		r.Body,
		r.Header.Get("Content-Type"),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}

// proxyMachbaseWeb godoc
// GET/POST /web/*
// /web/* 요청을 machbase-neo 로 중계한다.
// Authorization 헤더가 있으면 그대로 전달한다.
func (inst *Instance) proxyMachbaseWeb(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	var extra []http.Header
	if auth := r.Header.Get("Authorization"); auth != "" {
		extra = append(extra, http.Header{"Authorization": {auth}})
	}

	resp, err := inst.mc.Forward(
		r.Context(),
		r.Method,
		path,
		r.URL.RawQuery,
		r.Body,
		r.Header.Get("Content-Type"),
		extra...,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}
