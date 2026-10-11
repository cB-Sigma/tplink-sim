// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/cB-Sigma/tplink"
)

// StartTapoHTTPS starts a Tapo device on newer firmware that serves KLAP
// v2 over HTTPS (a self-signed certificate; real devices use port 4433).
// Its 20002 answer says is_support_https.
func StartTapoHTTPS(username, password string, info, usage map[string]any) *Tapo {
	t := &Tapo{auth: tplink.AuthHash(username, password), version: 2, Info: info, Usage: usage, sessions: map[string]*session{}, https: true}
	t.HTTP = httptest.NewTLSServer(http.HandlerFunc(t.serve))
	return t
}

// sslSession is one session of the HTTPS camera transport.
type sslSession struct {
	s      *tplink.AESSession // nil: the plain login, answers in clear
	cnonce string
	seq    int
}

// StartCamera starts a Tapo camera, doorbell or H200 hub on the HTTPS
// transport: the two-step login with nonces (the device proves the
// password first), then signed securePassthrough requests to
// /stok=<token>/ds, answered with camera requests ("smartcam":
// getDeviceInfo, getAppComponentList, get and set of CamSections,
// setDeviceAlias, multipleRequest) and, for a hub, getChildDeviceList,
// getChildDeviceComponentList and controlChild. info is the device's
// basic_info (device_type SMART.IPCAMERA, SMART.TAPOHUB, ...); username is
// the device's local user. Components are reported as the app components.
// Set PasswordMD5 for firmware that holds the MD5 hash, PlainLogin for
// firmware without the nonce login.
func StartCamera(username, password string, info map[string]any) *Tapo {
	t := &Tapo{Info: info, sessions: map[string]*session{}, cam: map[string]*sslSession{}, camUser: username, camPassword: password, https: true}
	t.HTTP = httptest.NewTLSServer(http.HandlerFunc(t.serveCam))
	return t
}

func hexUp(b []byte) string { return strings.ToUpper(hex.EncodeToString(b)) }

func (t *Tapo) camPwdHash() string {
	if t.PasswordMD5 {
		h := md5.Sum([]byte(t.camPassword))
		return hexUp(h[:])
	}
	h := sha256.Sum256([]byte(t.camPassword))
	return hexUp(h[:])
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hexUp(b)
}

// TagFailures counts camera requests whose Tapo_tag or Seq did not match.
func (t *Tapo) TagFailures() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tagFailures
}

// ForgetCameraSessions drops every HTTPS session (a reboot).
func (t *Tapo) ForgetCameraSessions() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cam = map[string]*sslSession{}
}

func (t *Tapo) serveCam(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	t.mu.Lock()
	defer t.mu.Unlock()
	writeJSON := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
	var req struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if json.Unmarshal(body, &req) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if r.URL.Path == "/" || r.URL.Path == "" {
		if req.Method != "login" {
			writeJSON(map[string]any{"error_code": -40401})
			return
		}
		writeJSON(t.camLogin(req.Params))
		return
	}
	stok, ok := strings.CutPrefix(r.URL.Path, "/stok=")
	stok, ok2 := strings.CutSuffix(stok, "/ds")
	sess := t.cam[stok]
	if !ok || !ok2 || sess == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if sess.s == nil { // plain login: requests and answers in clear
		var inner map[string]any
		_ = json.Unmarshal(body, &inner)
		writeJSON(t.camRequest(inner))
		return
	}
	seq, _ := strconv.Atoi(r.Header.Get("Seq"))
	if seq != sess.seq || r.Header.Get("Tapo_tag") != tplink.SSLAESTag(body, sess.cnonce, t.camPwdHash(), seq) {
		t.tagFailures++
		writeJSON(map[string]any{"error_code": -40401})
		return
	}
	sess.seq++
	if req.Method != "securePassthrough" {
		writeJSON(map[string]any{"error_code": -40210})
		return
	}
	b64, _ := req.Params["request"].(string)
	plain, err := sess.s.Decrypt(b64)
	if err != nil {
		writeJSON(map[string]any{"error_code": -40210})
		return
	}
	var inner map[string]any
	if json.Unmarshal(plain, &inner) != nil {
		writeJSON(map[string]any{"error_code": -40210})
		return
	}
	out, _ := json.Marshal(t.camRequest(inner))
	writeJSON(map[string]any{"error_code": 0, "result": map[string]any{"response": sess.s.Encrypt(out)}})
}

// camLogin answers both login steps and the plain login. The caller holds
// t.mu.
func (t *Tapo) camLogin(p map[string]any) map[string]any {
	user, _ := p["username"].(string)
	if t.PlainLogin {
		if p["hashed"] == true {
			h := md5.Sum([]byte(t.camPassword))
			if user != t.camUser || p["password"] != hexUp(h[:]) {
				return map[string]any{"error_code": -40401, "result": map[string]any{"data": map[string]any{"code": -40411}}}
			}
			stok := randHex(16)
			t.cam[stok] = &sslSession{}
			return map[string]any{"error_code": 0, "result": map[string]any{"stok": stok, "user_group": "root"}}
		}
		return map[string]any{"error_code": -40401, "result": map[string]any{"data": map[string]any{"code": -40401, "encrypt_type": []any{"1", "2"}}}}
	}
	cnonce, _ := p["cnonce"].(string)
	if user != t.camUser {
		return map[string]any{"error_code": -40401, "result": map[string]any{"data": map[string]any{"code": -40411, "encrypt_type": []any{"3"}}}}
	}
	if t.camNonces == nil {
		t.camNonces = map[string]string{}
	}
	ph := t.camPwdHash()
	if digest, ok := p["digest_passwd"].(string); ok {
		nonce := t.camNonces[cnonce]
		delete(t.camNonces, cnonce)
		if nonce == "" || digest != tplink.SSLAESDigest(cnonce, nonce, ph) {
			return map[string]any{"error_code": -40413, "result": map[string]any{"data": map[string]any{"code": -40413}}}
		}
		key, iv := tplink.SSLAESKeys(cnonce, nonce, ph)
		s, _ := tplink.NewAESSession(append(append([]byte(nil), key...), iv...))
		stok := randHex(16)
		t.cam[stok] = &sslSession{s: s, cnonce: cnonce, seq: 100}
		return map[string]any{"error_code": 0, "result": map[string]any{"stok": stok, "user_group": "root", "start_seq": 100}}
	}
	nonce := randHex(8)
	t.camNonces[cnonce] = nonce
	return map[string]any{"error_code": -40413, "result": map[string]any{"data": map[string]any{"code": -40413, "encrypt_type": []any{"3"}, "key": "",
		"nonce": nonce, "device_confirm": tplink.SSLAESConfirm(cnonce, nonce, ph)}}}
}

// camRequest answers one camera request object. The caller holds t.mu.
func (t *Tapo) camRequest(inner map[string]any) map[string]any {
	method, _ := inner["method"].(string)
	params, _ := inner["params"].(map[string]any)
	switch method {
	case "multipleRequest":
		reqs, _ := params["requests"].([]any)
		responses := []any{}
		for _, x := range reqs {
			rm, _ := x.(map[string]any)
			m, _ := rm["method"].(string)
			p, _ := rm["params"].(map[string]any)
			res, code := t.camOne(m, p)
			one := map[string]any{"method": m, "error_code": code}
			if code == 0 {
				one["result"] = res
			}
			responses = append(responses, one)
		}
		return map[string]any{"error_code": 0, "result": map[string]any{"responses": responses}}
	case "get", "set", "do":
		// The single form: {"method": "get", "<section>": {...}}.
		for k, v := range inner {
			if k == "method" {
				continue
			}
			vm, _ := v.(map[string]any)
			if method == "do" {
				if r, ok := t.camDo(k, vm); ok {
					return r
				}
			}
			sec := t.camSection(k)
			if method == "set" {
				mergeInto(sec, vm)
				return map[string]any{"error_code": 0}
			}
			return map[string]any{"error_code": 0, k: sec}
		}
		return map[string]any{"error_code": -40210}
	}
	res, code := t.camOne(method, params)
	if code != 0 {
		return map[string]any{"error_code": code}
	}
	return map[string]any{"error_code": 0, "result": res}
}

func (t *Tapo) camSection(name string) map[string]any {
	if t.CamSections == nil {
		t.CamSections = map[string]map[string]any{}
	}
	if t.CamSections[name] == nil {
		t.CamSections[name] = map[string]any{}
	}
	return t.CamSections[name]
}

func mergeInto(dst, src map[string]any) {
	for k, v := range src {
		if sub, ok := v.(map[string]any); ok {
			if cur, ok := dst[k].(map[string]any); ok {
				mergeInto(cur, sub)
				continue
			}
		}
		dst[k] = v
	}
}

// camOne answers one camera method with its result and error code.
func (t *Tapo) camOne(method string, p map[string]any) (map[string]any, int) {
	t.methods = append(t.methods, method)
	switch method {
	case "getDeviceInfo":
		return map[string]any{"device_info": map[string]any{"basic_info": t.Info}}, 0
	case "getAppComponentList":
		list := []any{}
		for _, id := range sortedKeys(t.Components) {
			list = append(list, map[string]any{"name": id, "version": t.Components[id]})
		}
		return map[string]any{"app_component": map[string]any{"app_component_list": list}}, 0
	case "getConnectionType":
		return map[string]any{"link_type": "wifi", "rssi": "3", "rssiValue": -50, "ssid": "RGVtby1Ib21l"}, 0
	case "setDeviceAlias":
		sys, _ := p["system"].(map[string]any)
		s, _ := sys["sys"].(map[string]any)
		if a, ok := s["dev_alias"].(string); ok {
			t.Info["device_alias"] = a
			return map[string]any{}, 0
		}
		return nil, -40209
	case "getChildDeviceList":
		if t.Children == nil {
			return nil, -40106
		}
		cc, _ := p["childControl"].(map[string]any)
		return page(t.Children, cc, "child_device_list", nil), 0
	case "getChildDeviceComponentList":
		if t.Children == nil {
			return nil, -40106
		}
		list := make([]map[string]any, len(t.Children))
		for i, c := range t.Children {
			id, _ := c["device_id"].(string)
			list[i] = map[string]any{"device_id": id, "component_list": componentList(t.childComponents(c))}
		}
		cc, _ := p["childControl"].(map[string]any)
		return page(list, cc, "child_component_list", nil), 0
	case "controlChild":
		cc, _ := p["childControl"].(map[string]any)
		id, _ := cc["device_id"].(string)
		rd, _ := cc["request_data"].(map[string]any)
		for _, c := range t.Children {
			if c["device_id"] == id {
				m, _ := rd["method"].(string)
				pp, _ := rd["params"].(map[string]any)
				return map[string]any{"response_data": t.childRequest(c, id, m, pp)}, 0
			}
		}
		return nil, -40209
	}
	if res, code, ok := t.camMore(method, p); ok {
		return res, code
	}
	if strings.HasPrefix(method, "get") {
		out := map[string]any{}
		for k := range p {
			sec, ok := t.CamSections[k]
			if k == "system" {
				// The clock, which clients read on every update, and the
				// time zone, kept once asked so that setTimezone changes it.
				if !ok {
					sec, ok = t.camSection("system"), true
				}
				if sec["basic"] == nil {
					sec["basic"] = map[string]any{"timezone": "UTC-00:00", "timing_mode": "ntp", "zone_id": "Europe/London"}
				}
				now := time.Now().UTC()
				sec["clock_status"] = map[string]any{"local_time": now.Format("2006-01-02 15:04:05"), "seconds_from_1970": now.Unix()}
			}
			if !ok {
				return nil, -40106
			}
			out[k] = sec
		}
		if len(out) == 0 {
			return nil, -40106
		}
		return out, 0
	}
	if strings.HasPrefix(method, "set") {
		for k, v := range p {
			if _, ok := t.CamSections[k]; !ok {
				return nil, -40106
			}
			vm, _ := v.(map[string]any)
			mergeInto(t.camSection(k), vm)
		}
		return map[string]any{}, 0
	}
	return nil, -40106
}
