package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

/*
Finding a widget inside a laid-out section.

These tests drive the real controls rather than a model of them, which means
reaching into a container tree that the test should not otherwise care about --
asserting that a row is a Border holding an HBox tests the code against itself
and breaks on every rearrangement. So the tree is walked for the first widget of
a kind, and the assertions are about what that widget does.
*/

// walk visits every object in a container tree, depth first.
func walk(o fyne.CanvasObject, visit func(fyne.CanvasObject) bool) bool {
	if o == nil {
		return false
	}
	if visit(o) {
		return true
	}
	if c, ok := o.(*fyne.Container); ok {
		for _, child := range c.Objects {
			if walk(child, visit) {
				return true
			}
		}
	}
	// A scrolled section is a widget, not a container, so its content would
	// otherwise be invisible to every test that walks a whole section.
	if s, ok := o.(*container.Scroll); ok {
		return walk(s.Content, visit)
	}
	return false
}

func findSliderMaybe(o fyne.CanvasObject) *widget.Slider {
	var found *widget.Slider
	walk(o, func(obj fyne.CanvasObject) bool {
		s, ok := obj.(*widget.Slider)
		if ok {
			found = s
		}
		return ok
	})
	return found
}

func findSlider(t *testing.T, o fyne.CanvasObject) *widget.Slider {
	t.Helper()
	s := findSliderMaybe(o)
	if s == nil {
		t.Fatal("no slider in this row")
	}
	return s
}

func findEntry(t *testing.T, o fyne.CanvasObject) *widget.Entry {
	t.Helper()
	var found *widget.Entry
	walk(o, func(obj fyne.CanvasObject) bool {
		e, ok := obj.(*widget.Entry)
		if ok {
			found = e
		}
		return ok
	})
	if found == nil {
		t.Fatal("no entry in this row")
	}
	return found
}

// testLabel is a stand-in for the card's reserved "changed since the last
// build" line, which paramRow writes into.
func testLabel() *widget.Label { return widget.NewLabel("") }

// cardText is every label in a card, joined, so a test can assert that a fact
// is or is not on it without knowing how the card is laid out.
func cardText(o fyne.CanvasObject) string {
	var parts []string
	walk(o, func(obj fyne.CanvasObject) bool {
		switch w := obj.(type) {
		case *widget.Label:
			parts = append(parts, w.Text)
		case *widget.Button:
			parts = append(parts, w.Text)
		case *widget.Check:
			parts = append(parts, w.Text)
		}
		return false
	})
	return strings.Join(parts, "\n")
}
