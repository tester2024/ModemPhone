package router

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// USSDState describes where a USSD exchange currently stands.
type USSDState int

// The states the firmware distinguishes.
const (
	// USSDIdle means no USSD session is open.
	USSDIdle USSDState = iota
	// USSDMenu means the network returned a list of options to choose from.
	USSDMenu
	// USSDResult means the network returned a final answer.
	USSDResult
)

// USSDResponse is the outcome of sending a USSD code.
type USSDResponse struct {
	State USSDState
	Text  string
	// Options holds the selectable entries when State is USSDMenu.
	Options []string
}

// ussdStatusRe reads the firmware's ussdStatus variable, where 0 is idle,
// 1 is an open menu and 2 is a result.
var ussdStatusRe = regexp.MustCompile(`var\s+ussdStatus\s*=\s*'?(\d)'?`)

// SendUSSD submits a code and waits for the network's reply.
//
// The firmware answers asynchronously, so the page is polled until it leaves
// the idle state. Because the modem carries only an IPv6 data context by
// default, a code can also time out here: USSD needs a data bearer.
func (c *Client) SendUSSD(code string) (USSDResponse, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return USSDResponse{}, errors.New("router: empty USSD code")
	}
	if err := c.cancelUSSD(); err != nil {
		return USSDResponse{}, err
	}
	form := url.Values{
		"ussdValue":       {code},
		"selectMenuValue": {""},
		"ussdStatusInput": {"ussd"},
		"ussdCancelInput": {"0"},
		"submitUrl":       {"/ussd.htm"},
	}
	if _, err := c.post("/boafrm/formUSSDSetup", form); err != nil {
		return USSDResponse{}, err
	}
	return c.waitUSSD(12 * time.Second)
}

// SelectUSSDOption answers an open USSD menu.
func (c *Client) SelectUSSDOption(choice string) (USSDResponse, error) {
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return USSDResponse{}, errors.New("router: empty USSD choice")
	}
	form := url.Values{
		"ussdValue":       {""},
		"selectMenuValue": {choice},
		"ussdStatusInput": {"menu"},
		"ussdCancelInput": {"0"},
		"submitUrl":       {"/ussd.htm"},
	}
	if _, err := c.post("/boafrm/formUSSDSetup", form); err != nil {
		return USSDResponse{}, err
	}
	return c.waitUSSD(12 * time.Second)
}

// CancelUSSD closes any open USSD session.
func (c *Client) CancelUSSD() error { return c.cancelUSSD() }

func (c *Client) cancelUSSD() error {
	form := url.Values{
		"ussdValue":       {""},
		"selectMenuValue": {""},
		"ussdStatusInput": {"menu"},
		"ussdCancelInput": {"1"},
		"submitUrl":       {"/ussd.htm"},
	}
	_, err := c.post("/boafrm/formUSSDSetup", form)
	return err
}

func (c *Client) waitUSSD(timeout time.Duration) (USSDResponse, error) {
	deadline := time.Now().Add(timeout)
	var lastState USSDState = USSDIdle
	for time.Now().Before(deadline) {
		time.Sleep(1200 * time.Millisecond)
		page, err := c.get("/ussd.htm")
		if err != nil {
			return USSDResponse{}, err
		}
		st := parseUSSDState(page)
		lastState = st
		switch st {
		case USSDMenu:
			text := ussdRegion(page, "ussd_menu_id")
			opts := parseMenuOptions(text)
			if len(opts) > 0 {
				return USSDResponse{State: USSDMenu, Text: text, Options: opts}, nil
			}
		case USSDResult:
			return USSDResponse{State: USSDResult, Text: ussdRegion(page, "ussd_result_id")}, nil
		}
	}
	if lastState == USSDIdle {
		return USSDResponse{}, errors.New("router: USSD timed out (the modem may have no IPv4 data context)")
	}
	return USSDResponse{State: lastState}, nil
}

func parseUSSDState(page string) USSDState {
	m := ussdStatusRe.FindStringSubmatch(page)
	if m == nil {
		return USSDIdle
	}
	switch m[1] {
	case "1":
		return USSDMenu
	case "2":
		return USSDResult
	}
	return USSDIdle
}

// ussdRegion pulls the rendered text out of one of the page's result panes.
func ussdRegion(page, id string) string {
	i := strings.Index(page, `id="`+id+`"`)
	if i < 0 {
		return ""
	}
	rest := page[i:]
	// Stop at the closing div of this pane.
	depth := 0
	for j := 0; j < len(rest); j++ {
		if rest[j] == '<' && j+3 < len(rest) && rest[j+1:j+4] == "div" {
			if k := strings.IndexByte(rest[j:], '>'); k > 0 && rest[j+k-1] == '/' {
				j += k
				continue
			}
			depth++
			j += 3
			continue
		}
		if rest[j] == '<' && strings.HasPrefix(rest[j:], "</div") {
			depth--
			if depth == 0 {
				return strings.TrimSpace(cleanBody(rest[:j]))
			}
		}
	}
	return strings.TrimSpace(cleanBody(rest[:min(len(rest), 2000)]))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseMenuOptions(text string) []string {
	var out []string
	seen := map[string]bool{}
	// Menu text reaches us flattened, with <br> turned into newlines. Work line
	// by line so a label never runs together with the next one.
	for _, line := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r'
	}) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip a leading option number such as "1." or "2)".
		label := menuOptionPrefixRe.ReplaceAllString(line, "")
		label = strings.TrimSpace(label)
		label = strings.Trim(label, "*-_ ")
		if label == "" || seen[label] {
			continue
		}
		if len([]rune(label)) > 60 {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}

var menuOptionPrefixRe = regexp.MustCompile(`^[0-9]{1,2}\s*[.)\]:\-]?\s+`)
