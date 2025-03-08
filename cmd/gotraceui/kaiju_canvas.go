package main

import (
	"honnef.co/go/gotraceui/layout"
	"honnef.co/go/gotraceui/theme"
)

var (
	lastFocusedTimeline int
)

func kaijuNewMainMenu(m *MainMenu, notMainDisabled func() bool) {
	m.Display.PrevTimelineUserRegion = theme.MenuItem{Shortcut: "P", Label: PlainLabel("Previous user region timeline"), Disabled: notMainDisabled}
	m.Display.NextTimelineUserRegion = theme.MenuItem{Shortcut: "N", Label: PlainLabel("Next user region timeline"), Disabled: notMainDisabled}
}

func kaijuRenderMainScene(win *theme.Window) {
	win.AddShortcut(theme.Shortcut{Name: "P"})
	win.AddShortcut(theme.Shortcut{Name: "N"})
}

func kaijuWinUpdate(mwin *MainWindow, win *theme.Window, gtx layout.Context) {
	if mwin.mainMenu.Display.PrevTimelineUserRegion.Clicked(gtx) {
		win.Menu.Close()
		mwin.canvas.ScrollToPreviousUserRegion(gtx)
	}
	if mwin.mainMenu.Display.NextTimelineUserRegion.Clicked(gtx) {
		win.Menu.Close()
		mwin.canvas.ScrollToNextUserRegion(gtx)
	}
}

func (cv *Canvas) findStartingTimeline() int {
	for i := range cv.timelines {
		if cv.timelines[i].displayed {
			return i
		}
	}
	return -1
}

func (cv *Canvas) ScrollToPreviousUserRegion(gtx layout.Context) {
	startingPoint := lastFocusedTimeline
	if !cv.timelines[lastFocusedTimeline].displayed {
		startingPoint = cv.findStartingTimeline()
		if startingPoint < 0 {
			return
		}
	}
	for i := startingPoint - 1; i >= 0; i-- {
		for j := range cv.timelines[i].tracks {
			t := cv.timelines[i].tracks[j]
			if t.kind == TrackKindUserRegions {
				cv.scrollToTimeline(gtx, cv.timelines[i])
				lastFocusedTimeline = i
				return
			}
		}
	}
}

func (cv *Canvas) ScrollToNextUserRegion(gtx layout.Context) {
	startingPoint := lastFocusedTimeline
	if !cv.timelines[lastFocusedTimeline].displayed {
		startingPoint = cv.findStartingTimeline()
		if startingPoint < 0 {
			return
		}
	}
	for i := startingPoint + 1; i < len(cv.timelines); i++ {
		for j := range cv.timelines[i].tracks {
			t := cv.timelines[i].tracks[j]
			if t.kind == TrackKindUserRegions {
				if (t.Start > cv.start && t.Start < cv.End()) ||
					(t.End > cv.start && t.End < cv.End()) {
					cv.scrollToTimeline(gtx, cv.timelines[i])
					lastFocusedTimeline = i
					return
				}
			}
		}
	}
}
