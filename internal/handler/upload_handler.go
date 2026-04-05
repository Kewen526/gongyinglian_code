package handler

import (
	"fmt"
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

// POST /api/v1/upload/image
// Form field: file (multipart file)
func (h *UploadHandler) UploadImage(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的图片文件")
		return
	}
	defer file.Close()

	// Validate extension
	ext := strings.ToLower(path.Ext(header.Filename))
	if !allowedImageExts[ext] {
		response.BadRequest(c, fmt.Sprintf("不支持的图片格式: %s，支持: jpg/jpeg/png/gif/webp/bmp", ext))
		return
	}

	// Max 10MB
	if header.Size > 10*1024*1024 {
		response.BadRequest(c, "图片文件不能超过10MB")
		return
	}

	url, err := oss.Upload("product/images", header.Filename, file)
	if err != nil {
		response.InternalError(c, "图片上传失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"url": url})
}

// POST /api/v1/upload/video
// Form field: file (multipart file)
func (h *UploadHandler) UploadVideo(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的视频文件")
		return
	}
	defer file.Close()

	// Validate extension
	ext := strings.ToLower(path.Ext(header.Filename))
	if !allowedVideoExts[ext] {
		response.BadRequest(c, fmt.Sprintf("不支持的视频格式: %s，支持: mp4/avi/mov/wmv/flv/mkv/webm", ext))
		return
	}

	// Max 200MB
	if header.Size > 200*1024*1024 {
		response.BadRequest(c, "视频文件不能超过200MB")
		return
	}

	url, err := oss.Upload("product/videos", header.Filename, file)
	if err != nil {
		response.InternalError(c, "视频上传失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"url": url})
}

// POST /api/v1/upload/file
// Form field: file (multipart file)
// Generic file upload, any type allowed
func (h *UploadHandler) UploadFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要上传的文件")
		return
	}
	defer file.Close()

	// Max 200MB
	if header.Size > 200*1024*1024 {
		response.BadRequest(c, "文件不能超过200MB")
		return
	}

	url, err := oss.Upload("product/files", header.Filename, file)
	if err != nil {
		response.InternalError(c, "文件上传失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"url": url})
}
