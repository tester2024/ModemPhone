package router

import (
	"strconv"
	"strings"
)

// Status is the router's live connection state, as reported by the CGI the
// home page polls.
type Status struct {
	// LTE
	LTEStatus   string
	LTEConnType string // carrier name the modem is camped on, e.g. "TEST-CARRIER-1"
	LTEUpTime   int    // seconds
	LTEIP       string
	LTESignal   string
	LTEErr      string

	IPv4Connected bool
	IPv4IP        string

	IPv6Connected bool
	IPv6IP        string

	UptimeSeconds int
	ClientCount   int
}

// Connected reports whether the LTE link is up.
func (s Status) Connected() bool {
	return strings.EqualFold(s.LTEStatus, "Connected") || strings.EqualFold(s.LTEStatus, "1")
}

// wanInfo mirrors the JSON served by /getwaninfo.cgi.
//
// Several numeric fields arrive as bare numbers rather than strings, so each is
// decoded as a loose number that tolerates both forms.
type wanInfo struct {
	IPv4 struct {
		ConnType string      `json:"connType"`
		Status   string      `json:"Status"`
		UpTime   looseNumber `json:"upTime"`
		IP       string      `json:"ip"`
	} `json:"ipv4"`
	IPv6 struct {
		ConnType string      `json:"connType"`
		Status   string      `json:"Status"`
		UpTime   looseNumber `json:"upTime"`
		IP       string      `json:"ip"`
	} `json:"ipv6"`
	LTE struct {
		RSRP     string      `json:"rsrp"`
		Signal   string      `json:"signal"`
		ConnType string      `json:"connType"`
		Status   string      `json:"Status"`
		UpTime   looseNumber `json:"upTime"`
		IP       string      `json:"ip"`
	} `json:"lte"`
	SystemUptime struct {
		UpTime looseNumber `json:"upTime"`
	} `json:"system_uptime"`
	ClientNum struct {
		Num looseNumber `json:"num"`
	} `json:"client_num"`
}

// looseNumber decodes a JSON value that the firmware may encode as a number, a
// numeric string, or a placeholder such as "Not Available".
type looseNumber int

func (n *looseNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" || s == "Not Available" {
		*n = 0
		return nil
	}
	if v, err := strconv.Atoi(s); err == nil {
		*n = looseNumber(v)
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		*n = looseNumber(int(f))
		return nil
	}
	*n = 0
	return nil
}

// Status reads the router's live connection state.
func (c *Client) Status() (Status, error) {
	body, err := c.get("/getwaninfo.cgi?cur_time=0")
	if err != nil {
		return Status{}, err
	}
	var w wanInfo
	if err := decodeJSONLoose(body, &w); err != nil {
		return Status{}, err
	}
	s := Status{
		LTEStatus:     w.LTE.Status,
		LTEConnType:   w.LTE.ConnType,
		LTEUpTime:     int(w.LTE.UpTime),
		LTEIP:         w.LTE.IP,
		LTESignal:     w.LTE.Signal,
		IPv4Connected: strings.EqualFold(w.IPv4.Status, "Connected"),
		IPv4IP:        w.IPv4.IP,
		IPv6Connected: strings.EqualFold(w.IPv6.Status, "Connected"),
		IPv6IP:        w.IPv6.IP,
		UptimeSeconds: int(w.SystemUptime.UpTime),
		ClientCount:   int(w.ClientNum.Num),
	}
	if !s.Connected() {
		s.LTEErr = "not registered"
	}
	return s, nil
}

// DeviceInfo is the identity block the home page shows.
type DeviceInfo struct {
	Model      string
	Hardware   string
	Firmware   string
	ModuleName string
	IMEI       string
	IMSI       string
	ICCID      string
}

// Device reads the model, firmware and SIM identity. The home page carries
// these in its data block, with some values filled in later by script, so this
// returns what the page actually contains.
func (c *Client) Device() (DeviceInfo, error) {
	page, err := c.get("/home.htm")
	if err != nil {
		return DeviceInfo{}, err
	}
	var d DeviceInfo
	d.Model = jsVarString(page, "modelInfo")
	d.ModuleName = betweenTags(page, "secindex_ShowLtemoduleName")
	if d.ModuleName == "" {
		d.ModuleName = betweenTags(page, "secindex_ShowLteModuleName")
	}
	d.IMEI = betweenTags(page, "secindex_ShowLteImei")
	if d.IMEI == "" {
		d.IMEI = betweenTags(page, "secindex_ShowLteIMEI")
	}
	if v, ok := extractJSString(page, "imei"); ok {
		d.IMEI = v
	}
	if v, ok := extractJSString(page, "imsi"); ok {
		d.IMSI = v
	}
	if v, ok := extractJSString(page, "iccid"); ok {
		d.ICCID = v
	}
	if d.Model == "" {
		d.Model = "unknown"
	}
	return d, nil
}

// betweenTags pulls the text between an element's id and its closing tag,
// which is how the home page exposes its status values.
func betweenTags(page, id string) string {
	i := strings.Index(page, `id="`+id+`"`)
	if i < 0 {
		return ""
	}
	rest := page[i:]
	j := strings.Index(rest, ">")
	if j < 0 {
		return ""
	}
	rest = rest[j+1:]
	k := strings.Index(rest, "<")
	if k < 0 {
		return ""
	}
	return strings.TrimSpace(cleanBody(rest[:k]))
}

// jsVarString reads a JavaScript variable assigned a quoted string, which is
// how the home page stores static values such as the model name.
func jsVarString(page, name string) string {
	if v, ok := extractJSString(page, name); ok {
		return v
	}
	return ""
}
