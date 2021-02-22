package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"gioui.org/app" // app contains Window handling.
	"gioui.org/f32"
	"gioui.org/font/gofont" // gofont is used for loading the default font.
	"gioui.org/io/key"      // key is used for keyboard events.
	"gioui.org/io/system"   // system is used for system events (e.g. closing the window).
	"gioui.org/layout"      // layout is used for layouting widgets.
	"gioui.org/op"          // op is used for recording different operations.
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"            // unit is used to define pixel-independent sizes
	"gioui.org/widget/material" // material contains material design widgets.
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/yofu/rz2"
)

var (
	cafile  = ""
	crtfile = ""
	keyfile = ""
)

func main() {
	// The ui loop is separated from the application window creation
	// such that it can be used for testing.
	ui := NewUI()

	// This creates a new application window and starts the UI.
	go func() {
		w := app.NewWindow(
			app.Title("Monitor"),
			app.Size(unit.Dp(360), unit.Dp(360)),
		)
		if err := ui.Run(w); err != nil {
			log.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}()

	StartSubscriber("tcp://192.168.0.100:1883", []string{"#"})

	// This starts Gio main.
	app.Main()
}

// defaultMargin is a margin applied in multiple places to give
// widgets room to breathe.
var defaultMargin = unit.Dp(10)

// UI holds all of the application state.
type UI struct {
	// Theme is used to hold the fonts used throughout the application.
	Theme *material.Theme
}

// NewUI creates a new UI using the Go Fonts.
func NewUI() *UI {
	ui := &UI{}
	ui.Theme = material.NewTheme(gofont.Collection())

	return ui
}

var (
	drawing = true
	// TODO: 一時停止中に右キーでコマ送り（現在の実装だと、右キーを押したままにすると落ちる）
)

// Run handles window events and renders the application.
func (ui *UI) Run(w *app.Window) error {

	var ops op.Ops
	for {
		select {
		case <-gcchan:
			if drawing {
				w.Invalidate()
			}
		case e := <-w.Events():
			// detect the type of the event.
			switch e := e.(type) {
			// this is sent when the application should re-render.
			case system.FrameEvent:
				// gtx is used to pass around rendering and event information.
				gtx := layout.NewContext(&ops, e)
				// render and handle UI.
				ui.Layout(gtx)
				// render and handle the operations from the UI.
				e.Frame(gtx.Ops)

			// handle a global key press.
			case key.Event:
				// fmt.Println(e.Name)
				switch e.Name {
				// when we click escape, let's close the window.
				case key.NameEscape:
					return nil
				case key.NameRightArrow:
				case "Space":
				case "J":
					if e.State == key.Release {
						factor /= 2.0
						w.Invalidate()
					}
				case "K":
					if e.State == key.Release {
						factor *= 2.0
						w.Invalidate()
					}
				}

			// this is sent when the application is closed.
			case system.DestroyEvent:
				return e.Err
			}
		}
	}

	return nil
}

// Layout displays the main program layout.
func (ui *UI) Layout(gtx layout.Context) layout.Dimensions {
	th := ui.Theme

	// inset is used to add padding around the window border.
	inset := layout.UniformInset(defaultMargin)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[0], 0, 0)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[1], 0, 1)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[2], 0, 2)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[3], 0, 3)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
				)
			})
		}),
		layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[0], 1, 0)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[1], 1, 1)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[2], 1, 2)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
					layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
						drawgraph(th, gtx, gclist[3], 1, 3)
						return layout.Dimensions{
							Size: gtx.Constraints.Min,
						}
					}),
				)
			})
		}),
	)
}

var (
	factor  = float32(100.0)
	fgcolor = []color.NRGBA{
		color.NRGBA{R: 0xD1, G: 0x4D, B: 0x56, A: 0xFF},
		color.NRGBA{R: 0x8C, G: 0xAB, B: 0x5E, A: 0xFF},
		color.NRGBA{R: 0x41, G: 0x72, B: 0x99, A: 0xFF},
	}
)

func drawgraph(th *material.Theme, gtx layout.Context, gc *GraphClient, direction, col int) {
	ops := gtx.Ops
	center := float32(gtx.Constraints.Max.Y) * 0.5
	if gc.tvalue[size-1] < gc.tvalue[0] {
		return
	}

	tfactor := float32(gtx.Constraints.Max.X) / float32(gc.tvalue[size-1]-gc.tvalue[0])

	dirname := []string{"X", "Y", "Z"}[direction]

	var bgcolor color.NRGBA
	switch col {
	default:
		bgcolor = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x88}
	case 0:
		bgcolor = color.NRGBA{R: 0xD1, G: 0x4D, B: 0x56, A: 0x88}
	case 1:
		bgcolor = color.NRGBA{R: 0x8C, G: 0xAB, B: 0x5E, A: 0x88}
	case 2:
		bgcolor = color.NRGBA{R: 0x41, G: 0x72, B: 0x99, A: 0x88}
	case 3:
		bgcolor = color.NRGBA{R: 0xEB, G: 0xAD, B: 0x4B, A: 0x88}
	}

	var ave float64
	max := gc.avalue[direction][0]
	min := gc.avalue[direction][0]
	for i := 0; i < size; i++ {
		tmp := gc.avalue[direction][i]
		ave += tmp
		if tmp > max {
			max = tmp
		}
		if tmp < min {
			min = tmp
		}
	}
	ave /= float64(size)

	stk := op.Save(ops)
	clip.Rect{Max: gtx.Constraints.Max}.Add(ops)
	paint.Fill(ops, bgcolor)
	stk.Load()

	title := material.H4(th, fmt.Sprintf("%s(%s): %s\n\n%.3f/%.3f/%.3f", gc.name, dirname, rz2.ConvertUnixtime(gc.lastmtime).Format("15:04:05.000"), max, min, ave))
	title.Color = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xdd}
	title.Layout(gtx)

	stk = op.Save(ops)
	var path clip.Path
	path.Begin(ops)
	path.Move(f32.Pt(0, center))
	for i := 0; i < gc.size; i++ {
		path.LineTo(f32.Pt(tfactor*float32(gc.tvalue[i]-gc.tvalue[0]), center+factor*float32(gc.avalue[direction][i]-ave)))
	}
	clip.Stroke{
		Path: path.End(),
		Style: clip.StrokeStyle{
			Width: 1,
			Cap:   clip.FlatCap,
			Join:  clip.BevelJoin,
			Miter: float32(math.Inf(+1)),
		},
	}.Op().Add(ops)
	paint.Fill(ops, fgcolor[direction])
	stk.Load()
}

type GraphClient struct {
	// sync.Mutex
	name      string
	frequency float64
	size      int
	mac       string
	num       string
	ext       string
	unit      string
	mode      int
	tvalue    []float64
	avalue    [][]float64
	lastmtime int64
}

func NewGraphClient(name string, size int, mac, num, ext string, mode string) *GraphClient {
	xvalue := make([]float64, size)
	yvalue := make([]float64, size)
	zvalue := make([]float64, size)
	gc := &GraphClient{
		name:      name,
		frequency: 125.0,
		size:      size,
		mac:       mac,
		num:       num,
		ext:       ext,
		unit:      unittext(ext),
		mode:      TIME,
		tvalue:    make([]float64, size),
		avalue:    [][]float64{xvalue, yvalue, zvalue},
	}
	return gc
}

const (
	TIME = iota
	AMP
)

var (
	modestring = []string{
		"TIME",
		"AMP",
	}
)

func unittext(ext string) string {
	switch ext {
	case "acc02":
		return "gal"
	case "str01":
		return "με"
	default:
		return ""
	}
}

var (
	maclist = []string{
		"b8:27:eb:fd:5a:ea", // moncli01
		"b8:27:eb:fe:df:5e", // moncli02
		"b8:27:eb:6a:16:44", // moncli03
		"b8:27:eb:52:27:d8", // moncli04
	}
	size      = 1024
	direction = 0
	gclist    = []*GraphClient{
		NewGraphClient("moncli01", size, maclist[0], "01", "acc02", "TIME"),
		NewGraphClient("moncli02", size, maclist[1], "01", "acc02", "TIME"),
		NewGraphClient("moncli03", size, maclist[2], "01", "acc02", "TIME"),
		NewGraphClient("moncli04", size, maclist[3], "01", "acc02", "TIME"),
	}
	gcchan     = make(chan *GraphClient)
	redrawchan = make(chan int)
)

func StartSubscriber(server string, topics []string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()

	opts.AddBroker(server)
	opts.SetAutoReconnect(false)
	opts.SetClientID(fmt.Sprintf("test_%d", time.Now().UnixNano()))
	if strings.HasPrefix(server, "ssl") {
		tlsconfig, err := rz2.NewTLSConfig(cafile, crtfile, keyfile)
		if err != nil {
			return nil, err
		}
		opts.SetTLSConfig(tlsconfig)
	}

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		lis := strings.Split(msg.Topic(), "/")
		if len(lis) < 3 {
			return
		}
		var gc *GraphClient
		for _, g := range gclist {
			if lis[0] == g.mac {
				gc = g
				break
			}
		}
		if gc == nil {
			return
		}
		var tdata, xdata, ydata, zdata []float64
		var avex, avey, avez float64
		switch lis[2] {
		case "acc02":
			var send_time int64
			buf := bytes.NewReader(msg.Payload()[:8])
			binary.Read(buf, binary.BigEndian, &send_time)
			acc, xind, err := rz2.ConvertAccPacket(msg.Payload()[12:])
			if err != nil {
				return
			}
			acc = acc[xind : len(acc)-3+xind]
			tdata = make([]float64, len(acc)/3)
			xdata = make([]float64, len(acc)/3)
			ydata = make([]float64, len(acc)/3)
			zdata = make([]float64, len(acc)/3)
			avex = 0.0
			avey = 0.0
			avez = 0.0
			if gc.lastmtime == 0 {
				gc.lastmtime = send_time - int64(gc.frequency*float64(len(xdata)))
			}
			dtime := float64(send_time-gc.lastmtime) / float64(len(xdata))
			gc.frequency = 1000.0 / dtime
			for i := 0; i < len(xdata); i++ {
				tdata[i] = float64(gc.lastmtime) + dtime*float64(i+1)
				xdata[i] = acc[3*i]
				ydata[i] = acc[3*i+1]
				zdata[i] = acc[3*i+2]
				avex += xdata[i]
				avey += ydata[i]
				avez += zdata[i]
			}
			avex /= float64(len(xdata))
			avey /= float64(len(ydata))
			avez /= float64(len(zdata))
			gc.lastmtime = send_time
		default:
		}
		update := func(orig, current []float64, size int) {
			if len(current) >= size {
				for i := 0; i < size; i++ {
					orig[i] = current[len(current)-size+i]
				}
			} else {
				for i := 0; i < len(orig)-len(current); i++ {
					orig[i] = orig[i+len(current)]
				}
				for i := 0; i < len(current); i++ {
					orig[len(orig)-len(current)+i] = current[i]
				}
			}
		}
		update(gc.tvalue, tdata, gc.size)
		update(gc.avalue[0], xdata, gc.size)
		update(gc.avalue[1], ydata, gc.size)
		update(gc.avalue[2], zdata, gc.size)
		gcchan <- gc
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		return client, token.Error()
	}

	for _, t := range topics {
		client.Subscribe(t, 0, nil)
	}

	return client, nil
}

func StartDatReader(fn string) {
	records, _ := rz2.ReadServerRecord(fn)
	for _, rec := range records {
		lis := strings.Split(rec.Topic, "/")
		if len(lis) < 3 {
			continue
		}
		var gc *GraphClient
		for _, g := range gclist {
			if lis[0] == g.mac {
				gc = g
				break
			}
		}
		if gc == nil {
			continue
		}
		var tdata, xdata, ydata, zdata []float64
		var avex, avey, avez float64
		switch lis[2] {
		case "acc02":
			var send_time int64
			buf := bytes.NewReader(rec.Content[:8])
			binary.Read(buf, binary.BigEndian, &send_time)
			acc, xind, err := rz2.ConvertAccPacket(rec.Content[12:])
			if err != nil {
				continue
			}
			acc = acc[xind : len(acc)-3+xind]
			tdata = make([]float64, len(acc)/3)
			xdata = make([]float64, len(acc)/3)
			ydata = make([]float64, len(acc)/3)
			zdata = make([]float64, len(acc)/3)
			avex = 0.0
			avey = 0.0
			avez = 0.0
			if gc.lastmtime == 0 {
				gc.lastmtime = send_time - int64(gc.frequency*float64(len(xdata)))
			}
			dtime := float64(send_time-gc.lastmtime) / float64(len(xdata))
			gc.frequency = 1000.0 / dtime
			for i := 0; i < len(xdata); i++ {
				tdata[i] = float64(gc.lastmtime) + dtime*float64(i+1)
				xdata[i] = acc[3*i]
				ydata[i] = acc[3*i+1]
				zdata[i] = acc[3*i+2]
				avex += xdata[i]
				avey += ydata[i]
				avez += zdata[i]
			}
			avex /= float64(len(xdata))
			avey /= float64(len(ydata))
			avez /= float64(len(zdata))
			gc.lastmtime = send_time
		default:
		}
		update := func(orig, current []float64, size int) {
			if len(current) >= size {
				for i := 0; i < size; i++ {
					orig[i] = current[len(current)-size+i]
				}
			} else {
				for i := 0; i < len(orig)-len(current); i++ {
					orig[i] = orig[i+len(current)]
				}
				for i := 0; i < len(current); i++ {
					orig[len(orig)-len(current)+i] = current[i]
				}
			}
		}
		update(gc.tvalue, tdata, gc.size)
		update(gc.avalue[0], xdata, gc.size)
		update(gc.avalue[1], ydata, gc.size)
		update(gc.avalue[2], zdata, gc.size)
		gcchan <- gc
		time.Sleep(10 * time.Millisecond)
		<-redrawchan
		fmt.Println(gc.lastmtime)
	}
}
