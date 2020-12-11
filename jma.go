package rz2

import (
	"fmt"
	"math"
	"sort"

	"github.com/mjibson/go-dsp/fft"
)

type Shindo struct {
	Name  string
	Range []float64
	Color []int
}

var (
	ShindoList = []*Shindo{
		&Shindo{
			Name:  "0",
			Range: []float64{-math.MaxFloat64, 0.5},
			Color: []int{255, 255, 255},
		},
		&Shindo{
			Name:  "1",
			Range: []float64{0.5, 1.5},
			Color: []int{242, 242, 255},
		},
		&Shindo{
			Name:  "2",
			Range: []float64{1.5, 2.5},
			Color: []int{0, 170, 255},
		},
		&Shindo{
			Name:  "3",
			Range: []float64{2.5, 3.5},
			Color: []int{0, 65, 255},
		},
		&Shindo{
			Name:  "4",
			Range: []float64{3.5, 4.5},
			Color: []int{255, 230, 150},
		},
		&Shindo{
			Name:  "5弱",
			Range: []float64{4.5, 5.0},
			Color: []int{255, 230, 0},
		},
		&Shindo{
			Name:  "5強",
			Range: []float64{5.0, 5.5},
			Color: []int{255, 153, 0},
		},
		&Shindo{
			Name:  "6弱",
			Range: []float64{5.5, 6.0},
			Color: []int{255, 40, 0},
		},
		&Shindo{
			Name:  "6強",
			Range: []float64{6.0, 6.5},
			Color: []int{165, 0, 33},
		},
		&Shindo{
			Name:  "7",
			Range: []float64{6.5, math.MaxFloat64},
			Color: []int{180, 0, 104},
		},
	}
)

func (s *Shindo) Mid() float64 {
	return 0.5 * (s.Range[0] + s.Range[1])
}

func shindoIndex(value float64) int {
	for i, s := range ShindoList {
		if value < s.Range[1] {
			return i
		}
	}
	return 9
}

func ShindoName(value float64) string {
	return ShindoList[shindoIndex(value)].Name
}

func ShindoColor(value float64) []int {
	ind := shindoIndex(value)
	if ind == 0 || ind == 9 {
		return ShindoList[ind].Color
	}
	shindo := ShindoList[ind]
	mid := shindo.Mid()
	col := shindo.Color
	rtn := make([]int, 3)
	if value < mid {
		last := ShindoList[ind-1]
		lmid := last.Mid()
		lcol := last.Color
		for i := 0; i < 3; i++ {
			rtn[i] = lcol[i] + int(float64(col[i]-lcol[i])*(value-lmid)/(mid-lmid))
		}
	} else {
		next := ShindoList[ind+1]
		nmid := next.Mid()
		ncol := next.Color
		for i := 0; i < 3; i++ {
			rtn[i] = col[i] + int(float64(ncol[i]-col[i])*(value-mid)/(nmid-mid))
		}
	}
	return rtn
}

func JMASeismicIntensityScale(acc []float64) (float64, error) {
	vacc := applyFilter(acc)
	sort.Float64s(vacc)
	ind := int(math.Round(0.3 * freq355))
	if len(vacc) <= ind {
		return 0, fmt.Errorf("not enough data for calculating JMA seismic intensity scale")
	}
	a0 := vacc[len(vacc)-1-ind] // m/s2
	return 2*math.Log10(a0) + 0.94, nil
}

func applyFilter(acc []float64) []float64 {
	dx, dy, dz := separateAcc(acc)
	fftx := fft.FFTReal(dx)
	ffty := fft.FFTReal(dy)
	fftz := fft.FFTReal(dz)
	fil := constructFilter(len(dx), 1.0/freq355)
	for i := 0; i < len(dx); i++ {
		fftx[i] = fftx[i] * complex(fil[i], 0)
		ffty[i] = ffty[i] * complex(fil[i], 0)
		fftz[i] = fftz[i] * complex(fil[i], 0)
	}
	ax := fft.IFFT(fftx)
	ay := fft.IFFT(ffty)
	az := fft.IFFT(fftz)
	rtn := make([]float64, len(dx))
	for i := 0; i < len(dx); i++ {
		rtn[i] = math.Sqrt(math.Pow(real(ax[i]), 2.0) + math.Pow(real(ay[i]), 2.0) + math.Pow(real(az[i]), 2.0))
	}
	return rtn
}

func constructFilter(n int, dt float64) []float64 {
	nh := n/2 + 1
	dur := float64(n) * dt
	rtn := make([]float64, n)
	for i := 1; i < nh; i++ {
		freq := float64(i) / dur
		y := freq / 10.0
		f1 := math.Sqrt(1 / freq)
		f2 := 1.0 / math.Sqrt(1+0.694*math.Pow(y, 2)+0.241*math.Pow(y, 4)+0.0557*math.Pow(y, 6)+0.009664*math.Pow(y, 8)+0.00134*math.Pow(y, 10)+0.000155*math.Pow(y, 12))
		y = 2 * freq
		f3 := math.Sqrt(1.0 - math.Exp(-math.Pow(y, 3)))
		rtn[i] = f1 * f2 * f3
		rtn[n-i] = f1 * f2 * f3
	}
	rtn[0] = 0
	return rtn
}
