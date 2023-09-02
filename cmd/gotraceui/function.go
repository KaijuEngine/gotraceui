package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	rtrace "runtime/trace"
	"strings"
	"time"

	"honnef.co/go/gotraceui/clip"
	"honnef.co/go/gotraceui/layout"
	"honnef.co/go/gotraceui/mem"
	"honnef.co/go/gotraceui/theme"
	"honnef.co/go/gotraceui/trace"
	"honnef.co/go/gotraceui/trace/ptrace"
	"honnef.co/go/gotraceui/widget"

	"gioui.org/font"
	"gioui.org/op"
	"gioui.org/text"
)

type FunctionInfo struct {
	mwin          *theme.Window
	fn            *ptrace.Function
	trace         *Trace
	title         string
	tabbedState   theme.TabbedState
	goroutineList GoroutineList

	filterGoroutines widget.Bool
	histGoroutines   []*ptrace.Goroutine
	hist             InteractiveHistogram

	descriptionText Text
	hoveredLink     *TextSpan

	initialized bool

	theme.PanelButtons
}

func NewFunctionInfo(tr *Trace, mwin *theme.Window, fn *ptrace.Function) *FunctionInfo {
	fi := &FunctionInfo{
		fn:             fn,
		mwin:           mwin,
		histGoroutines: fn.Goroutines,
		trace:          tr,
	}

	return fi
}

func (fi *FunctionInfo) HoveredLink() *TextSpan {
	return fi.hoveredLink
}

func (fi *FunctionInfo) buildDescription(win *theme.Window, gtx layout.Context) Description {
	tb := TextBuilder{Theme: win.Theme}
	var attrs []DescriptionAttribute

	attrs = append(attrs, DescriptionAttribute{
		Key:   "Function",
		Value: *(tb.Span(fi.fn.Fn)),
	})

	// TODO(dh): make file link clickable
	displayPath := fi.fn.File
	if goroot := fi.trace.GOROOT; goroot != "" && strings.HasPrefix(fi.fn.File, goroot) {
		displayPath = filepath.Join("$GOROOT", strings.TrimPrefix(fi.fn.File, goroot))
	} else if gopath := fi.trace.GOPATH; gopath != "" && strings.HasPrefix(fi.fn.File, gopath) {
		displayPath = filepath.Join("$GOPATH", strings.TrimPrefix(fi.fn.File, gopath))
	} else if goroot == "" && gopath == "" {
		// We couldn't detect goroot, which makes it very likely that the executable had paths trimmed. Detect if
		// the trimmed path is in GOROOT or GOPATH based on if the first path element has a dot in it or not. Module
		// paths without dots are reserved for the standard library. This has a small but negligible chance of false
		// positives.

		left, _, ok := strings.Cut(fi.fn.File, "/")
		if ok {
			if strings.Contains(left, ".") {
				if strings.Contains(fi.fn.File, "@v") {
					displayPath = filepath.Join("$GOPATH", "pkg", "mod", fi.fn.File)
				} else {
					displayPath = filepath.Join("$GOPATH", "src", fi.fn.File)
				}
			} else {
				displayPath = filepath.Join("$GOROOT", "src", fi.fn.File)
			}
		}
	}
	attrs = append(attrs, DescriptionAttribute{
		Key:   "Location",
		Value: *(tb.Span(fmt.Sprintf("%s:%d", displayPath, fi.fn.Line))),
	})

	attrs = append(attrs, DescriptionAttribute{
		Key:   "# of goroutines",
		Value: *(tb.Span(local.Sprintf("%d", len(fi.fn.Goroutines)))),
	})

	var total time.Duration
	for _, g := range fi.fn.Goroutines {
		d := time.Duration(g.Spans[len(g.Spans)-1].End - g.Spans[0].Start)
		total += d
	}

	attrs = append(attrs, DescriptionAttribute{
		Key:   "Total time",
		Value: *(tb.Span(total.String())),
	})

	desc := Description{Attributes: attrs}
	return desc
}

func (fi *FunctionInfo) init(win *theme.Window) {
	// Build histogram
	cfg := &widget.HistogramConfig{RejectOutliers: true, Bins: widget.DefaultHistogramBins}
	fi.computeHistogram(win, cfg)
}

func (fi *FunctionInfo) Title() string {
	return fi.title
}

func (fi *FunctionInfo) Layout(win *theme.Window, gtx layout.Context) layout.Dimensions {
	defer rtrace.StartRegion(context.Background(), "main.FunctionInfo.Layout").End()

	if !fi.initialized {
		fi.init(win)
		fi.initialized = true
	}

	// Inset of 5 pixels on all sides. We can't use layout.Inset because it doesn't decrease the minimum constraint,
	// which we do care about here.
	gtx.Constraints.Min = gtx.Constraints.Min.Sub(image.Pt(2*5, 2*5))
	gtx.Constraints.Max = gtx.Constraints.Max.Sub(image.Pt(2*5, 2*5))
	gtx.Constraints = layout.Normalize(gtx.Constraints)
	defer op.Offset(image.Pt(5, 5)).Push(gtx.Ops).Pop()

	nothing := func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	tabs := []string{"Goroutines", "Histogram"}

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, nothing),
				layout.Rigid(theme.Dumb(win, fi.PanelButtons.Layout)),
			)
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: 10}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			fi.descriptionText.Reset(win.Theme)
			return fi.buildDescription(win, gtx).Layout(win, gtx, &fi.descriptionText)
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: 10}.Layout(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return theme.Tabbed(&fi.tabbedState, tabs).Layout(win, gtx, func(win *theme.Window, gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = gtx.Constraints.Max
				switch tabs[fi.tabbedState.Current] {
				case "Goroutines":
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return theme.CheckBox(win.Theme, &fi.filterGoroutines, "Filter list to range of durations selected in histogram").Layout(win, gtx)
						}),

						layout.Rigid(layout.Spacer{Height: 5}.Layout),

						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							var gs []*ptrace.Goroutine
							if fi.filterGoroutines.Value {
								gs = fi.histGoroutines
							} else {
								gs = fi.fn.Goroutines
							}
							return fi.goroutineList.Layout(win, gtx, gs)
						}),
					)
				case "Histogram":
					return fi.hist.Layout(win, gtx)
				default:
					panic("unreachable")
				}
			})
		}),
	)

	for _, ev := range fi.goroutineList.Clicked() {
		handleLinkClick(win, ev.Event, ev.Span.ObjectLink)
	}
	for _, ev := range fi.descriptionText.Events() {
		handleLinkClick(win, ev.Event, ev.Span.ObjectLink)
	}
	fi.hoveredLink = firstNonNil(
		fi.goroutineList.Hovered(),
		fi.descriptionText.Hovered(),
	)

	for fi.PanelButtons.Backed() {
		fi.mwin.EmitAction(&PrevPanelAction{})
	}

	if fi.hist.Changed() {
		fi.histGoroutines = fi.computeHistogram(win, &fi.hist.Config)
	}

	return dims
}

func (fi *FunctionInfo) computeHistogram(win *theme.Window, cfg *widget.HistogramConfig) []*ptrace.Goroutine {
	var goroutineDurations []time.Duration

	var gs []*ptrace.Goroutine
	for _, g := range fi.fn.Goroutines {
		d := time.Duration(g.Spans[len(g.Spans)-1].End - g.Spans[0].Start)
		if fd := widget.FloatDuration(d); fd >= cfg.Start && (cfg.End == 0 || fd <= cfg.End) {
			goroutineDurations = append(goroutineDurations, d)
			gs = append(gs, g)
		}
	}

	fi.hist.Set(win, goroutineDurations)

	return gs
}

type GoroutineList struct {
	table       *theme.Table
	scrollState theme.YScrollableListState

	timestampObjects mem.BucketSlice[trace.Timestamp]
	texts            mem.BucketSlice[Text]
}

// Hovered returns the text span that has been hovered during the last call to Layout.
func (evs *GoroutineList) Hovered() *TextSpan {
	for i := 0; i < evs.texts.Len(); i++ {
		txt := evs.texts.Ptr(i)
		if h := txt.Hovered(); h != nil {
			return h
		}
	}
	return nil
}

func (gs *GoroutineList) Layout(win *theme.Window, gtx layout.Context, goroutines []*ptrace.Goroutine) layout.Dimensions {
	defer rtrace.StartRegion(context.Background(), "main.GoroutineList.Layout").End()

	if gs.table == nil {
		gs.table = &theme.Table{}
		gs.table.SetColumns(win, gtx, "Goroutine", "Start time", "Duration")

		// Find space needed for largest goroutine ID
		n := len(goroutines)
		s := n - 32
		if s < 0 {
			s = 0
		}
		var maxID uint64
		// Look at the last 32 goroutines for this function. This has a high likelyhood of telling us the greatest ID.
		for _, g := range goroutines[s:n] {
			if g.ID > maxID {
				maxID = g.ID
			}
		}
		r0 := theme.Record(win, gtx, func(win *theme.Window, gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			gtx.Constraints.Max = image.Pt(99999, 99999)
			return widget.Label{}.Layout(gtx, win.Theme.Shaper, font.Font{Weight: font.Bold}, 12, "Goroutine", widget.ColorTextMaterial(gtx, color.NRGBA{}))
		})
		r1 := theme.Record(win, gtx, func(win *theme.Window, gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			gtx.Constraints.Max = image.Pt(99999, 99999)
			return widget.Label{}.Layout(gtx, win.Theme.Shaper, font.Font{}, 12, local.Sprintf("%d", maxID), widget.ColorTextMaterial(gtx, color.NRGBA{}))
		})
		w := r0.Dimensions.Size.X
		if x := r1.Dimensions.Size.X; x > w {
			w = x
		}

		w += gtx.Dp(5) * 2
		d := float32(w) - gs.table.ColumnWidths[0]
		gs.table.ColumnWidths[0] = float32(w)
		gs.table.ColumnWidths[1] = max(0, gs.table.ColumnWidths[1]-float32(d))
	}

	gs.table.Resize(win, gtx)
	gs.timestampObjects.Reset()

	var txtCnt int
	// OPT(dh): reuse memory
	cellFn := func(gtx layout.Context, row, col int) layout.Dimensions {
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()

		tb := TextBuilder{Theme: win.Theme}
		var txt *Text
		if txtCnt < gs.texts.Len() {
			txt = gs.texts.Ptr(txtCnt)
		} else {
			txt = gs.texts.Append(Text{})
		}
		txtCnt++
		txt.Reset(win.Theme)

		g := goroutines[row]
		switch col {
		case 0: // ID
			tb.DefaultLink(local.Sprintf("%d", g.ID), "", g)
			txt.Alignment = text.End
		case 1: // Time
			start := g.Spans[0].Start
			tb.DefaultLink(formatTimestamp(nil, start), "", gs.timestampObjects.Append(start))
			txt.Alignment = text.End
		case 2: // Duration
			start := g.Spans[0].Start
			end := g.Spans[len(g.Spans)-1].End
			d := time.Duration(end - start)
			value, unit := durationNumberFormatSITable.format(d)
			tb.Span(value)
			tb.Span(" ")
			s := tb.Span(unit)
			s.Font.Typeface = "Go Mono"
			txt.Alignment = text.End
		}

		dims := txt.Layout(win, gtx, tb.Spans)
		dims.Size = gtx.Constraints.Constrain(dims.Size)
		return dims
	}

	return theme.YScrollableList(&gs.scrollState).Layout(win, gtx, func(win *theme.Window, gtx layout.Context, list *theme.RememberingList) layout.Dimensions {
		return layout.Rigids(gtx, layout.Vertical,
			func(gtx layout.Context) layout.Dimensions {
				return theme.TableHeaderRow(gs.table).Layout(win, gtx, gs.table.ColumnNames, []bool{true, true, true})
			},

			func(gtx layout.Context) layout.Dimensions {
				return list.Layout(gtx, len(goroutines), func(gtx layout.Context, index int) layout.Dimensions {
					return theme.TableSimpleRow(gs.table).Layout(win, gtx, index, func(win *theme.Window, gtx layout.Context, row, col int) layout.Dimensions {
						// g := goroutines[row]
						return cellFn(gtx, row, col)
					})
				})
			},
		)
	})
}

// Clicked returns all objects of text spans that have been clicked since the last call to Layout.
func (gs *GoroutineList) Clicked() []TextEvent {
	// This only allocates when links have been clicked, which is a very low frequency event.
	var out []TextEvent
	for i := 0; i < gs.texts.Len(); i++ {
		txt := gs.texts.Ptr(i)
		out = append(out, txt.Events()...)
	}
	return out
}
