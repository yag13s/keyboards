// gocon2026badge の名札 / QR / 追加スライドの画像を生成する。
//
//	# 名札と QR を作る
//	go run . -name やぎ -x yag13s -github yag13s -qr https://x.com/yag13s -out OUT
//
//	# 任意の PNG をスライドに変換する (240x240 に整形される)
//	go run . -slides logo.png,photo.png -out OUT
//
// 出力は 240x240 の PNG、RGB565(BE) の生データ、およびファームが実際に
// //go:embed する RLE 圧縮データ (.rle) の 3 つ。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/fixed"
)

const (
	W = 240
	H = 240

	bandTop    = 26 // 上帯の高さ
	bandBottom = 8  // 下帯の高さ
	qrQuiet    = 32 // QR のクワイエットゾーン
	qrModule   = 7  // QR の 1 モジュールの px 数
)

var (
	goCyan = color.RGBA{0x00, 0xAD, 0xD8, 0xFF}
	white  = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	black  = color.RGBA{0x00, 0x00, 0x00, 0xFF}
)

// 元画像の実測に合わせたベースライン位置
const (
	headerBaseline = 18  // ヘッダ (グリフ 7..18)
	nameBaseline   = 143 // 名前 (グリフ 54..152)
	line1Baseline  = 184 // X 行 (グリフ 169..189)
	line2Baseline  = 214 // GitHub 行 (グリフ 199..219)

	nameMaxWidth = 200 // 名前はこの幅に収まるよう自動縮小する
	nameSize     = 104
	headerSize   = 15
	lineSize     = 18
)

const (
	fontGothic = "/System/Library/Fonts/ヒラギノ角ゴシック W3.ttc"
	fontMono   = "/System/Library/Fonts/Menlo.ttc"
)

func main() {
	var (
		name    = flag.String("name", "", "名札に大きく表示する名前")
		xHandle = flag.String("x", "", "X のハンドル (@ は不要)")
		ghUser  = flag.String("github", "", "GitHub のハンドル")
		qrURL   = flag.String("qr", "", "QR コードに埋め込む URL")
		slides  = flag.String("slides", "", "追加スライドにする画像 (カンマ区切り)")
		pixArt  = flag.String("pixelart", "", "ドット絵として扱う画像 (カンマ区切り)。論理グリッドを検出して整数倍に拡大する")
		outDir  = flag.String("out", ".", "出力先ディレクトリ")
	)
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	if *name != "" {
		img, err := renderNametag(*name, *xHandle, *ghUser)
		if err != nil {
			fatal(err)
		}
		if err := writeAll(filepath.Join(*outDir, "nametag"), img); err != nil {
			fatal(err)
		}
	}

	if *qrURL != "" {
		qr, err := renderQR(*qrURL)
		if err != nil {
			fatal(err)
		}
		if err := writeAll(filepath.Join(*outDir, "qrcode"), qr); err != nil {
			fatal(err)
		}
	}

	for _, spec := range []struct {
		list string
		load func(string) (*image.RGBA, error)
	}{
		{*slides, loadAndFit},
		{*pixArt, loadPixelArt},
	} {
		for _, p := range strings.Split(spec.list, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			img, err := spec.load(p)
			if err != nil {
				fatal(err)
			}
			base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			if err := writeAll(filepath.Join(*outDir, base), img); err != nil {
				fatal(err)
			}
		}
	}
}

// detectGrid はドット絵の論理グリッド数を推定する。
// 「ブロック内の色が一様」となる最大のブロックサイズを探し、その分割数を返す。
// JPEG 由来の滲みを避けるため、各ブロックの中央 50% だけを見る。
func detectGrid(src image.Image) int {
	b := src.Bounds()
	w := b.Dx()
	const tolerance = 8 // JPEG のわずかなノイズは許容する

	uniform := func(blk int) bool {
		n := w / blk
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				mn := [3]int{255, 255, 255}
				mx := [3]int{}
				for y := gy*blk + blk/4; y < gy*blk+blk*3/4; y++ {
					for x := gx*blk + blk/4; x < gx*blk+blk*3/4; x++ {
						r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
						for k, v := range [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)} {
							if v < mn[k] {
								mn[k] = v
							}
							if v > mx[k] {
								mx[k] = v
							}
						}
					}
				}
				for k := range mn {
					if mx[k]-mn[k] > tolerance {
						return false
					}
				}
			}
		}
		return true
	}

	best := 1
	for blk := 2; blk <= w/4; blk++ {
		if w%blk == 0 && uniform(blk) {
			best = blk
		}
	}
	return w / best
}

// loadPixelArt はドット絵を 240x240 に整形する。
// 論理グリッドを検出して各マスの代表色を取り、整数倍に引き伸ばす。
// これで JPEG の圧縮ノイズが消え、ドットの輪郭も鈍らない。
func loadPixelArt(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	b := src.Bounds()
	if b.Dx() != b.Dy() {
		return nil, fmt.Errorf("%s: 正方形の画像のみ対応 (%dx%d)", path, b.Dx(), b.Dy())
	}

	n := detectGrid(src)
	blk := b.Dx() / n

	// 各マスの中心色を取り出して n x n のドット絵に戻す
	cells := make([]color.RGBA, n*n)
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			r, g, bl, _ := src.At(b.Min.X+gx*blk+blk/2, b.Min.Y+gy*blk+blk/2).RGBA()
			cells[gy*n+gx] = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 0xFF}
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	scale := W / n
	if scale*n == W {
		// 整数倍にぴったり収まる
		for gy := 0; gy < n; gy++ {
			for gx := 0; gx < n; gx++ {
				r := image.Rect(gx*scale, gy*scale, (gx+1)*scale, (gy+1)*scale)
				draw.Draw(dst, r, &image.Uniform{cells[gy*n+gx]}, image.Point{}, draw.Src)
			}
		}
		fmt.Printf("  %s: %dx%d ドット (元 %dpx/マス) -> %d 倍に拡大\n",
			filepath.Base(path), n, n, blk, scale)
	} else {
		// 割り切れないので最近傍で伸ばす (輪郭は保たれるがマスの幅が不揃いになる)
		small := image.NewRGBA(image.Rect(0, 0, n, n))
		for i, c := range cells {
			small.SetRGBA(i%n, i/n, c)
		}
		xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), small, small.Bounds(), draw.Src, nil)
		fmt.Printf("  %s: %dx%d ドット -> 240 の整数倍でないため最近傍で拡大\n",
			filepath.Base(path), n, n)
	}
	return dst, nil
}

// loadAndFit は任意の画像を 240x240 に整形する。
// アスペクト比を保ったまま短辺に合わせて拡縮し、中央で切り出す。
func loadAndFit(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	b := src.Bounds()
	scale := float64(W) / float64(b.Dx())
	if s := float64(H) / float64(b.Dy()); s > scale {
		scale = s
	}
	dw, dh := int(float64(b.Dx())*scale+0.5), int(float64(b.Dy())*scale+0.5)

	scaled := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, b, draw.Over, nil)

	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)
	off := image.Pt((dw-W)/2, (dh-H)/2)
	draw.Draw(dst, dst.Bounds(), scaled, off, draw.Src)
	return dst, nil
}

func renderNametag(name, xHandle, ghUser string) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// 上下のシアン帯
	draw.Draw(img, image.Rect(0, 0, W, bandTop), &image.Uniform{goCyan}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, H-bandBottom, W, H), &image.Uniform{goCyan}, image.Point{}, draw.Src)

	mono, err := loadFace(fontMono, headerSize)
	if err != nil {
		return nil, err
	}
	defer mono.Close()
	drawCentered(img, mono, "Go Conference 2026", headerBaseline, white)

	// 名前は幅に収まるまで段階的に縮める
	for size := float64(nameSize); ; size -= 2 {
		face, err := loadFace(fontGothic, size)
		if err != nil {
			return nil, err
		}
		if font.MeasureString(face, name).Ceil() <= nameMaxWidth || size <= 24 {
			drawCentered(img, face, name, nameBaseline, black)
			face.Close()
			break
		}
		face.Close()
	}

	lines, err := loadFace(fontMono, lineSize)
	if err != nil {
		return nil, err
	}
	defer lines.Close()
	if xHandle != "" {
		drawCentered(img, lines, "X:@"+xHandle, line1Baseline, black)
	}
	if ghUser != "" {
		drawCentered(img, lines, "GitHub:"+ghUser, line2Baseline, black)
	}
	return img, nil
}

func renderQR(url string) (*image.RGBA, error) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	qr.DisableBorder = true
	bitmap := qr.Bitmap()
	n := len(bitmap)

	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	mod := qrModule
	if n*mod > W-2*qrQuiet {
		mod = (W - 2*qrQuiet) / n
	}
	if mod < 1 {
		return nil, fmt.Errorf("URL が長すぎて 240x240 に収まりません (%d modules)", n)
	}
	off := (W - n*mod) / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if bitmap[y][x] {
				r := image.Rect(off+x*mod, off+y*mod, off+(x+1)*mod, off+(y+1)*mod)
				draw.Draw(img, r, &image.Uniform{black}, image.Point{}, draw.Src)
			}
		}
	}
	fmt.Printf("  QR: %d modules x %dpx, offset %d\n", n, mod, off)
	return img, nil
}

func loadFace(path string, size float64) (font.Face, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	coll, err := opentype.ParseCollection(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f, err := coll.Font(0)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func drawCentered(dst *image.RGBA, face font.Face, s string, baseline int, c color.Color) {
	w := font.MeasureString(face, s)
	d := &font.Drawer{
		Dst:  dst,
		Src:  &image.Uniform{c},
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(W/2) - w/2, Y: fixed.I(baseline)},
	}
	d.DrawString(s)
}

// toRGB565BE は画像を RGB565 ビッグエンディアンの生データに変換する
func toRGB565BE(img image.Image) []byte {
	raw := make([]byte, 0, W*H*2)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			v := uint16(r>>8)>>3<<11 | uint16(g>>8)>>2<<5 | uint16(b>>8)>>3
			raw = append(raw, byte(v>>8), byte(v))
		}
	}
	return raw
}

// encodeRLE は RGB565 の生データを (count:u16be, pixel:u16be) の並びに圧縮する。
// ファーム側の drawSlide がこれをそのまま展開する。
func encodeRLE(raw []byte) []byte {
	out := make([]byte, 0, len(raw)/4)
	n := len(raw) / 2
	for i := 0; i < n; {
		hi, lo := raw[2*i], raw[2*i+1]
		j := i + 1
		for j < n && raw[2*j] == hi && raw[2*j+1] == lo && j-i < 0xFFFF {
			j++
		}
		cnt := j - i
		out = append(out, byte(cnt>>8), byte(cnt), hi, lo)
		i = j
	}
	return out
}

// decodeRLE は encodeRLE の逆。生成時の自己検証に使う。
func decodeRLE(enc []byte) []byte {
	out := make([]byte, 0, W*H*2)
	for i := 0; i+3 < len(enc); i += 4 {
		cnt := int(enc[i])<<8 | int(enc[i+1])
		for c := 0; c < cnt; c++ {
			out = append(out, enc[i+2], enc[i+3])
		}
	}
	return out
}

// writeAll は PNG / RGB565 raw / RLE を書き出す。
// RLE は書き出す前に必ず展開して元データと突き合わせる。
func writeAll(base string, img *image.RGBA) error {
	f, err := os.Create(base + ".png")
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	raw := toRGB565BE(img)
	if err := os.WriteFile(base+".rgb565", raw, 0o644); err != nil {
		return err
	}

	enc := encodeRLE(raw)
	dec := decodeRLE(enc)
	if len(dec) != len(raw) {
		return fmt.Errorf("%s: RLE 展開後の長さが違う (%d != %d)", base, len(dec), len(raw))
	}
	for i := range raw {
		if raw[i] != dec[i] {
			return fmt.Errorf("%s: RLE 往復で不一致 (offset %d)", base, i)
		}
	}
	if err := os.WriteFile(base+".rle", enc, 0o644); err != nil {
		return err
	}

	fmt.Printf("%-28s raw=%6d  rle=%6d (%.1f%%)  往復一致 OK\n",
		filepath.Base(base), len(raw), len(enc), float64(len(enc))/float64(len(raw))*100)
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
