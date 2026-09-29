package router

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Message is one SMS in one of the phone's boxes.
type Message struct {
	Index   string // firmware's numeric id, needed to delete
	Status  string
	Number  string
	Time    string
	Content string
	Read    bool
}

// Folder identifies which message box to read.
type Folder string

// The boxes the firmware exposes.
const (
	FolderInbox  Folder = "inbox"
	FolderOutbox Folder = "outbox"
	FolderDrafts Folder = "drafts"
)

func (f Folder) page() string {
	switch f {
	case FolderInbox:
		return "/sms_inbox.htm"
	case FolderOutbox:
		return "/sms_outbox.htm"
	case FolderDrafts:
		return "/sms_drafts.htm"
	}
	return "/sms_inbox.htm"
}

// listVar is the JavaScript variable each box uses to inline its messages.
func (f Folder) listVar() string { return "smsListInfo" }

// Messages reads the given box. The firmware embeds the whole box in the page
// as a JavaScript literal, so one request is enough.
func (c *Client) Messages(f Folder) ([]Message, error) {
	page, err := c.get(f.page())
	if err != nil {
		return nil, err
	}
	raw, ok := extractJSString(page, f.listVar())
	if !ok {
		return nil, fmt.Errorf("router: %s page carried no message list", f)
	}
	recs := parseRecords(raw)
	out := make([]Message, 0, len(recs))
	for _, r := range recs {
		if len(r) < 5 {
			continue
		}
		m := Message{
			Index:   r[0],
			Status:  r[1],
			Number:  sanitizeUTF8(strings.TrimSpace(r[2])),
			Time:    strings.TrimSpace(r[3]),
			Content: sanitizeUTF8(cleanBody(r[4])),
		}
		// status 0 means unread in the boxes the firmware renders that way.
		m.Read = m.Status != "0"
		out = append(out, m)
	}
	return out, nil
}

// Inbox is a shorthand for Messages(FolderInbox).
func (c *Client) Inbox() ([]Message, error) { return c.Messages(FolderInbox) }

// ErrNoNumber is returned when a send is attempted without a recipient.
var ErrNoNumber = errors.New("router: no recipient number")

// SendResult reports what happened to a send request.
type SendResult struct {
	// Recorded is true when the modem wrote the message to its outbox, which
	// is the only confirmation the firmware offers.
	Recorded bool
	// Note is a short human-readable summary for the UI.
	Note string
}

// ErrNotRecorded reports that the modem accepted the form but did not store the
// message. The network or the SIM usually refuses it, most often a daily quota.
var ErrNotRecorded = errors.New("the modem did not record the message; the SIM or network likely refused it")

// Send delivers an SMS.
//
// number may hold several recipients separated by semicolons, which is how the
// firmware's own compose page expresses a bulk send. The firmware validates
// the number field against digits and semicolons only, so a leading country
// code belongs in countryCode rather than in number.
//
// The firmware gives no feedback on a send: it always answers with an empty
// page whether or not the message went out. So the outbox is checked
// afterwards, and a message the modem did not record is reported rather than
// reported as sent.
func (c *Client) Send(countryCode, number, body string) (SendResult, error) {
	number = strings.TrimSpace(number)
	body = strings.TrimSpace(body)
	if number == "" {
		return SendResult{}, ErrNoNumber
	}
	if body == "" {
		return SendResult{}, errors.New("router: message body is empty")
	}
	if err := validateNumber(number); err != nil {
		return SendResult{}, err
	}
	if err := validateCountryCode(countryCode); err != nil {
		return SendResult{}, err
	}
	if n := len([]rune(body)); n > MaxBodyRunes {
		return SendResult{}, fmt.Errorf("router: body is %d characters, limit is %d", n, MaxBodyRunes)
	}

	before := c.outboxSize()

	form := url.Values{
		"action_id":      {"sendMsg"},
		"action_value":   {"tmp"},
		"countryCode":    {countryCode},
		"sendMsgNumber":  {number},
		"sendMsgContent": {body},
		"submitUrl":      {"/sms_new.htm"},
	}
	if _, err := c.post("/boafrm/formSmsManage", form); err != nil {
		return SendResult{}, err
	}

	// The modem writes the sent copy to its outbox within a second or two. A
	// short poll distinguishes a real send from a silently refused one.
	if c.waitForOutbox(before) {
		return SendResult{Recorded: true, Note: "sent"}, nil
	}
	return SendResult{Recorded: false, Note: "not recorded"}, ErrNotRecorded
}

// outboxSize returns how many sent messages the modem is holding, or -1 when
// the outbox cannot be read.
func (c *Client) outboxSize() int {
	msgs, err := c.Messages(FolderOutbox)
	if err != nil {
		return -1
	}
	return len(msgs)
}

// waitForOutbox polls the outbox until it grows past before, or the deadline
// passes.
func (c *Client) waitForOutbox(before int) bool {
	if before < 0 {
		// The starting state was unknown, so there is nothing to compare with.
		return true
	}
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		if n := c.outboxSize(); n > before {
			return true
		}
	}
	return false
}

// MaxBodyRunes is the firmware's own limit on the compose field. Persian text
// is UCS-2, so 765 characters is about eleven 70-character SMS segments.
const MaxBodyRunes = 765

// Delete removes messages by their firmware indices. The indices are passed as
// a comma separated list, matching the delete button on the message pages.
func (c *Client) Delete(f Folder, indices ...string) error {
	if len(indices) == 0 {
		return errors.New("router: no messages selected")
	}
	for _, ix := range indices {
		if err := validateIndices(ix); err != nil {
			return err
		}
	}
	form := url.Values{
		"action_id":    {"delete"},
		"action_value": {strings.Join(indices, ",")},
		"submitUrl":    {string(f.page())},
	}
	_, err := c.post("/boafrm/formSmsManage", form)
	return err
}

// MarkRead flags a message as read, which the firmware does by asking for the
// message view with its index.
//
// A long SMS is stored as several parts and the firmware reports it as one row
// with a comma separated index, so the value is validated as a list.
func (c *Client) MarkRead(f Folder, index string) error {
	if err := validateIndices(index); err != nil {
		return err
	}
	form := url.Values{
		"action_id":    {"readMsg"},
		"action_value": {strings.TrimSpace(index)},
		"submitUrl":    {string(f.page())},
	}
	_, err := c.post("/boafrm/formSmsManage", form)
	return err
}

// validateIndices checks a comma separated list of firmware message indices.
func validateIndices(list string) error {
	list = strings.TrimSpace(list)
	if list == "" {
		return errors.New("router: empty message index")
	}
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("router: bad message index %q", list)
		}
		if _, err := strconv.Atoi(part); err != nil {
			return fmt.Errorf("router: bad message index %q", part)
		}
	}
	return nil
}

// Storage selects where the modem keeps new messages: "SM" for the SIM card,
// "ME" for the module's own memory.
func (c *Client) Storage() (string, error) {
	page, err := c.get("/sms_settings.htm")
	if err != nil {
		return "", err
	}
	// The settings page states the current choice in a script variable and
	// ticks the matching radio from it, so the variable is the reliable source.
	if v, ok := extractJSString(page, "smsStorage"); ok && v != "" {
		return v, nil
	}
	// Fall back to whichever radio the markup marks as checked.
	for _, r := range storageRadios(page) {
		if r.checked {
			return r.value, nil
		}
	}
	return "", errors.New("router: storage setting not found")
}

type storageRadio struct {
	value   string
	checked bool
}

// storageRadios finds the sms_storage radios in document order and whether the
// markup marks them as selected.
func storageRadios(page string) []storageRadio {
	const key = storageKey
	var out []storageRadio
	rest := page
	for {
		i := strings.Index(rest, key)
		if i < 0 {
			return out
		}
		rest = rest[i+len(key):]
		if r, ok := findStorageRadioTag(rest); ok {
			out = append(out, r)
		}
		if gt := strings.IndexByte(rest, '>'); gt >= 0 {
			rest = rest[gt+1:]
		}
	}
}

// storageKey is the name attribute the firmware gives its storage radios.
const storageKey = `name="sms_storage"`

// findStorageRadioTag walks forward from a name attribute to the nearest
// following input tag and reports it if it is an sms_storage radio.
func findStorageRadioTag(rest string) (storageRadio, bool) {
	scan := rest
	if len(scan) > 400 {
		scan = scan[:400]
	}
	for {
		lt := strings.IndexByte(scan, '<')
		if lt < 0 {
			return storageRadio{}, false
		}
		gt := strings.IndexByte(scan[lt:], '>')
		if gt < 0 {
			return storageRadio{}, false
		}
		tag := scan[lt : lt+gt]
		lower := strings.ToLower(tag)
		if strings.HasPrefix(lower, "<input") && strings.Contains(tag, storageKey) {
			return storageRadio{
				value:   attrValue(tag, "value"),
				checked: strings.Contains(lower, "checked"),
			}, true
		}
		if strings.HasPrefix(lower, "<tr") || strings.HasPrefix(lower, "</tr") {
			return storageRadio{}, false
		}
		scan = scan[lt+gt:]
	}
}

// SetStorage changes where new messages are kept.
func (c *Client) SetStorage(mode string) error {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "SM", "ME":
	default:
		return fmt.Errorf("router: storage must be SM or ME, got %q", mode)
	}
	form := url.Values{
		"action_id":    {"sms_settings"},
		"action_value": {"tmp"},
		"sms_storage":  {strings.ToUpper(strings.TrimSpace(mode))},
		"submitUrl":    {"/sms_settings.htm"},
	}
	_, err := c.post("/boafrm/formSmsManage", form)
	return err
}

func attrValue(fragment, attr string) string {
	key := attr + `="`
	i := strings.Index(fragment, key)
	if i < 0 {
		return ""
	}
	rest := fragment[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func firstAttrValue(fragment string) string {
	rest := fragment
	for {
		i := strings.Index(rest, `value="`)
		if i < 0 {
			return ""
		}
		rest = rest[i+len(`value="`):]
		j := strings.Index(rest, `"`)
		if j < 0 {
			return ""
		}
		if v := rest[:j]; v != "" {
			return v
		}
		rest = rest[j:]
	}
}

// validateNumber mirrors the firmware's compose-page rule: digits and
// semicolons only, where semicolons separate bulk recipients.
func validateNumber(number string) error {
	for i, r := range number {
		switch {
		case r >= '0' && r <= '9':
		case r == ';':
		default:
			return fmt.Errorf("router: %q is not valid in a number at position %d", r, i)
		}
	}
	if strings.HasPrefix(number, ";") || strings.HasSuffix(number, ";") {
		return errors.New("router: number has a trailing or leading semicolon")
	}
	return nil
}

// validateCountryCode mirrors the firmware's rule: an optional leading plus
// followed by at least one digit. A bare plus, or any sign with no digits, is
// rejected here rather than being silently sent to the modem.
func validateCountryCode(cc string) error {
	if cc == "" {
		return errors.New("router: country code is empty")
	}
	digits := 0
	for i, r := range cc {
		switch {
		case r == '+':
			if i != 0 {
				return fmt.Errorf("router: %q is not valid in a country code at position %d", r, i)
			}
		case r >= '0' && r <= '9':
			digits++
		default:
			return fmt.Errorf("router: %q is not valid in a country code at position %d", r, i)
		}
	}
	if digits == 0 {
		return errors.New("router: country code has no digits")
	}
	return nil
}
