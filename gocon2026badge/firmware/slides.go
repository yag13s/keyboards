package main

import (
	_ "embed"

	"tinygo.org/x/drivers/st7789"
)

// 名札画面とスライド。画像は PC 側でレンダリングした 240x240 の RGB565(BE) を
// (count:u16be, pixel:u16be) の並びに RLE 圧縮して焼き込む。生のままだと 1 枚
// 115200 バイト使ってフラッシュに 2 枚しか載らないため。
//
// 画像の生成は gen-nametag ツールを使う (元 PNG は firmware 直下)。

//go:embed images/nametag.rle
var slideNametag string

//go:embed images/qrcode.rle
var slideQRCode string

//go:embed images/avatar.rle
var slideAvatar string

// スライド一覧。L で 1 枚目に入り、D/U で送り戻しする。
// 画像を足すときは images/ に .rle を置き、//go:embed を書いてここに追加する。
var slides = []string{
	slideNametag,
	slideAvatar,
	slideQRCode,
}

// drawSlide は RLE 圧縮された 240x240 の画像を展開して全画面に表示する
func drawSlide(display st7789.Device, idx int) error {
	if idx < 0 || idx >= len(slides) {
		return nil
	}

	// 前の DMA 転送が pixelBuf を読んでいる間は書き換えない
	spiBus.Wait()

	buf := pixelBuf.RawBuffer()
	data := slides[idx]
	o := 0
	for i := 0; i+3 < len(data); i += 4 {
		cnt := int(data[i])<<8 | int(data[i+1])
		hi, lo := data[i+2], data[i+3]
		for c := 0; c < cnt && o+1 < len(buf); c++ {
			buf[o], buf[o+1] = hi, lo
			o += 2
		}
	}
	return display.DrawBitmap(0, 0, pixelBuf)
}

// nextSlide は idx を len(slides) の範囲で巡回させる
func nextSlide(idx, delta int) int {
	n := len(slides)
	if n == 0 {
		return 0
	}
	return ((idx+delta)%n + n) % n
}
