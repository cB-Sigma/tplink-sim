package sim

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/cB-Sigma/tplink"
)

// smart answers one Tapo request ({"method", "params"}) the way a device
// does: get_device_info, set_device_info, get_energy_usage,
// get_current_power, get_device_time, component_nego, multipleRequest, and
// for hubs and strips get_child_device_list (paged by start_index, with
// "sum") and control_child (passed to the child named by device_id). An
// unknown method answers -1002. The caller holds t.mu.
func (t *Tapo) smart(method string, params map[string]any) map[string]any {
	t.methods = append(t.methods, method)
	out := map[string]any{"error_code": 0}
	switch method {
	case "multipleRequest":
		// Several requests in one, answered in order, each with its own
		// error code (python-kasa sends most of its requests so).
		reqs, _ := params["requests"].([]any)
		responses := make([]any, 0, len(reqs))
		for _, r := range reqs {
			rm, _ := r.(map[string]any)
			m, _ := rm["method"].(string)
			p, _ := rm["params"].(map[string]any)
			res := t.smart(m, p)
			one := map[string]any{"method": m, "error_code": res["error_code"]}
			if v, ok := res["result"]; ok {
				one["result"] = v
			}
			responses = append(responses, one)
		}
		out["result"] = map[string]any{"responses": responses}
	case "component_nego":
		// The components the simulator answers for: the device itself and
		// its clock, the energy meter when it has one, children when it is
		// a hub or strip.
		list := []any{map[string]any{"id": "device", "ver_code": 2}, map[string]any{"id": "time", "ver_code": 1}}
		if t.Usage != nil {
			list = append(list, map[string]any{"id": "energy_monitoring", "ver_code": 2})
		}
		if t.Children != nil {
			list = append(list, map[string]any{"id": "control_child", "ver_code": 2}, map[string]any{"id": "child_device", "ver_code": 2})
		}
		for _, id := range sortedKeys(t.Components) {
			list = append(list, map[string]any{"id": id, "ver_code": t.Components[id]})
		}
		out["result"] = map[string]any{"component_list": list}
	case "get_device_time":
		out["result"] = map[string]any{"time_diff": 0, "timestamp": time.Now().Unix(), "region": "Europe/London"}
	case "get_current_power":
		if t.Usage == nil {
			return map[string]any{"error_code": -1002}
		}
		out["result"] = map[string]any{"current_power": t.Usage["current_power"]}
	case "get_device_info":
		out["result"] = t.Info
	case "set_device_info":
		for k, v := range params {
			t.Info[k] = v
		}
		// No result: the tapo crate reads a result here as {"response"},
		// and an empty one fails it (found by the TPAP interop test).
	case "get_energy_usage":
		if t.Usage == nil {
			return map[string]any{"error_code": -1002}
		}
		out["result"] = t.Usage
	case "get_child_device_list":
		if t.Children == nil {
			return map[string]any{"error_code": -1002}
		}
		start := int(num(params["start_index"]))
		size := t.PageSize
		if size <= 0 {
			size = 10
		}
		end := min(start+size, len(t.Children))
		page := []any{}
		for i := start; i < end; i++ {
			page = append(page, t.Children[i])
		}
		out["result"] = map[string]any{"child_device_list": page, "start_index": start, "sum": len(t.Children)}
	case "control_child":
		id, _ := params["device_id"].(string)
		rd, _ := params["requestData"].(map[string]any)
		var ch map[string]any
		for _, c := range t.Children {
			if c["device_id"] == id {
				ch = c
			}
		}
		if ch == nil {
			return map[string]any{"error_code": -1003}
		}
		m, _ := rd["method"].(string)
		p, _ := rd["params"].(map[string]any)
		out["result"] = map[string]any{"responseData": t.childRequest(ch, id, m, p)}
	default:
		if r, ok := t.smartMore(method, params); ok {
			return r
		}
		return map[string]any{"error_code": -1002}
	}
	return out
}

// childRequest answers one request to a child (a multipleRequest is
// answered request by request). The caller holds t.mu.
func (t *Tapo) childRequest(ch map[string]any, id, m string, p map[string]any) map[string]any {
	t.methods = append(t.methods, "child:"+m)
	inner := map[string]any{"error_code": 0}
	switch m {
	case "multipleRequest":
		reqs, _ := p["requests"].([]any)
		responses := make([]any, 0, len(reqs))
		for _, r := range reqs {
			rm, _ := r.(map[string]any)
			mm, _ := rm["method"].(string)
			pp, _ := rm["params"].(map[string]any)
			res := t.childRequest(ch, id, mm, pp)
			one := map[string]any{"method": mm, "error_code": res["error_code"]}
			if v, ok := res["result"]; ok {
				one["result"] = v
			}
			responses = append(responses, one)
		}
		inner["result"] = map[string]any{"responses": responses}
		return inner
	case "get_device_time":
		inner["result"] = map[string]any{"time_diff": 0, "timestamp": time.Now().Unix(), "region": "Europe/London"}
		return inner
	case "component_nego":
		inner["result"] = map[string]any{"component_list": componentList(t.childComponents(ch))}
		return inner
	case "get_connect_cloud_state":
		inner["result"] = map[string]any{"status": 0}
		return inner
	case "get_temp_humidity_records":
		if _, ok := ch["current_temp"]; !ok {
			return map[string]any{"error_code": -1002}
		}
		temps, hums := []any{}, []any{}
		for i := 0; i < 24; i++ {
			temps = append(temps, num(ch["current_temp"])*10)
			hums = append(hums, num(ch["current_humidity"]))
		}
		inner["result"] = map[string]any{"local_time": time.Now().Unix(), "past24h_temp": temps, "past24h_humidity": hums,
			"past24h_temp_exception": make([]any, 24), "past24h_humidity_exception": make([]any, 24)}
		return inner
	}
	switch m {
	case "get_device_info":
		inner["result"] = ch
	case "set_device_info":
		for k, v := range p {
			ch[k] = v
		}
		inner["result"] = map[string]any{}
	case "get_trigger_logs":
		logs := t.TriggerLogs[id]
		if cat, _ := ch["category"].(string); logs == nil && !strings.Contains(cat, "button") {
			return map[string]any{"error_code": -1002}
		}
		l := make([]any, len(logs))
		for i, x := range logs {
			l[i] = x
		}
		startID := 0
		if len(logs) > 0 {
			startID = int(num(logs[0]["id"]))
		}
		inner["result"] = map[string]any{"logs": l, "start_id": startID, "sum": len(logs)}
	default:
		return map[string]any{"error_code": -1002}
	}
	return inner
}

// Press adds a press to a button's trigger log (event singleClick,
// doubleClick or rotation).
func (t *Tapo) Press(deviceID, event string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.TriggerLogs == nil {
		t.TriggerLogs = map[string][]map[string]any{}
	}
	t.logID++
	e := map[string]any{"id": float64(t.logID), "event": event, "eventId": fmt.Sprintf("evt-%d", t.logID), "timestamp": float64(1759600000 + t.logID)}
	t.TriggerLogs[deviceID] = append([]map[string]any{e}, t.TriggerLogs[deviceID]...)
}

// Child returns a child's field.
func (t *Tapo) Child(deviceID, key string) any {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.Children {
		if c["device_id"] == deviceID {
			return c[key]
		}
	}
	return nil
}

// SetChild changes a child's field.
func (t *Tapo) SetChild(deviceID, key string, v any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.Children {
		if c["device_id"] == deviceID {
			c[key] = v
		}
	}
}

// aesSession is one securePassthrough session.
type aesSession struct {
	s     *tplink.AESSession
	token string
}

// StartTapoAES starts a Tapo device on older firmware, which speaks
// securePassthrough instead of KLAP: POST /app with method handshake
// (the client's RSA public key; the answer is a fresh AES key and IV
// encrypted to it with PKCS#1 v1.5 and a TP_SESSIONID cookie), then
// securePassthrough requests; login_device checks the account (login
// version 2: password2 = base64 of the SHA-1 hex of the password;
// version 1: password = base64 of the password) and answers a token
// that later requests carry in the URL. A refused login answers -1501;
// an unknown token 9999.
func StartTapoAES(username, password string, loginVersion int, info, usage map[string]any) *Tapo {
	t := &Tapo{version: loginVersion, Info: info, Usage: usage, aes: map[string]*aesSession{}, sessions: map[string]*session{}}
	user := base64.StdEncoding.EncodeToString([]byte(sha1hex(username)))
	pw1 := base64.StdEncoding.EncodeToString([]byte(password))
	pw2 := base64.StdEncoding.EncodeToString([]byte(sha1hex(password)))
	t.HTTP = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound) // no KLAP here
			return
		}
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		writeJSON := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch req.Method {
		case "handshake":
			key, _ := req.Params["key"].(string)
			blk, _ := pem.Decode([]byte(key))
			if blk == nil {
				writeJSON(map[string]any{"error_code": -1010})
				return
			}
			pk, err := x509.ParsePKIXPublicKey(blk.Bytes)
			rk, ok := pk.(*rsa.PublicKey)
			if err != nil || !ok {
				writeJSON(map[string]any{"error_code": -1010})
				return
			}
			keyIV := make([]byte, 32)
			_, _ = rand.Read(keyIV)
			enc, err := rsa.EncryptPKCS1v15(rand.Reader, rk, keyIV)
			if err != nil {
				writeJSON(map[string]any{"error_code": -1010})
				return
			}
			s, _ := tplink.NewAESSession(keyIV)
			t.next++
			id := fmt.Sprintf("AESSESSION%04d", t.next)
			t.aes[id] = &aesSession{s: s}
			w.Header().Add("Set-Cookie", "TP_SESSIONID="+id+";TIMEOUT=1440")
			writeJSON(map[string]any{"error_code": 0, "result": map[string]any{"key": base64.StdEncoding.EncodeToString(enc)}})
		case "securePassthrough":
			c, _ := r.Cookie("TP_SESSIONID")
			sess := t.aes[valueOf(c)]
			if sess == nil {
				writeJSON(map[string]any{"error_code": 9999})
				return
			}
			b64, _ := req.Params["request"].(string)
			plain, err := sess.s.Decrypt(b64)
			if err != nil {
				writeJSON(map[string]any{"error_code": -1003})
				return
			}
			var in struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			_ = json.Unmarshal(plain, &in)
			var inner map[string]any
			if in.Method == "login_device" {
				u, _ := in.Params["username"].(string)
				ok := u == user
				if p2, has := in.Params["password2"].(string); has {
					ok = ok && t.version == 2 && p2 == pw2
				} else {
					p1, _ := in.Params["password"].(string)
					ok = ok && t.version != 2 && p1 == pw1
				}
				if !ok {
					inner = map[string]any{"error_code": -1501}
				} else {
					sess.token = fmt.Sprintf("TOKEN%04d", t.next)
					inner = map[string]any{"error_code": 0, "result": map[string]any{"token": sess.token}}
				}
			} else if sess.token == "" || r.URL.Query().Get("token") != sess.token {
				writeJSON(map[string]any{"error_code": 9999})
				return
			} else {
				inner = t.smart(in.Method, in.Params)
			}
			ib, _ := json.Marshal(inner)
			writeJSON(map[string]any{"error_code": 0, "result": map[string]any{"response": sess.s.Encrypt(ib)}})
		default:
			writeJSON(map[string]any{"error_code": -1002})
		}
	}))
	return t
}

// ForgetAES drops every securePassthrough session (a device reboot).
func (t *Tapo) ForgetAES() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.aes = map[string]*aesSession{}
}

func sha1hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// Responder answers the UDP 20002 discovery probe for simulated Tapo
// devices on one loopback port, as each real device does on its own
// address: the probe must be well formed (version 2, op code 1, length
// and CRC-32) and carry an RSA key, or it is ignored.
type Responder struct {
	conn *net.UDPConn
	wg   sync.WaitGroup

	mu      sync.Mutex
	devices []*Tapo
	probes  int
}

// StartResponder starts a responder for the devices.
func StartResponder(devices ...*Tapo) (*Responder, error) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}
	r := &Responder{conn: c, devices: devices}
	r.wg.Add(1)
	go r.serve()
	return r, nil
}

// Addr is the responder's address (127.0.0.1:port), to send probes to
// instead of the broadcast address.
func (r *Responder) Addr() string { return r.conn.LocalAddr().String() }

// Probes counts the well-formed probes received.
func (r *Responder) Probes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.probes
}

// Set replaces the devices that answer (a device that moved).
func (r *Responder) Set(devices ...*Tapo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.devices = devices
}

// Close stops the responder.
func (r *Responder) Close() { r.conn.Close(); r.wg.Wait() }

func (r *Responder) serve() {
	defer r.wg.Done()
	buf := make([]byte, 8192)
	for {
		n, from, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		payload, err := tplink.CheckTapoProbe(buf[:n])
		if err != nil {
			continue
		}
		var p struct {
			Params struct {
				RSAKey string `json:"rsa_key"`
			} `json:"params"`
		}
		if json.Unmarshal(payload, &p) != nil || !strings.Contains(p.Params.RSAKey, "PUBLIC KEY") {
			continue
		}
		r.mu.Lock()
		r.probes++
		devs := append([]*Tapo(nil), r.devices...)
		r.mu.Unlock()
		for _, t := range devs {
			_, _ = r.conn.WriteToUDP(t.discoveryAnswer([]byte(p.Params.RSAKey)), from)
		}
	}
}

// discoveryAnswer is the device's 20002 answer.
func (t *Tapo) discoveryAnswer(probeKey []byte) []byte {
	t.mu.Lock()
	info := copyMap(t.Info) // copied: requests change it while the answer is built
	sysinfo := map[string]any(nil)
	if t.iot != nil {
		t.iot.mu.Lock()
		sysinfo = copyMap(t.iot.Sysinfo)
		t.iot.mu.Unlock()
	}
	enc, lv := "KLAP", t.version
	if t.aes != nil {
		enc = "AES"
	}
	secret := copyMap(t.DiscoverySecret)
	https, camera := t.https, t.cam != nil
	if camera {
		enc = "AES"
	}
	var tpap map[string]any
	if t.tpap != nil {
		tpap = t.tpapObject()
		if !camera {
			enc = "TPAP"
		}
	}
	t.mu.Unlock()
	_, port, _ := net.SplitHostPort(t.Addr())
	var p int
	fmt.Sscan(port, &p)
	model, typ, mac, id := "", "", "", ""
	if camera {
		model, _ = info["device_model"].(string)
		typ, _ = info["device_type"].(string)
		mac, _ = info["mac"].(string)
		id, _ = info["dev_id"].(string)
	} else if info != nil {
		model, _ = info["model"].(string)
		typ, _ = info["type"].(string)
		mac, _ = info["mac"].(string)
		id, _ = info["device_id"].(string)
	} else if sysinfo != nil {
		model, _ = sysinfo["model"].(string)
		mac, _ = sysinfo["mac"].(string)
		typ = "SMART.KASAPLUG"
		id, _ = sysinfo["deviceId"].(string)
	}
	res := map[string]any{"device_id": id, "owner": "0123456789ABCDEF", "device_type": typ, "device_model": model + "(EU)", "ip": "127.0.0.1",
		"mac": strings.ToUpper(strings.ReplaceAll(mac, ":", "-")), "is_support_iot_cloud": true, "obd_src": "tplink", "factory_default": false,
		"mgt_encrypt_schm": map[string]any{"is_support_https": https, "encrypt_type": enc, "http_port": p, "lv": lv}}
	if camera {
		res["encrypt_type"] = []any{"3"}
		res["mgt_encrypt_schm"] = map[string]any{"is_support_https": true, "encrypt_type": enc, "http_port": p}
	}
	if tpap != nil {
		res["tpap"] = tpap
	}
	if secret != nil {
		if ei, err := tplink.SealTapoDiscovery(probeKey, secret); err == nil {
			res["encrypt_info"] = ei
		}
	}
	body, _ := json.Marshal(map[string]any{"error_code": 0, "result": res})
	hdr := make([]byte, 16)
	hdr[0], hdr[1] = 2, 0
	binary.BigEndian.PutUint16(hdr[2:], 1)
	binary.BigEndian.PutUint16(hdr[4:], uint16(len(body)))
	hdr[6] = 0x11
	msg := append(hdr, body...)
	binary.BigEndian.PutUint32(msg[12:], 0x5A6B7C8D)
	binary.BigEndian.PutUint32(msg[12:], crc32.ChecksumIEEE(msg))
	return msg
}
