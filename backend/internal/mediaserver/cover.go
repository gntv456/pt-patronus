package mediaserver

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	// 注册图片解码器，供 image.Decode 识别 jpeg/png/gif（标准库默认仅 png 注册）。
	_ "image/gif"
	_ "image/jpeg"
)

// decodeImage 拉取图片字节并解码为 image.Image（支持 jpeg/png/gif）。
func decodeImage(ctx context.Context, hc httpDoer, url string, headers map[string]string) (image.Image, error) {
	resp, err := fetchImageBytes(ctx, hc, url, headers)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

// CoverOptions 封面拼图参数。
type CoverOptions struct {
	Cols int // 列数
	Rows int // 行数
	// ThumbW / ThumbH 单张缩略图尺寸（像素）；海报比例 ~2:3。
	ThumbW int
	ThumbH int
}

// DefaultCoverOptions 合理默认：3×3 网格，每张 200×300。
func DefaultCoverOptions() CoverOptions {
	return CoverOptions{Cols: 3, Rows: 3, ThumbW: 200, ThumbH: 300}
}

// GenerateCover 取某库最近 N=Cols×Rows 条目的封面，拼成网格 JPEG 字节。
//
// 单张拉取失败跳过（对应格子留空背景色），保证整体可用；这是「海报墙」预览，
// 个别条目无封面属正常。缩略图用 area-average 降采样（无第三方依赖，画质优于最近邻）。
func GenerateCover(ctx context.Context, c Client, libraryID string, opt CoverOptions) ([]byte, error) {
	if opt.Cols <= 0 || opt.Rows <= 0 {
		opt = DefaultCoverOptions()
	}
	if opt.ThumbW <= 0 {
		opt.ThumbW = 200
	}
	if opt.ThumbH <= 0 {
		opt.ThumbH = 300
	}
	limit := opt.Cols * opt.Rows
	items, err := c.RecentItems(ctx, libraryID, limit)
	if err != nil {
		return nil, fmt.Errorf("拉取条目: %w", err)
	}

	canvasW := opt.Cols * opt.ThumbW
	canvasH := opt.Rows * opt.ThumbH
	canvas := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	// 深色背景：缺失 / 透明封面处不突兀。
	fillBG(canvas, color.RGBA{R: 20, G: 22, B: 28, A: 255})

	for i, it := range items {
		if i >= limit {
			break
		}
		if it.ImageURL == "" {
			continue
		}
		img, err := c.FetchImage(ctx, it)
		if err != nil {
			continue // 单张失败跳过
		}
		thumb := resizeImage(img, opt.ThumbW, opt.ThumbH)
		col, row := i%opt.Cols, i/opt.Cols
		dp := image.Pt(col*opt.ThumbW, row*opt.ThumbH)
		drawAt(canvas, thumb, dp)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: 82}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// fillBG 用纯色填满整张图。
func fillBG(m *image.RGBA, c color.Color) {
	bounds := m.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			m.Set(x, y, c)
		}
	}
}

// drawAt 把 src 贴到 dst 的 (x,y) 起点，alpha 叠加（复用标准库 draw.Over 语义）。
func drawAt(dst *image.RGBA, src image.Image, dp image.Point) {
	sp := image.Pt(0, 0)
	// 不导入 image/draw，手写 Over 合成以避免接口边界（src 已是 RGBA，等价于 Src）。
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			off := dst.PixOffset(dp.X+x, dp.Y+y)
			if off < 0 || off+3 >= len(dst.Pix) {
				continue
			}
			sr, sg, sb, sa := src.At(sp.X+x, sp.Y+y).RGBA()
			if sa == 0 {
				continue
			}
			if sa == 0xFFFF {
				dst.Pix[off] = uint8(sr >> 8)
				dst.Pix[off+1] = uint8(sg >> 8)
				dst.Pix[off+2] = uint8(sb >> 8)
				dst.Pix[off+3] = 255
				continue
			}
			// alpha 混合（src premultiplied；dst 假定不透明背景）
			dr := uint32(dst.Pix[off]) * 257
			dg := uint32(dst.Pix[off+1]) * 257
			db := uint32(dst.Pix[off+2]) * 257
			a := sa
			ia := 0xFFFF - a
			dst.Pix[off] = uint8((sr + dr*ia/0xFFFF) >> 8)
			dst.Pix[off+1] = uint8((sg + dg*ia/0xFFFF) >> 8)
			dst.Pix[off+2] = uint8((sb + db*ia/0xFFFF) >> 8)
			dst.Pix[off+3] = 255
		}
	}
}

// resizeImage 用 area-average（盒滤波）降采样：对每个目标像素，累加其覆盖的所有源像素取均值。
// 降采样画质显著优于最近邻；纯标准库实现，无第三方依赖。源像素来自 image.At（统一 RGBA 16-bit）。
func resizeImage(src image.Image, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return dst
	}
	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)
	for dy := 0; dy < dstH; dy++ {
		sy0 := int(float64(dy) * yRatio)
		sy1 := int(float64(dy+1) * yRatio)
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		if sy1 > srcH {
			sy1 = srcH
		}
		for dx := 0; dx < dstW; dx++ {
			sx0 := int(float64(dx) * xRatio)
			sx1 := int(float64(dx+1) * xRatio)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sx1 > srcW {
				sx1 = srcW
			}
			var r, g, b, a, n uint64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					pr, pg, pb, pa := src.At(sb.Min.X+x, sb.Min.Y+y).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					b += uint64(pb)
					a += uint64(pa)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			off := dst.PixOffset(dx, dy)
			dst.Pix[off] = uint8(r / n >> 8)
			dst.Pix[off+1] = uint8(g / n >> 8)
			dst.Pix[off+2] = uint8(b / n >> 8)
			dst.Pix[off+3] = uint8(a / n >> 8)
		}
	}
	return dst
}
