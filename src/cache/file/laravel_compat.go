// Package file Laravel缓存兼容性实现
package file

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/trim21/go-phpserialize"
)

// LaravelCacheEntry Laravel缓存条目格式（PHP序列化）
// Laravel缓存文件格式：{serialized_data}i:{expiration_timestamp};
// 例如：s:10:"test_value"i:1744686812;

// LaravelFileCache Laravel文件缓存兼容实现
type LaravelFileCache struct {
	cacheDir string
}

// NewLaravelFileCache 创建Laravel兼容的文件缓存
func NewLaravelFileCache(cacheDir string) *LaravelFileCache {
	return &LaravelFileCache{
		cacheDir: cacheDir,
	}
}

// GetCacheFilePath 获取缓存文件路径（Laravel兼容）
func (c *LaravelFileCache) GetCacheFilePath(key string) string {
	// Laravel使用MD5哈希作为文件名
	hash := md5.Sum([]byte(key))
	hashStr := hex.EncodeToString(hash[:])

	// Laravel的文件缓存路径格式：cache/data/{hash[0:2]}/{hash[2:4]}/{hash}
	return filepath.Join(c.cacheDir, "data", hashStr[0:2], hashStr[2:4], hashStr)
}

// Store 存储数据到Laravel兼容的缓存文件
func (c *LaravelFileCache) Store(key string, data interface{}, ttl time.Duration) error {
	// 使用PHP序列化库序列化数据
	serializedData, err := phpserialize.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	// 创建Laravel格式的缓存内容
	// 格式：{php_serialized_data}i:{expiration_timestamp};
	expiresAt := time.Now().Add(ttl).Unix()
	cacheContent := fmt.Sprintf("%si:%d;", string(serializedData), expiresAt)

	// 获取文件路径
	filePath := c.GetCacheFilePath(key)

	// 创建目录
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, []byte(cacheContent), 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Get 从Laravel兼容的缓存文件获取数据
func (c *LaravelFileCache) Get(key string, target interface{}) error {
	filePath := c.GetCacheFilePath(key)

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cache not found")
		}
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	// 解析Laravel缓存格式：{php_serialized_data}i:{expiration_timestamp};
	content := string(data)
	serializedData, expiresAt, err := c.ParseLaravelCache(content)
	if err != nil {
		return fmt.Errorf("failed to parse Laravel cache: %w", err)
	}

	// 检查是否过期
	if time.Now().Unix() > expiresAt {
		// 删除过期文件
		os.Remove(filePath)
		return fmt.Errorf("cache expired")
	}

	// 使用PHP序列化库反序列化数据
	if err := phpserialize.Unmarshal([]byte(serializedData), target); err != nil {
		return fmt.Errorf("failed to unserialize cached data: %w", err)
	}

	return nil
}

// Delete 删除Laravel兼容的缓存文件
func (c *LaravelFileCache) Delete(key string) error {
	filePath := c.GetCacheFilePath(key)
	err := os.Remove(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete cache file: %w", err)
	}
	return nil
}

// CleanupExpired 清理过期的缓存文件
func (c *LaravelFileCache) CleanupExpired() error {
	return filepath.Walk(c.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略错误，继续处理
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 读取文件
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // 忽略读取错误
		}

		// 解析Laravel缓存格式
		content := string(data)
		_, expiresAt, err := c.ParseLaravelCache(content)
		if err != nil {
			return nil // 忽略解析错误
		}

		// 检查是否过期
		if time.Now().Unix() > expiresAt {
			os.Remove(path) // 删除过期文件
		}

		return nil
	})
}

// generateYggdrasilTokenKey 生成Yggdrasil Token缓存键（与BlessingSkin兼容）
func generateYggdrasilTokenKey(accessToken string) string {
	return fmt.Sprintf("yggdrasil-token-%s", accessToken)
}

// generateYggdrasilUserTokensKey 生成用户Token列表缓存键（与BlessingSkin兼容）
func generateYggdrasilUserTokensKey(userEmail string) string {
	return fmt.Sprintf("yggdrasil-id-%s", userEmail)
}

// generateYggdrasilSessionKey 生成Session缓存键（与BlessingSkin兼容）
func generateYggdrasilSessionKey(serverID string) string {
	return fmt.Sprintf("yggdrasil-server-%s", serverID)
}

// ParseLaravelCache 解析Laravel缓存格式
func (c *LaravelFileCache) ParseLaravelCache(content string) (string, int64, error) {
	// Laravel缓存格式：{php_serialized_data}i:{expiration_timestamp};
	// 例如：s:10:"test_value"i:1744686812; 或者 i:9999999999;i:1744686812;

	// 查找最后的 i: 位置
	lastI := strings.LastIndex(content, "i:")
	if lastI == -1 {
		return "", 0, fmt.Errorf("invalid Laravel cache format: missing expiration")
	}

	// 提取过期时间
	expirationPart := content[lastI+2:]
	if !strings.HasSuffix(expirationPart, ";") {
		return "", 0, fmt.Errorf("invalid Laravel cache format: missing semicolon")
	}

	expirationStr := expirationPart[:len(expirationPart)-1]
	expiresAt, err := strconv.ParseInt(expirationStr, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid expiration timestamp: %w", err)
	}

	// 提取PHP序列化数据
	serializedData := content[:lastI]

	return serializedData, expiresAt, nil
}

// 注意：手动PHP序列化方法已移除，现在使用 github.com/trim21/go-phpserialize 库
