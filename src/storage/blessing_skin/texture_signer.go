// Package blessing_skin 材质签名器
package blessing_skin

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
	"sync"
	"time"

	"yggdrasil-api-go/src/yggdrasil"

	"github.com/bytedance/sonic"
)

// TextureSigner 材质签名器
type TextureSigner struct {
	storage          *Storage
	cachedPrivateKey *rsa.PrivateKey
	cachedPublicKey  *rsa.PublicKey
	keyPairCached    bool
	keyPairMutex     sync.RWMutex
}

// NewTextureSigner 创建材质签名器
func NewTextureSigner(storage *Storage) *TextureSigner {
	return &TextureSigner{
		storage: storage,
	}
}

// TextureData 材质数据结构（与BlessingSkin兼容）
type TextureData struct {
	Timestamp   int64          `json:"timestamp"`
	ProfileID   string         `json:"profileId"`
	ProfileName string         `json:"profileName"`
	IsPublic    bool           `json:"isPublic"`
	Textures    map[string]any `json:"textures"`
}

// SignProfile 签名角色材质
func (ts *TextureSigner) SignProfile(profile *yggdrasil.Profile, unsigned bool) error {
	// 构建材质数据
	textureData := TextureData{
		Timestamp:   time.Now().UnixMilli(),
		ProfileID:   strings.ReplaceAll(profile.ID, "-", ""),
		ProfileName: profile.Name,
		IsPublic:    true,
		Textures:    make(map[string]any),
	}

	// 获取站点URL
	siteURL, _ := ts.storage.optionsMgr.GetOption("site_url")
	if siteURL == "" {
		siteURL = "http://localhost"
	}

	// 获取角色对应的Player记录
	var player Player
	err := ts.storage.db.Where("uuid = ?", profile.ID).First(&player).Error
	if err != nil {
		return fmt.Errorf("player not found: %w", err)
	}

	// 添加皮肤材质
	if player.TIDSkin > 0 {
		var skin Texture
		if err := ts.storage.db.First(&skin, player.TIDSkin).Error; err == nil {
			skinTexture := map[string]any{
				"url": fmt.Sprintf("%s/textures/%s", siteURL, skin.Hash),
			}

			// 添加模型信息
			if skin.Type == "alex" {
				skinTexture["metadata"] = map[string]string{"model": "slim"}
			}

			textureData.Textures["SKIN"] = skinTexture
		}
	}

	// 添加披风材质
	if player.TIDCape > 0 {
		var cape Texture
		if err := ts.storage.db.First(&cape, player.TIDCape).Error; err == nil {
			textureData.Textures["CAPE"] = map[string]any{
				"url": fmt.Sprintf("%s/textures/%s", siteURL, cape.Hash),
			}
		}
	}

	// 序列化材质数据
	textureJSON, err := sonic.Marshal(textureData)
	if err != nil {
		return fmt.Errorf("failed to marshal texture data: %w", err)
	}

	// Base64编码
	textureValue := base64.StdEncoding.EncodeToString(textureJSON)

	// 创建材质属性
	properties := []yggdrasil.ProfileProperty{
		{
			Name:  "textures",
			Value: textureValue,
		},
	}

	// 检查是否支持材质上传
	if ts.storage.IsUploadSupported() {
		properties = append(properties, yggdrasil.ProfileProperty{
			Name:  "uploadableTextures",
			Value: "skin,cape",
		})
	}

	// 如果需要签名
	if !unsigned {
		// 获取缓存的RSA密钥对
		privateKey, _, err := ts.getCachedRSAKeyPair()
		if err != nil {
			return fmt.Errorf("failed to get RSA key pair: %w", err)
		}

		// 签名材质属性
		for i := range properties {
			if properties[i].Name == "textures" {
				signature, err := ts.signData(properties[i].Value, privateKey)
				if err != nil {
					return fmt.Errorf("failed to sign texture: %w", err)
				}
				properties[i].Signature = base64.StdEncoding.EncodeToString(signature)
			}
		}
	}

	// 更新Profile的Properties
	profile.Properties = properties

	return nil
}

// parsePrivateKey 解析PEM格式的私钥
func (ts *TextureSigner) parsePrivateKey(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// 尝试PKCS1格式
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, nil
	}

	// 尝试PKCS8格式
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}

	return rsaKey, nil
}

// signData 使用RSA私钥签名数据（与BlessingSkin兼容）
func (ts *TextureSigner) signData(data string, privateKey *rsa.PrivateKey) ([]byte, error) {
	// 使用SHA1哈希（与BlessingSkin的openssl_sign兼容）
	hash := sha1.Sum([]byte(data))
	return rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA1, hash[:])
}

// GetPublicKey 获取公钥（用于客户端验证）
func (ts *TextureSigner) GetPublicKey() (string, error) {
	privateKeyPEM, err := ts.storage.optionsMgr.GetOption("ygg_private_key")
	if err != nil || privateKeyPEM == "" {
		return "", fmt.Errorf("RSA private key not configured")
	}

	// 解析私钥
	privateKey, err := ts.parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("invalid RSA private key: %w", err)
	}

	// 提取公钥
	publicKey := &privateKey.PublicKey

	// 编码公钥为PEM格式
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}

// GetSignatureKeyPair 获取签名用的密钥对（私钥和公钥）
func (ts *TextureSigner) GetSignatureKeyPair() (privateKey string, publicKey string, err error) {
	privateKeyPEM, err := ts.storage.optionsMgr.GetOption("ygg_private_key")
	if err != nil || privateKeyPEM == "" {
		return "", "", fmt.Errorf("RSA private key not configured")
	}

	// 验证私钥格式
	_, err = ts.parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", "", fmt.Errorf("invalid RSA private key: %w", err)
	}

	// 获取公钥
	publicKeyPEM, err := ts.GetPublicKey()
	if err != nil {
		return "", "", fmt.Errorf("failed to get public key: %w", err)
	}

	return privateKeyPEM, publicKeyPEM, nil
}

// getCachedRSAKeyPair 获取缓存的RSA密钥对
func (ts *TextureSigner) getCachedRSAKeyPair() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// 先检查缓存
	ts.keyPairMutex.RLock()
	if ts.keyPairCached {
		defer ts.keyPairMutex.RUnlock()
		return ts.cachedPrivateKey, ts.cachedPublicKey, nil
	}
	ts.keyPairMutex.RUnlock()

	// 获取写锁进行加载
	ts.keyPairMutex.Lock()
	defer ts.keyPairMutex.Unlock()

	// 双重检查，防止并发加载
	if ts.keyPairCached {
		return ts.cachedPrivateKey, ts.cachedPublicKey, nil
	}

	// 从options表读取私钥
	privateKeyPEM, err := ts.storage.optionsMgr.GetOption("ygg_private_key")
	if err != nil || privateKeyPEM == "" {
		return nil, nil, fmt.Errorf("RSA private key not configured")
	}

	// 解析私钥
	privateKey, err := ts.parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid RSA private key: %w", err)
	}

	// 提取公钥
	publicKey := &privateKey.PublicKey

	// 缓存密钥对
	ts.cachedPrivateKey = privateKey
	ts.cachedPublicKey = publicKey
	ts.keyPairCached = true

	return privateKey, publicKey, nil
}

// VerifySignature 验证签名（用于测试）
func (ts *TextureSigner) VerifySignature(data, signature string) error {
	publicKeyPEM, err := ts.GetPublicKey()
	if err != nil {
		return err
	}

	// 解析公钥
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("not an RSA public key")
	}

	// 解码签名
	signatureBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// 验证签名
	hash := sha1.Sum([]byte(data))
	return rsa.VerifyPKCS1v15(rsaPublicKey, crypto.SHA1, hash[:], signatureBytes)
}
