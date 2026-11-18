package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	FPS       int     `json:"fps"`
	Fireflies int     `json:"fireflies"`
	Seed      int64   `json:"seed"`
	Duration  float64 `json:"duration"`
	Wind      float64 `json:"wind"`
	Trails    float64 `json:"trails"`
	Bloom     float64 `json:"bloom"`
	Speed     float64 `json:"speed"`
	Scene     string  `json:"scene"`
	Palette   string  `json:"palette"`
	Snapshot  string  `json:"snapshot"`
	Metrics   string  `json:"metrics"`
	Quiet     bool    `json:"quiet"`
}
type Particle struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	VX      float64 `json:"vx"`
	VY      float64 `json:"vy"`
	Phase   float64 `json:"phase"`
	Rate    float64 `json:"rate"`
	Energy  float64 `json:"energy"`
	Species int     `json:"species"`
	Age     float64 `json:"age"`
	Group   int     `json:"group"`
}
type RGB struct{ R, G, B float64 }
type Metrics struct {
	Frame     int     `json:"frame"`
	Time      float64 `json:"time"`
	Active    int     `json:"active"`
	MeanGlow  float64 `json:"mean_glow"`
	Coherence float64 `json:"coherence"`
	Wind      float64 `json:"wind"`
	Scene     string  `json:"scene"`
}
type Engine struct {
	C      Config
	RNG    *rand.Rand
	P      []Particle
	Light  []RGB
	After  []RGB
	Frames int
	T      float64
	Last   Metrics
}

func clamp(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
func lerp(a, b, t float64) float64 { return a + (b-a)*t }
func color(r, g, b float64) RGB    { return RGB{r, g, b} }
func add(a, b RGB) RGB             { return RGB{a.R + b.R, a.G + b.G, a.B + b.B} }
func scale(a RGB, v float64) RGB   { return RGB{a.R * v, a.G * v, a.B * v} }
func glowColor(s int, p string) RGB {
	switch p {
	case "amber":
		return []RGB{{1, .72, .19}, {1, .39, .12}, {.9, .9, .37}, {.6, .82, .25}}[s%4]
	case "moon":
		return []RGB{{.31, .85, 1}, {.48, .53, 1}, {.76, .65, 1}, {.42, 1, .8}}[s%4]
	case "forest":
		return []RGB{{.55, 1, .19}, {.2, .85, .5}, {.98, .8, .26}, {.42, 1, .74}}[s%4]
	default:
		return []RGB{{.52, 1, .4}, {.96, .85, .34}, {.32, .88, 1}, {.88, .5, 1}}[s%4]
	}
}
func New(c Config) *Engine {
	e := &Engine{C: c, RNG: rand.New(rand.NewSource(c.Seed)), P: make([]Particle, c.Fireflies), Light: make([]RGB, c.Width*c.Height), After: make([]RGB, c.Width*c.Height)}
	for i := range e.P {
		e.spawn(i, true)
	}
	return e
}
func (e *Engine) spawn(i int, initial bool) {
	w, h := float64(e.C.Width), float64(e.C.Height)
	group := i % 7
	x := e.RNG.Float64() * w
	y := e.RNG.Float64() * h
	if e.C.Scene == "grove" {
		x = w * (.12 + .76*e.RNG.Float64())
		y = h * (.25 + .68*e.RNG.Float64())
	}
	if e.C.Scene == "river" {
		y = h*(.48+.17*math.Sin(x/w*8)) + (e.RNG.Float64()-.5)*h*.3
	}
	if e.C.Scene == "canopy" {
		y = h * (.08 + .55*e.RNG.Float64())
	}
	if !initial {
		if e.RNG.Intn(2) == 0 {
			x = 0
		} else {
			x = w - 1
		}
	}
	e.P[i] = Particle{X: x, Y: y, VX: (e.RNG.Float64() - .5) * .6, VY: (e.RNG.Float64() - .5) * .3, Phase: e.RNG.Float64() * 2 * math.Pi, Rate: .6 + e.RNG.Float64()*1.5, Energy: .6 + e.RNG.Float64()*.4, Species: i % 4, Group: group, Age: e.RNG.Float64() * 100}
}
func (e *Engine) field(x, y, t float64) (float64, float64) {
	w := float64(e.C.Width)
	h := float64(e.C.Height)
	nx := x / w
	ny := y / h
	a := math.Sin(nx*10+math.Cos(ny*7+t*.19)*1.7+t*.3) + math.Cos(ny*9-nx*4-t*.23)
	return math.Cos(a) * .17, math.Sin(a) * .13
}
func (e *Engine) step(dt float64) {
	e.T += dt
	w, h := float64(e.C.Width), float64(e.C.Height)
	for i := range e.After {
		e.After[i] = scale(e.After[i], math.Pow(e.C.Trails, dt*30))
		e.Light[i] = RGB{}
	}
	active := 0
	mean := 0.
	sx, sy := 0., 0.
	for i := range e.P {
		p := &e.P[i]
		fx, fy := e.field(p.X, p.Y, e.T)
		drift := math.Sin(e.T*.11+float64(p.Group)*.9) * e.C.Wind
		targetX := fx + drift*.13
		targetY := fy
		if e.C.Scene == "river" {
			targetX += .1
			targetY += (h*.5 + .17*h*math.Sin(p.X/w*8) - p.Y) * .003
		}
		if e.C.Scene == "canopy" {
			targetY += (h*.35 - p.Y) * .002
		}
		if e.C.Scene == "grove" {
			targetX += (w*.5 - p.X) * .0009
			targetY += (h*.55 - p.Y) * .0009
		}
		p.VX = lerp(p.VX, targetX, .045*dt*30)
		p.VY = lerp(p.VY, targetY, .045*dt*30)
		p.X += p.VX * dt * 30 * e.C.Speed
		p.Y += p.VY * dt * 30 * e.C.Speed
		p.Age += dt
		if p.X < 0 {
			p.X += w
		}
		if p.X >= w {
			p.X -= w
		}
		if p.Y < 0 {
			p.Y += h
		}
		if p.Y >= h {
			p.Y -= h
		}
		near := 0.
		for j := 0; j < 7; j++ {
			q := e.P[(i+j*31+1)%len(e.P)]
			dx := q.X - p.X
			dy := q.Y - p.Y
			if dx*dx+dy*dy < 70 {
				near += math.Sin(q.Phase - p.Phase)
			}
		}
		p.Phase += dt * (p.Rate + .04*near)
		pulse := math.Pow(math.Max(0, math.Sin(p.Phase)), 5)
		shimmer := .6 + .4*math.Sin(e.T*2.3+float64(i)*.7)
		intensity := pulse * p.Energy * shimmer
		mean += intensity
		sx += math.Cos(p.Phase)
		sy += math.Sin(p.Phase)
		if intensity > .16 {
			active++
		}
		base := glowColor(p.Species, e.C.Palette)
		radius := 2.2 + e.C.Bloom*2.8
		minX := int(math.Max(0, math.Floor(p.X-radius)))
		maxX := int(math.Min(w-1, math.Ceil(p.X+radius)))
		minY := int(math.Max(0, math.Floor(p.Y-radius)))
		maxY := int(math.Min(h-1, math.Ceil(p.Y+radius)))
		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				dx := (float64(x) - p.X) * 1.1
				dy := (float64(y) - p.Y) * 2.1
				d2 := dx*dx + dy*dy
				v := intensity * (.15*math.Exp(-d2/(radius*radius)) + .85*math.Exp(-d2/1.2))
				idx := y*e.C.Width + x
				e.Light[idx] = add(e.Light[idx], scale(base, v))
			}
		}
	}
	for i := range e.Light {
		e.After[i] = add(e.After[i], e.Light[i])
	}
	e.Frames++
	e.Last = Metrics{Frame: e.Frames, Time: e.T, Active: active, MeanGlow: mean / float64(len(e.P)), Coherence: math.Hypot(sx, sy) / float64(len(e.P)), Wind: e.C.Wind, Scene: e.C.Scene}
}
func byteColor(v float64) int { return int(255 * clamp(1-math.Exp(-v*1.8))) }
func (e *Engine) pixel(x, y int) RGB {
	c := e.After[y*e.C.Width+x]
	u := float64(x) / float64(e.C.Width)
	v := float64(y) / float64(e.C.Height)
	sky := .012 + .015*(1-v)
	c = add(c, color(sky*.35, sky*.7, sky*1.7))
	if e.C.Scene == "grove" {
		ground := .85 + .04*math.Sin(u*18) + .025*math.Sin(u*41)
		if v > ground {
			c = add(c, color(.025, .075, .035))
		}
	}
	if e.C.Scene == "river" {
		bank := .51 + .16*math.Sin(u*8)
		if math.Abs(v-bank) < .04 {
			c = add(c, color(.01, .04, .055))
		}
	}
	return c
}
func (e *Engine) render() string {
	var b strings.Builder
	b.Grow(e.C.Width * e.C.Height * 35)
	b.WriteString("\x1b[H")
	for y := 0; y < e.C.Height; y += 2 {
		for x := 0; x < e.C.Width; x++ {
			top := e.pixel(x, y)
			bottom := top
			if y+1 < e.C.Height {
				bottom = e.pixel(x, y+1)
			}
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", byteColor(top.R), byteColor(top.G), byteColor(top.B), byteColor(bottom.R), byteColor(bottom.G), byteColor(bottom.B))
		}
		b.WriteString("\x1b[0m\n")
	}
	if !e.C.Quiet {
		fmt.Fprintf(&b, "\x1b[0m AURORA ENGINE  %s  %s  %d lights  active %d  sync %.2f  t %.1fs\x1b[K", strings.ToUpper(e.C.Scene), strings.ToUpper(e.C.Palette), len(e.P), e.Last.Active, e.Last.Coherence, e.T)
	}
	return b.String()
}
func (e *Engine) snapshot(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	fmt.Fprintf(bw, "P6\n%d %d\n255\n", e.C.Width, e.C.Height)
	for y := 0; y < e.C.Height; y++ {
		for x := 0; x < e.C.Width; x++ {
			c := e.pixel(x, y)
			bw.Write([]byte{byte(byteColor(c.R)), byte(byteColor(c.G)), byte(byteColor(c.B))})
		}
	}
	return bw.Flush()
}
func (e *Engine) writeMetrics(path string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Config    Config     `json:"config"`
		Metrics   Metrics    `json:"metrics"`
		Particles []Particle `json:"particles"`
	}{e.C, e.Last, e.P})
}
func load(path string, c *Config) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, c)
}
func main() {
	var configPath string
	var c Config
	flag.StringVar(&configPath, "config", "", "JSON configuration file")
	flag.IntVar(&c.Width, "width", 100, "render width in terminal cells")
	flag.IntVar(&c.Height, "height", 48, "render height in pixel rows")
	flag.IntVar(&c.FPS, "fps", 24, "frames per second")
	flag.IntVar(&c.Fireflies, "fireflies", 140, "number of living lights")
	flag.Int64Var(&c.Seed, "seed", time.Now().UnixNano(), "deterministic random seed")
	flag.Float64Var(&c.Duration, "duration", 0, "seconds to run, zero means continuous")
	flag.Float64Var(&c.Wind, "wind", .7, "wind strength")
	flag.Float64Var(&c.Trails, "trails", .83, "phosphorescence persistence 0..0.999")
	flag.Float64Var(&c.Bloom, "bloom", 1.2, "light halo radius")
	flag.Float64Var(&c.Speed, "speed", 1, "movement speed")
	flag.StringVar(&c.Scene, "scene", "grove", "grove, river, canopy, meadow")
	flag.StringVar(&c.Palette, "palette", "forest", "forest, amber, moon, mixed")
	flag.StringVar(&c.Snapshot, "snapshot", "", "write PPM snapshot when exiting")
	flag.StringVar(&c.Metrics, "metrics", "", "write JSON metrics and particle state when exiting")
	flag.BoolVar(&c.Quiet, "quiet", false, "hide status line")
	flag.Parse()
	if err := load(configPath, &c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if c.Width < 16 || c.Width > 400 || c.Height < 8 || c.Height > 200 || c.FPS < 1 || c.FPS > 120 || c.Fireflies < 1 || c.Fireflies > 3000 || c.Trails < 0 || c.Trails >= 1 || c.Speed <= 0 {
		fmt.Fprintln(os.Stderr, "invalid configuration dimensions or parameters")
		os.Exit(2)
	}
	e := New(c)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	interactive := c.Duration <= 0
	if interactive {
		fmt.Print("\x1b[?1049h\x1b[?25l")
		defer fmt.Print("\x1b[0m\x1b[?25h\x1b[?1049l")
	}
	ticker := time.NewTicker(time.Second / time.Duration(c.FPS))
	defer ticker.Stop()
	duration := c.Duration
	if duration <= 0 {
		duration = math.MaxFloat64
	}
	frames := 0
	if c.Duration > 0 {
		frames = int(math.Ceil(duration * float64(c.FPS)))
	}
	for n := 0; frames == 0 || n < frames; n++ {
		select {
		case <-sig:
			goto finish
		default:
		}
		e.step(1 / float64(c.FPS))
		if interactive {
			fmt.Print(e.render())
			<-ticker.C
		}
	}
finish:
	if c.Snapshot != "" {
		if err := e.snapshot(c.Snapshot); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	if err := e.writeMetrics(c.Metrics); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if !interactive {
		fmt.Printf("Rendered %d frames, %.2f seconds, %d fireflies, coherence %.3f\n", e.Frames, e.T, len(e.P), e.Last.Coherence)
	}
	_ = strconv.IntSize
}

type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Habitat struct {
	Name        string  `json:"name"`
	Gravity     Vec2    `json:"gravity"`
	Turbulence  float64 `json:"turbulence"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	Night       float64 `json:"night"`
}

type Colony struct {
	Name      string  `json:"name"`
	Members   []int   `json:"members"`
	Center    Vec2    `json:"center"`
	Radius    float64 `json:"radius"`
	Coherence float64 `json:"coherence"`
	Energy    float64 `json:"energy"`
}

type Sample struct {
	Frame            int     `json:"frame"`
	Time             float64 `json:"time"`
	MeanSpeed        float64 `json:"mean_speed"`
	MeanEnergy       float64 `json:"mean_energy"`
	CircularVariance float64 `json:"circular_variance"`
	Dispersion       float64 `json:"dispersion"`
	Active           int     `json:"active"`
}

type Timeline struct {
	Samples  []Sample `json:"samples"`
	Capacity int      `json:"capacity"`
}

func (v Vec2) Add(b Vec2) Vec2    { return Vec2{v.X + b.X, v.Y + b.Y} }
func (v Vec2) Sub(b Vec2) Vec2    { return Vec2{v.X - b.X, v.Y - b.Y} }
func (v Vec2) Mul(k float64) Vec2 { return Vec2{v.X * k, v.Y * k} }
func (v Vec2) Dot(b Vec2) float64 { return v.X*b.X + v.Y*b.Y }
func (v Vec2) Norm() float64      { return math.Hypot(v.X, v.Y) }
func (v Vec2) Unit() Vec2 {
	n := v.Norm()
	if n < 1e-12 {
		return Vec2{}
	}
	return v.Mul(1 / n)
}
func (v Vec2) Rotate(a float64) Vec2 {
	c, s := math.Cos(a), math.Sin(a)
	return Vec2{v.X*c - v.Y*s, v.X*s + v.Y*c}
}
func (v Vec2) Limit(max float64) Vec2 {
	if v.Norm() > max {
		return v.Unit().Mul(max)
	}
	return v
}
func (v Vec2) Distance(b Vec2) float64     { return v.Sub(b).Norm() }
func (v Vec2) Angle() float64              { return math.Atan2(v.Y, v.X) }
func (v Vec2) Lerp(b Vec2, t float64) Vec2 { return v.Mul(1 - t).Add(b.Mul(t)) }
func (v Vec2) Finite() bool {
	return !math.IsNaN(v.X) && !math.IsNaN(v.Y) && !math.IsInf(v.X, 0) && !math.IsInf(v.Y, 0)
}

func habitatFor(scene string) Habitat {
	switch scene {
	case "grove":
		return Habitat{"grove", Vec2{0, .015}, .25, .62, .78, .96}
	case "river":
		return Habitat{"river", Vec2{.025, 0}, .42, .53, .96, .91}
	case "canopy":
		return Habitat{"canopy", Vec2{0, -.02}, .31, .68, .72, .99}
	case "meadow":
		return Habitat{"meadow", Vec2{0, .005}, .48, .58, .83, .89}
	default:
		return Habitat{scene, Vec2{}, .3, .6, .7, .9}
	}
}

func sampleEngine(e *Engine) Sample {
	s := Sample{Frame: e.Frames, Time: e.T}
	if len(e.P) == 0 {
		return s
	}
	var sum Vec2
	var phase Vec2
	for _, p := range e.P {
		s.MeanSpeed += math.Hypot(p.VX, p.VY)
		s.MeanEnergy += p.Energy
		sum = sum.Add(Vec2{p.X, p.Y})
		phase = phase.Add(Vec2{math.Cos(p.Phase), math.Sin(p.Phase)})
		if p.Energy > .1 {
			s.Active++
		}
	}
	n := float64(len(e.P))
	center := sum.Mul(1 / n)
	for _, p := range e.P {
		s.Dispersion += center.Distance(Vec2{p.X, p.Y})
	}
	s.MeanSpeed /= n
	s.MeanEnergy /= n
	s.Dispersion /= n
	s.CircularVariance = 1 - phase.Norm()/n
	return s
}

func (t *Timeline) Add(s Sample) {
	if t.Capacity <= 0 {
		t.Capacity = 2048
	}
	if len(t.Samples) >= t.Capacity {
		copy(t.Samples, t.Samples[1:])
		t.Samples[len(t.Samples)-1] = s
	} else {
		t.Samples = append(t.Samples, s)
	}
}

func (t Timeline) ExportJSON(path string) error {
	return atomicJSON(path, t)
}

func (t Timeline) ExportCSV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err = w.Write([]string{"frame", "time", "mean_speed", "mean_energy", "circular_variance", "dispersion", "active"}); err != nil {
		return err
	}
	for _, s := range t.Samples {
		row := []string{strconv.Itoa(s.Frame), formatNumber(s.Time), formatNumber(s.MeanSpeed), formatNumber(s.MeanEnergy), formatNumber(s.CircularVariance), formatNumber(s.Dispersion), strconv.Itoa(s.Active)}
		if err = w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func formatNumber(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }

func atomicJSON(path string, v any) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".aurora-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(v); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func computeColonies(e *Engine, groups int) []Colony {
	if groups <= 0 {
		return nil
	}
	result := make([]Colony, groups)
	phase := make([]Vec2, groups)
	for i := range result {
		result[i].Name = fmt.Sprintf("colony-%02d", i+1)
	}
	for i, p := range e.P {
		k := p.Group % groups
		c := &result[k]
		c.Members = append(c.Members, i)
		c.Center = c.Center.Add(Vec2{p.X, p.Y})
		c.Energy += p.Energy
		phase[k] = phase[k].Add(Vec2{math.Cos(p.Phase), math.Sin(p.Phase)})
	}
	for i := range result {
		c := &result[i]
		n := float64(len(c.Members))
		if n == 0 {
			continue
		}
		c.Center = c.Center.Mul(1 / n)
		c.Energy /= n
		c.Coherence = phase[i].Norm() / n
		for _, id := range c.Members {
			c.Radius += c.Center.Distance(Vec2{e.P[id].X, e.P[id].Y})
		}
		c.Radius /= n
	}
	return result
}

type SpatialGrid struct {
	Width    int
	Height   int
	CellSize float64
	Columns  int
	Rows     int
	Buckets  [][]int
}

func newSpatialGrid(width, height int, cell float64) *SpatialGrid {
	if cell < 1 {
		cell = 1
	}
	cols := int(math.Ceil(float64(width) / cell))
	rows := int(math.Ceil(float64(height) / cell))
	return &SpatialGrid{Width: width, Height: height, CellSize: cell, Columns: cols, Rows: rows, Buckets: make([][]int, cols*rows)}
}

func (g *SpatialGrid) index(x, y float64) int {
	cx := int(math.Floor(x / g.CellSize))
	cy := int(math.Floor(y / g.CellSize))
	if cx < 0 {
		cx = 0
	}
	if cy < 0 {
		cy = 0
	}
	if cx >= g.Columns {
		cx = g.Columns - 1
	}
	if cy >= g.Rows {
		cy = g.Rows - 1
	}
	return cy*g.Columns + cx
}

func (g *SpatialGrid) Rebuild(particles []Particle) {
	for i := range g.Buckets {
		g.Buckets[i] = g.Buckets[i][:0]
	}
	for i, p := range particles {
		g.Buckets[g.index(p.X, p.Y)] = append(g.Buckets[g.index(p.X, p.Y)], i)
	}
}

func (g *SpatialGrid) Nearby(x, y, r float64, particles []Particle) []int {
	if r <= 0 {
		return nil
	}
	x0 := int(math.Floor((x - r) / g.CellSize))
	x1 := int(math.Floor((x + r) / g.CellSize))
	y0 := int(math.Floor((y - r) / g.CellSize))
	y1 := int(math.Floor((y + r) / g.CellSize))
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= g.Columns {
		x1 = g.Columns - 1
	}
	if y1 >= g.Rows {
		y1 = g.Rows - 1
	}
	out := make([]int, 0, 32)
	for row := y0; row <= y1; row++ {
		for col := x0; col <= x1; col++ {
			for _, i := range g.Buckets[row*g.Columns+col] {
				if math.Hypot(particles[i].X-x, particles[i].Y-y) <= r {
					out = append(out, i)
				}
			}
		}
	}
	return out
}

type FlowField struct {
	Seed        float64
	Scale       float64
	Strength    float64
	Octaves     int
	Lacunarity  float64
	Persistence float64
}

func hashNoise(x, y int, seed float64) float64 {
	n := math.Sin(float64(x)*127.1+float64(y)*311.7+seed*74.7) * 43758.5453123
	return 2*(n-math.Floor(n)) - 1
}

func smoothNoise(x, y, seed float64) float64 {
	ix, iy := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-math.Floor(x), y-math.Floor(y)
	fx = fx * fx * (3 - 2*fx)
	fy = fy * fy * (3 - 2*fy)
	a := hashNoise(ix, iy, seed)
	b := hashNoise(ix+1, iy, seed)
	c := hashNoise(ix, iy+1, seed)
	d := hashNoise(ix+1, iy+1, seed)
	return lerp(lerp(a, b, fx), lerp(c, d, fx), fy)
}

func (f FlowField) Noise(x, y float64) float64 {
	oct := f.Octaves
	if oct < 1 {
		oct = 1
	}
	if oct > 12 {
		oct = 12
	}
	lac := f.Lacunarity
	if lac <= 0 {
		lac = 2
	}
	persistence := f.Persistence
	if persistence <= 0 {
		persistence = .5
	}
	freq, amp, total, norm := 1., 1., 0., 0.
	for i := 0; i < oct; i++ {
		total += smoothNoise(x*freq, y*freq, f.Seed+float64(i)*17) * amp
		norm += amp
		freq *= lac
		amp *= persistence
	}
	return total / norm
}

func (f FlowField) Velocity(x, y, t float64) Vec2 {
	scale := f.Scale
	if scale <= 0 {
		scale = .025
	}
	epsilon := .02
	xx, yy := x*scale, y*scale
	a := f.Noise(xx, yy+epsilon+t*.01)
	b := f.Noise(xx, yy-epsilon+t*.01)
	c := f.Noise(xx+epsilon, yy+t*.01)
	d := f.Noise(xx-epsilon, yy+t*.01)
	return Vec2{(a - b) / (2 * epsilon), -(c - d) / (2 * epsilon)}.Limit(2).Mul(f.Strength)
}

type LightProfile struct {
	Core       RGB
	Halo       RGB
	Tail       RGB
	Radius     float64
	Flicker    float64
	Saturation float64
}

func lightProfile(species int, palette string) LightProfile {
	base := paletteColor(palette, species)
	return LightProfile{Core: RGB{1, 1, .88}, Halo: base, Tail: scale(base, .35), Radius: 1.5 + float64(species%3)*.6, Flicker: .12 + float64(species%4)*.06, Saturation: .85}
}

func paletteColor(name string, species int) RGB {
	switch name {
	case "forest":
		return []RGB{{.3, 1, .1}, {.7, 1, .2}, {1, .72, .2}, {.1, 1, .65}}[species%4]
	case "amber":
		return []RGB{{1, .5, .06}, {1, .8, .22}, {1, .3, .1}, {.9, .95, .3}}[species%4]
	case "moon":
		return []RGB{{.25, .8, 1}, {.5, .45, 1}, {.6, 1, .9}, {.85, .7, 1}}[species%4]
	default:
		return []RGB{{.3, 1, .4}, {1, .6, .3}, {.4, .8, 1}, {.8, .5, 1}}[species%4]
	}
}

func rgbLuminance(c RGB) float64 { return .2126*c.R + .7152*c.G + .0722*c.B }
func rgbMix(a, b RGB, t float64) RGB {
	return RGB{lerp(a.R, b.R, t), lerp(a.G, b.G, t), lerp(a.B, b.B, t)}
}
func rgbMax(a, b RGB) RGB      { return RGB{math.Max(a.R, b.R), math.Max(a.G, b.G), math.Max(a.B, b.B)} }
func rgbMultiply(a, b RGB) RGB { return RGB{a.R * b.R, a.G * b.G, a.B * b.B} }
func rgbToneMap(c RGB) RGB     { return RGB{c.R / (1 + c.R), c.G / (1 + c.G), c.B / (1 + c.B)} }
func rgbGamma(c RGB, g float64) RGB {
	if g <= 0 {
		g = 2.2
	}
	return RGB{math.Pow(math.Max(c.R, 0), 1/g), math.Pow(math.Max(c.G, 0), 1/g), math.Pow(math.Max(c.B, 0), 1/g)}
}
func rgbSaturate(c RGB, s float64) RGB {
	l := rgbLuminance(c)
	return RGB{lerp(l, c.R, s), lerp(l, c.G, s), lerp(l, c.B, s)}
}
func rgbExposure(c RGB, stops float64) RGB { return scale(c, math.Pow(2, stops)) }

func sampleBilinear(buf []RGB, w, h int, x, y float64) RGB {
	if w < 1 || h < 1 || len(buf) < w*h {
		return RGB{}
	}
	x = math.Max(0, math.Min(float64(w-1), x))
	y = math.Max(0, math.Min(float64(h-1), y))
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := int(math.Min(float64(w-1), float64(x0+1))), int(math.Min(float64(h-1), float64(y0+1)))
	tx, ty := x-float64(x0), y-float64(y0)
	a := rgbMix(buf[y0*w+x0], buf[y0*w+x1], tx)
	b := rgbMix(buf[y1*w+x0], buf[y1*w+x1], tx)
	return rgbMix(a, b, ty)
}

func blurBuffer(src []RGB, w, h int, radius int) []RGB {
	if radius < 1 {
		return append([]RGB(nil), src...)
	}
	if radius > 12 {
		radius = 12
	}
	tmp := make([]RGB, len(src))
	dst := make([]RGB, len(src))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum RGB
			count := 0.
			for dx := -radius; dx <= radius; dx++ {
				xx := x + dx
				if xx >= 0 && xx < w {
					sum = add(sum, src[y*w+xx])
					count++
				}
			}
			tmp[y*w+x] = scale(sum, 1/count)
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum RGB
			count := 0.
			for dy := -radius; dy <= radius; dy++ {
				yy := y + dy
				if yy >= 0 && yy < h {
					sum = add(sum, tmp[yy*w+x])
					count++
				}
			}
			dst[y*w+x] = scale(sum, 1/count)
		}
	}
	return dst
}

func renderHistogram(values []float64, bins int) []int {
	if bins < 1 {
		return nil
	}
	out := make([]int, bins)
	if len(values) == 0 {
		return out
	}
	low, high := values[0], values[0]
	for _, v := range values {
		if v < low {
			low = v
		}
		if v > high {
			high = v
		}
	}
	if high == low {
		out[0] = len(values)
		return out
	}
	for _, v := range values {
		k := int((v - low) / (high - low) * float64(bins))
		if k >= bins {
			k = bins - 1
		}
		if k < 0 {
			k = 0
		}
		out[k]++
	}
	return out
}

func energyQuantiles(p []Particle) [5]float64 {
	var result [5]float64
	if len(p) == 0 {
		return result
	}
	v := make([]float64, len(p))
	for i, x := range p {
		v[i] = x.Energy
	}
	sort.Float64s(v)
	for i, q := range []float64{0, .25, .5, .75, 1} {
		index := int(math.Round(q * float64(len(v)-1)))
		result[i] = v[index]
	}
	return result
}

type StateArchive struct {
	Version   int        `json:"version"`
	Created   string     `json:"created"`
	Config    Config     `json:"config"`
	Frame     int        `json:"frame"`
	Time      float64    `json:"time"`
	Particles []Particle `json:"particles"`
	Metrics   Metrics    `json:"metrics"`
	Colonies  []Colony   `json:"colonies"`
	Sample    Sample     `json:"sample"`
	Habitat   Habitat    `json:"habitat"`
}

func archiveEngine(e *Engine) StateArchive {
	particles := append([]Particle(nil), e.P...)
	return StateArchive{Version: 1, Created: time.Now().UTC().Format(time.RFC3339), Config: e.C, Frame: e.Frames, Time: e.T, Particles: particles, Metrics: e.Last, Colonies: computeColonies(e, 7), Sample: sampleEngine(e), Habitat: habitatFor(e.C.Scene)}
}

func (a StateArchive) Save(path string) error { return atomicJSON(path, a) }

func loadArchive(path string) (StateArchive, error) {
	var a StateArchive
	f, err := os.Open(path)
	if err != nil {
		return a, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 64<<20))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&a); err != nil {
		return a, err
	}
	if a.Version != 1 {
		return a, fmt.Errorf("unsupported archive version: %d", a.Version)
	}
	if len(a.Particles) > 100000 {
		return a, errors.New("archive exceeds particle limit")
	}
	return a, nil
}

func restoreEngine(a StateArchive) (*Engine, error) {
	if a.Config.Width < 16 || a.Config.Height < 8 {
		return nil, errors.New("invalid archive canvas")
	}
	if len(a.Particles) == 0 {
		return nil, errors.New("archive contains no particles")
	}
	a.Config.Fireflies = len(a.Particles)
	e := New(a.Config)
	copy(e.P, a.Particles)
	e.Frames = a.Frame
	e.T = a.Time
	e.Last = a.Metrics
	return e, nil
}

type RenderStats struct {
	Frames         int     `json:"frames"`
	Pixels         int     `json:"pixels"`
	MeanBrightness float64 `json:"mean_brightness"`
	PeakBrightness float64 `json:"peak_brightness"`
	LitPixels      int     `json:"lit_pixels"`
	Coverage       float64 `json:"coverage"`
}

func analyzeFrame(e *Engine) RenderStats {
	s := RenderStats{Frames: e.Frames, Pixels: len(e.Light)}
	if s.Pixels == 0 {
		return s
	}
	for i := range e.Light {
		l := rgbLuminance(add(e.Light[i], e.After[i]))
		s.MeanBrightness += l
		if l > s.PeakBrightness {
			s.PeakBrightness = l
		}
		if l > .05 {
			s.LitPixels++
		}
	}
	s.MeanBrightness /= float64(s.Pixels)
	s.Coverage = float64(s.LitPixels) / float64(s.Pixels)
	return s
}

func reportEngine(e *Engine) string {
	var b strings.Builder
	s := sampleEngine(e)
	r := analyzeFrame(e)
	fmt.Fprintf(&b, "Aurora Engine Observatory\n")
	fmt.Fprintf(&b, "Scene: %s | Palette: %s | Seed: %d\n", e.C.Scene, e.C.Palette, e.C.Seed)
	fmt.Fprintf(&b, "Frame: %d | Simulated: %.2fs\n", e.Frames, e.T)
	fmt.Fprintf(&b, "Fireflies: %d | Active: %d\n", len(e.P), s.Active)
	fmt.Fprintf(&b, "Mean velocity: %.5f | Energy: %.5f\n", s.MeanSpeed, s.MeanEnergy)
	fmt.Fprintf(&b, "Phase variance: %.5f | Dispersion: %.5f\n", s.CircularVariance, s.Dispersion)
	fmt.Fprintf(&b, "Brightness: %.5f | Peak: %.5f | Coverage: %.2f%%\n", r.MeanBrightness, r.PeakBrightness, r.Coverage*100)
	for _, c := range computeColonies(e, 7) {
		fmt.Fprintf(&b, "%s: members=%d coherence=%.4f energy=%.4f radius=%.3f\n", c.Name, len(c.Members), c.Coherence, c.Energy, c.Radius)
	}
	return b.String()
}

func validateConfig(c Config) error {
	if c.Width < 16 || c.Width > 400 {
		return errors.New("width must be 16..400")
	}
	if c.Height < 8 || c.Height > 200 {
		return errors.New("height must be 8..200")
	}
	if c.FPS < 1 || c.FPS > 120 {
		return errors.New("fps must be 1..120")
	}
	if c.Fireflies < 1 || c.Fireflies > 3000 {
		return errors.New("fireflies must be 1..3000")
	}
	if c.Duration < 0 || math.IsNaN(c.Duration) || math.IsInf(c.Duration, 0) {
		return errors.New("invalid duration")
	}
	if c.Speed <= 0 || math.IsNaN(c.Speed) || math.IsInf(c.Speed, 0) {
		return errors.New("invalid speed")
	}
	if c.Trails < 0 || c.Trails >= 1 {
		return errors.New("trails must be in [0,1)")
	}
	if c.Bloom < 0 || c.Bloom > 20 {
		return errors.New("bloom must be 0..20")
	}
	if math.Abs(c.Wind) > 50 {
		return errors.New("wind magnitude must not exceed 50")
	}
	switch c.Scene {
	case "grove", "river", "canopy", "meadow":
	default:
		return fmt.Errorf("unknown scene: %s", c.Scene)
	}
	switch c.Palette {
	case "forest", "amber", "moon", "mixed":
	default:
		return fmt.Errorf("unknown palette: %s", c.Palette)
	}
	return nil
}

func describeConfig(c Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Canvas: %dx%d pixels\n", c.Width, c.Height)
	fmt.Fprintf(&b, "Render: %d FPS | Simulation speed: %.2fx\n", c.FPS, c.Speed)
	fmt.Fprintf(&b, "World: %s | Palette: %s\n", c.Scene, c.Palette)
	fmt.Fprintf(&b, "Population: %d | Wind: %.3f\n", c.Fireflies, c.Wind)
	fmt.Fprintf(&b, "Trails: %.3f | Bloom: %.3f\n", c.Trails, c.Bloom)
	fmt.Fprintf(&b, "Seed: %d | Duration: %.2f\n", c.Seed, c.Duration)
	return b.String()
}

func renderMiniMap(e *Engine, columns, rows int) string {
	if columns < 1 || rows < 1 {
		return ""
	}
	counts := make([]int, columns*rows)
	for _, p := range e.P {
		x := int(p.X / float64(e.C.Width) * float64(columns))
		y := int(p.Y / float64(e.C.Height) * float64(rows))
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= columns {
			x = columns - 1
		}
		if y >= rows {
			y = rows - 1
		}
		counts[y*columns+x]++
	}
	chars := []rune(" .:-=+*#%@")
	var b strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < columns; x++ {
			n := counts[y*columns+x]
			k := int(math.Log1p(float64(n)) * 2)
			if k >= len(chars) {
				k = len(chars) - 1
			}
			b.WriteRune(chars[k])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func circularMean(phases []float64) (float64, float64) {
	if len(phases) == 0 {
		return 0, 0
	}
	x, y := 0., 0.
	for _, a := range phases {
		x += math.Cos(a)
		y += math.Sin(a)
	}
	return math.Atan2(y, x), math.Hypot(x, y) / float64(len(phases))
}

func kuramotoOrder(p []Particle) float64 {
	phases := make([]float64, len(p))
	for i, v := range p {
		phases[i] = v.Phase
	}
	_, order := circularMean(phases)
	return order
}

func neighborSynchronization(p []Particle, g *SpatialGrid, radius float64) float64 {
	if len(p) == 0 {
		return 0
	}
	total, count := 0., 0.
	for i, a := range p {
		nearby := g.Nearby(a.X, a.Y, radius, p)
		for _, j := range nearby {
			if j <= i {
				continue
			}
			total += math.Cos(a.Phase - p[j].Phase)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func entropyOfHistogram(hist []int) float64 {
	sum := 0.
	for _, n := range hist {
		sum += float64(n)
	}
	if sum == 0 {
		return 0
	}
	entropy := 0.
	for _, n := range hist {
		if n == 0 {
			continue
		}
		p := float64(n) / sum
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func spatialEntropy(e *Engine, bins int) float64 {
	if bins < 1 {
		return 0
	}
	hist := make([]int, bins*bins)
	for _, p := range e.P {
		x := int(float64(bins) * p.X / float64(e.C.Width))
		y := int(float64(bins) * p.Y / float64(e.C.Height))
		if x < 0 {
			x = 0
		}
		if x >= bins {
			x = bins - 1
		}
		if y < 0 {
			y = 0
		}
		if y >= bins {
			y = bins - 1
		}
		hist[y*bins+x]++
	}
	return entropyOfHistogram(hist)
}

func colonySeparation(c []Colony) float64 {
	total, count := 0., 0.
	for i := range c {
		for j := i + 1; j < len(c); j++ {
			if len(c[i].Members) == 0 || len(c[j].Members) == 0 {
				continue
			}
			total += c[i].Center.Distance(c[j].Center)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func temperatureCurve(t float64, habitat Habitat) float64 {
	daily := .5 + .5*math.Sin(t*.001-.7)
	return habitat.Temperature * (.7 + .3*daily)
}

func humidityCurve(t float64, habitat Habitat) float64 {
	return clamp(habitat.Humidity + .08*math.Sin(t*.0007+1.2))
}

func nocturnalActivity(t float64, habitat Habitat) float64 {
	return clamp(habitat.Night * (.8 + .2*math.Sin(t*.0004+2.1)))
}

func densityAt(e *Engine, x, y, radius float64) float64 {
	if radius <= 0 {
		return 0
	}
	count := 0
	r2 := radius * radius
	for _, p := range e.P {
		dx, dy := p.X-x, p.Y-y
		if dx*dx+dy*dy <= r2 {
			count++
		}
	}
	return float64(count) / (math.Pi * r2)
}

func nearestParticle(e *Engine, x, y float64) (int, float64) {
	best := -1
	distance := math.Inf(1)
	for i, p := range e.P {
		d := math.Hypot(p.X-x, p.Y-y)
		if d < distance {
			best = i
			distance = d
		}
	}
	return best, distance
}

func boundsOfParticles(p []Particle) (Vec2, Vec2) {
	if len(p) == 0 {
		return Vec2{}, Vec2{}
	}
	min := Vec2{p[0].X, p[0].Y}
	max := min
	for _, v := range p[1:] {
		if v.X < min.X {
			min.X = v.X
		}
		if v.Y < min.Y {
			min.Y = v.Y
		}
		if v.X > max.X {
			max.X = v.X
		}
		if v.Y > max.Y {
			max.Y = v.Y
		}
	}
	return min, max
}

func kineticEnergy(p []Particle) float64 {
	sum := 0.
	for _, v := range p {
		sum += .5 * (v.VX*v.VX + v.VY*v.VY)
	}
	return sum
}

func angularMomentum(p []Particle, center Vec2) float64 {
	sum := 0.
	for _, v := range p {
		dx, dy := v.X-center.X, v.Y-center.Y
		sum += dx*v.VY - dy*v.VX
	}
	return sum
}

func centerOfMass(p []Particle) Vec2 {
	var center Vec2
	if len(p) == 0 {
		return center
	}
	for _, v := range p {
		center = center.Add(Vec2{v.X, v.Y})
	}
	return center.Mul(1 / float64(len(p)))
}

func populationStatistics(e *Engine) map[string]float64 {
	result := map[string]float64{}
	result["kuramoto_order"] = kuramotoOrder(e.P)
	result["spatial_entropy"] = spatialEntropy(e, 12)
	result["kinetic_energy"] = kineticEnergy(e.P)
	result["angular_momentum"] = angularMomentum(e.P, centerOfMass(e.P))
	result["colony_separation"] = colonySeparation(computeColonies(e, 7))
	result["coverage"] = analyzeFrame(e).Coverage
	result["phase_variance"] = sampleEngine(e).CircularVariance
	return result
}
