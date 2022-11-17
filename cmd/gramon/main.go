package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/cmplx"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andlabs/ui"
	_ "github.com/andlabs/ui/winmanifest"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	dproxy "github.com/koron/go-dproxy"
	"github.com/mjibson/go-dsp/fft"
	toml "github.com/pelletier/go-toml"
	"github.com/yofu/rz2"
)

type Status struct {
	name      string
	frequency float64
	size      int
	mac       string
	num       string
	ext       string
	direction int
	mode      int
}

func (stat *Status) Set(tree *toml.Tree, key string) error {
	stat.mac = tree.Get(fmt.Sprintf("%s.macaddress", key)).(string)
	stat.num = tree.Get(fmt.Sprintf("%s.sensorno", key)).(string)
	stat.ext = tree.Get(fmt.Sprintf("%s.extension", key)).(string)
	dir, err := strconv.ParseInt(tree.Get(fmt.Sprintf("%s.direction", key)).(string), 10, 64)
	if err != nil {
		return err
	}
	stat.direction = int(dir)
	return nil
}

var (
	defaultsrc   = "tcp://192.168.1.23:18884"
	defaultstat1 = &Status{
		name:      "gc1",
		frequency: 125.0,
		size:      4096,
		mac:       "b8:27:eb:a5:88:33",
		num:       "01",
		ext:       "acc02",
		direction: 0,
		mode:      TIME,
	}
	defaultstat2 = &Status{
		name:      "gc2",
		frequency: 125.0,
		size:      4096,
		mac:       "b8:27:eb:a5:88:33",
		num:       "01",
		ext:       "acc02",
		direction: 1,
		mode:      TIME,
	}
	defaultstat3 = &Status{
		name:      "gc3",
		frequency: 125.0,
		size:      4096,
		mac:       "b8:27:eb:a5:88:33",
		num:       "01",
		ext:       "acc02",
		direction: 2,
		mode:      TIME,
	}
	defaultstat4 = &Status{
		name:      "gc4",
		frequency: 125.0,
		size:      4096,
		mac:       "b8:27:eb:db:e7:f8",
		num:       "01",
		ext:       "acc02",
		direction: 0,
		mode:      TIME,
	}
	syncdraw     = true
	srcentry     *ui.Entry
	srcclient    mqtt.Client
	conbutton    *ui.Button
	connected    = false
	graphclient1 *GraphClient
	graphclient2 *GraphClient
	graphclient3 *GraphClient
	graphclient4 *GraphClient
	recorder     *rz2.Recorder
	ampxrange    = []float64{0.0, 8.0}
	peakxrange   = []float64{1.0, 4.0}
)

var (
	cafile  = ""
	crtfile = ""
	keyfile = ""
)

type Axis struct {
	ndiv  int
	scale float64
}

var (
	updateGraph = true

	horizontalAxis = &Axis{
		ndiv:  16,
		scale: 1.0,
	}

	font = &ui.FontDescriptor{
		Family:  "Noto Sans CJK JP",
		Size:    12,
		Weight:  400,
		Italic:  ui.TextItalicNormal,
		Stretch: ui.TextStretchNormal,
	}
)

// some metrics
const (
	xoffLeft    = 20 // drawarea margins
	yoffTop     = 20
	xoffRight   = 100
	yoffBottom  = 50
	pointRadius = 5
)

// helper to quickly set a brush color
func mkSolidBrush(color uint32, alpha float64) *ui.DrawBrush {
	brush := new(ui.DrawBrush)
	brush.Type = ui.DrawBrushTypeSolid
	component := uint8((color >> 16) & 0xFF)
	brush.R = float64(component) / 255
	component = uint8((color >> 8) & 0xFF)
	brush.G = float64(component) / 255
	component = uint8(color & 0xFF)
	brush.B = float64(component) / 255
	brush.A = alpha
	return brush
}

const (
	colorWhite = 0xFFFFFF
	colorBlack = 0x000000
	colorGray  = 0xc8c8c8
)

var (
	blackbrush = mkSolidBrush(colorBlack, 0.7)
	graybrush  = mkSolidBrush(colorGray, 0.7)
)

var (
	cepscoeff = 128
)

func constructGraph(psize int, width, height float64, xvalue, yvalue []float64, mode string, factor float64, freq float64) ([]float64, []float64, []float64, []float64, []float64, []float64) {
	switch mode {
	case "AMP":
		df := freq / float64(len(yvalue))
		freqs := make([]float64, len(yvalue))
		for i := 0; i < len(yvalue); i++ {
			freqs[i] = df * float64(i)
		}
		fftval := CalcFFT(yvalue)
		fftabs := make([]float64, len(fftval))
		ceps1 := make([]float64, len(fftval))
		for i := 0; i < len(fftval); i++ {
			fftabs[i] = cmplx.Abs(fftval[i])
			ceps1[i] = 20 * math.Log10(cmplx.Abs(fftval[i]))
		}
		ceps2 := fft.IFFTReal(ceps1)
		ceps3 := make([]float64, len(fftval))
		for i := 0; i < len(fftval); i++ {
			if i > cepscoeff && i < len(fftval)-cepscoeff+1 {
				ceps3[i] = 0.0
			} else {
				ceps3[i] = real(ceps2[i])
			}
		}
		ceps4 := fft.FFTReal(ceps3)
		cepstrum := make([]float64, len(fftval))
		for i := 0; i < len(fftval); i++ {
			cepstrum[i] = math.Pow(10.0, cmplx.Abs(ceps4[i])/20.0)
		}
		startind := 0
		endind := 0
		for i := 0; i < len(freqs); i++ {
			if freqs[i] < ampxrange[0] {
				startind = i
			}
			endind = i
			if freqs[i] > ampxrange[1] {
				break
			}
		}
		xs := make([]float64, endind-startind+1)
		ys := make([]float64, endind-startind+1)
		ys2 := make([]float64, endind-startind+1)
		for i := 0; i <= endind-startind; i++ {
			xs[i] = width * (freqs[startind+i] - ampxrange[0]) / (ampxrange[1] - ampxrange[0])
			ys[i] = height - fftabs[startind+i]*factor
			ys2[i] = height - cepstrum[startind+i]*factor
		}
		return freqs[startind : endind+1], fftabs[startind : endind+1], cepstrum[startind : endind+1], xs, ys, ys2
	default:
		xincr := width / float64(psize-1)
		xs := make([]float64, psize)
		ys := make([]float64, psize)
		for i := 0; i < psize; i++ {
			xs[i] = xincr * float64(i)
			ys[i] = height*0.5 - yvalue[i]*factor
		}
		return xvalue, yvalue, yvalue, xs, ys, ys
	}
}

func graphSize(clientWidth, clientHeight float64) (graphWidth, graphHeight float64) {
	return clientWidth - xoffLeft - xoffRight,
		clientHeight - yoffTop - yoffBottom
}

type GraphClient struct {
	sync.Mutex
	*Status
	unit            string
	macentry        *ui.Entry
	numentry        *ui.Entry
	extentry        *ui.Entry
	direntry        *ui.Entry
	modebutton      *ui.RadioButtons
	drawarea        *ui.Area
	xvalue          []float64
	yvalue          []float64
	xbuffer         []float64
	ybuffer         []float64
	lastmtime       int64
	bufferlastmtime int64
	offsetmtime     int64
	verticalAxis    *Axis
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

func NewGraphClient(status *Status) *GraphClient {
	macentry := ui.NewEntry()
	macentry.SetText(status.mac)
	numentry := ui.NewEntry()
	numentry.SetText(status.num)
	extentry := ui.NewEntry()
	extentry.SetText(status.ext)
	direntry := ui.NewEntry()
	direntry.SetText(fmt.Sprintf("%d", status.direction))
	modebutton := ui.NewRadioButtons()
	modebutton.Append("TIME")
	modebutton.Append("AMP")
	modebutton.SetSelected(status.mode)
	gc := &GraphClient{
		Status:     status,
		unit:       unittext(status.ext),
		macentry:   macentry,
		numentry:   numentry,
		extentry:   extentry,
		direntry:   direntry,
		modebutton: modebutton,
		xvalue:     make([]float64, status.size),
		yvalue:     make([]float64, status.size),
		xbuffer:    make([]float64, status.size),
		ybuffer:    make([]float64, status.size),
		verticalAxis: &Axis{
			ndiv:  4,
			scale: 0.1,
		},
	}
	gc.modebutton.OnSelected(func(b *ui.RadioButtons) {
		gc.mode = b.Selected()
		log.Printf("%s: %s mode\n", gc.name, modestring[gc.mode])
	})
	gc.drawarea = ui.NewArea(gc)
	return gc
}

func unittext(ext string) string {
	switch ext {
	case "acc02":
		return "gal"
	case "str01":
		return "με"
	case "ill01":
		return "lx"
	case "gill01":
		return "m/s"
	default:
		return ""
	}
}

func (gc *GraphClient) GetTopic() string {
	return fmt.Sprintf("%s/%s/%s", gc.mac, gc.num, gc.ext)
}

func (gc *GraphClient) SetSubscribe(client mqtt.Client) {
	client.Subscribe(gc.GetTopic(), 2, nil)
	client.Subscribe(fmt.Sprintf("%s/01/info", gc.mac), 2, nil)
}

func (gc *GraphClient) SetInfo() error {
	dir, err := strconv.ParseInt(gc.direntry.Text(), 10, 64)
	if err != nil {
		return err
	}
	gc.direction = int(dir)
	gc.mac = gc.macentry.Text()
	gc.num = gc.numentry.Text()
	gc.ext = gc.extentry.Text()
	gc.unit = unittext(gc.ext)
	gc.mode = gc.modebutton.Selected()
	log.Printf("%s: set topic %s/%s/%s, direction %d, %s mode\n", gc.name, gc.mac, gc.num, gc.ext, gc.direction, modestring[gc.mode])
	return nil
}

func (gc *GraphClient) ClearData() {
	gc.xvalue = make([]float64, gc.size)
	gc.yvalue = make([]float64, gc.size)
	gc.xbuffer = make([]float64, gc.size)
	gc.ybuffer = make([]float64, gc.size)
	gc.lastmtime = 0
	gc.bufferlastmtime = 0
}

func (gc *GraphClient) UpdateData(msg mqtt.Message) error {
	gc.Lock()
	defer gc.Unlock()
	ext := strings.Split(msg.Topic(), "/")[2]
	if ext != gc.ext {
		return nil
	}
	var xdata, ydata []float64
	var ave float64
	switch ext {
	case "acc02":
		var send_time int64
		buf := bytes.NewReader(msg.Payload()[:8])
		binary.Read(buf, binary.BigEndian, &send_time)
		acc, xind, err := rz2.ConvertAccPacket(msg.Payload()[12:])
		if err != nil {
			return err
		}
		acc = acc[xind : len(acc)-3+xind]
		xdata = make([]float64, len(acc)/3)
		ydata = make([]float64, len(acc)/3)
		ave = 0.0
		if gc.lastmtime == 0 {
			gc.lastmtime = send_time - int64(gc.frequency*float64(len(ydata)))
		}
		dtime := float64(send_time-gc.lastmtime) / float64(len(ydata))
		gc.frequency = 1000.0 / dtime
		for i := 0; i < len(xdata); i++ {
			xdata[i] = float64(gc.lastmtime) + dtime*float64(i+1)
			ydata[i] = acc[3*i+gc.direction]
			ave += ydata[i]
		}
		ave /= float64(len(ydata))
		gc.lastmtime = send_time
	case "str01":
		mtime, strain, err := rz2.ConvertStrain(msg.Payload(), 103, true)
		if err != nil {
			return err
		}
		if len(strain) == 0 {
			return fmt.Errorf("no data")
		}
		xdata = make([]float64, len(strain))
		ydata = make([]float64, len(strain))
		ave = 0.0
		for i := 0; i < len(xdata); i++ {
			xdata[i] = float64(mtime[i])
			ydata[i] = strain[i]
			ave += ydata[i]
		}
		ave /= float64(len(ydata))
		gc.frequency = 1000.0 * float64(len(mtime)-1) / float64(mtime[len(mtime)-1]-mtime[0])
		gc.lastmtime = mtime[len(mtime)-1]
	case "ill01":
		var send_time int64
		buf := bytes.NewReader(msg.Payload()[:8])
		binary.Read(buf, binary.BigEndian, &send_time)
		send_time, data, err := rz2.ConvertIll(msg.Payload())
		if err != nil {
			return err
		}
		xdata = make([]float64, len(data))
		ydata = make([]float64, len(data))
		ave = 0.0
		if gc.lastmtime == 0 {
			gc.lastmtime = send_time - int64(gc.frequency*float64(len(ydata)))
		}
		dtime := float64(send_time-gc.lastmtime) / float64(len(ydata))
		gc.frequency = 1000.0 / dtime
		for i := 0; i < len(xdata); i++ {
			xdata[i] = float64(gc.lastmtime) + dtime*float64(i+1)
			ydata[i] = float64(data[i])
			ave += ydata[i]
		}
		ave /= float64(len(ydata))
		gc.lastmtime = send_time
	case "gill01":
		send_time, data0, err := rz2.ConvertGill(msg.Payload())
		if err != nil {
			return err
		}
		data := []float64{data0[1]}
		switch gc.direction {
		case 0:
			data[0] = data0[0]
		case 1:
			data[0] = data0[1]
		}
		xdata = make([]float64, len(data))
		ydata = make([]float64, len(data))
		ave = 0.0
		if gc.lastmtime == 0 {
			gc.lastmtime = send_time - int64(gc.frequency*float64(len(ydata)))
		}
		dtime := float64(send_time-gc.lastmtime) / float64(len(ydata))
		gc.frequency = 1000.0 / dtime
		for i := 0; i < len(xdata); i++ {
			xdata[i] = float64(gc.lastmtime) + dtime*float64(i+1)
			ydata[i] = float64(data[i])
			ave += ydata[i]
		}
		ave /= float64(len(ydata))
		gc.lastmtime = send_time
	default:
		return nil
	}
	if len(xdata) >= gc.size {
		for i := 0; i < gc.size; i++ {
			gc.xvalue[i] = xdata[len(xdata)-gc.size+i]
			gc.yvalue[i] = ydata[len(ydata)-gc.size+i]
		}
	} else {
		for i := 0; i < len(gc.xvalue)-len(xdata); i++ {
			gc.xvalue[i] = gc.xvalue[i+len(xdata)]
			gc.yvalue[i] = gc.yvalue[i+len(ydata)]
		}
		for i := 0; i < len(xdata); i++ {
			gc.xvalue[len(gc.xvalue)-len(xdata)+i] = xdata[i]
			gc.yvalue[len(gc.yvalue)-len(ydata)+i] = ydata[i]
		}
	}
	if updateGraph {
		for i := 0; i < gc.size; i++ {
			gc.xbuffer[i] = gc.xvalue[i]
			gc.ybuffer[i] = gc.yvalue[i]
		}
		gc.bufferlastmtime = gc.lastmtime
		if !syncdraw {
			gc.drawarea.QueueRedrawAll()
		}
	}
	return nil
}

func (gc *GraphClient) UpdateInfo(msg mqtt.Message) error {
	var v interface{}
	json.Unmarshal(msg.Payload(), &v)
	p := dproxy.New(v)
	var d dproxy.Drain
	gc.name = d.String(p.M("name"))
	log.Println(d.String(p.M("name")), d.String(p.M("time")), d.String(p.M("location")), d.String(p.M("hardwareaddress")), d.String(p.M("hostaddress")))
	return d.CombineErrors()
}

func (gc *GraphClient) Draw(a *ui.Area, p *ui.AreaDrawParams) {
	// fill the area with white
	brush := mkSolidBrush(colorWhite, 1.0)
	path := ui.DrawNewPath(ui.DrawFillModeWinding)
	path.AddRectangle(0, 0, p.AreaWidth, p.AreaHeight)
	path.End()
	p.Context.Fill(path, brush)
	path.Free()

	graphWidth, graphHeight := graphSize(p.AreaWidth, p.AreaHeight)

	sp := &ui.DrawStrokeParams{
		Cap:        ui.DrawLineCapFlat,
		Join:       ui.DrawLineJoinMiter,
		Thickness:  2,
		MiterLimit: ui.DrawDefaultMiterLimit,
	}

	// draw the axes
	path = ui.DrawNewPath(ui.DrawFillModeWinding)
	for i := 0; i <= horizontalAxis.ndiv; i++ {
		path.NewFigure(xoffLeft+graphWidth*float64(i)/float64(horizontalAxis.ndiv), yoffTop)
		path.LineTo(xoffLeft+graphWidth*float64(i)/float64(horizontalAxis.ndiv), yoffTop+graphHeight)
	}
	path.End()
	p.Context.Stroke(path, graybrush, sp)
	path.Free()

	path = ui.DrawNewPath(ui.DrawFillModeWinding)
	for i := 0; i <= gc.verticalAxis.ndiv; i++ {
		path.NewFigure(xoffLeft, yoffTop+graphHeight*float64(i)/float64(gc.verticalAxis.ndiv))
		path.LineTo(xoffLeft+graphWidth, yoffTop+graphHeight*float64(i)/float64(gc.verticalAxis.ndiv))
	}
	path.End()
	p.Context.Stroke(path, graybrush, sp)
	path.Free()

	// now transform the coordinate space so (0, 0) is the top-left corner of the graph
	m := ui.DrawNewMatrix()
	m.Translate(xoffLeft, yoffTop)
	p.Context.Transform(m)

	brush.Type = ui.DrawBrushTypeSolid

	if gc.bufferlastmtime == 0 {
		return
	}
	length := int(gc.frequency * horizontalAxis.scale * float64(horizontalAxis.ndiv))
	xdata := make([]float64, length)
	ydata := make([]float64, length)
	min := math.MaxFloat64
	max := -math.MaxFloat64
	ind := 0
	if syncdraw {
		t0 := gc.bufferlastmtime - gc.offsetmtime
		for i := 0; i < gc.size; i++ {
			if gc.xbuffer[gc.size-1-i] > float64(t0) {
				continue
			}
			ind = i
			break
		}
	}
	for i := 0; i < length; i++ {
		xdata[i] = gc.xbuffer[gc.size-length+i-ind]
		ydata[i] = gc.ybuffer[gc.size-length+i-ind]
		if ydata[i] == 0.0 {
			continue
		}
		if ydata[i] < min {
			min = ydata[i]
		} else if ydata[i] > max {
			max = ydata[i]
		}
	}
	mid := 0.5 * (min + max)
	mid2 := math.Round(mid/gc.verticalAxis.scale) * gc.verticalAxis.scale

	var xvalue, yvalue, xs, ys, ys2 []float64
	switch gc.mode {
	case TIME:
		for j := 0; j < length; j++ {
			ydata[j] -= mid2
		}
		xvalue, yvalue, _, xs, ys, ys2 = constructGraph(length, graphWidth, graphHeight, xdata, ydata, "", (float64(graphHeight)/float64(gc.verticalAxis.ndiv))/gc.verticalAxis.scale, gc.frequency)
	case AMP:
		xvalue, yvalue, _, xs, ys, ys2 = constructGraph(length, graphWidth, graphHeight, xdata, ydata, "AMP", (float64(graphHeight)/float64(gc.verticalAxis.ndiv))/gc.verticalAxis.scale, gc.frequency)
	}

	path = ui.DrawNewPath(ui.DrawFillModeWinding)
	path.NewFigure(xs[0], ys[0])
	for i := 1; i < len(xs); i++ {
		path.LineTo(xs[i], ys[i])
	}
	path.End()

	var ylabel string
	switch gc.ext {
	case "acc02":
		ylabel = "Acc. [gal]"
		switch gc.direction {
		case 0:
			brush.R = 0.9
			brush.G = 0.2
			brush.B = 0.0
			brush.A = 1.0
		case 1:
			brush.R = 0.0
			brush.G = 0.9
			brush.B = 0.2
			brush.A = 1.0
		case 2:
			brush.R = 0.0
			brush.G = 0.2
			brush.B = 0.9
			brush.A = 1.0
		}
	case "str01":
		ylabel = "Strain [με]"
		brush.R = 0.9
		brush.G = 0.2
		brush.B = 0.9
		brush.A = 1.0
	case "ill01":
		ylabel = "Illuminance [lx]"
		brush.R = 0.9
		brush.G = 0.9
		brush.B = 0.2
		brush.A = 1.0
	case "gill01":
		ylabel = "Wind Speed [m/s]"
		brush.R = 0.2
		brush.G = 0.9
		brush.B = 0.9
		brush.A = 1.0
	}
	p.Context.Stroke(path, brush, sp)
	path.Free()

	// cepstrum
	if gc.mode == AMP {
		path = ui.DrawNewPath(ui.DrawFillModeWinding)
		path.NewFigure(xs[0], ys2[0])
		for i := 1; i < len(xs); i++ {
			path.LineTo(xs[i], ys2[i])
		}
		path.End()
		brush.R = 0.9
		brush.G = 0.9
		brush.B = 0.0
		brush.A = 1.0
		p.Context.Stroke(path, brush, sp)
		path.Free()
	}

	switch gc.mode {
	case TIME:
		// hozirontal axis value
		for i := 0; i < horizontalAxis.ndiv; i++ {
			str := ui.NewAttributedString(rz2.ConvertUnixtime(int64(xdata[i*length/horizontalAxis.ndiv])).Format("15:04:05.000"))
			str.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(str.String()))
			str.SetAttribute(ui.TextSize(12), 0, len(str.String()))
			str.SetAttribute(ui.TextWeight(400), 0, len(str.String()))
			tl := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
				String:      str,
				DefaultFont: font,
				Width:       500,
				Align:       ui.DrawTextAlign(ui.DrawTextAlignLeft),
			})
			defer tl.Free()
			p.Context.Text(tl, graphWidth*float64(i)/float64(horizontalAxis.ndiv), graphHeight)
		}
		str := ui.NewAttributedString(rz2.ConvertUnixtime(int64(xdata[len(xdata)-1])).Format("15:04:05.000"))
		str.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(str.String()))
		str.SetAttribute(ui.TextSize(12), 0, len(str.String()))
		str.SetAttribute(ui.TextWeight(400), 0, len(str.String()))
		tl := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
			String:      str,
			DefaultFont: font,
			Width:       500,
			Align:       ui.DrawTextAlign(ui.DrawTextAlignLeft),
		})
		defer tl.Free()
		p.Context.Text(tl, graphWidth, graphHeight)
		// vertical axis value
		for i := 0; i <= gc.verticalAxis.ndiv; i++ {
			str := ui.NewAttributedString(fmt.Sprintf("%.3f", mid2+float64(gc.verticalAxis.ndiv/2-i)*gc.verticalAxis.scale))
			str.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(str.String()))
			str.SetAttribute(ui.TextSize(12), 0, len(str.String()))
			str.SetAttribute(ui.TextWeight(400), 0, len(str.String()))
			tl := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
				String:      str,
				DefaultFont: font,
				Width:       xoffRight - float64(font.Size)*0.5,
				Align:       ui.DrawTextAlign(ui.DrawTextAlignLeft),
			})
			defer tl.Free()
			p.Context.Text(tl, graphWidth+float64(font.Size)*0.5, graphHeight*float64(i)/float64(gc.verticalAxis.ndiv)-float64(font.Size)*2.0)
		}
		// horizontal axis label
		xlabel := ui.NewAttributedString("Time [sec]")
		xlabel.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(xlabel.String()))
		xlabel.SetAttribute(ui.TextSize(12), 0, len(xlabel.String()))
		xlabel.SetAttribute(ui.TextWeight(400), 0, len(xlabel.String()))
		tlxlabel := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
			String:      xlabel,
			DefaultFont: font,
			Width:       graphWidth,
			Align:       ui.DrawTextAlign(ui.DrawTextAlignCenter),
		})
		defer tlxlabel.Free()
		p.Context.Text(tlxlabel, 0, graphHeight+yoffBottom/2)
		// vertical axis label
		ylabel := ui.NewAttributedString(ylabel)
		ylabel.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(ylabel.String()))
		ylabel.SetAttribute(ui.TextSize(12), 0, len(ylabel.String()))
		ylabel.SetAttribute(ui.TextWeight(400), 0, len(ylabel.String()))
		tlylabel := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
			String:      ylabel,
			DefaultFont: font,
			Width:       xoffRight,
			Align:       ui.DrawTextAlign(ui.DrawTextAlignCenter),
		})
		defer tlylabel.Free()
		p.Context.Text(tlylabel, graphWidth, graphHeight/2+float64(font.Size))
	case AMP:
		// peak value
		pval := -math.MaxFloat64
		pind := 0
		for i := 0; i < len(xvalue); i++ {
			if xvalue[i] < peakxrange[0] || xvalue[i] > peakxrange[1] {
				continue
			}
			if yvalue[i] > pval {
				pind = i
				pval = yvalue[i]
			}
		}
		path = ui.DrawNewPath(ui.DrawFillModeWinding)
		path.NewFigureWithArc(xs[pind], ys[pind], 5, 0.0, 0.0, false)
		path.ArcTo(xs[pind], ys[pind], 5, 0.0, math.Pi*2.0, false)
		path.CloseFigure()
		path.End()
		p.Context.Fill(path, brush)
		path.Free()
		pstr := ui.NewAttributedString(fmt.Sprintf("%.3f[Hz], %.3f[%s]", xvalue[pind], yvalue[pind], gc.unit))
		pstr.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(pstr.String()))
		pstr.SetAttribute(ui.TextSize(12), 0, len(pstr.String()))
		pstr.SetAttribute(ui.TextWeight(400), 0, len(pstr.String()))
		ptl := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
			String:      pstr,
			DefaultFont: font,
			Width:       500,
			Align:       ui.DrawTextAlign(ui.DrawTextAlignLeft),
		})
		defer ptl.Free()
		p.Context.Text(ptl, xs[pind]+float64(font.Size)*0.5, ys[pind]-float64(font.Size)*2)
		//
		// horizontal axis value
		for i := 0; i <= horizontalAxis.ndiv; i++ {
			// str := ui.NewAttributedString(fmt.Sprintf("%.3f", float64(i)*0.5*gc.frequency/float64(horizontalAxis.ndiv)))
			str := ui.NewAttributedString(fmt.Sprintf("%.3f", ampxrange[0]+float64(i)*(ampxrange[1]-ampxrange[0])/float64(horizontalAxis.ndiv)))
			str.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(str.String()))
			str.SetAttribute(ui.TextSize(12), 0, len(str.String()))
			str.SetAttribute(ui.TextWeight(400), 0, len(str.String()))
			tl := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
				String:      str,
				DefaultFont: font,
				Width:       500,
				Align:       ui.DrawTextAlign(ui.DrawTextAlignLeft),
			})
			defer tl.Free()
			p.Context.Text(tl, graphWidth*float64(i)/float64(horizontalAxis.ndiv), graphHeight)
		}
		// vertical axis value
		for i := 0; i <= gc.verticalAxis.ndiv; i++ {
			str := ui.NewAttributedString(fmt.Sprintf("%.3f", float64(gc.verticalAxis.ndiv-i)*gc.verticalAxis.scale))
			str.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(str.String()))
			str.SetAttribute(ui.TextSize(12), 0, len(str.String()))
			str.SetAttribute(ui.TextWeight(400), 0, len(str.String()))
			tl := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
				String:      str,
				DefaultFont: font,
				Width:       xoffRight - float64(font.Size)*0.5,
				Align:       ui.DrawTextAlign(ui.DrawTextAlignLeft),
			})
			defer tl.Free()
			p.Context.Text(tl, graphWidth+float64(font.Size)*0.5, graphHeight*float64(i)/float64(gc.verticalAxis.ndiv)-float64(font.Size)*2.0)
		}
		// horizontal axis label
		xlabel := ui.NewAttributedString("Freq. [Hz]")
		xlabel.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(xlabel.String()))
		xlabel.SetAttribute(ui.TextSize(12), 0, len(xlabel.String()))
		xlabel.SetAttribute(ui.TextWeight(400), 0, len(xlabel.String()))
		tlxlabel := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
			String:      xlabel,
			DefaultFont: font,
			Width:       graphWidth,
			Align:       ui.DrawTextAlign(ui.DrawTextAlignCenter),
		})
		defer tlxlabel.Free()
		p.Context.Text(tlxlabel, 0, graphHeight+yoffBottom/2)
		// vertical axis label
		ylabel := ui.NewAttributedString(ylabel)
		ylabel.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(ylabel.String()))
		ylabel.SetAttribute(ui.TextSize(12), 0, len(ylabel.String()))
		ylabel.SetAttribute(ui.TextWeight(400), 0, len(ylabel.String()))
		tlylabel := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
			String:      ylabel,
			DefaultFont: font,
			Width:       xoffRight,
			Align:       ui.DrawTextAlign(ui.DrawTextAlignCenter),
		})
		defer tlylabel.Free()
		p.Context.Text(tlylabel, graphWidth, graphHeight/2+float64(font.Size))
	}

	var str *ui.AttributedString
	if min == math.MaxFloat64 {
		str = ui.NewAttributedString(fmt.Sprintf("%s\nFREQ.:%.3f[Hz]\nMAX:---[%s]\nMIN:---[%s]\nRANGE:%.3f[%s]", gc.name, gc.frequency, gc.unit, gc.unit, gc.verticalAxis.scale, gc.unit))
	} else {
		str = ui.NewAttributedString(fmt.Sprintf("%s\nFREQ.:%.3f[Hz]\nMAX:%.3f[%s]\nMIN:%.3f[%s]\nRANGE:%.3f[%s]", gc.name, gc.frequency, max, gc.unit, min, gc.unit, gc.verticalAxis.scale, gc.unit))
	}
	str.SetAttribute(ui.TextColor{0.0, 0.0, 0.0, 1.0}, 0, len(str.String()))
	str.SetAttribute(ui.TextSize(12), 0, len(str.String()))
	str.SetAttribute(ui.TextWeight(400), 0, len(str.String()))
	tl := ui.DrawNewTextLayout(&ui.DrawTextLayoutParams{
		String:      str,
		DefaultFont: font,
		Width:       graphWidth - float64(font.Size),
		Align:       ui.DrawTextAlign(ui.DrawTextAlignRight),
	})
	defer tl.Free()
	p.Context.Text(tl, 0, float64(font.Size))
}

func (gc *GraphClient) MouseEvent(a *ui.Area, me *ui.AreaMouseEvent) {
	// do nothing
}

func (gc *GraphClient) MouseCrossed(a *ui.Area, left bool) {
	// do nothing
}

func (gc *GraphClient) DragBroken(a *ui.Area) {
	// do nothing
}

func (gc *GraphClient) KeyEvent(a *ui.Area, ke *ui.AreaKeyEvent) (handled bool) {
	if !ke.Up {
		return false
	}
	if ke.Key != 0 {
		switch ke.Key {
		// case 'q':
		// 	ui.Quit()
		// 	return false
		// case 'h':
		// 	horizontalAxis.scale /= 2.0
		case 'j':
			gc.verticalAxis.scale /= 2.0
		case 'k':
			gc.verticalAxis.scale *= 2.0
		// case 'l':
		// 	if float64(gc.size)/(float64(horizontalAxis.ndiv)*gc.frequency*horizontalAxis.scale*2) > 1.0 {
		// 		horizontalAxis.scale *= 2.0
		// 	}
		case 's':
			updateGraph = !updateGraph
		default:
			return false
		}
		if !syncdraw {
			gc.drawarea.QueueRedrawAll()
		}
		return true
	} else if ke.ExtKey != 0 {
		switch ke.ExtKey {
		case ui.Escape:
			horizontalAxis.scale = 1.0
			gc.verticalAxis.scale = 0.1
			updateGraph = true
		// case ui.Right:
		// 	if offsetTime < (int(float64(gc.size)/(float64(horizontalAxis.ndiv)*gc.frequency*horizontalAxis.scale))-1)*horizontalAxis.ndiv {
		// 		offsetTime++
		// 	}
		// case ui.Left:
		// 	if offsetTime > 0 {
		// 		offsetTime--
		// 	}
		default:
			return false
		}
		if !syncdraw {
			gc.drawarea.QueueRedrawAll()
		}
		return true
	} else {
		return false
	}
}

func createGraphClientArea(gc *GraphClient) *ui.Box {
	vbox := ui.NewVerticalBox()
	vbox.SetPadded(true)
	vbox.Append(gc.macentry, false)
	vbox.Append(gc.numentry, false)
	vbox.Append(gc.extentry, false)
	vbox.Append(gc.direntry, false)
	gcbutton := ui.NewButton("Set")
	gcbutton.OnClicked(func(b *ui.Button) {
		if connected {
			log.Println("disconnect before changing client info")
		}
		gc.SetInfo()
	})
	vbox.Append(gcbutton, false)
	vbox.Append(gc.modebutton, false)
	return vbox
}

func setupUI() {
	mainwin := ui.NewWindow("gramon", 1280, 960, true)
	mainwin.SetMargined(true)
	mainwin.OnClosing(func(*ui.Window) bool {
		mainwin.Destroy()
		ui.Quit()
		return false
	})
	ui.OnShouldQuit(func() bool {
		mainwin.Destroy()
		return true
	})

	grid := ui.NewGrid()
	grid.SetPadded(true)

	srcentry = ui.NewEntry()
	srcentry.SetText(defaultsrc)
	conbutton = ui.NewButton("Connect")
	conbutton.OnClicked(func(b *ui.Button) {
		if !connected {
			err := connect()
			if err != nil {
				log.Fatal(err)
			}
			if srcclient.IsConnected() {
				conbutton.SetText("Disconnect")
				connected = true
			}
		} else {
			srcclient.Disconnect(1000)
			log.Println("disconnected")
			conbutton.SetText("Connect")
			connected = false
		}
	})

	grid.Append(srcentry, 0, 0, 1, 1, true, ui.AlignFill, false, ui.AlignFill)
	grid.Append(conbutton, 1, 0, 1, 1, false, ui.AlignFill, false, ui.AlignFill)

	graphclient1 = NewGraphClient(defaultstat1)
	graphclient2 = NewGraphClient(defaultstat2)
	graphclient3 = NewGraphClient(defaultstat3)
	graphclient4 = NewGraphClient(defaultstat4)

	vbox1 := createGraphClientArea(graphclient1)
	grid.Append(graphclient1.drawarea, 0, 1, 1, 1, true, ui.AlignFill, true, ui.AlignFill)
	grid.Append(vbox1, 1, 1, 1, 1, false, ui.AlignFill, true, ui.AlignFill)

	vbox2 := createGraphClientArea(graphclient2)
	grid.Append(graphclient2.drawarea, 0, 2, 1, 1, true, ui.AlignFill, true, ui.AlignFill)
	grid.Append(vbox2, 1, 2, 1, 1, false, ui.AlignFill, true, ui.AlignFill)

	vbox3 := createGraphClientArea(graphclient3)
	grid.Append(graphclient3.drawarea, 0, 3, 1, 1, true, ui.AlignFill, true, ui.AlignFill)
	grid.Append(vbox3, 1, 3, 1, 1, false, ui.AlignFill, true, ui.AlignFill)

	vbox4 := createGraphClientArea(graphclient4)
	grid.Append(graphclient4.drawarea, 0, 4, 1, 1, true, ui.AlignFill, true, ui.AlignFill)
	grid.Append(vbox4, 1, 4, 1, 1, false, ui.AlignFill, true, ui.AlignFill)

	mainwin.SetChild(grid)

	mainwin.Show()
}

func CalcFFT(acc []float64) []complex128 {
	// Base line correction
	ave := 0.0
	for i := 0; i < len(acc); i++ {
		ave += acc[i]
	}
	ave /= float64(len(acc))
	for i := 0; i < len(acc); i++ {
		acc[i] -= ave
	}
	// FFT
	return fft.FFTReal(acc)
}

func publishHandler(client mqtt.Client, msg mqtt.Message) {
	lis := strings.Split(msg.Topic(), "/")
	if len(lis) < 3 {
		return
	}
	if lis[2] == "info" {
		if lis[0] == graphclient1.mac {
			graphclient1.UpdateInfo(msg)
		}
		if lis[0] == graphclient2.mac {
			graphclient2.UpdateInfo(msg)
		}
		if lis[0] == graphclient3.mac {
			graphclient3.UpdateInfo(msg)
		}
		if lis[0] == graphclient4.mac {
			graphclient4.UpdateInfo(msg)
		}
	}
	if msg.Topic() == graphclient1.GetTopic() {
		err := graphclient1.UpdateData(msg)
		if err != nil {
			log.Fatal(err)
		}
	}
	if msg.Topic() == graphclient2.GetTopic() {
		err := graphclient2.UpdateData(msg)
		if err != nil {
			log.Fatal(err)
		}
	}
	if msg.Topic() == graphclient3.GetTopic() {
		err := graphclient3.UpdateData(msg)
		if err != nil {
			log.Fatal(err)
		}
	}
	if msg.Topic() == graphclient4.GetTopic() {
		err := graphclient4.UpdateData(msg)
		if err != nil {
			log.Fatal(err)
		}
	}
	err := recorder.Record(msg)
	if err != nil {
		log.Println(err.Error())
	}
}

func syncDraw() {
	graphclient1.Lock()
	graphclient2.Lock()
	graphclient3.Lock()
	graphclient4.Lock()
	defer graphclient1.Unlock()
	defer graphclient2.Unlock()
	defer graphclient3.Unlock()
	defer graphclient4.Unlock()
	minmtime := graphclient1.lastmtime
	if graphclient2.lastmtime < minmtime {
		minmtime = graphclient2.lastmtime
	}
	if graphclient3.lastmtime < minmtime {
		minmtime = graphclient3.lastmtime
	}
	if graphclient4.lastmtime < minmtime {
		minmtime = graphclient4.lastmtime
	}
	graphclient1.offsetmtime = graphclient1.lastmtime - minmtime
	graphclient2.offsetmtime = graphclient2.lastmtime - minmtime
	graphclient3.offsetmtime = graphclient3.lastmtime - minmtime
	graphclient4.offsetmtime = graphclient4.lastmtime - minmtime
	graphclient1.drawarea.QueueRedrawAll()
	graphclient2.drawarea.QueueRedrawAll()
	graphclient3.drawarea.QueueRedrawAll()
	graphclient4.drawarea.QueueRedrawAll()
}

func connect() error {
	graphclient1.ClearData()
	graphclient2.ClearData()
	graphclient3.ClearData()
	graphclient4.ClearData()
	id := fmt.Sprintf("gramon_%d", time.Now().UnixNano())
	src := srcentry.Text()

	srcopts := mqtt.NewClientOptions()
	srcopts.AddBroker(src)
	srcopts.SetAutoReconnect(false)
	srcopts.SetClientID(id)
	if strings.HasPrefix(src, "ssl") {
		tlsconfig, err := rz2.NewTLSConfig(cafile, crtfile, keyfile)
		if err != nil {
			return err
		}
		srcopts.SetTLSConfig(tlsconfig)
	}
	srcopts.SetDefaultPublishHandler(publishHandler)

	srcclient = mqtt.NewClient(srcopts)
	token := srcclient.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		log.Printf(fmt.Sprintf("%s", token.Error()))
		return nil
	}
	if srcclient.IsConnected() {
		log.Printf("connected to %s\n", srcopts.Servers[0])
	} else {
		return fmt.Errorf("connection error")
	}

	graphclient1.SetSubscribe(srcclient)
	graphclient2.SetSubscribe(srcclient)
	graphclient3.SetSubscribe(srcclient)
	graphclient4.SetSubscribe(srcclient)

	ticker := time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				if srcclient.IsConnected() {
					//
				} else {
					log.Printf("disconnected: %s\n", time.Now().Format("15:04:05.000"))
					connected = false
					log.Println("reconnecting...")
					for {
						if srcclient.IsConnected() {
							srcclient.Subscribe("#", 0, nil)
							connected = true
							break
						}
						token := srcclient.Connect()
						token.WaitTimeout(time.Second * 5)
					}
				}
			}
		}
	}()

	if syncdraw {
		ticker := time.NewTicker(1000 * time.Millisecond)
		go func() {
			for {
				select {
				case <-ticker.C:
					if connected {
						syncDraw()
					}
				}
			}
		}()
	}

	return nil
}

func rangevalue(txt string) (float64, float64, error) {
	lis := strings.Split(txt, ":")
	if len(lis) < 2 {
		return 0.0, 0.0, fmt.Errorf("unknown format")
	}
	minval, err := strconv.ParseFloat(lis[0], 64)
	if err != nil {
		return 0.0, 0.0, err
	}
	maxval, err := strconv.ParseFloat(lis[1], 64)
	if err != nil {
		return 0.0, 0.0, err
	}
	if minval > maxval {
		return maxval, minval, nil
	} else {
		return minval, maxval, nil
	}
}

func ReadConfig(fn string) error {
	tree, err := toml.LoadFile(fn)
	if err != nil {
		return err
	}
	defaultsrc = tree.Get("mosquitto.server").(string)
	defaultstat1.Set(tree, "unit.first")
	defaultstat2.Set(tree, "unit.second")
	defaultstat3.Set(tree, "unit.third")
	defaultstat4.Set(tree, "unit.forth")
	minval, maxval, err := rangevalue(tree.Get("range.amp").(string))
	if err == nil {
		ampxrange[0] = minval
		ampxrange[1] = maxval
	}
	minval, maxval, err = rangevalue(tree.Get("range.peak").(string))
	if err == nil {
		peakxrange[0] = minval
		peakxrange[1] = maxval
	}
	if val := tree.Get("cepstrum.coeff"); val != nil {
		cepscoeff = int(val.(int64))
	}
	return nil
}

//TODO: debugモードを追加
func main() {
	syncd := flag.Bool("sync", false, "synchronized drawing (default: false)")
	cafn := flag.String("cafile", "", "ca file")
	crtfn := flag.String("crtfile", "", "crt file")
	keyfn := flag.String("keyfile", "", "key file")
	directory := flag.String("dir", ".", "save directory")
	amprange := flag.String("freq", "", "freq. range (min:max)")
	peakrange := flag.String("peak", "", "peak range (min:max)")
	conffn := flag.String("config", "", "config file")
	flag.Parse()
	syncdraw = *syncd
	if *cafn != "" {
		cafile = *cafn
	}
	if *crtfn != "" {
		crtfile = *crtfn
	}
	if *keyfn != "" {
		keyfile = *keyfn
	}
	if *amprange != "" {
		minval, maxval, err := rangevalue(*amprange)
		if err == nil {
			ampxrange[0] = minval
			ampxrange[1] = maxval
		}
	}
	if *peakrange != "" {
		minval, maxval, err := rangevalue(*peakrange)
		if err == nil {
			peakxrange[0] = minval
			peakxrange[1] = maxval
		}
	}
	if *conffn != "" {
		err := ReadConfig(*conffn)
		if err != nil {
			log.Fatal(err)
		}
	}

	dest, err := rz2.TimeStampDest(*directory)
	if err != nil {
		log.Fatal(err)
	}
	recorder = rz2.NewRecorder(dest)

	go func() {
		ticker := time.NewTicker(time.Minute * 60)
		for {
			select {
			case <-ticker.C:
				if connected {
					dest, err := rz2.TimeStampDest(*directory)
					if err != nil {
						log.Fatal(err)
					}
					recorder.SetDest(dest)
				}
			}
		}
	}()

	ui.Main(setupUI)
}
