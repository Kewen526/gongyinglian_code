package handler

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"supply-chain/internal/oss"
	"supply-chain/pkg/response"

	"github.com/gin-gonic/gin"
)

type UploadHandler struct{}

func NewUploadHandler() *UploadHandler {
	return &UploadHandler{}
}

// allowedImageExts defines allowed image extensions.
var allowedImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true,
}

// allowedVideoExts defines allowed video extensions.
var allowedVideoExts = map[string]bool{
	".mp4": true, ".avi": true, ".mov": true, ".wmv": true, ".flv": true, ".mkv": true, ".webm": true,
}

// allowedFileMimes restricts generic file uploads to safe types.
var allowedFileMimes = map[string]bool{
	"image/jpeg":                true,
	"image/png":                 true,
	"image/gif":                 true,
	"image/webp":                true,
	"image/bmp":                 true,
	"video/mp4":                 true,
	"application/pdf":           true,
	"application/zip":           true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.ms-excel":  true,
	"text/plain":                true,
	"text/csv":                  true,
}

// detectMIME reads the first 512 bytes to detect the actual MIME type.
func detectMIME(reader io.ReadSeeker) (string, error) {
	buf := make([]byte, 512)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

// POST /api/v1/upload/image
func (h *UploadHandler) UploadImage(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的图片文件")
		return
	}
	defer file.Close()

	ext := strings.ToLower(path.Ext(header.Filename))
	if !allowedImageExts[ext] {
		response.BadRequest(c, fmt.Sprintf("不支持的图片格式: %s，支持: jpg/jpeg/png/gif/webp/bmp", ext))
		return
	}

	if header.Size > 10*1024*1024 {
		response.BadRequest(c, "图片文件不能超过10MB")
		return
	}

	mime, err := detectMIME(file)
	if err != nil || !strings.HasPrefix(mime, "image/") {
		response.BadRequest(c, "文件内容不是有效的图片")
		return
	}

	url, err := oss.Upload("product/images", header.Filename, file)
	if err != nil {
		response.InternalError(c, "图片上传失败")
		return
	}

	response.Success(c, gin.H{"url": url})
}

// POST /api/v1/upload/video
func (h *UploadHandler) UploadVideo(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的视频文件")
		return
	}
	defer file.Close()

	ext := strings.ToLower(path.Ext(header.Filename))
	if !allowedVideoExts[ext] {
		response.BadRequest(c, fmt.Sprintf("不支持的视频格式: %s，支持: mp4/avi/mov/wmv/flv/mkv/webm", ext))
		return
	}

	if header.Size > 200*1024*1024 {
		response.BadRequest(c, "视频文件不能超过200MB")
		return
	}

	url, err := oss.Upload("product/videos", header.Filename, file)
	if err != nil {
		response.InternalError(c, "视频上传失败")
		return
	}

	response.Success(c, gin.H{"url": url})
}

// POST /api/v1/upload/file
func (h *UploadHandler) UploadFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的文件")
		return
	}
	defer file.Close()

	if header.Size > 200*1024*1024 {
		response.BadRequest(c, "文件不能超过200MB")
		return
	}

	mime, err := detectMIME(file)
	if err != nil || !allowedFileMimes[mime] {
		response.BadRequest(c, "不支持的文件类型，仅支持图片/视频/PDF/Office/CSV/TXT")
		return
	}

	url, err := oss.Upload("product/files", header.Filename, file)
	if err != nil {
		response.InternalError(c, "文件上传失败")
		return
	}

	response.Success(c, gin.H{"url": url})
}
