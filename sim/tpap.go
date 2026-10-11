// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cB-Sigma/tplink"
)

// The TPAP login of newer firmware, on the device side: discover,
// pake_register and pake_share as JSON to the root path, then AES-128-CCM
// requests to /stok=<token>/ds (see tplink's tpap.go for the protocol).
// A Tapo device in TPAP mode refuses KLAP with 403; a camera keeps its
// HTTPS login next to TPAP.

// tpapState holds a TPAP device's login and sessions.
type tpapState struct {
	passcode string // what the login proves: SHA-1 or (cameras) MD5 hex of the password
	camera   bool
	salt     []byte
	last     *tplink.TPAPVerifier // pake_register's verifier, for the next pake_share
	sessions map[string]*tpapSession
	failures int
	logins   int
	n        int
}

type tpapSession struct {
	c    *tplink.TPAPCipher
	next int32 // the next sequence number taken
}

// TPAP settings of a simulated device; change them before the first login.
type TPAPSettings struct {
	// Iterations is the PBKDF2 count announced (default 1000; real devices
	// announce a few thousand).
	Iterations int
	// LockAfter locks the device after that many failed logins in a row
	// (0: never). A locked device answers -2101 to every login message.
	LockAfter int
	// StartSeq is the first sequence number (default 1).
	StartSeq int32
	// Port is the port announced in the tpap object (default: the
	// simulator's own).
	Port int
}

func newTPAPState(password string, camera bool) *tpapState {
	s := &tpapState{camera: camera, sessions: map[string]*tpapSession{}, salt: make([]byte, 16)}
	_, _ = rand.Read(s.salt)
	if camera {
		h := md5.Sum([]byte(password))
		s.passcode = hex.EncodeToString(h[:])
	} else {
		s.passcode = tplink.TPAPPasscodeSHA1(password)
	}
	return s
}

// StartTapoTPAP starts a Tapo device on newer firmware with the
// third-party compatibility option off: it speaks TPAP with the account
// password (passcode type 2, extra_crypt password_shadow 2), answers
// discover with its tpap object and refuses KLAP with 403. Its 20002
// answer names encrypt_type TPAP with the tpap object. The username is not
// used by TPAP; it is kept for symmetry with StartTapo.
func StartTapoTPAP(username, password string, info, usage map[string]any) *Tapo {
	t := &Tapo{auth: tplink.AuthHash(username, password), version: 2, Info: info, Usage: usage, sessions: map[string]*session{},
		tpap: newTPAPState(password, false)}
	t.HTTP = httptest.NewServer(http.HandlerFunc(t.serve))
	return t
}

// StartCameraTPAP starts a Tapo camera (or doorbell, or H200 hub) on newer
// firmware that takes TPAP over HTTPS with the MD5 of the password (no
// extra_crypt) next to its HTTPS login (StartCamera). Its 20002 answer
// carries the tpap object.
func StartCameraTPAP(username, password string, info map[string]any) *Tapo {
	t := StartCamera(username, password, info)
	t.mu.Lock()
	t.tpap = newTPAPState(password, true)
	t.mu.Unlock()
	return t
}

// TPAPLogins counts successful TPAP logins.
func (t *Tapo) TPAPLogins() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tpap == nil {
		return 0
	}
	return t.tpap.logins
}

// ForgetTPAPSessions drops every TPAP session (a reboot).
func (t *Tapo) ForgetTPAPSessions() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tpap != nil {
		t.tpap.sessions = map[string]*tpapSession{}
	}
}

// UnlockTPAP clears the failed-login count (the lock's timeout).
func (t *Tapo) UnlockTPAP() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tpap != nil {
		t.tpap.failures = 0
	}
}

// tpapObject is the tpap object the device announces. The caller holds
// t.mu.
func (t *Tapo) tpapObject() map[string]any {
	port := t.TPAP.Port
	if port == 0 {
		port = portOf(t.Addr())
	}
	tls := 0
	if t.https {
		tls = 1
	}
	o := map[string]any{"tls": tls, "pake": []any{2}, "port": port, "noc": 0}
	if t.tpap.camera {
		o["noc"] = 1
	} else {
		o["dac"] = 0
	}
	return o
}

func portOf(addr string) int {
	i := strings.LastIndexByte(addr, ':')
	p := 0
	for _, c := range addr[i+1:] {
		p = p*10 + int(c-'0')
	}
	return p
}

// serveTPAP answers TPAP messages; it reports false for a request that is
// not one. The caller holds t.mu.
func (t *Tapo) serveTPAP(w http.ResponseWriter, r *http.Request, body []byte) bool {
	s := t.tpap
	if stok, ok := strings.CutPrefix(r.URL.Path, "/stok="); ok {
		stok, ok = strings.CutSuffix(stok, "/ds")
		sess := s.sessions[stok]
		if sess == nil {
			if !ok || t.cam == nil {
				w.WriteHeader(http.StatusUnauthorized)
				return true
			}
			return false // a session of the camera's HTTPS login
		}
		t.tpapRequest(w, sess, body)
		return true
	}
	if r.URL.Path != "/" && r.URL.Path != "" {
		return false
	}
	var req struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if json.Unmarshal(body, &req) != nil || req.Method != "login" {
		return false
	}
	sub, _ := req.Params["sub_method"].(string)
	if sub == "" {
		return false // the camera's HTTPS login
	}
	writeJSON := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
	if t.TPAP.LockAfter > 0 && s.failures >= t.TPAP.LockAfter {
		writeJSON(map[string]any{"error_code": -2101})
		return true
	}
	switch sub {
	case "discover":
		if s.camera {
			// Cameras do not answer discover.
			writeJSON(map[string]any{"error_code": -40210})
			return true
		}
		writeJSON(map[string]any{"error_code": 0, "result": map[string]any{"sub_method": "discover", "tpap_preferred": true,
			"mac": strings.ToUpper(strings.NewReplacer("-", "", ":", "").Replace(str(t.Info["mac"]))), "tpap": t.tpapObject()}})
	case "pake_register":
		ur, err := base64.StdEncoding.DecodeString(str(req.Params["user_random"]))
		suites, _ := req.Params["cipher_suites"].([]any)
		encs, _ := req.Params["encryption"].([]any)
		if err != nil || len(ur) == 0 {
			writeJSON(map[string]any{"error_code": -1008})
			return true
		}
		if !hasNum(suites, 1) || !hasStr(encs, "aes_128_ccm") {
			writeJSON(map[string]any{"error_code": -2201})
			return true
		}
		if req.Params["passcode_type"] != "userpw" {
			writeJSON(map[string]any{"error_code": -2202})
			return true
		}
		dr := make([]byte, 32)
		_, _ = rand.Read(dr)
		iter := t.TPAP.Iterations
		if iter == 0 {
			iter = 1000
		}
		v, err := tplink.NewTPAPVerifier(s.passcode, s.salt, iter, ur, dr)
		if err != nil {
			writeJSON(map[string]any{"error_code": -1008})
			return true
		}
		s.last = v
		res := map[string]any{"dev_random": base64.StdEncoding.EncodeToString(dr), "dev_salt": base64.StdEncoding.EncodeToString(s.salt),
			"dev_share": base64.StdEncoding.EncodeToString(v.Share()), "cipher_suites": 1, "iterations": iter, "encryption": "aes_128_ccm"}
		if !s.camera {
			res["extra_crypt"] = map[string]any{"type": "password_shadow", "params": map[string]any{"passwd_id": 2}}
		}
		writeJSON(map[string]any{"error_code": 0, "result": res})
	case "pake_share":
		us, err1 := base64.StdEncoding.DecodeString(str(req.Params["user_share"]))
		uc, err2 := base64.StdEncoding.DecodeString(str(req.Params["user_confirm"]))
		v := s.last
		s.last = nil
		if v == nil || err1 != nil || err2 != nil {
			writeJSON(map[string]any{"error_code": -1008})
			return true
		}
		dc, key, err := v.Finish(us, uc)
		if err != nil {
			s.failures++
			code := -2203
			if s.camera {
				code = -40401
			}
			writeJSON(map[string]any{"error_code": code})
			return true
		}
		c, _ := tplink.NewTPAPCipher(key)
		s.failures = 0
		s.logins++
		s.n++
		stok := randHex(16)
		start := t.TPAP.StartSeq
		if start == 0 {
			start = 1
		}
		s.sessions[stok] = &tpapSession{c: c, next: start}
		writeJSON(map[string]any{"error_code": 0, "result": map[string]any{"dev_confirm": base64.StdEncoding.EncodeToString(dc), "stok": stok, "start_seq": start}})
	default:
		writeJSON(map[string]any{"error_code": -1008})
	}
	return true
}

// tpapRequest answers one encrypted request: under the request's sequence
// number, which must not be older than the last one taken.
func (t *Tapo) tpapRequest(w http.ResponseWriter, sess *tpapSession, body []byte) {
	if len(body) < 4 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	seq := int32(binary.BigEndian.Uint32(body[:4]))
	plain, err := sess.c.Open(seq, body)
	if err != nil || seq < sess.next {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error_code":-2402}`))
		return
	}
	sess.next = seq + 1
	var inner map[string]any
	if json.Unmarshal(plain, &inner) != nil {
		_, _ = w.Write(sess.c.Seal(seq, []byte(`{"error_code":-1003}`)))
		return
	}
	var out map[string]any
	if t.cam != nil {
		out = t.camRequest(inner)
	} else {
		method, _ := inner["method"].(string)
		params, _ := inner["params"].(map[string]any)
		out = t.smart(method, params)
	}
	b, _ := json.Marshal(out)
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(sess.c.Seal(seq, b))
}

func hasNum(l []any, n float64) bool {
	for _, x := range l {
		if f, ok := x.(float64); ok && f == n {
			return true
		}
	}
	return false
}

func hasStr(l []any, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
