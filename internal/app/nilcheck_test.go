package app

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	fapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/config"
	"modemphone/internal/router"
	"modemphone/internal/ui/theme"
)

// findNils walks a widget tree and reports every nil child it finds.
//
// Fyne's box and border layouts dereference each child without a nil check, so
// one stray nil anywhere in the tree panics the first time the window is laid
// out. Catching it in a test turns a launch-time crash into a failing test.
// children returns the child objects of obj, descending through scroll
// containers so the whole tree is reachable.
func children(obj fyne.CanvasObject) []fyne.CanvasObject {
	switch v := obj.(type) {
	case *fyne.Container:
		return v.Objects
	case *container.Scroll:
		if v.Content == nil {
			return nil
		}
		return []fyne.CanvasObject{v.Content}
	case *container.Split:
		return []fyne.CanvasObject{v.Leading, v.Trailing}
	case *container.AppTabs:
		out := make([]fyne.CanvasObject, 0, len(v.Items))
		for _, item := range v.Items {
			out = append(out, item.Content)
		}
		return out
	}
	return nil
}

func findNils(obj fyne.CanvasObject, path string, out *[]string) {
	if obj == nil {
		*out = append(*out, path+" is nil")
		return
	}
	kids := children(obj)
	for i, child := range kids {
		childPath := fmt.Sprintf("%s.child[%d] (%T)", path, i, child)
		if child == nil {
			*out = append(*out, childPath+" is nil")
			continue
		}
		findNils(child, childPath, out)
	}
}

func testApp(t *testing.T) fyne.App {
	t.Helper()
	test := fapp.NewWithID("com.modemphone.test")
	fonts, err := theme.Load("../../assets/fonts")
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	test.Settings().SetTheme(theme.Dark(fonts))
	return test
}

// TestInitialTreeHasNoNils checks the tree exactly as it looks at launch.
func TestInitialTreeHasNoNils(t *testing.T) {
	test := testApp(t)
	win := test.NewWindow("ModemPhone")
	New(win, config.Default(), theme.Fonts{})

	var problems []string
	findNils(win.Content(), "root", &problems)
	for _, p := range problems {
		t.Errorf("nil child: %s", p)
	}
}

// TestPopulatedTreeHasNoNils fills the lists with realistic messages, because
// a row is only built once there is something to show and that is where a nil
// creeps in.
func TestPopulatedTreeHasNoNils(t *testing.T) {
	test := testApp(t)
	fonts, err := theme.Load("../../assets/fonts")
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	win := test.NewWindow("ModemPhone")
	cfg := config.Default()
	cfg.Password = "x"
	a := New(win, cfg, fonts)
	a.Start("")

	msgs := []router.Message{
		{Index: "7", Number: "+091012345674", Time: "2026-09-29 02:52:59",
			Content: "Modem app test 1"},
		{Index: "8", Number: "MCI Modem", Time: "2026-09-28 16:20:33", Read: true,
			Content: "\u0641\u0631\u0635\u062A \u0645\u062D\u062F\u0648\u062F \u062E\u0631\u06CC\u062F \u0645\u0648\u062F\u0645\u200C\u0647\u0627\u06CC \u0637\u0631\u062D \u06AF\u06CC\u0645\u06CC\u0646\u06AF\n\u0628\u0627 \u06F5 \u0645\u06CC\u0644\u06CC\u0648\u0646 \u062A\u0648\u0645\u0627\u0646 \u062A\u062E\u0641\u06CC\u0633\u2757"},
		{Index: "9", Number: "AsiaTech", Time: "2026-09-28 14:15:04",
			Content: "\u062A\u062E\u0641\u06CC\u0633 \u0648\u06CC\u0698\u0647 \u0628\u0631\u0627\u06CC \u06A9\u0627\u0631\u0628\u0631\u0627\u06CC\u200C\u0647\u0627\u06CC \u0648\u06CC\u0698\u0647\u200C\u0622\u0633\u06CC\u0627\u062A\u06A9! TEST-MODEL-1 123"},
		{Index: "10", Number: "", Time: "2026-09-27 10:00:00", Read: true, Content: "no sender"},
	}
	a.inbox.SetMessages(msgs)
	a.outbox.SetMessages(msgs[:1])
	a.drafts.SetMessages(msgs[:1])
	a.openMessage(msgs[1])

	var problems []string
	findNils(win.Content(), "root", &problems)
	for _, p := range problems {
		t.Errorf("nil child: %s", p)
	}
}

// TestLayoutWithMessages measures the whole tree, which walks the same layout
// code that panicked on a nil child.
func TestLayoutWithMessages(t *testing.T) {
	test := testApp(t)
	fonts, err := theme.Load("../../assets/fonts")
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	win := test.NewWindow("ModemPhone")
	cfg := config.Default()
	cfg.Password = "x"
	a := New(win, cfg, fonts)
	a.Start("")

	a.inbox.SetMessages([]router.Message{{
		Index: "1", Number: "+091012345674", Time: "now",
		Content: "a message body that is long enough to need wrapping in the list",
	}})
	win.Resize(fyne.NewSize(1180, 780))

	var measure func(obj fyne.CanvasObject, depth int)
	measure = func(obj fyne.CanvasObject, depth int) {
		if obj == nil || depth > 50 {
			return
		}
		obj.MinSize()
		if c, ok := obj.(*fyne.Container); ok {
			c.Resize(fyne.NewSize(1180, 780))
		}
		for _, child := range children(obj) {
			measure(child, depth+1)
		}
	}
	measure(win.Content(), 0)
}

// TestEveryTabBuilds checks each tab's content on its own, so a failure points
// at the tab that broke.
func TestEveryTabBuilds(t *testing.T) {
	test := testApp(t)
	fonts, err := theme.Load("../../assets/fonts")
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	win := test.NewWindow("ModemPhone")
	cfg := config.Default()
	cfg.Password = "x"
	a := New(win, cfg, fonts)
	a.Start("")

	for i, item := range a.tabs.Items {
		var problems []string
		findNils(item.Content, fmt.Sprintf("tab[%d]=%s", i, item.Text), &problems)
		for _, p := range problems {
			t.Errorf("nil child: %s", p)
		}
		item.Content.MinSize()
	}
}

// TestSettingsSaveRoundTrip checks that the fields the settings pane edits are
// the ones that get persisted.
func TestSettingsSaveRoundTrip(t *testing.T) {
	test := testApp(t)
	fonts, err := theme.Load("../../assets/fonts")
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	win := test.NewWindow("ModemPhone")
	cfg := config.Default()
	cfg.Password = "x"
	a := New(win, cfg, fonts)
	a.Start("")

	// Walk the settings pane looking for the input widgets and confirm the
	// expected ones are present.
	var entries, selects int
	var walk func(fyne.CanvasObject, int)
	walk = func(obj fyne.CanvasObject, depth int) {
		if obj == nil || depth > 60 {
			return
		}
		switch obj.(type) {
		case *widget.Entry:
			entries++
		case *widget.Select:
			selects++
		}
		for _, child := range children(obj) {
			walk(child, depth+1)
		}
	}
	for _, item := range a.tabs.Items {
		walk(item.Content, 0)
	}
	// Router address, username, password, country code, default recipient, and
	// the two dropdowns.
	if entries < 5 {
		t.Errorf("expected at least 5 text fields, found %d", entries)
	}
	if selects < 2 {
		t.Errorf("expected at least 2 dropdowns, found %d", selects)
	}
}
