package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These are the device's addresses as reported by a fake firmware. They are
// invented, not taken from anyone's modem.
const (
	testHost    = "192.0.2.10"       // TEST-NET-1, reserved for documentation
	testCarrier = "TEST-CARRIER-1"   //
	testModule  = "TEST-MODULE-1"    //
	testModel   = "TEST-MODEL-1"     //
	testIMEI    = "000000000000000"  //
	testPhone   = "09101234567"      // a number no network will route
	testIP      = "203.0.113.7"      // TEST-NET-3
)

// fakeRouter serves the firmware's endpoints with canned responses so the
// client can be tested without the hardware.
type fakeRouter struct {
	srv *httptest.Server
	// challenges counts how many times a key was issued.
	challenges int
	// setups counts how many times login was attempted.
	setups int
	// failLogin makes the setup step bounce back to the login page.
	failLogin bool
	// password is what the fake expects as the folded password.
	password string
	// pageOverride lets a test inject a page for a given path.
	pages map[string]string
	// postLog records every form the client submitted.
	postLog []string
	// lastPost holds the most recent form body.
	lastPost map[string][]string
	// outboxGrows makes the outbox page gain a record after a send, which is how
	// a successful send appears.
	outboxGrows bool
	// sent is set once a send form has been submitted.
	sent bool
}

func newFake(t *testing.T) *fakeRouter {
	t.Helper()
	f := &fakeRouter{
		password: "test-password",
		pages:    map[string]string{},
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/boafrm/formLoginKey", func(w http.ResponseWriter, r *http.Request) {
		f.challenges++
		r.ParseForm()
		if r.Form.Get("username") != "testuser" {
			json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "Invalid User Name !"})
			return
		}
		w.Header().Set("Set-Cookie", "webuicookie=fakesession; path=/")
		json.NewEncoder(w).Encode(map[string]any{
			"Challenge": "chal123",
			"PublicKey": "pub456",
			"status":    1,
		})
	})

	mux.HandleFunc("/boafrm/formLoginSetup", func(w http.ResponseWriter, r *http.Request) {
		f.setups++
		r.ParseForm()
		f.record(r)
		want := hmacMD5Hex(hmacMD5Hex("pub456"+f.password, "chal123"), "chal123")
		if f.failLogin || r.Form.Get("password") != want {
			http.Redirect(w, r, "/login.htm", http.StatusFound)
			return
		}
		w.Header().Set("Set-Cookie", "webuicookie=fakesession; path=/")
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/boafrm/formSmsManage", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.record(r)
		if r.Form.Get("action_id") == "sendMsg" {
			f.sent = true
		}
		w.Write([]byte(""))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// A send makes the outbox gain a record, so the page is built here
		// rather than served from the static map.
		if r.URL.Path == "/sms_outbox.htm" && f.outboxGrows && f.sent {
			w.Write([]byte(`var smsListInfo = "9}-{3}-{09101234567}-{2026-01-01 05:00:00}-{hello}-{|,|";`))
			return
		}
		if body, ok := f.pages[r.URL.Path]; ok {
			w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeRouter) record(r *http.Request) {
	f.postLog = append(f.postLog, r.URL.Path+" "+r.PostForm.Encode())
	m := map[string][]string{}
	for k, v := range r.PostForm {
		m[k] = v
	}
	f.lastPost = m
}

func (f *fakeRouter) client() *Client {
	return New(f.srv.URL, "testuser", f.password)
}

func TestFakeLoginSucceeds(t *testing.T) {
	f := newFake(t)
	c := f.client()
	if err := c.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if f.setups != 1 {
		t.Errorf("expected one setup call, got %d", f.setups)
	}
}

func TestFakeLoginRejectsWrongPassword(t *testing.T) {
	f := newFake(t)
	c := New(f.srv.URL, "testuser", "wrong")
	err := c.Login()
	if err == nil {
		t.Fatal("expected an error for a wrong password")
	}
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("got %v, want ErrAuthFailed", err)
	}
}

func TestFakeLoginRejectsUnknownUser(t *testing.T) {
	f := newFake(t)
	c := New(f.srv.URL, "someone", f.password)
	if err := c.Login(); err == nil {
		t.Fatal("expected an error for an unknown user")
	}
}

func TestGetReauthenticatesAfterSessionLoss(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "1}-{0}-{+09101234567}-{2026-01-01 10:00}-{hi}-{|,|";`

	c := f.client()
	if err := c.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}
	// Simulate the firmware's inactivity timeout clearing the session.
	c.Logout()
	if _, err := c.Inbox(); err != nil {
		t.Fatalf("Inbox after session loss: %v", err)
	}
	if f.setups < 2 {
		t.Errorf("expected the client to log in again, setups=%d", f.setups)
	}
}

// TestSendReportsNotRecordedWhenOutboxDoesNotGrow covers the case that matters
// most in practice: the firmware accepts the form and answers with an empty
// page even when the SIM or network refused the message, so only the outbox
// reveals the truth.
func TestSendReportsNotRecordedWhenOutboxDoesNotGrow(t *testing.T) {
	f := newFake(t)
	// The outbox is empty both before and after, so nothing was recorded.
	f.pages["/sms_outbox.htm"] = `var smsListInfo = "";`
	c := f.client()

	res, err := c.Send("+98", testPhone, "hello")
	if !errors.Is(err, ErrNotRecorded) {
		t.Errorf("err = %v, want ErrNotRecorded", err)
	}
	if res.Recorded {
		t.Error("Recorded should be false when the outbox did not grow")
	}
}

func TestSendReportsRecordedWhenOutboxGrows(t *testing.T) {
	f := newFake(t)
	// The outbox is empty at first and holds the message afterwards, which is
	// what a successful send looks like.
	f.outboxGrows = true
	f.pages["/sms_outbox.htm"] = `var smsListInfo = "";`
	c := f.client()

	res, err := c.Send("+98", testPhone, "hello")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !res.Recorded {
		t.Error("Recorded should be true when the outbox grew")
	}
}

func TestSendPostsExpectedForm(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "";`
	c := f.client()
	if _, err := c.Send("+98", testPhone, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := map[string]string{
		"action_id":      "sendMsg",
		"countryCode":    "+98",
		"sendMsgNumber":  testPhone,
		"sendMsgContent": "hello",
		"submitUrl":      "/sms_new.htm",
	}
	for k, v := range want {
		if got := f.lastPost[k]; len(got) == 0 || got[0] != v {
			t.Errorf("form %s = %v, want %q", k, got, v)
		}
	}
}

func TestSendBulkRecipients(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "";`
	c := f.client()
	if _, err := c.Send("+98", testPhone+";09101234568", "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := f.lastPost["sendMsgNumber"]; got[0] != testPhone+";09101234568" {
		t.Errorf("recipients = %v", got)
	}
}

func TestDeletePassesCommaList(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "";`
	c := f.client()
	if err := c.Delete(FolderInbox, "4", "5", "6"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := f.lastPost["action_value"]; got[0] != "4,5,6" {
		t.Errorf("indices = %v, want 4,5,6", got)
	}
	if got := f.lastPost["action_id"]; got[0] != "delete" {
		t.Errorf("action_id = %v", got)
	}
}

// TestMarkReadAcceptsMultiPartIndex covers a long SMS, which the firmware stores
// as several parts and reports as one row with a comma separated index.
func TestMarkReadAcceptsMultiPartIndex(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "70,71,72}-{0}-{SENDER}-{2026-01-01 10:00}-{a long message}-{|,|";`
	c := f.client()
	if err := c.MarkRead(FolderInbox, "70,71,72"); err != nil {
		t.Fatalf("MarkRead with a multi-part index: %v", err)
	}
	if got := f.lastPost["action_id"]; got[0] != "readMsg" {
		t.Errorf("action_id = %v", got)
	}
	if got := f.lastPost["action_value"]; got[0] != "70,71,72" {
		t.Errorf("action_value = %v, want 70,71,72", got)
	}
}

func TestMarkReadRejectsBadIndex(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "";`
	c := f.client()
	for _, bad := range []string{"", "abc", "1,abc", "1,", ",1"} {
		if err := c.MarkRead(FolderInbox, bad); err == nil {
			t.Errorf("expected an error for index %q", bad)
		}
	}
}

func TestDeleteRejectsBadIndex(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "";`
	c := f.client()
	if err := c.Delete(FolderInbox, "1,x"); err == nil {
		t.Error("expected an error for a malformed index list")
	}
}

func TestSetStoragePosts(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_settings.htm"] = `var x = 1;`
	c := f.client()
	if err := c.SetStorage("ME"); err != nil {
		t.Fatalf("SetStorage: %v", err)
	}
	if got := f.lastPost["sms_storage"]; got[0] != "ME" {
		t.Errorf("sms_storage = %v", got)
	}
	if got := f.lastPost["action_id"]; got[0] != "sms_settings" {
		t.Errorf("action_id = %v", got)
	}
}

func TestMessagesParsesInbox(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_inbox.htm"] = `var smsListInfo = "7}-{0}-{+09101234567}-{2026-01-01 10:00:00}-{line one<br><br>line two}-{|,|8}-{1}-{A SENDER}-{2026-01-01 09:00}-{ok}-{|,|";`
	c := f.client()
	msgs, err := c.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Index != "7" || msgs[0].Number != "+09101234567" {
		t.Errorf("first message = %+v", msgs[0])
	}
	if msgs[0].Content != "line one\nline two" {
		t.Errorf("content = %q, want newlines converted", msgs[0].Content)
	}
	if msgs[0].Read {
		t.Error("status 0 should be unread")
	}
	if !msgs[1].Read {
		t.Error("status 1 should be read")
	}
}

func TestMessagesEmptyList(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_outbox.htm"] = `var smsListInfo = "";`
	c := f.client()
	msgs, err := c.Messages(FolderOutbox)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d messages, want 0", len(msgs))
	}
}

func TestStatusParsesJSON(t *testing.T) {
	f := newFake(t)
	f.pages["/getwaninfo.cgi"] = `{
      "ipv4": {"Status": "Disconnected", "ip": "Not Available"},
      "ipv6": {"Status": "Connected", "ip": "2001:db8::1"},
      "lte": {"signal": "18,99", "connType": "` + testCarrier + `", "Status": "Connected",
              "upTime": "235549", "ip": "` + testIP + `"},
      "system_uptime": {"upTime": "259486"},
      "client_num": {"num": "2"}
    }`
	c := f.client()
	s, err := c.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !s.Connected() {
		t.Error("expected the LTE link to read as connected")
	}
	if s.LTEConnType != testCarrier {
		t.Errorf("carrier = %q, want %q", s.LTEConnType, testCarrier)
	}
	if s.LTEIP != testIP || s.LTEUpTime != 235549 {
		t.Errorf("lte = %+v", s)
	}
	if s.IPv4Connected {
		t.Error("IPv4 should read as disconnected")
	}
	if !s.IPv6Connected {
		t.Error("IPv6 should read as connected")
	}
	if s.ClientCount != 2 {
		t.Errorf("clients = %d, want 2", s.ClientCount)
	}
}

func TestUSSDTimesOutWithClearError(t *testing.T) {
	f := newFake(t)
	f.pages["/ussd.htm"] = `var ussdStatus = '0';`
	c := f.client()
	_, err := c.SendUSSD("*100#")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "USSD timed out") {
		t.Errorf("error = %v, want a USSD timeout", err)
	}
}

func TestUSSDReturnsResult(t *testing.T) {
	f := newFake(t)
	// The first poll reports a result; later polls would report idle, so the
	// client must return on the first non-idle answer.
	f.pages["/ussd.htm"] = `var ussdStatus = '2';
      <div id="ussd_result_id">Your balance is 1000</div>
      <div id="ussd_menu_id"></div>`
	c := f.client()
	resp, err := c.SendUSSD("*100#")
	if err != nil {
		t.Fatalf("SendUSSD: %v", err)
	}
	if resp.State != USSDResult {
		t.Errorf("state = %v, want USSDResult", resp.State)
	}
	if !strings.Contains(resp.Text, "1000") {
		t.Errorf("text = %q, want it to mention the balance", resp.Text)
	}
}

func TestUSSDMenuOptions(t *testing.T) {
	f := newFake(t)
	f.pages["/ussd.htm"] = `var ussdStatus = '1';
      <div id="ussd_menu_id">1. Balance<br>2. Data<br>3. Top up</div>
      <div id="ussd_result_id"></div>`
	c := f.client()
	resp, err := c.SendUSSD("*100#")
	if err != nil {
		t.Fatalf("SendUSSD: %v", err)
	}
	if resp.State != USSDMenu {
		t.Fatalf("state = %v, want USSDMenu", resp.State)
	}
	if len(resp.Options) < 3 {
		t.Errorf("options = %v, want at least 3", resp.Options)
	}
}

func TestStorageReadsCheckedValue(t *testing.T) {
	f := newFake(t)
	f.pages["/sms_settings.htm"] = `<tr>
      <input type="radio" name="sms_storage" id="sms_storage" value="SM">
      <input type="radio" name="sms_storage" id="sms_storage2" value="ME" checked>
      </tr>`
	c := f.client()
	got, err := c.Storage()
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	if got != "ME" {
		t.Errorf("storage = %q, want ME", got)
	}
}
