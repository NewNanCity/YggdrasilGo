// Package file 文件存储材质管理
package file

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
	storage "yggdrasil-api-go/src/storage/interface"

	"github.com/bytedance/sonic"
)

// TextureMetadata 材质元数据
type TextureMetadata struct {
	Type       storage.TextureType `json:"type"`
	PlayerUUID string              `json:"player_uuid"`
	Hash       string              `json:"hash"`
	FileSize   int64               `json:"file_size"`
	UploadedAt time.Time           `json:"uploaded_at"`
	Slim       bool                `json:"slim,omitempty"`
}

// UploadTexture 上传材质文件
func (s *Storage) UploadTexture(textureType storage.TextureType, playerUUID string, data []byte, metadata *storage.TextureMetadata) (*storage.TextureInfo, error) {
	if !s.textureConfig.UploadEnabled {
		return nil, fmt.Errorf("texture upload is disabled")
	}

	if int64(len(data)) > s.textureConfig.MaxFileSize {
		return nil, fmt.Errorf("texture file too large")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 计算文件哈希
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// 确定文件扩展名
	extension := ".png"
	if len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		extension = ".jpg"
	}

	// 保存材质文件
	textureDir := string(textureType) + "s" // skins, capes
	filePath := s.getHashPath(filepath.Join("textures", textureDir), hashStr, extension)

	if err := s.ensureDir(filePath); err != nil {
		return nil, fmt.Errorf("failed to create texture directory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to save texture file: %w", err)
	}

	// 保存材质元数据
	textureMetadata := &TextureMetadata{
		Type:       textureType,
		PlayerUUID: playerUUID,
		Hash:       hashStr,
		FileSize:   int64(len(data)),
		UploadedAt: time.Now(),
	}

	if metadata != nil {
		textureMetadata.Slim = metadata.Slim
	}

	metadataPath := s.getHashPath(filepath.Join("textures", textureDir), hashStr, ".json")
	if err := s.saveTextureMetadata(metadataPath, textureMetadata); err != nil {
		return nil, fmt.Errorf("failed to save texture metadata: %w", err)
	}

	// 构建材质URL
	textureURL := fmt.Sprintf("%s/textures/%s/%s%s", s.textureConfig.BaseURL, textureDir, hashStr, extension)

	return &storage.TextureInfo{
		Type: textureType,
		URL:  textureURL,
		Metadata: &storage.TextureMetadata{
			Hash:       hashStr,
			FileSize:   int64(len(data)),
			UploadedAt: time.Now(),
			Slim:       textureMetadata.Slim,
		},
	}, nil
}

// GetTexture 获取材质信息
func (s *Storage) GetTexture(textureType storage.TextureType, playerUUID string) (*storage.TextureInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 查找材质元数据文件
	textureDir := string(textureType) + "s"
	metadataPattern := filepath.Join(s.dataDir, "textures", textureDir, "*", "*", "*.json")

	matches, err := filepath.Glob(metadataPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search texture metadata: %w", err)
	}

	for _, metadataPath := range matches {
		metadata, err := s.loadTextureMetadata(metadataPath)
		if err != nil {
			continue
		}

		if metadata.PlayerUUID == playerUUID && metadata.Type == textureType {
			// 确定文件扩展名
			extension := ".png"
			texturePath := metadataPath[:len(metadataPath)-5] + extension // 移除.json，添加.png
			if _, err := os.Stat(texturePath); os.IsNotExist(err) {
				texturePath = metadataPath[:len(metadataPath)-5] + ".jpg"
			}

			textureURL := fmt.Sprintf("%s/textures/%s/%s%s", s.textureConfig.BaseURL, textureDir, metadata.Hash, extension)

			return &storage.TextureInfo{
				Type: textureType,
				URL:  textureURL,
				Metadata: &storage.TextureMetadata{
					Hash:       metadata.Hash,
					FileSize:   metadata.FileSize,
					UploadedAt: metadata.UploadedAt,
					Slim:       metadata.Slim,
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("texture not found")
}

// DeleteTexture 删除材质
func (s *Storage) DeleteTexture(textureType storage.TextureType, playerUUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查找并删除材质文件和元数据
	textureDir := string(textureType) + "s"
	metadataPattern := filepath.Join(s.dataDir, "textures", textureDir, "*", "*", "*.json")

	matches, err := filepath.Glob(metadataPattern)
	if err != nil {
		return fmt.Errorf("failed to search texture metadata: %w", err)
	}

	for _, metadataPath := range matches {
		metadata, err := s.loadTextureMetadata(metadataPath)
		if err != nil {
			continue
		}

		if metadata.PlayerUUID == playerUUID && metadata.Type == textureType {
			// 删除材质文件
			extension := ".png"
			texturePath := metadataPath[:len(metadataPath)-5] + extension
			if _, err := os.Stat(texturePath); os.IsNotExist(err) {
				texturePath = metadataPath[:len(metadataPath)-5] + ".jpg"
			}
			os.Remove(texturePath)

			// 删除元数据文件
			os.Remove(metadataPath)
			return nil
		}
	}

	return fmt.Errorf("texture not found")
}

// GetTextureURL 计算材质URL
func (s *Storage) GetTextureURL(textureType storage.TextureType, playerUUID string) string {
	return fmt.Sprintf("%s/textures/%s/%s", s.textureConfig.BaseURL, textureType, playerUUID)
}

// IsUploadSupported 检查是否支持材质上传
func (s *Storage) IsUploadSupported() bool {
	return s.textureConfig.UploadEnabled
}

// saveTextureMetadata 保存材质元数据
func (s *Storage) saveTextureMetadata(path string, metadata *TextureMetadata) error {
	if err := s.ensureDir(path); err != nil {
		return err
	}

	data, err := sonic.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// loadTextureMetadata 加载材质元数据
func (s *Storage) loadTextureMetadata(path string) (*TextureMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var metadata TextureMetadata
	if err := sonic.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}
