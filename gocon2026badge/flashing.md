# gocon2026badge ファームウェア書き込み手順

ピンアサインと操作方法は [README](../README.md#gocon2026badge) を参照。

macOS / Apple Silicon・TinyGo 0.42.0・Go 1.26.4 で確認。

## 必要なもの

* gocon2026badge 本体
* **データ通信対応の USB ケーブル** — 充電専用ケーブルでは認識しない

## セットアップ

```
brew install go
brew tap tinygo-org/tools
brew install tinygo
```

Homebrew の `tinygo` は Go を一緒に入れないため、Go は別途必要。

```
$ tinygo version
tinygo version 0.42.0 darwin/arm64 (using go version go1.26.4 and LLVM version 22.1.4)
```

## 書き込み

**BOOT ボタンを押しながら** USB ケーブルを挿す。すでに挿さっている場合は BOOT を押したまま RESET を押して離す。`RPI-RP2` ドライブがマウントされたら、リポジトリのルートで実行する。

```
tinygo flash --target waveshare-rp2040-zero --size short --stack-size 8kb ./gocon2026badge/firmware/
```

一度このファームを書き込んだ個体は、次回から BOOT ボタンを押す必要がない。TinyGo が 1200bps リセットで自動的にブートローダへ落とす。

## 確認

次の 2 つが成り立っていれば成功。

* `RPI-RP2` が消えている（ブートローダを抜けて再起動した）
* `/dev/cu.usbmodem*` が生えている（ファームの USB スタックが動いている）

シリアルは `run()` がエラーを返したときだけ、そのエラーを 1 秒ごとに出力する。**無出力が正常**。

```
tinygo monitor -port /dev/cu.usbmodem31201
```

なお ST7789 は書き込み専用のため、画面が繋がっていなくてもエラーにはならない。シリアルが静かなことは表示が出ることの確認にはならない。

## トラブルシューティング

### `unable to locate any volume: [RPI-RP2]`

BOOTSEL に入っていない。BOOT を押しながら挿し直す。

TinyGo の 1200bps 自動リセットは `waveshare-rp2040-zero.json` の `serial-port: ["2e8a:0003"]` に一致するポートしか探さないため、新品や MicroPython 入り（`2e8a:0005`）の個体では効かない。

### USB で認識されない

ケーブルを疑う。充電専用ケーブルではデータ線が繋がっておらず、BOOTSEL に入れても `RPI-RP2` は出てこない。

### RP2040 を複数挿していて、どれがバッジかわからない

```
ioreg -p IOUSB -l -w 0 | grep -E '\+-o |"USB Product Name"|"idVendor"|"idProduct"'
```

| 表示 | VID:PID | 状態 |
|---|---|---|
| `RP2 Boot` | `2e8a:0003` | BOOTSEL 中（書き込める） |
| `RP2040_Zero` | `2e8a:0003` | このファームが動作中 |
| `Board in FS mode` | `2e8a:0005` | MicroPython 入りの別ボード |

シリアル番号は RP2040 のフラッシュ固有 ID なので機種の判別には使えない。確実なのはケーブルを抜いて、消えたものを見ること。

### `tinygo monitor` が `open /dev/tty: device not configured` で落ちる

対話的な TTY が必要なので、実ターミナルで実行する。非対話シェルから読むならポートを直接読む。

```
stty -f /dev/cu.usbmodem31201 115200 raw -echo
cat /dev/cu.usbmodem31201
```

`stty` に `1200` を指定するとボードがブートローダへ落ちるので注意。

### 「ディスクの不正な取り出し」の警告が出る

正常。書き込み完了と同時にボードが再起動し、`RPI-RP2` がアンマウントされるため。
