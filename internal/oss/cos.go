package oss

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"supply-chain/internal/config"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

var Client *cos.Client

func InitCOS(cfg *config.COSConfig) {
	bucketURL, _ := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cfg.Bucket, cfg.Region))
	serviceURL, _ := url.Parse(fmt.Sprintf("https://cos.%s.myqcloud.com", cfg.Region))

	Client = cos.NewClient(&cos.BaseURL{
		BucketURL:  bucketURL,
		ServiceURL: serviceURL,
	}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
	})
}

// Upload uploads a file to COS and returns the public URL.
// folder: the COS folder, e.g. "product/images", "product/videos"
// filename: original filename
// reader: file content reader
func Upload(folder string, filename string, reader io.Reader) (string, error) {
	cfg := config.GlobalConfig.COS

	// Generate unique filename with timestamp
	ext := path.Ext(filename)
	key := fmt.Sprintf("%s/%d%s", folder, time.Now().UnixNano(), ext)

	_, err := Client.Object.Put(context.Background(), key, reader, nil)
	if err != nil {
		return "", fmt.Errorf("COS upload failed: %w", err)
	}

	// Build public URL
	fileURL := fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s", cfg.Bucket, cfg.Region, key)
	return fileURL, nil
}
