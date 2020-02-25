// SPDX-License-Identifier: Unlicense OR MIT

/*
Package gpu implements the rendering of Gio drawing operations. It
is used by package app and package app/headless and is otherwise not
useful except for integrating with external window implementations.
*/
package gpu

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"
	"unsafe"

	"gioui.org/f32"
	"gioui.org/internal/opconst"
	"gioui.org/internal/ops"
	"gioui.org/internal/path"
	gunsafe "gioui.org/internal/unsafe"
	"gioui.org/op"
	"gioui.org/op/paint"
)

type GPU struct {
	pathCache *opCache
	cache     *resourceCache

	profile                                           string
	timers                                            *timers
	frameStart                                        time.Time
	zopsTimer, stencilTimer, coverTimer, cleanupTimer *timer
	drawOps                                           drawOps
	ctx                                               Backend
	renderer                                          *renderer
}

type renderer struct {
	ctx           Backend
	blitter       *blitter
	pather        *pather
	packer        packer
	intersections packer
}

type drawOps struct {
	profile    bool
	reader     ops.Reader
	cache      *resourceCache
	viewport   image.Point
	clearColor [3]float32
	imageOps   []imageOp
	// zimageOps are the rectangle clipped opaque images
	// that can use fast front-to-back rendering with z-test
	// and no blending.
	zimageOps   []imageOp
	pathOps     []*pathOp
	pathOpCache []pathOp
}

type drawState struct {
	clip  f32.Rectangle
	t     op.TransformOp
	cpath *pathOp
	rect  bool
	z     int

	matType materialType
	// Current paint.ImageOp
	image imageOpData
	// Current paint.ColorOp, if any.
	color color.RGBA
}

type pathOp struct {
	off f32.Point
	// clip is the union of all
	// later clip rectangles.
	clip      image.Rectangle
	pathKey   ops.Key
	path      bool
	pathVerts []byte
	parent    *pathOp
	place     placement
}

type imageOp struct {
	z        float32
	path     *pathOp
	off      f32.Point
	clip     image.Rectangle
	material material
	clipType clipType
	place    placement
}

type material struct {
	material materialType
	opaque   bool
	// For materialTypeColor.
	color [4]float32
	// For materialTypeTexture.
	texture  *texture
	uvScale  f32.Point
	uvOffset f32.Point
}

// clipOp is the shadow of clip.Op.
type clipOp struct {
	bounds f32.Rectangle
}

// imageOpData is the shadow of paint.ImageOp.
type imageOpData struct {
	rect   image.Rectangle
	src    *image.RGBA
	handle interface{}
}

func (op *clipOp) decode(data []byte) {
	if opconst.OpType(data[0]) != opconst.TypeClip {
		panic("invalid op")
	}
	bo := binary.LittleEndian
	r := f32.Rectangle{
		Min: f32.Point{
			X: math.Float32frombits(bo.Uint32(data[1:])),
			Y: math.Float32frombits(bo.Uint32(data[5:])),
		},
		Max: f32.Point{
			X: math.Float32frombits(bo.Uint32(data[9:])),
			Y: math.Float32frombits(bo.Uint32(data[13:])),
		},
	}
	*op = clipOp{
		bounds: r,
	}
}

func decodeImageOp(data []byte, refs []interface{}) imageOpData {
	if opconst.OpType(data[0]) != opconst.TypeImage {
		panic("invalid op")
	}
	handle := refs[1]
	if handle == nil {
		return imageOpData{}
	}
	bo := binary.LittleEndian
	return imageOpData{
		rect: image.Rectangle{
			Min: image.Point{
				X: int(bo.Uint32(data[1:])),
				Y: int(bo.Uint32(data[5:])),
			},
			Max: image.Point{
				X: int(bo.Uint32(data[9:])),
				Y: int(bo.Uint32(data[13:])),
			},
		},
		src:    refs[0].(*image.RGBA),
		handle: handle,
	}
}

func decodeColorOp(data []byte) color.RGBA {
	if opconst.OpType(data[0]) != opconst.TypeColor {
		panic("invalid op")
	}
	return color.RGBA{
		R: data[1],
		G: data[2],
		B: data[3],
		A: data[4],
	}
}

func decodePaintOp(data []byte) paint.PaintOp {
	bo := binary.LittleEndian
	if opconst.OpType(data[0]) != opconst.TypePaint {
		panic("invalid op")
	}
	r := f32.Rectangle{
		Min: f32.Point{
			X: math.Float32frombits(bo.Uint32(data[1:])),
			Y: math.Float32frombits(bo.Uint32(data[5:])),
		},
		Max: f32.Point{
			X: math.Float32frombits(bo.Uint32(data[9:])),
			Y: math.Float32frombits(bo.Uint32(data[13:])),
		},
	}
	return paint.PaintOp{
		Rect: r,
	}
}

type clipType uint8

type resource interface {
	release()
}

type texture struct {
	src *image.RGBA
	tex Texture
}

type blitter struct {
	ctx      Backend
	viewport image.Point
	prog     [2]Program
	vars     [2]struct {
		z                   Uniform
		uScale, uOffset     Uniform
		uUVScale, uUVOffset Uniform
		uColor              Uniform
	}
	quadVerts Buffer
}

type materialType uint8

const (
	clipTypeNone clipType = iota
	clipTypePath
	clipTypeIntersection
)

const (
	materialColor materialType = iota
	materialTexture
)

var (
	blitAttribs = []string{"pos", "uv"}
)

const (
	attribPos = 0
	attribUV  = 1
)

func New(ctx Backend) (*GPU, error) {
	g := &GPU{
		pathCache: newOpCache(),
		cache:     newResourceCache(),
	}
	if err := g.init(ctx); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *GPU) init(ctx Backend) error {
	g.ctx = ctx
	g.renderer = newRenderer(ctx)
	return nil
}

func (g *GPU) Release() {
	g.renderer.release()
	g.pathCache.release()
	g.cache.release()
	if g.timers != nil {
		g.timers.release()
	}
}

func (g *GPU) Collect(viewport image.Point, frameOps *op.Ops) {
	g.renderer.blitter.viewport = viewport
	g.renderer.pather.viewport = viewport
	g.drawOps.reset(g.cache, viewport)
	g.drawOps.collect(g.cache, frameOps, viewport)
	g.frameStart = time.Now()
	if g.drawOps.profile && g.timers == nil && g.ctx.Caps().Features.Has(FeatureTimers) {
		g.timers = newTimers(g.ctx)
		g.zopsTimer = g.timers.newTimer()
		g.stencilTimer = g.timers.newTimer()
		g.coverTimer = g.timers.newTimer()
		g.cleanupTimer = g.timers.newTimer()
	}
	for _, p := range g.drawOps.pathOps {
		if _, exists := g.pathCache.get(p.pathKey); !exists {
			data := buildPath(g.ctx, p.pathVerts)
			g.pathCache.put(p.pathKey, data)
		}
		p.pathVerts = nil
	}
}

func (g *GPU) BeginFrame() {
	g.ctx.BeginFrame()
	defer g.ctx.EndFrame()
	viewport := g.renderer.blitter.viewport
	for _, img := range g.drawOps.imageOps {
		expandPathOp(img.path, img.clip)
	}
	if g.drawOps.profile {
		g.zopsTimer.begin()
	}
	g.ctx.DepthFunc(DepthFuncGreater)
	g.ctx.ClearColor(g.drawOps.clearColor[0], g.drawOps.clearColor[1], g.drawOps.clearColor[2], 1.0)
	g.ctx.ClearDepth(0.0)
	g.ctx.Clear(BufferAttachmentColor | BufferAttachmentDepth)
	g.ctx.Viewport(0, 0, viewport.X, viewport.Y)
	g.renderer.drawZOps(g.drawOps.zimageOps)
	g.zopsTimer.end()
	g.stencilTimer.begin()
	g.ctx.SetBlend(true)
	g.renderer.packStencils(&g.drawOps.pathOps)
	g.renderer.stencilClips(g.pathCache, g.drawOps.pathOps)
	g.renderer.packIntersections(g.drawOps.imageOps)
	g.renderer.intersect(g.drawOps.imageOps)
	g.stencilTimer.end()
	g.coverTimer.begin()
	g.ctx.Viewport(0, 0, viewport.X, viewport.Y)
	g.renderer.drawOps(g.drawOps.imageOps)
	g.ctx.SetBlend(false)
	g.renderer.pather.stenciler.invalidateFBO()
	g.coverTimer.end()
}

func (g *GPU) EndFrame() {
	g.cleanupTimer.begin()
	g.cache.frame()
	g.pathCache.frame()
	g.cleanupTimer.end()
	if g.drawOps.profile && g.timers.ready() {
		zt, st, covt, cleant := g.zopsTimer.Elapsed, g.stencilTimer.Elapsed, g.coverTimer.Elapsed, g.cleanupTimer.Elapsed
		ft := zt + st + covt + cleant
		q := 100 * time.Microsecond
		zt, st, covt = zt.Round(q), st.Round(q), covt.Round(q)
		frameDur := time.Since(g.frameStart).Round(q)
		ft = ft.Round(q)
		g.profile = fmt.Sprintf("draw:%7s gpu:%7s zt:%7s st:%7s cov:%7s", frameDur, ft, zt, st, covt)
	}
}

func (g *GPU) Profile() string {
	return g.profile
}

func (r *renderer) texHandle(t *texture) Texture {
	if t.tex != nil {
		return t.tex
	}
	t.tex = r.ctx.NewTexture(FilterLinear, FilterLinear)
	t.tex.Upload(t.src)
	return t.tex
}

func (t *texture) release() {
	if t.tex != nil {
		t.tex.Release()
	}
}

func newRenderer(ctx Backend) *renderer {
	r := &renderer{
		ctx:     ctx,
		blitter: newBlitter(ctx),
		pather:  newPather(ctx),
	}
	r.packer.maxDim = ctx.Caps().MaxTextureSize
	r.intersections.maxDim = r.packer.maxDim
	return r
}

func (r *renderer) release() {
	r.pather.release()
	r.blitter.release()
}

func newBlitter(ctx Backend) *blitter {
	prog, err := createColorPrograms(ctx, blitVSrc, blitFSrc)
	if err != nil {
		panic(err)
	}
	quadVerts := ctx.NewBuffer(BufferTypeData)
	quadVerts.Upload(BufferUsageStaticDraw,
		gunsafe.BytesView([]float32{
			-1, +1, 0, 0,
			+1, +1, 1, 0,
			-1, -1, 0, 1,
			+1, -1, 1, 1,
		}))
	b := &blitter{
		ctx:       ctx,
		prog:      prog,
		quadVerts: quadVerts,
	}
	for i, prog := range prog {
		switch materialType(i) {
		case materialTexture:
			uTex := prog.UniformFor("tex")
			prog.Uniform1i(uTex, 0)
			b.vars[i].uUVScale = prog.UniformFor("uvScale")
			b.vars[i].uUVOffset = prog.UniformFor("uvOffset")
		case materialColor:
			b.vars[i].uColor = prog.UniformFor("color")
		}
		b.vars[i].z = prog.UniformFor("z")
		b.vars[i].uScale = prog.UniformFor("scale")
		b.vars[i].uOffset = prog.UniformFor("offset")
	}
	return b
}

func (b *blitter) release() {
	b.quadVerts.Release()
	for _, p := range b.prog {
		p.Release()
	}
}

func createColorPrograms(ctx Backend, vsSrc, fsSrc string) ([2]Program, error) {
	var prog [2]Program
	frep := strings.NewReplacer(
		"HEADER", `
uniform sampler2D tex;
`,
		"GET_COLOR", `texture2D(tex, vUV)`,
	)
	fsSrcTex := frep.Replace(fsSrc)
	var err error
	prog[materialTexture], err = ctx.NewProgram(vsSrc, fsSrcTex, blitAttribs)
	if err != nil {
		return prog, err
	}
	frep = strings.NewReplacer(
		"HEADER", `
uniform vec4 color;
`,
		"GET_COLOR", `color`,
	)
	fsSrcCol := frep.Replace(fsSrc)
	prog[materialColor], err = ctx.NewProgram(vsSrc, fsSrcCol, blitAttribs)
	if err != nil {
		prog[materialTexture].Release()
		return prog, err
	}
	return prog, nil
}

func (r *renderer) stencilClips(pathCache *opCache, ops []*pathOp) {
	if len(r.packer.sizes) == 0 {
		return
	}
	fbo := -1
	r.pather.begin(r.packer.sizes)
	for _, p := range ops {
		if fbo != p.place.Idx {
			fbo = p.place.Idx
			f := r.pather.stenciler.cover(fbo)
			bindFramebuffer(f.fbo)
			r.ctx.Clear(BufferAttachmentColor)
		}
		data, _ := pathCache.get(p.pathKey)
		r.pather.stencilPath(p.clip, p.off, p.place.Pos, data.(*pathData))
	}
	r.pather.end()
}

func (r *renderer) intersect(ops []imageOp) {
	if len(r.intersections.sizes) == 0 {
		return
	}
	fbo := -1
	r.pather.stenciler.beginIntersect(r.intersections.sizes)
	r.blitter.quadVerts.Bind()
	r.ctx.SetupVertexArray(attribPos, 2, DataTypeFloat, 4*4, 0)
	r.ctx.SetupVertexArray(attribUV, 2, DataTypeFloat, 4*4, 4*2)
	for _, img := range ops {
		if img.clipType != clipTypeIntersection {
			continue
		}
		if fbo != img.place.Idx {
			fbo = img.place.Idx
			f := r.pather.stenciler.intersections.fbos[fbo]
			bindFramebuffer(f.fbo)
			r.ctx.Clear(BufferAttachmentColor)
		}
		r.ctx.Viewport(img.place.Pos.X, img.place.Pos.Y, img.clip.Dx(), img.clip.Dy())
		r.intersectPath(img.path, img.clip)
	}
	r.pather.stenciler.endIntersect()
}

func (r *renderer) intersectPath(p *pathOp, clip image.Rectangle) {
	if p.parent != nil {
		r.intersectPath(p.parent, clip)
	}
	if !p.path {
		return
	}
	o := p.place.Pos.Add(clip.Min).Sub(p.clip.Min)
	uv := image.Rectangle{
		Min: o,
		Max: o.Add(clip.Size()),
	}
	fbo := r.pather.stenciler.cover(p.place.Idx)
	fbo.tex.Bind(0)
	coverScale, coverOff := texSpaceTransform(toRectF(uv), fbo.size)
	r.pather.stenciler.iprog.Uniform2f(r.pather.stenciler.uIntersectUVScale, coverScale.X, coverScale.Y)
	r.pather.stenciler.iprog.Uniform2f(r.pather.stenciler.uIntersectUVOffset, coverOff.X, coverOff.Y)
	r.ctx.DrawArrays(DrawModeTriangleStrip, 0, 4)
}

func (r *renderer) packIntersections(ops []imageOp) {
	r.intersections.clear()
	for i, img := range ops {
		var npaths int
		var onePath *pathOp
		for p := img.path; p != nil; p = p.parent {
			if p.path {
				onePath = p
				npaths++
			}
		}
		switch npaths {
		case 0:
		case 1:
			place := onePath.place
			place.Pos = place.Pos.Sub(onePath.clip.Min).Add(img.clip.Min)
			ops[i].place = place
			ops[i].clipType = clipTypePath
		default:
			sz := image.Point{X: img.clip.Dx(), Y: img.clip.Dy()}
			place, ok := r.intersections.add(sz)
			if !ok {
				panic("internal error: if the intersection fit, the intersection should fit as well")
			}
			ops[i].clipType = clipTypeIntersection
			ops[i].place = place
		}
	}
}

func (r *renderer) packStencils(pops *[]*pathOp) {
	r.packer.clear()
	ops := *pops
	// Allocate atlas space for cover textures.
	var i int
	for i < len(ops) {
		p := ops[i]
		if p.clip.Empty() {
			ops[i] = ops[len(ops)-1]
			ops = ops[:len(ops)-1]
			continue
		}
		sz := image.Point{X: p.clip.Dx(), Y: p.clip.Dy()}
		place, ok := r.packer.add(sz)
		if !ok {
			// The clip area is at most the entire screen. Hopefully no
			// screen is larger than GL_MAX_TEXTURE_SIZE.
			panic(fmt.Errorf("clip area %v is larger than maximum texture size %dx%d", p.clip, r.packer.maxDim, r.packer.maxDim))
		}
		p.place = place
		i++
	}
	*pops = ops
}

// intersects intersects clip and b where b is offset by off.
// ceilRect returns a bounding image.Rectangle for a f32.Rectangle.
func boundRectF(r f32.Rectangle) image.Rectangle {
	return image.Rectangle{
		Min: image.Point{
			X: int(floor(r.Min.X)),
			Y: int(floor(r.Min.Y)),
		},
		Max: image.Point{
			X: int(ceil(r.Max.X)),
			Y: int(ceil(r.Max.Y)),
		},
	}
}

func toRectF(r image.Rectangle) f32.Rectangle {
	return f32.Rectangle{
		Min: f32.Point{
			X: float32(r.Min.X),
			Y: float32(r.Min.Y),
		},
		Max: f32.Point{
			X: float32(r.Max.X),
			Y: float32(r.Max.Y),
		},
	}
}

func ceil(v float32) int {
	return int(math.Ceil(float64(v)))
}

func floor(v float32) int {
	return int(math.Floor(float64(v)))
}

func (d *drawOps) reset(cache *resourceCache, viewport image.Point) {
	d.profile = false
	d.clearColor = [3]float32{1.0, 1.0, 1.0}
	d.cache = cache
	d.viewport = viewport
	d.imageOps = d.imageOps[:0]
	d.zimageOps = d.zimageOps[:0]
	d.pathOps = d.pathOps[:0]
	d.pathOpCache = d.pathOpCache[:0]
}

func (d *drawOps) collect(cache *resourceCache, root *op.Ops, viewport image.Point) {
	d.reset(cache, viewport)
	clip := f32.Rectangle{
		Max: f32.Point{X: float32(viewport.X), Y: float32(viewport.Y)},
	}
	d.reader.Reset(root)
	state := drawState{
		clip:  clip,
		rect:  true,
		color: color.RGBA{A: 0xff},
	}
	d.collectOps(&d.reader, state)
}

func (d *drawOps) newPathOp() *pathOp {
	d.pathOpCache = append(d.pathOpCache, pathOp{})
	return &d.pathOpCache[len(d.pathOpCache)-1]
}

func (d *drawOps) collectOps(r *ops.Reader, state drawState) int {
	var aux []byte
	var auxKey ops.Key
loop:
	for encOp, ok := r.Decode(); ok; encOp, ok = r.Decode() {
		switch opconst.OpType(encOp.Data[0]) {
		case opconst.TypeProfile:
			d.profile = true
		case opconst.TypeTransform:
			dop := ops.DecodeTransformOp(encOp.Data)
			state.t = state.t.Multiply(op.TransformOp(dop))
		case opconst.TypeAux:
			aux = encOp.Data[opconst.TypeAuxLen:]
			// The first data byte stores whether the MaxY
			// fields have been initialized.
			maxyFilled := aux[0] == 1
			aux[0] = 1
			aux = aux[1:]
			if !maxyFilled {
				fillMaxY(aux)
			}
			auxKey = encOp.Key
		case opconst.TypeClip:
			var op clipOp
			op.decode(encOp.Data)
			off := state.t.Transform(f32.Point{})
			state.clip = state.clip.Intersect(op.bounds.Add(off))
			if state.clip.Empty() {
				continue
			}
			npath := d.newPathOp()
			*npath = pathOp{
				parent: state.cpath,
				off:    off,
			}
			state.cpath = npath
			if len(aux) > 0 {
				state.rect = false
				state.cpath.pathKey = auxKey
				state.cpath.path = true
				state.cpath.pathVerts = aux
				d.pathOps = append(d.pathOps, state.cpath)
			}
			aux = nil
			auxKey = ops.Key{}
		case opconst.TypeColor:
			state.matType = materialColor
			state.color = decodeColorOp(encOp.Data)
		case opconst.TypeImage:
			state.matType = materialTexture
			state.image = decodeImageOp(encOp.Data, encOp.Refs)
		case opconst.TypePaint:
			op := decodePaintOp(encOp.Data)
			off := state.t.Transform(f32.Point{})
			clip := state.clip.Intersect(op.Rect.Add(off))
			if clip.Empty() {
				continue
			}
			bounds := boundRectF(clip)
			mat := state.materialFor(d.cache, op.Rect, off, bounds)
			if bounds.Min == (image.Point{}) && bounds.Max == d.viewport && state.rect && mat.opaque && mat.material == materialColor {
				// The image is a uniform opaque color and takes up the whole screen.
				// Scrap images up to and including this image and set clear color.
				d.zimageOps = d.zimageOps[:0]
				d.imageOps = d.imageOps[:0]
				state.z = 0
				copy(d.clearColor[:], mat.color[:3])
				continue
			}
			state.z++
			// Assume 16-bit depth buffer.
			const zdepth = 1 << 16
			// Convert z to window-space, assuming depth range [0;1].
			zf := float32(state.z)*2/zdepth - 1.0
			img := imageOp{
				z:        zf,
				path:     state.cpath,
				off:      off,
				clip:     bounds,
				material: mat,
			}
			if state.rect && img.material.opaque {
				d.zimageOps = append(d.zimageOps, img)
			} else {
				d.imageOps = append(d.imageOps, img)
			}
		case opconst.TypePush:
			state.z = d.collectOps(r, state)
		case opconst.TypePop:
			break loop
		}
	}
	return state.z
}

func expandPathOp(p *pathOp, clip image.Rectangle) {
	for p != nil {
		pclip := p.clip
		if !pclip.Empty() {
			clip = clip.Union(pclip)
		}
		p.clip = clip
		p = p.parent
	}
}

func (d *drawState) materialFor(cache *resourceCache, rect f32.Rectangle, off f32.Point, clip image.Rectangle) material {
	var m material
	switch d.matType {
	case materialColor:
		m.material = materialColor
		m.color = gamma(d.color.RGBA())
		m.opaque = m.color[3] == 1.0
	case materialTexture:
		m.material = materialTexture
		dr := boundRectF(rect.Add(off))
		sz := d.image.src.Bounds().Size()
		sr := toRectF(d.image.rect)
		if dx := float32(dr.Dx()); dx != 0 {
			// Don't clip 1 px width sources.
			if sdx := sr.Dx(); sdx > 1 {
				sr.Min.X += (float32(clip.Min.X-dr.Min.X)*sdx + dx/2) / dx
				sr.Max.X -= (float32(dr.Max.X-clip.Max.X)*sdx + dx/2) / dx
			}
		}
		if dy := float32(dr.Dy()); dy != 0 {
			// Don't clip 1 px height sources.
			if sdy := sr.Dy(); sdy > 1 {
				sr.Min.Y += (float32(clip.Min.Y-dr.Min.Y)*sdy + dy/2) / dy
				sr.Max.Y -= (float32(dr.Max.Y-clip.Max.Y)*sdy + dy/2) / dy
			}
		}
		tex, exists := cache.get(d.image.handle)
		if !exists {
			t := &texture{
				src: d.image.src,
			}
			cache.put(d.image.handle, t)
			tex = t
		}
		m.texture = tex.(*texture)
		m.uvScale, m.uvOffset = texSpaceTransform(sr, sz)
	}
	return m
}

func (r *renderer) drawZOps(ops []imageOp) {
	r.ctx.SetDepthTest(true)
	r.blitter.quadVerts.Bind()
	r.ctx.SetupVertexArray(attribPos, 2, DataTypeFloat, 4*4, 0)
	r.ctx.SetupVertexArray(attribUV, 2, DataTypeFloat, 4*4, 4*2)
	// Render front to back.
	for i := len(ops) - 1; i >= 0; i-- {
		img := ops[i]
		m := img.material
		switch m.material {
		case materialTexture:
			r.texHandle(m.texture).Bind(0)
		}
		drc := img.clip
		scale, off := clipSpaceTransform(drc, r.blitter.viewport)
		r.blitter.blit(img.z, m.material, m.color, scale, off, m.uvScale, m.uvOffset)
	}
	r.ctx.SetDepthTest(false)
}

func (r *renderer) drawOps(ops []imageOp) {
	r.ctx.SetDepthTest(true)
	r.ctx.DepthMask(false)
	r.ctx.BlendFunc(BlendFactorOne, BlendFactorOneMinusSrcAlpha)
	r.blitter.quadVerts.Bind()
	r.ctx.SetupVertexArray(attribPos, 2, DataTypeFloat, 4*4, 0)
	r.ctx.SetupVertexArray(attribUV, 2, DataTypeFloat, 4*4, 4*2)
	var coverTex Texture
	for _, img := range ops {
		m := img.material
		switch m.material {
		case materialTexture:
			r.texHandle(m.texture).Bind(0)
		}
		drc := img.clip
		scale, off := clipSpaceTransform(drc, r.blitter.viewport)
		var fbo stencilFBO
		switch img.clipType {
		case clipTypeNone:
			r.blitter.blit(img.z, m.material, m.color, scale, off, m.uvScale, m.uvOffset)
			continue
		case clipTypePath:
			fbo = r.pather.stenciler.cover(img.place.Idx)
		case clipTypeIntersection:
			fbo = r.pather.stenciler.intersections.fbos[img.place.Idx]
		}
		if coverTex != fbo.tex {
			coverTex = fbo.tex
			coverTex.Bind(1)
		}
		uv := image.Rectangle{
			Min: img.place.Pos,
			Max: img.place.Pos.Add(drc.Size()),
		}
		coverScale, coverOff := texSpaceTransform(toRectF(uv), fbo.size)
		r.pather.cover(img.z, m.material, m.color, scale, off, m.uvScale, m.uvOffset, coverScale, coverOff)
	}
	r.ctx.DepthMask(true)
	r.ctx.SetDepthTest(false)
}

func gamma(r, g, b, a uint32) [4]float32 {
	color := [4]float32{float32(r) / 0xffff, float32(g) / 0xffff, float32(b) / 0xffff, float32(a) / 0xffff}
	// Assume that image.Uniform colors are in sRGB space. Linearize.
	for i := 0; i <= 2; i++ {
		c := color[i]
		// Use the formula from EXT_sRGB.
		if c <= 0.04045 {
			c = c / 12.92
		} else {
			c = float32(math.Pow(float64((c+0.055)/1.055), 2.4))
		}
		color[i] = c
	}
	return color
}

func (b *blitter) blit(z float32, mat materialType, col [4]float32, scale, off, uvScale, uvOff f32.Point) {
	p := b.prog[mat]
	p.Bind()
	switch mat {
	case materialColor:
		p.Uniform4f(b.vars[mat].uColor, col[0], col[1], col[2], col[3])
	case materialTexture:
		p.Uniform2f(b.vars[mat].uUVScale, uvScale.X, uvScale.Y)
		p.Uniform2f(b.vars[mat].uUVOffset, uvOff.X, uvOff.Y)
	}
	p.Uniform1f(b.vars[mat].z, z)
	p.Uniform2f(b.vars[mat].uScale, scale.X, scale.Y)
	p.Uniform2f(b.vars[mat].uOffset, off.X, off.Y)
	b.ctx.DrawArrays(DrawModeTriangleStrip, 0, 4)
}

// texSpaceTransform return the scale and offset that transforms the given subimage
// into quad texture coordinates.
func texSpaceTransform(r f32.Rectangle, bounds image.Point) (f32.Point, f32.Point) {
	size := f32.Point{X: float32(bounds.X), Y: float32(bounds.Y)}
	scale := f32.Point{X: r.Dx() / size.X, Y: r.Dy() / size.Y}
	offset := f32.Point{X: r.Min.X / size.X, Y: r.Min.Y / size.Y}
	return scale, offset
}

// clipSpaceTransform returns the scale and offset that transforms the given
// rectangle from a viewport into OpenGL clip space.
func clipSpaceTransform(r image.Rectangle, viewport image.Point) (f32.Point, f32.Point) {
	// First, transform UI coordinates to OpenGL coordinates:
	//
	//	[(-1, +1) (+1, +1)]
	//	[(-1, -1) (+1, -1)]
	//
	x, y := float32(r.Min.X), float32(r.Min.Y)
	w, h := float32(r.Dx()), float32(r.Dy())
	vx, vy := 2/float32(viewport.X), 2/float32(viewport.Y)
	x = x*vx - 1
	y = 1 - y*vy
	w *= vx
	h *= vy

	// Then, compute the transformation from the fullscreen quad to
	// the rectangle at (x, y) and dimensions (w, h).
	scale := f32.Point{X: w * .5, Y: h * .5}
	offset := f32.Point{X: x + w*.5, Y: y - h*.5}
	return scale, offset
}

func bindFramebuffer(fbo Framebuffer) {
	fbo.Bind()
	if err := fbo.IsComplete(); err != nil {
		panic(fmt.Errorf("AA FBO not complete: %v", err))
	}
}

// Fill in maximal Y coordinates of the NW and NE corners.
func fillMaxY(verts []byte) {
	contour := 0
	bo := binary.LittleEndian
	for len(verts) > 0 {
		maxy := float32(math.Inf(-1))
		i := 0
		for ; i+path.VertStride*4 <= len(verts); i += path.VertStride * 4 {
			vert := verts[i : i+path.VertStride]
			// MaxY contains the integer contour index.
			pathContour := int(bo.Uint32(vert[int(unsafe.Offsetof(((*path.Vertex)(nil)).MaxY)):]))
			if contour != pathContour {
				contour = pathContour
				break
			}
			fromy := math.Float32frombits(bo.Uint32(vert[int(unsafe.Offsetof(((*path.Vertex)(nil)).FromY)):]))
			ctrly := math.Float32frombits(bo.Uint32(vert[int(unsafe.Offsetof(((*path.Vertex)(nil)).CtrlY)):]))
			toy := math.Float32frombits(bo.Uint32(vert[int(unsafe.Offsetof(((*path.Vertex)(nil)).ToY)):]))
			if fromy > maxy {
				maxy = fromy
			}
			if ctrly > maxy {
				maxy = ctrly
			}
			if toy > maxy {
				maxy = toy
			}
		}
		fillContourMaxY(maxy, verts[:i])
		verts = verts[i:]
	}
}

func fillContourMaxY(maxy float32, verts []byte) {
	bo := binary.LittleEndian
	for i := 0; i < len(verts); i += path.VertStride {
		off := int(unsafe.Offsetof(((*path.Vertex)(nil)).MaxY))
		bo.PutUint32(verts[i+off:], math.Float32bits(maxy))
	}
}

const blitVSrc = `
#version 100

precision highp float;

uniform float z;
uniform vec2 scale;
uniform vec2 offset;

attribute vec2 pos;

attribute vec2 uv;
uniform vec2 uvScale;
uniform vec2 uvOffset;

varying vec2 vUV;

void main() {
	vec2 p = pos;
	p *= scale;
	p += offset;
	gl_Position = vec4(p, z, 1);
	vUV = uv*uvScale + uvOffset;
}
`

const blitFSrc = `
#version 100

precision mediump float;

varying vec2 vUV;

HEADER

void main() {
	gl_FragColor = GET_COLOR;
}
`
