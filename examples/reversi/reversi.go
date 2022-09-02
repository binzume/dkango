package main

import (
	"bytes"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"image"
	"image/color"
	"image/gif"

	"github.com/binzume/dkango"
)

var black, white, green []byte // GIF icons. TODO: Use more awesome images

func init() {
	img := image.NewPaletted(image.Rectangle{Max: image.Point{256, 256}}, []color.Color{color.Black, color.Transparent})
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.SetColorIndex(x, y, 0)
		}
	}
	genicon := func(c color.Color) []byte {
		img.Palette[0] = c
		b := bytes.NewBuffer(nil)
		err := gif.Encode(b, img, nil)
		if err != nil {
			panic(err)
		}
		return b.Bytes()
	}
	black = genicon(color.Black)
	white = genicon(color.White)
	green = genicon(color.RGBA{0, 128, 0, 255})
}

type board [8][8]int

func NewBoard() *board {
	b := &board{}
	b[3][3], b[3][4], b[4][3], b[4][4] = 2, 1, 1, 2
	return b
}

func (b *board) scan(x, y, c int, f func(x, y int)) {
	if b[y][x] != 0 {
		return
	}
	d := [8][2]int{{0, 1}, {1, 0}, {-1, 0}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for i := 0; i < 8; i++ {
		for px, py, n := x+d[i][0], y+d[i][1], 0; py >= 0 && py < len(b) && px >= 0 && px < len(b[0]); px, py, n = px+d[i][0], py+d[i][1], n+1 {
			if b[py][px] == c {
				for j := 1; j <= n; j++ {
					f(x+d[i][0]*j, y+d[i][1]*j)
				}
				break
			}
			if b[py][px] == 0 {
				break
			}
		}
	}
}

func (b *board) FlipCount(x, y, c int) int {
	flip := 0
	b.scan(x, y, c, func(x, y int) { flip++ })
	return flip
}

func (b *board) Set(x, y, c int) {
	b.scan(x, y, c, func(x, y int) { b[y][x] = c })
	b[y][x] = c
}

type ai struct {
	cellScore [8][8]int
	col       int
}

func NewAi(c int) *ai {
	return &ai{
		cellScore: [8][8]int{
			{100, -1, 1, 0, 0, 1, -1, 100},
			{-1, -2, 0, 0, 0, 0, -2, -1},
			{1, 0, 1, 0, 0, 1, 0, 1},
			{0, 0, 0, 0, 0, 0, 0, 0},
			{0, 0, 0, 0, 0, 0, 0, 0},
			{1, 0, 1, 0, 0, 1, 0, 1},
			{-1, -2, 0, 0, 0, 0, -2, -1},
			{100, -1, 1, 0, 0, 1, -1, 100},
		},
		col: c,
	}
}

func (a *ai) score(b *board, x, y, c int) int {
	sc := b.FlipCount(x, y, c) * 10
	if sc > 0 {
		sc += a.cellScore[y][x]
	}
	return sc
}

func (a *ai) do(b *board) (int, int) {
	var max, px, py int
	for y := 0; y < len(b); y++ {
		for x := 0; x < len(b[y]); x++ {
			var s = a.score(b, x, y, a.col)
			if s > max {
				max = s
				px = x
				py = y
			}
		}
	}
	if max > 0 {
		b.Set(px, py, a.col)
		return px, py
	} else {
		return -1, -1 // PASS
	}
}

func RestoreBoard(path []string, vsai bool) *board {
	b := NewBoard()
	a := NewAi(2)
	for i, o := range path {
		if o == "PASS" {
			continue
		}
		if len(o) != 2 {
			return nil
		}
		if vsai {
			i *= 2
		}
		x, y, c := int(o[0]-'A'), int(o[1]-'0'), i%2+1
		if x < 0 || x > len(b[0]) || y < 0 || y > len(b) || b.FlipCount(x, y, c) == 0 {
			return nil
		}
		b.Set(x, y, c)
		if vsai {
			a.do(b)
		}
	}
	return b
}

type reversiCell struct {
	name    string
	content []byte
}

func (cell *reversiCell) Name() string {
	return path.Base(cell.name)

}
func (cell *reversiCell) IsDir() bool {
	return cell.content == nil
}
func (cell *reversiCell) Mode() fs.FileMode {
	if cell.IsDir() {
		return fs.ModeDir
	}
	return fs.ModePerm
}
func (cell *reversiCell) Size() int64 {
	return int64(len(cell.content))
}
func (cell *reversiCell) ModTime() time.Time {
	return time.Now()
}
func (cell *reversiCell) Sys() any {
	return nil
}
func (cell *reversiCell) Type() fs.FileMode {
	return cell.Mode().Type()
}
func (cell *reversiCell) Info() (fs.FileInfo, error) {
	return cell, nil
}

type reversiFile struct {
	*reversiCell
	pos int64
}

func (cell *reversiFile) Stat() (fs.FileInfo, error) {
	return cell, nil
}
func (cell *reversiFile) ReadAt(b []byte, ofs int64) (int, error) {
	if ofs >= int64(len(cell.content)) {
		return 0, io.EOF
	}
	n := copy(b, cell.content[ofs:])
	cell.pos = ofs + int64(n)
	return n, nil
}
func (cell *reversiFile) Read(b []byte) (int, error) {
	return cell.ReadAt(b, cell.pos)
}
func (cell *reversiFile) Close() error {
	return nil
}

type reversiFS struct{}

func isValidPath(name string) bool {
	if name == "." || name == "" || name == "README.txt" {
		return true
	}
	p := strings.Split(name, "/")
	top := strings.ToUpper(p[0])
	if top != "START" && top != "START_VS_AI" {
		return false
	}
	for _, n := range p[1:] {
		if n == "PASS" {
			continue
		}
		n = strings.TrimSuffix(n, "_BK")
		n = strings.TrimSuffix(n, "_WH")
		if len(n) != 2 {
			return false
		}
	}

	return true
}

func getReadme() *reversiCell {
	b, err := os.ReadFile("./README.txt")
	if err != nil {
		return nil
	}
	return &reversiCell{name: "README.txt", content: b}
}

func (*reversiFS) Open(name string) (fs.File, error) {
	p := strings.Split(name, "/")
	if len(p) > 1 && p[len(p)-1] == "folder.gif" {
		content := green
		if strings.HasSuffix(p[len(p)-2], "_BK") {
			content = black
		} else if strings.HasSuffix(p[len(p)-2], "_WH") {
			content = white
		}
		return &reversiFile{reversiCell: &reversiCell{name: name, content: content}}, nil
	}
	if !isValidPath(name) {
		return nil, fs.ErrNotExist
	}
	if strings.ToLower(name) == "readme.txt" {
		r := getReadme()
		if r == nil {
			return nil, fs.ErrNotExist
		}
		return &reversiFile{reversiCell: r}, nil
	}
	// normal cell
	return &reversiFile{reversiCell: &reversiCell{name: name}}, nil
}

func (*reversiFS) ReadDir(name string) ([]fs.DirEntry, error) {
	log.Println("ReadDir ", name)
	p := strings.Split(name, "/")
	top := strings.ToUpper(p[0])

	var files []fs.DirEntry
	switch top {
	case "", ".":
		if len(p) != 1 {
			return nil, fs.ErrNotExist
		}
		files = append(files, &reversiCell{name: "START"})
		files = append(files, &reversiCell{name: "START_VS_AI"})
		r := getReadme()
		if r != nil {
			files = append(files, r)
		}
	case "START", "START_VS_AI":
		b := RestoreBoard(p[1:], top == "START_VS_AI")
		if b == nil {
			return nil, fs.ErrPermission // cannot access
		}
		for y := 0; y < len(b); y++ {
			for x := 0; x < len(b[y]); x++ {
				name := string([]byte{'A' + byte(x), '0' + byte(y)})
				if b[y][x] == 1 {
					name += "_BK"
				} else if b[y][x] == 2 {
					name += "_WH"
				}
				files = append(files, &reversiCell{name: name})
			}
		}
	default:
		return nil, fs.ErrNotExist
	}
	return files, nil
}

func main() {
	mount, err := dkango.MountFS("O:", &reversiFS{}, nil)
	if err != nil {
		panic(err)
	}
	defer mount.Close()

	// Block forever
	select {}
}
