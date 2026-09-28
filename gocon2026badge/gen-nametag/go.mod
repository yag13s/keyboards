// gocon2026badge の画像生成ツール。
// ファーム本体 (github.com/sago35/keyboards) とは別モジュールにして、
// x/image などの依存がファームのビルドに混ざらないようにしている。
module github.com/sago35/keyboards/gocon2026badge/gen-nametag

go 1.26.0

require (
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	golang.org/x/image v0.46.0
)

require (
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)
