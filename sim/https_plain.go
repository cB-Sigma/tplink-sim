// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cB-Sigma/tplink"
)

// StartKasaCamera starts a Kasa camera (KC series) on its HTTPS transport:
// POST /data/LINKIE2.json with HTTP basic authentication (username,
// password; set AnyLogin to take any) and the legacy JSON, obscured and in
// base64, in the form field "content"; answered the same way from the
// sysinfo and the legacy modules.
func StartKasaCamera(username, password string, sysinfo map[string]any) *Kasa {
	k := &Kasa{Sysinfo: sysinfo}
	k.web = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/LINKIE2.json" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		u, p, ok := r.BasicAuth()
		k.mu.Lock()
		anyLogin := k.AnyLogin
		k.mu.Unlock()
		if !ok || (!anyLogin && (u != username || p != password)) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(r.PostForm.Get("content"))
		var m map[string]any
		if err != nil || json.Unmarshal(tplink.Decrypt(raw), &m) != nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"err_code":-1,"err_msg":"bad request"}`))
			return
		}
		out, _ := json.Marshal(k.handle(m))
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(tplink.Encrypt(out))))
	}))
	return k
}

// StartTapoSSL starts a Tapo device on the plain HTTPS transport of older
// robot vacuum firmware: POST /app with login (username and the MD5 of
// the password, upper-case hexadecimal) answering a token, then Tapo
// requests in clear to /app?token=….
func StartTapoSSL(username, password string, info map[string]any) *Tapo {
	t := &Tapo{Info: info, sessions: map[string]*session{}, https: true}
	sum := md5.Sum([]byte(password))
	pw := hexUp(sum[:])
	tokens := map[string]bool{}
	t.HTTP = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(b, &req) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		if req.Method == "login" {
			if str(req.Params["username"]) != username || !strings.EqualFold(str(req.Params["password"]), pw) {
				_ = enc.Encode(map[string]any{"error_code": -1501})
				return
			}
			tok := randHex(16)
			tokens[tok] = true
			_ = enc.Encode(map[string]any{"error_code": 0, "result": map[string]any{"token": tok}})
			return
		}
		if !tokens[r.URL.Query().Get("token")] {
			_ = enc.Encode(map[string]any{"error_code": 9999})
			return
		}
		_ = enc.Encode(t.smart(req.Method, req.Params))
	}))
	return t
}
