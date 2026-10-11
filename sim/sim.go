// Package sim simulates TP-Link devices for tests and demos: Kasa devices
// on the legacy protocol (TCP and UDP on one loopback port, get_sysinfo,
// relay and child relay state, dimmer brightness, bulb light state, the
// energy meter with day and month statistics, runtime statistics,
// schedule, countdown and away-mode rules, dimmer transitions and fades,
// light strip effects, LED, name, reboot, cloud and Wi-Fi), Kasa and Tapo
// devices on KLAP v1 and v2 over HTTP or HTTPS (the two-step handshake,
// signed AES-128-CBC requests), Tapo devices on securePassthrough, Tapo
// hubs and strips with children, Tapo settings, rules, light effects,
// energy history, firmware update and set-up (TapoState), Tapo cameras and
// the H200 hub on their HTTPS transport (StartCamera), Kasa cameras
// (StartKasaCamera), the plain HTTPS transport of robot vacuums
// (StartTapoSSL), and a responder to the UDP 20002 discovery probe, with
// the answer's encrypted part on demand. Built from the protocol facts in the
// tplink module, which come from public descriptions; not from any
// TP-Link or open-source client code.
package sim

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"time"

	"github.com/cB-Sigma/tplink"
)

// Kasa is a simulated Kasa device.
type Kasa struct {
	KasaState // rules, dimmer settings, Wi-Fi and the rest (kasa_modules.go)

	mu      sync.Mutex
	Sysinfo map[string]any
	// Emeter is the get_realtime answer (nil: no energy meter).
	Emeter map[string]any
	// ChildEmeter are a strip's outlets' get_realtime answers, by child ID
	// (asked with a child_ids context, as HS300 strips are).
	ChildEmeter map[string]map[string]any
	requests    []map[string]any
	tcp         net.Listener
	udp         *net.UDPConn
	wg          sync.WaitGroup
	web         *httptest.Server // the HTTPS transport (StartKasaCamera)
}

// StartKasa starts a Kasa device on 127.0.0.1 (TCP and UDP on one port).
// A bulb whose sysinfo has "length" is a light strip (KL4xx): it answers
// only the strip lighting service. A dimmer (sysinfo "brightness") keeps
// its relay state when its brightness is set, as HS220 firmware does.
func StartKasa(sysinfo, emeter map[string]any) (*Kasa, error) {
	k := &Kasa{Sysinfo: sysinfo, Emeter: emeter}
	for i := 0; i < 20; i++ {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		port := l.Addr().(*net.TCPAddr).Port
		u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			l.Close()
			continue
		}
		k.tcp, k.udp = l, u
		break
	}
	if k.tcp == nil {
		return nil, fmt.Errorf("no loopback port free for both TCP and UDP")
	}
	k.wg.Add(2)
	go k.serveTCP()
	go k.serveUDP()
	return k, nil
}

// Addr is the device's address (127.0.0.1:port).
func (k *Kasa) Addr() string {
	if k.web != nil {
		return k.web.Listener.Addr().String()
	}
	return k.tcp.Addr().String()
}

// Close stops the device.
func (k *Kasa) Close() {
	if k.web != nil {
		k.web.Close()
		return
	}
	k.tcp.Close()
	k.udp.Close()
	k.wg.Wait()
}

// Requests returns the requests received.
func (k *Kasa) Requests() []map[string]any {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]map[string]any(nil), k.requests...)
}

// Set changes a sysinfo field.
func (k *Kasa) Set(key string, v any) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Sysinfo[key] = v
}

// Get returns a sysinfo field.
func (k *Kasa) Get(key string) any {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.Sysinfo[key]
}

func (k *Kasa) serveUDP() {
	defer k.wg.Done()
	buf := make([]byte, 4096)
	for {
		n, from, err := k.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(tplink.Decrypt(buf[:n]), &m) != nil {
			continue
		}
		out, _ := json.Marshal(k.handle(m))
		_, _ = k.udp.WriteToUDP(tplink.Encrypt(out), from)
	}
}

func (k *Kasa) serveTCP() {
	defer k.wg.Done()
	for {
		c, err := k.tcp.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			b, err := tplink.ReadFrame(c)
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(b, &m) != nil {
				return
			}
			out, _ := json.Marshal(k.handle(m))
			_, _ = c.Write(tplink.Frame(out))
		}()
	}
}

func num(v any) float64 { f, _ := v.(float64); return f }

// handle answers a request the way a device does, module by module.
func (k *Kasa) handle(req map[string]any) map[string]any {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.requests = append(k.requests, req)
	out := map[string]any{}
	ok := map[string]any{"err_code": 0.0}
	var childIDs []any
	if ctx, has := req["context"].(map[string]any); has {
		childIDs, _ = ctx["child_ids"].([]any)
	}
	for mod, v := range req {
		if mod == "context" {
			continue
		}
		mm, _ := v.(map[string]any)
		res := map[string]any{}
		for meth, arg := range mm {
			a, _ := arg.(map[string]any)
			if r, done := k.module(canonical(mod), meth, a, childIDs); done {
				res[meth] = r
				continue
			}
			switch canonical(mod) + "." + meth {
			case "system.get_sysinfo":
				cp := map[string]any{"err_code": 0.0}
				for kk, vv := range k.Sysinfo {
					cp[kk] = vv
				}
				res[meth] = cp
			case "system.set_relay_state":
				if len(childIDs) > 0 {
					ch, _ := k.Sysinfo["children"].([]any)
					for _, c := range ch {
						cm := c.(map[string]any)
						for _, id := range childIDs {
							if cm["id"] == id {
								cm["state"] = num(a["state"])
							}
						}
					}
				} else {
					k.Sysinfo["relay_state"] = num(a["state"])
				}
				res[meth] = ok
			case "smartlife.iot.dimmer.set_brightness":
				k.Sysinfo["brightness"] = num(a["brightness"])
				res[meth] = ok
			case "smartlife.iot.smartbulb.lightingservice.transition_light_state", "smartlife.iot.lightStrip.set_light_state":
				_, strip := k.Sysinfo["length"]
				if strip != (mod == "smartlife.iot.lightStrip") {
					res[meth] = map[string]any{"err_code": -1.0, "err_msg": "module not support"}
					continue
				}
				ls, _ := k.Sysinfo["light_state"].(map[string]any)
				for kk, vv := range a {
					if kk != "ignore_default" && kk != "transition_period" {
						ls[kk] = vv
					}
				}
				res[meth] = ls
			case "time.get_time":
				// The device clock, which python-kasa reads on every update.
				now := time.Now().UTC()
				res[meth] = map[string]any{"err_code": 0.0, "year": float64(now.Year()), "month": float64(now.Month()), "mday": float64(now.Day()),
					"hour": float64(now.Hour()), "min": float64(now.Minute()), "sec": float64(now.Second())}
			case "time.get_timezone":
				res[meth] = map[string]any{"err_code": 0.0, "index": 39.0} // a fixed zone of the device's table (GB)
			case "emeter.get_realtime":
				if len(childIDs) == 1 && k.ChildEmeter != nil {
					id, _ := childIDs[0].(string)
					if e, ok := k.ChildEmeter[id]; ok {
						cp := map[string]any{"err_code": 0.0}
						for kk, vv := range e {
							cp[kk] = vv
						}
						res[meth] = cp
					} else {
						res[meth] = map[string]any{"err_code": -14.0, "err_msg": "entry not exist"}
					}
					continue
				}
				if k.Emeter == nil {
					res[meth] = map[string]any{"err_code": -1.0, "err_msg": "module not support"}
				} else {
					cp := map[string]any{"err_code": 0.0}
					for kk, vv := range k.Emeter {
						cp[kk] = vv
					}
					res[meth] = cp
				}
			default:
				res[meth] = map[string]any{"err_code": -2.0, "err_msg": "member not support"}
			}
		}
		out[mod] = res
	}
	return out
}

// Tapo is a simulated KLAP device: a Tapo device (KLAP v2, SMART
// methods), or with StartKasaKLAP a Kasa device on newer firmware (KLAP
// v1 or v2 carrying the legacy Kasa JSON).
type Tapo struct {
	TapoState // settings, rules, firmware, Wi-Fi (smart_modules.go)

	HTTP *httptest.Server
	// BadAnswerSignature signs answers with a wrong signature (a firmware
	// that does not sign them the way requests are signed).
	BadAnswerSignature bool

	mu      sync.Mutex
	auth    []byte
	version int   // KLAP version: 1 (MD5 auth hash, one-seed proofs) or 2
	iot     *Kasa // non-nil: the payload is legacy Kasa JSON
	Info    map[string]any
	Usage   map[string]any
	// Children are a hub's paired devices or a strip's outlets, as
	// get_child_device_list lists them (device_id, category, …).
	Children []map[string]any
	// PageSize is how many children one get_child_device_list page holds
	// (default 10).
	PageSize int
	// TriggerLogs are buttons' logs by device_id, newest first.
	TriggerLogs map[string][]map[string]any
	logID       int
	aes         map[string]*aesSession // securePassthrough sessions by cookie
	sessions    map[string]*session
	methods     []string
	next        int

	https       bool                   // served over TLS
	cam         map[string]*sslSession // HTTPS camera sessions by token (StartCamera)
	camUser     string
	camPassword string
	camNonces   map[string]string // first login step's nonces by cnonce
	tagFailures int
}

type session struct {
	local, remote []byte
	s             *tplink.Session
	ok            bool
}

// StartTapo starts a Tapo device that knows the account (username,
// password).
func StartTapo(username, password string, info, usage map[string]any) *Tapo {
	t := &Tapo{auth: tplink.AuthHash(username, password), version: 2, Info: info, Usage: usage, sessions: map[string]*session{}}
	t.HTTP = httptest.NewServer(http.HandlerFunc(t.serve))
	return t
}

// StartKasaKLAP starts a Kasa device on newer firmware: KLAP (version 1
// or 2) with the legacy Kasa JSON (get_sysinfo, set_relay_state, ...)
// inside instead of the Tapo methods. Kasa returns its state.
func StartKasaKLAP(username, password string, version int, sysinfo, emeter map[string]any) (*Tapo, *Kasa) {
	k := &Kasa{Sysinfo: sysinfo, Emeter: emeter}
	auth := tplink.AuthHash(username, password)
	if version == 1 {
		auth = tplink.AuthHashV1(username, password)
	}
	t := &Tapo{auth: auth, version: version, iot: k, sessions: map[string]*session{}}
	t.HTTP = httptest.NewServer(http.HandlerFunc(t.serve))
	return t, k
}

// proofs are the handshake1 server proof and the handshake2 client proof
// for the device's KLAP version.
func (t *Tapo) proofs(local, remote []byte) (server, client []byte) {
	if t.version == 1 {
		return hash(local, t.auth), hash(remote, t.auth)
	}
	return hash(local, remote, t.auth), hash(remote, local, t.auth)
}

// Addr is the device's address (127.0.0.1:port).
func (t *Tapo) Addr() string { return t.HTTP.Listener.Addr().String() }

// Close stops the device.
func (t *Tapo) Close() { t.HTTP.Close() }

// Methods returns the methods called.
func (t *Tapo) Methods() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.methods...)
}

// Get returns a device_info field.
func (t *Tapo) Get(key string) any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Info[key]
}

// ForgetSessions drops every session (a device reboot).
func (t *Tapo) ForgetSessions() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions = map[string]*session{}
}

func hash(parts ...[]byte) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

func (t *Tapo) serve(w http.ResponseWriter, r *http.Request) {
	body := make([]byte, 0, 1024)
	buf := bytes.NewBuffer(body)
	_, _ = buf.ReadFrom(r.Body)
	body = buf.Bytes()
	t.mu.Lock()
	defer t.mu.Unlock()
	cookie, _ := r.Cookie("TP_SESSIONID")
	switch r.URL.Path {
	case "/app/handshake1":
		if len(body) != 16 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		remote := make([]byte, 16)
		_, _ = rand.Read(remote)
		t.next++
		id := fmt.Sprintf("SIMSESSION%04d", t.next)
		t.sessions[id] = &session{local: append([]byte(nil), body...), remote: remote}
		w.Header().Add("Set-Cookie", "TP_SESSIONID="+id+";TIMEOUT=86400")
		proof, _ := t.proofs(body, remote)
		_, _ = w.Write(append(remote, proof...))
	case "/app/handshake2":
		s := t.sessions[valueOf(cookie)]
		var want []byte
		if s != nil {
			_, want = t.proofs(s.local, s.remote)
		}
		if s == nil || !bytes.Equal(body, want) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		s.ok = true
		s.s = tplink.NewSession(s.local, s.remote, t.auth)
		w.WriteHeader(http.StatusOK)
	case "/app/request":
		s := t.sessions[valueOf(cookie)]
		if s == nil || !s.ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		seq, err := strconv.ParseInt(r.URL.Query().Get("seq"), 10, 32)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		plain, err := s.s.Decrypt(int32(seq), body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if t.iot != nil {
			// Legacy Kasa JSON inside KLAP; a Tapo request is not understood.
			var m map[string]any
			_ = json.Unmarshal(plain, &m)
			var out map[string]any
			if _, tapo := m["method"]; tapo {
				out = map[string]any{"err_code": -1.0, "err_msg": "module not support"}
			} else {
				out = t.iot.handle(m)
			}
			b, _ := json.Marshal(out)
			t.reply(w, s.s, int32(seq), b)
			return
		}
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal(plain, &req)
		b, _ := json.Marshal(t.smart(req.Method, req.Params))
		t.reply(w, s.s, int32(seq), b)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (t *Tapo) reply(w http.ResponseWriter, s *tplink.Session, seq int32, b []byte) {
	body := s.Seal(seq, b)
	if t.BadAnswerSignature {
		body[0] ^= 0xff
	}
	_, _ = w.Write(body)
}

func valueOf(c *http.Cookie) string {
	if c == nil {
		return ""
	}
	return c.Value
}

// Demo account of the Tapo devices.
const (
	DemoUser     = "demo@example.invalid"
	DemoPassword = "demo-tapo-password"
)

// Demo devices' MACs.
const (
	MACPlug  = "5c628b000001"
	MACStrip = "5c628b000002"
	MACBulb  = "5c628b000003"
	MACTapo  = "3c52a1000004"
	MACTapoB = "3c52a1000005"

	MACDimmer     = "5c628b000006"
	MACLightStrip = "5c628b000007"
	MACHub        = "3c52a1000008"
	MACCamera     = "003192000009"
)

// Demo camera account (the camera's local user).
const (
	DemoCameraUser     = "admin"
	DemoCameraPassword = "demo-camera-password"
)

// Demo is a set of simulated devices.
type Demo struct {
	Plug, Strip, Bulb  *Kasa
	TapoPlug, TapoBulb *Tapo
	// Added in v0.2.0.
	Dimmer, LightStrip *Kasa
	TapoHub, Camera    *Tapo
}

// Close stops every device.
func (d *Demo) Close() {
	for _, k := range []*Kasa{d.Plug, d.Strip, d.Bulb, d.Dimmer, d.LightStrip} {
		if k != nil {
			k.Close()
		}
	}
	for _, t := range []*Tapo{d.TapoPlug, d.TapoBulb, d.TapoHub, d.Camera} {
		if t != nil {
			t.Close()
		}
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// NewDemo starts a Kasa HS110 plug with an energy meter, a Kasa HS300-like
// strip with three outlets, a Kasa KL130 colour bulb, a Tapo P110 plug
// with energy use and a Tapo L530 colour bulb; since v0.2.0 also a Kasa
// HS220 dimmer, a Kasa KL430 light strip, a Tapo H100 hub with a
// temperature and humidity sensor, a button and a contact sensor, and a
// Tapo C210 camera on the HTTPS transport (DemoCameraUser,
// DemoCameraPassword).
func NewDemo() (*Demo, error) {
	d := &Demo{}
	var err error
	if d.Plug, err = StartKasa(map[string]any{"alias": "Washing machine", "model": "HS110(EU)", "sw_ver": "1.5.4 Build 180815", "mac": "5C:62:8B:00:00:01",
		"mic_type": "IOT.SMARTPLUGSWITCH", "relay_state": 1.0, "deviceId": "8006A1"}, map[string]any{"voltage_mv": 229400.0, "current_ma": 910.0, "power_mw": 205300.0, "total_wh": 88213.0}); err != nil {
		return nil, err
	}
	if d.Strip, err = StartKasa(map[string]any{"alias": "Desk strip", "model": "HS300(US)", "sw_ver": "1.0.21", "mac": "5C:62:8B:00:00:02",
		"mic_type": "IOT.SMARTPLUGSWITCH", "children": []any{
			map[string]any{"id": "8006A200", "state": 1.0, "alias": "Monitor"}, map[string]any{"id": "8006A201", "state": 0.0, "alias": "Lamp"},
			map[string]any{"id": "8006A202", "state": 1.0, "alias": "Charger"}}}, nil); err != nil {
		d.Close()
		return nil, err
	}
	if d.Bulb, err = StartKasa(map[string]any{"alias": "Reading lamp", "model": "KL130(EU)", "sw_ver": "1.8.11", "mic_mac": "5C628B000003",
		"mic_type": "IOT.SMARTBULB", "is_color": 1.0, "is_dimmable": 1.0, "is_variable_color_temp": 1.0,
		"light_state": map[string]any{"on_off": 1.0, "brightness": 70.0, "hue": 0.0, "saturation": 0.0, "color_temp": 2700.0}}, nil); err != nil {
		d.Close()
		return nil, err
	}
	d.TapoPlug = StartTapo(DemoUser, DemoPassword, map[string]any{"device_id": "80223F", "model": "P110", "type": "SMART.TAPOPLUG", "fw_ver": "1.3.0 Build 230905",
		"mac": "3C-52-A1-00-00-04", "nickname": b64("Microwave"), "device_on": false}, map[string]any{"current_power": 0.0, "today_energy": 120.0, "month_energy": 5400.0})
	d.TapoBulb = StartTapo(DemoUser, DemoPassword, map[string]any{"device_id": "80223G", "model": "L530", "type": "SMART.TAPOBULB", "fw_ver": "1.1.9 Build 230614",
		"mac": "3C-52-A1-00-00-05", "nickname": b64("Balcony bulb"), "device_on": true, "brightness": 60.0, "hue": 30.0, "saturation": 90.0, "color_temp": 0.0}, nil)

	if d.Dimmer, err = StartKasa(map[string]any{"alias": "Hall dimmer", "model": "HS220(US)", "sw_ver": "1.5.7", "mac": "5C:62:8B:00:00:06",
		"mic_type": "IOT.SMARTPLUGSWITCH", "relay_state": 1.0, "brightness": 40.0, "deviceId": "8006A6"}, nil); err != nil {
		d.Close()
		return nil, err
	}
	if d.LightStrip, err = StartKasa(map[string]any{"alias": "Shelf strip", "model": "KL430(US)", "sw_ver": "1.0.11", "mic_mac": "5C628B000007",
		"mic_type": "IOT.SMARTBULB", "is_color": 1.0, "is_dimmable": 1.0, "is_variable_color_temp": 1.0, "length": 16.0,
		"lighting_effect_state": map[string]any{"enable": 0.0, "name": "", "brightness": 50.0, "custom": 0.0, "id": ""},
		"light_state":           map[string]any{"on_off": 1.0, "brightness": 50.0, "hue": 200.0, "saturation": 80.0, "color_temp": 0.0}},
		map[string]any{"power_mw": 2700.0, "total_wh": 1193.0}); err != nil {
		d.Close()
		return nil, err
	}
	d.TapoHub = StartTapo(DemoUser, DemoPassword, map[string]any{"device_id": "80223H", "model": "H100", "type": "SMART.TAPOHUB", "fw_ver": "1.5.10 Build 240207",
		"mac": "3C-52-A1-00-00-08", "nickname": b64("Hall hub")}, nil)
	d.TapoHub.Children = []map[string]any{
		{"device_id": "80223H01", "model": "T315", "category": "subg.trigger.temp-hmdt-sensor", "type": "SMART.TAPOSENSOR", "nickname": b64("Nursery"),
			"current_temp": 22.4, "current_humidity": 51.0, "temp_unit": "celsius", "battery_percentage": 90.0, "at_low_battery": false, "status": "online"},
		{"device_id": "80223H02", "model": "S200B", "category": "subg.trigger.button", "type": "SMART.TAPOSENSOR", "nickname": b64("Door button"),
			"at_low_battery": false, "status": "online"},
		{"device_id": "80223H03", "model": "T110", "category": "subg.trigger.contact-sensor", "type": "SMART.TAPOSENSOR", "nickname": b64("Back door"),
			"open": false, "at_low_battery": false, "status": "online"},
	}
	d.Camera = StartCamera(DemoCameraUser, DemoCameraPassword, map[string]any{"device_type": "SMART.IPCAMERA", "device_model": "C210", "device_alias": "Porch",
		"sw_version": "1.3.11 Build 231117", "hw_version": "2.0", "mac": "00-31-92-00-00-09", "dev_id": "80223C", "device_info": "C210 2.0 IPC"})
	d.Camera.CamSections = map[string]map[string]any{"lens_mask": {"lens_mask_info": map[string]any{"enabled": "off"}}}
	return d, nil
}
